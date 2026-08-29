package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeResource lays a file down beside a skill's body.
func writeResource(t *testing.T, skillDir, rel, contents string) {
	t.Helper()
	p := filepath.Join(skillDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// The caps exist to admit the published ecosystem. These are the real measured
// extremes of the Snitch family, which is what motivated raising them: the
// largest description is 1341 bytes and the largest body is a little over 50KB.
// If a future change tightens either cap back below these, every one of those
// skills silently stops loading -- which is exactly the failure this asserts.
func TestNewSkill_AdmitsEcosystemSizedDescriptionsAndBodies(t *testing.T) {
	desc := strings.Repeat("d", 1341)
	body := strings.Repeat("b", 50*1024)
	raw := "---\ndescription: " + desc + "\n---\n" + body

	skill, err := newSkill("big", raw, SourceUser, "/tmp/big/SKILL.md", "/tmp/big")
	if err != nil {
		t.Fatalf("an ecosystem-sized skill must load: %v", err)
	}
	if len(skill.Description) != 1341 || len(skill.Body) != 50*1024 {
		t.Fatalf("description %d body %d", len(skill.Description), len(skill.Body))
	}
}

func TestNewSkill_StillRejectsPastTheCaps(t *testing.T) {
	over := "---\ndescription: " + strings.Repeat("d", MaxDescriptionBytes+1) + "\n---\nbody\n"
	if _, err := newSkill("x", over, SourceUser, "", ""); err == nil {
		t.Fatal("an over-cap description must still be refused")
	}
	big := "---\ndescription: ok\n---\n" + strings.Repeat("b", MaxBodyBytes+1)
	if _, err := newSkill("x", big, SourceUser, "", ""); err == nil {
		t.Fatal("an over-cap body must still be refused")
	}
}

func TestLoad_PopulatesDirForEveryScope(t *testing.T) {
	home := isolate(t)
	project := t.TempDir()
	writeSkill(t, filepath.Join(home, dirName), "userskill", "---\ndescription: u\n---\nbody\n")
	writeSkill(t, ProjectSkillsDir(project), "projskill", "---\ndescription: p\n---\nbody\n")

	reg := Load(project)
	for name, want := range map[string]string{
		"userskill": filepath.Join(home, dirName, "userskill"),
		"projskill": filepath.Join(ProjectSkillsDir(project), "projskill"),
	} {
		s, ok := reg.Lookup(name)
		if !ok {
			t.Fatalf("%s did not resolve", name)
		}
		if s.Dir != want {
			t.Fatalf("%s Dir = %q, want %q", name, s.Dir, want)
		}
	}
	// A builtin's Dir points into the embedded FS, not the filesystem. It is
	// still set, because ReadResource dispatches on Source to decide which.
	b, ok := reg.Lookup("packetcode-hooks")
	if !ok {
		t.Fatal("builtin did not resolve")
	}
	if b.Dir != "builtin/packetcode-hooks" {
		t.Fatalf("builtin Dir = %q", b.Dir)
	}
	if b.Path != "" {
		t.Fatalf("a builtin has no on-disk path, got %q", b.Path)
	}
}

func TestResources_ListsSiblingsAndSkipsBodyAndDotfiles(t *testing.T) {
	home := isolate(t)
	dir := filepath.Join(home, dirName, "audit")
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	writeResource(t, dir, "references/rules.md", "rules")
	writeResource(t, dir, "categories/01-sql.md", "sql")
	writeResource(t, dir, ".hidden/secret.md", "no")
	writeResource(t, dir, ".dotfile", "no")

	list, truncated := Load("").Resources("audit")
	if truncated {
		t.Fatal("a four-file skill must not truncate")
	}
	want := []string{"categories/01-sql.md", "references/rules.md"}
	if strings.Join(list, ",") != strings.Join(want, ",") {
		t.Fatalf("Resources = %v, want %v", list, want)
	}
}

func TestReadResource_ServesASibling(t *testing.T) {
	home := isolate(t)
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	writeResource(t, filepath.Join(home, dirName, "audit"), "references/rules.md", "the method")

	data, err := Load("").ReadResource("audit", "references/rules.md")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "the method" {
		t.Fatalf("got %q", data)
	}
}

// rel is model-supplied. These are the shapes that turn a resource read into an
// arbitrary file read, and every one must be refused rather than sanitised.
func TestReadResource_RefusesEscapes(t *testing.T) {
	home := isolate(t)
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	writeResource(t, filepath.Join(home, dirName, "audit"), "references/rules.md", "ok")
	reg := Load("")

	for _, rel := range []string{
		"../../../etc/passwd",
		"references/../../../../etc/passwd",
		"/etc/passwd",
		"..",
		".",
		"",
		"   ",
		`..\..\windows\win.ini`,
		"C:/Windows/win.ini",
		`\\server\share\file`,
	} {
		if _, err := reg.ReadResource("audit", rel); err == nil {
			t.Fatalf("ReadResource(%q) must be refused", rel)
		}
	}
}

// A separator choice must not decide whether a check runs. Backslashes are
// normalised before validation precisely so this case cannot slip past.
func TestReadResource_AcceptsBackslashSeparators(t *testing.T) {
	home := isolate(t)
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	writeResource(t, filepath.Join(home, dirName, "audit"), "references/rules.md", "ok")

	if _, err := Load("").ReadResource("audit", `references\rules.md`); err != nil {
		t.Fatalf("a backslash separator must resolve the same file: %v", err)
	}
}

// A symlink planted inside a skill directory is the case lexical validation
// alone cannot catch: the path never contains "..", and only resolving it
// reveals that it leaves.
func TestReadResource_RefusesSymlinkOutOfTheSkillDirectory(t *testing.T) {
	home := isolate(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	dir := filepath.Join(home, dirName, "audit")
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	if err := os.Symlink(outside, filepath.Join(dir, "escape.md")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation needs privilege on this platform")
		}
		t.Fatalf("symlink: %v", err)
	}

	if _, err := Load("").ReadResource("audit", "escape.md"); err == nil {
		t.Fatal("a symlink leaving the skill directory must be refused")
	}
}

func TestReadResource_RefusesTheBodyAndUnknownSkills(t *testing.T) {
	home := isolate(t)
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	reg := Load("")

	if _, err := reg.ReadResource("audit", FileName); err == nil {
		t.Fatal("the body is served by the no-file call, not as a resource")
	}
	if _, err := reg.ReadResource("nope", "x.md"); err == nil {
		t.Fatal("an unknown skill must error")
	}
}

func TestReadResource_RefusesOverCapFiles(t *testing.T) {
	home := isolate(t)
	writeSkill(t, filepath.Join(home, dirName), "audit", "---\ndescription: a\n---\nbody\n")
	writeResource(t, filepath.Join(home, dirName, "audit"), "big.md", strings.Repeat("x", MaxResourceBytes+1))

	if _, err := Load("").ReadResource("audit", "big.md"); err == nil {
		t.Fatal("an over-cap resource must be refused")
	}
}

// The index is what every request pays for. With descriptions at 1536 bytes a
// large skill directory could otherwise put an unbounded block in front of the
// system prompt.
func TestIndexBlock_DropsWholeEntriesPastTheAggregateCap(t *testing.T) {
	home := isolate(t)
	desc := strings.Repeat("d", 1200)
	// Enough to exceed MaxIndexBytes several times over.
	for i := 0; i < 40; i++ {
		writeSkill(t, filepath.Join(home, dirName),
			"skill-"+string(rune('a'+i/26))+string(rune('a'+i%26)),
			"---\ndescription: "+desc+"\n---\nbody\n")
	}

	block := Load("").IndexBlock()
	if len(block) > MaxIndexBytes+1024 {
		// The closing tag and the omission notice are written after the last
		// entry is admitted, so the block can exceed the cap by their length
		// and no more.
		t.Fatalf("index is %d bytes, cap is %d", len(block), MaxIndexBytes)
	}
	if !strings.Contains(block, "further skills were omitted") {
		t.Fatal("omitted skills must be reported, not silently dropped")
	}
	// A dropped entry must be dropped whole: no half-description may survive,
	// because it costs bytes while having lost the part that says when to
	// choose the skill.
	for _, line := range strings.Split(block, "\n") {
		if !strings.HasPrefix(line, "- skill-") {
			// Builtins carry their own descriptions and are not part of this
			// fixture; only the generated entries are checked.
			continue
		}
		if !strings.Contains(line, desc) {
			t.Fatalf("entry was truncated mid-description: %q", line)
		}
	}
}

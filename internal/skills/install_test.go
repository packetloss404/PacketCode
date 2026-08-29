package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// fakeRepo builds a checkout on disk. Install reads a local directory in place,
// which is what lets every test here run without a network or a git binary.
func fakeRepo(t *testing.T, layout map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, contents := range layout {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

const goodSkill = "---\ndescription: an installable skill\n---\nthe body\n"

func TestInstall_MarketplaceLayoutWithResources(t *testing.T) {
	home := isolate(t)
	repo := fakeRepo(t, map[string]string{
		"skills/alpha/SKILL.md":            goodSkill,
		"skills/alpha/references/rules.md": "the method",
		"skills/beta/SKILL.md":             goodSkill,
		"README.md":                        "not a skill",
	})

	result, err := Install(InstallOptions{Repo: repo, Scope: ScopeUser}, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	sort.Strings(result.Installed)
	if strings.Join(result.Installed, ",") != "alpha,beta" {
		t.Fatalf("installed = %v", result.Installed)
	}

	// The whole folder must arrive, not SKILL.md alone: the resource files are
	// where an ecosystem skill keeps its actual method.
	reg := Load("")
	list, _ := reg.Resources("alpha")
	if strings.Join(list, ",") != "references/rules.md" {
		t.Fatalf("resources did not survive the install: %v", list)
	}
	data, err := reg.ReadResource("alpha", "references/rules.md")
	if err != nil || string(data) != "the method" {
		t.Fatalf("read resource: %q %v", data, err)
	}
	_ = home
}

func TestInstall_RepositoryThatIsItselfOneSkill(t *testing.T) {
	isolate(t)
	repo := t.TempDir()
	// The directory name becomes the skill name, so it must be a valid one.
	single := filepath.Join(repo, "my-skill")
	if err := os.MkdirAll(single, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(single, FileName), []byte(goodSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := Install(InstallOptions{Repo: single, Scope: ScopeUser}, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if strings.Join(result.Installed, ",") != "my-skill" {
		t.Fatalf("installed = %v", result.Installed)
	}
}

// A skills/ directory wins outright. Falling through to the repository root
// after finding one would sweep up whatever else happens to sit there.
func TestDiscoverSkills_PrefersTheSkillsDirectory(t *testing.T) {
	repo := fakeRepo(t, map[string]string{
		"skills/alpha/SKILL.md": goodSkill,
		"tooling/SKILL.md":      goodSkill,
	})
	found, err := discoverSkills(repo, "whatever")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(found) != 1 || found["alpha"] == "" {
		t.Fatalf("found = %v", found)
	}
}

func TestInstall_SkipsExistingUnlessForced(t *testing.T) {
	isolate(t)
	repo := fakeRepo(t, map[string]string{"skills/alpha/SKILL.md": goodSkill})

	if _, err := Install(InstallOptions{Repo: repo, Scope: ScopeUser}, nil); err != nil {
		t.Fatalf("first install: %v", err)
	}
	again, err := Install(InstallOptions{Repo: repo, Scope: ScopeUser}, nil)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if strings.Join(again.Skipped, ",") != "alpha" || len(again.Installed) != 0 {
		t.Fatalf("a second install must skip, got %+v", again)
	}

	forced, err := Install(InstallOptions{Repo: repo, Scope: ScopeUser, Force: true}, nil)
	if err != nil {
		t.Fatalf("forced install: %v", err)
	}
	if strings.Join(forced.Replaced, ",") != "alpha" {
		t.Fatalf("--force must replace, got %+v", forced)
	}
}

// Validation happens before the copy, so a malformed skill never lands in the
// user's skills directory at all. Copying first and reporting after would leave
// the user to clean up by hand to get back to where they started.
func TestInstall_RefusesMalformedSkillsWithoutWritingThem(t *testing.T) {
	home := isolate(t)
	repo := fakeRepo(t, map[string]string{
		"skills/good/SKILL.md": goodSkill,
		// No frontmatter description: unindexable, therefore unusable.
		"skills/bad/SKILL.md": "just a body with no frontmatter\n",
	})

	result, err := Install(InstallOptions{Repo: repo, Scope: ScopeUser}, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if strings.Join(result.Installed, ",") != "good" {
		t.Fatalf("installed = %v", result.Installed)
	}
	if len(result.Rejected) != 1 || !strings.Contains(result.Rejected[0], "bad") {
		t.Fatalf("rejected = %v", result.Rejected)
	}
	if _, err := os.Stat(filepath.Join(home, dirName, "bad")); !os.IsNotExist(err) {
		t.Fatal("a refused skill must not be written to disk")
	}
}

func TestInstall_SelectsASubsetAndNamesWhatIsMissing(t *testing.T) {
	isolate(t)
	repo := fakeRepo(t, map[string]string{
		"skills/alpha/SKILL.md": goodSkill,
		"skills/beta/SKILL.md":  goodSkill,
	})

	result, err := Install(InstallOptions{Repo: repo, Scope: ScopeUser, Names: []string{"beta"}}, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if strings.Join(result.Installed, ",") != "beta" {
		t.Fatalf("installed = %v", result.Installed)
	}

	_, err = Install(InstallOptions{Repo: repo, Scope: ScopeUser, Names: []string{"gamma"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "gamma") {
		t.Fatalf("a missing name must be reported: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("the error must name what is available: %v", err)
	}
}

func TestInstall_ProjectScopeWritesUnderTheProject(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	repo := fakeRepo(t, map[string]string{"skills/alpha/SKILL.md": goodSkill})

	result, err := Install(InstallOptions{
		Repo: repo, Scope: ScopeProject, WorkingDir: project,
	}, nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if result.Dest != ProjectSkillsDir(project) {
		t.Fatalf("dest = %q", result.Dest)
	}
	if _, err := os.Stat(filepath.Join(ProjectSkillsDir(project), "alpha", FileName)); err != nil {
		t.Fatalf("skill not written to the project scope: %v", err)
	}
	// A project skill is untrusted, and must say so however it was installed.
	s, ok := Load(project).Lookup("alpha")
	if !ok || s.Trusted() {
		t.Fatalf("an installed project skill must stay untrusted: %+v", s)
	}
}

// Recreating a symlink inside the skills directory would plant exactly the
// escape ReadResource then has to refuse. Skipping it at copy time means the
// escape never exists.
func TestCopyTree_SkipsSymlinks(t *testing.T) {
	src := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, FileName), []byte(goodSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "escape.md")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation needs privilege on this platform")
		}
		t.Fatalf("symlink: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "out")
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "escape.md")); !os.IsNotExist(err) {
		t.Fatal("a symlink must not be reproduced in the install")
	}
	if _, err := os.Stat(filepath.Join(dst, FileName)); err != nil {
		t.Fatalf("the body must still be copied: %v", err)
	}
}

func TestCopyTree_SkipsRepositoryMetadata(t *testing.T) {
	src := fakeRepo(t, map[string]string{
		FileName:          goodSkill,
		".git/config":     "[core]",
		"node_modules/x":  "junk",
		"references/a.md": "keep",
	})
	dst := filepath.Join(t.TempDir(), "out")
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	for _, gone := range []string{".git", "node_modules"} {
		if _, err := os.Stat(filepath.Join(dst, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s must not be installed", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "references", "a.md")); err != nil {
		t.Fatalf("resources must be installed: %v", err)
	}
}

func TestRemove_DeletesAndReportsWhatIsNotThere(t *testing.T) {
	home := isolate(t)
	writeSkill(t, filepath.Join(home, dirName), "alpha", goodSkill)

	if _, err := Remove("alpha", ScopeUser, ""); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, dirName, "alpha")); !os.IsNotExist(err) {
		t.Fatal("the directory must be gone")
	}
	if _, err := Remove("alpha", ScopeUser, ""); err == nil {
		t.Fatal("removing what is not installed must error")
	}
	// A name is a directory name; one carrying a separator must never reach
	// RemoveAll.
	if _, err := Remove("../../etc", ScopeUser, ""); err == nil {
		t.Fatal("an invalid name must be refused")
	}
}

func TestScopeDir_RejectsUnknownScopes(t *testing.T) {
	isolate(t)
	if _, err := scopeDir("elsewhere", ""); err == nil {
		t.Fatal("an unknown scope must error")
	}
	if _, err := scopeDir(ScopeProject, ""); err == nil {
		t.Fatal("the project scope needs a working directory")
	}
}

func TestFetchRepo_RequiresARepository(t *testing.T) {
	if _, _, _, err := fetchRepo("", "", nil); err == nil {
		t.Fatal("an empty repo argument must error")
	}
}

// A remote clone lands in an os.MkdirTemp directory whose basename is a valid
// skill name, so deriving the name from the checkout directory installed a
// single-skill repository as "packetcode-skills-1642398117" and reported
// success. The name must come from the repository.
func TestDiscoverSkills_NamesASingleSkillRepoAfterTheRepoNotTheCheckout(t *testing.T) {
	checkout := t.TempDir() // stands in for the temp clone dir
	if err := os.WriteFile(filepath.Join(checkout, FileName), []byte(goodSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	found, err := discoverSkills(checkout, "single-skill-repo")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if _, ok := found["single-skill-repo"]; !ok || len(found) != 1 {
		t.Fatalf("found = %v, want the repository name", found)
	}
	if _, ok := found[filepath.Base(checkout)]; ok {
		t.Fatal("the checkout directory name must never become the skill name")
	}
}

// An unusable repository name must say so rather than falling through to a
// scan that finds nothing and reports the wrong reason.
func TestDiscoverSkills_ReportsAnUnusableSingleSkillName(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, FileName), []byte(goodSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := discoverSkills(checkout, "not.a.valid.name")
	if err == nil || !strings.Contains(err.Error(), "not.a.valid.name") {
		t.Fatalf("want an error naming the bad name, got %v", err)
	}
}

func TestRepoBaseName(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/naieum/snitchmarketplace":     "snitchmarketplace",
		"https://github.com/naieum/snitchmarketplace.git": "snitchmarketplace",
		"https://github.com/owner/repo/":                  "repo",
		"git@github.com:owner/repo.git":                   "repo",
		"repo":                                            "repo",
	} {
		if got := repoBaseName(url); got != want {
			t.Fatalf("repoBaseName(%q) = %q, want %q", url, got, want)
		}
	}
}

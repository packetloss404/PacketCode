package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A skill in the user's own ~/.claude/skills is theirs. It loads on sight,
// with no approval, exactly like ~/.packetcode/skills -- anyone with Claude
// Code installed already has skills there, and requiring them to be reinstalled
// under a second layout is friction with no safety behind it.
func TestLoad_ReadsForeignUserScopeWithoutApproval(t *testing.T) {
	isolate(t)
	project := t.TempDir()

	writeSkill(t, osHomeSkillsDir(t, ".claude"), "fromclaude",
		"---\ndescription: claude scope\n---\nbody\n")
	writeSkill(t, osHomeSkillsDir(t, ".agents"), "fromagents",
		"---\ndescription: agents scope\n---\nbody\n")

	reg := Load(project)
	for _, tc := range []struct{ name, origin string }{
		{"fromclaude", OriginClaude},
		{"fromagents", OriginAgents},
	} {
		s, ok := reg.Lookup(tc.name)
		if !ok {
			t.Fatalf("%s did not resolve", tc.name)
		}
		if s.Source != SourceUser {
			t.Fatalf("%s: Source = %q, want %q", tc.name, s.Source, SourceUser)
		}
		if s.Origin != tc.origin {
			t.Fatalf("%s: Origin = %q, want %q", tc.name, s.Origin, tc.origin)
		}
		if !s.Enabled() {
			t.Fatalf("%s: a user-scope skill must not need approval", tc.name)
		}
		if !s.Trusted() {
			t.Fatalf("%s: a user-scope skill is trusted wherever it was filed", tc.name)
		}
	}
}

// PACKETCODE_HOME relocates packetcode's own data home. It must not relocate
// another program's directory: pointing it at scratch space must not change
// which ~/.claude skills are visible, in either direction.
func TestLoad_ForeignUserScopeIgnoresPacketcodeHome(t *testing.T) {
	isolate(t)
	project := t.TempDir()

	// A .claude skill under the OS home resolves.
	writeSkill(t, osHomeSkillsDir(t, ".claude"), "here",
		"---\ndescription: d\n---\nbody\n")
	// One under the packetcode home does not: that directory is not part of
	// any layout.
	packetHome, err := UserSkillsDir()
	if err != nil {
		t.Fatalf("UserSkillsDir: %v", err)
	}
	writeSkill(t, filepath.Join(filepath.Dir(packetHome), ".claude", "skills"), "notthere",
		"---\ndescription: d\n---\nbody\n")

	reg := Load(project)
	if _, ok := reg.Lookup("here"); !ok {
		t.Fatal("a ~/.claude skill did not resolve")
	}
	if _, ok := reg.Lookup("notthere"); ok {
		t.Fatal("a .claude directory under PACKETCODE_HOME was scanned; it is not a layout")
	}
}

// The whole point of the gate. A repository's .claude/skills is content found
// by opening a directory, not by anyone installing it -- so it is discovered,
// listed, and does nothing.
func TestLoad_ForeignProjectSkillNeedsApproval(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, filepath.Join(project, ".claude", "skills"), "deploy",
		"---\ndescription: repo supplied\n---\nREPO-BODY\n")

	reg := Load(project)
	s, ok := reg.Lookup("deploy")
	if !ok {
		t.Fatal("a pending skill must still be discovered and listed")
	}
	if s.Enabled() {
		t.Fatal("a foreign project skill must not load unapproved")
	}
	if s.Origin != OriginClaude {
		t.Fatalf("Origin = %q", s.Origin)
	}
	// Not named to the model. This is most of the exposure: a description in
	// the system prompt is repository text in the highest-authority position
	// in the conversation.
	if idx := reg.IndexBlock(); strings.Contains(idx, "deploy") || strings.Contains(idx, "repo supplied") {
		t.Fatalf("a pending skill reached the index:\n%s", idx)
	}
}

// A repository's .packetcode/skills is unchanged: it loads without approval,
// as it always has. The gate exists to avoid a NEW auto-load surface, not to
// retroactively fence off a directory projects already rely on.
func TestLoad_NativeProjectSkillNeedsNoApproval(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, ProjectSkillsDir(project), "deploy",
		"---\ndescription: d\n---\nbody\n")

	s, ok := Load(project).Lookup("deploy")
	if !ok {
		t.Fatal("deploy did not resolve")
	}
	if !s.Enabled() {
		t.Fatal("a .packetcode project skill must keep loading without approval")
	}
	if s.Origin != OriginNative {
		t.Fatalf("Origin = %q, want %q", s.Origin, OriginNative)
	}
}

// Approving is per skill, per workspace, and per body.
func TestApprove_LoadsTheSkillAndPersists(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, filepath.Join(project, ".claude", "skills"), "deploy",
		"---\ndescription: d\n---\nORIGINAL-BODY\n")

	reg := Load(project)
	next, err := reg.Approve("deploy")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	s, ok := next.Lookup("deploy")
	if !ok || !s.Enabled() {
		t.Fatalf("approval did not enable the skill: %+v", s)
	}
	if idx := next.IndexBlock(); !strings.Contains(idx, "deploy") {
		t.Fatalf("an approved skill is still absent from the index:\n%s", idx)
	}

	// Survives a fresh load: the decision is on disk, not in the process.
	if s, ok := Load(project).Lookup("deploy"); !ok || !s.Enabled() {
		t.Fatalf("approval did not persist: %+v", s)
	}
}

// The approval is of content, not of a filename. A repository that rewrites an
// approved skill is asked again -- otherwise the first benign version buys
// permanent trust for every version after it, which is the whole attack.
func TestApprove_DoesNotCarryToARewrittenBody(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	dir := filepath.Join(project, ".claude", "skills")
	writeSkill(t, dir, "deploy", "---\ndescription: d\n---\nORIGINAL-BODY\n")

	reg := Load(project)
	if _, err := reg.Approve("deploy"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if s, _ := Load(project).Lookup("deploy"); !s.Enabled() {
		t.Fatal("approval did not take")
	}

	writeSkill(t, dir, "deploy", "---\ndescription: d\n---\nREWRITTEN-BODY\n")
	s, ok := Load(project).Lookup("deploy")
	if !ok {
		t.Fatal("deploy vanished")
	}
	if s.Enabled() {
		t.Fatal("approval carried over to a body nobody approved")
	}
}

// An approval is bound to one workspace. Approving a repository's `deploy`
// must not answer the question for a different repository's `deploy`.
func TestApprove_DoesNotCarryToAnotherWorkspace(t *testing.T) {
	isolate(t)
	one, two := t.TempDir(), t.TempDir()
	body := "---\ndescription: d\n---\nSAME-BODY\n"
	writeSkill(t, filepath.Join(one, ".claude", "skills"), "deploy", body)
	writeSkill(t, filepath.Join(two, ".claude", "skills"), "deploy", body)

	if _, err := Load(one).Approve("deploy"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if s, _ := Load(one).Lookup("deploy"); !s.Enabled() {
		t.Fatal("approval did not take in its own workspace")
	}
	// Same name, same bytes, different repository. Still pending.
	if s, _ := Load(two).Lookup("deploy"); s.Enabled() {
		t.Fatal("an approval leaked into another workspace")
	}
}

func TestRevoke_UnloadsTheSkill(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, filepath.Join(project, ".claude", "skills"), "deploy",
		"---\ndescription: d\n---\nbody\n")

	reg := Load(project)
	next, err := reg.Approve("deploy")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	after, err := next.Revoke("deploy")
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if s, _ := after.Lookup("deploy"); s.Enabled() {
		t.Fatal("revoke did not unload the skill")
	}
	if s, _ := Load(project).Lookup("deploy"); s.Enabled() {
		t.Fatal("revoke did not persist")
	}
}

// An unreadable or unrecognised store means nothing is approved. Reading it as
// "everything is approved" would turn a corrupt file into an open door.
func TestLoadApprovals_UnreadableStoreApprovesNothing(t *testing.T) {
	home := isolate(t)
	project := t.TempDir()
	writeSkill(t, filepath.Join(project, ".claude", "skills"), "deploy",
		"---\ndescription: d\n---\nbody\n")

	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, approvalsFile), []byte("{ not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg := Load(project)
	if s, _ := reg.Lookup("deploy"); s.Enabled() {
		t.Fatal("a corrupt approval store enabled a pending skill")
	}
	// And it is reported: a store that silently stopped working would look
	// exactly like a repository that added a skill.
	found := false
	for _, e := range reg.Errors() {
		if strings.Contains(e, "skill approvals") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a corrupt approval store was not reported: %v", reg.Errors())
	}
}

// A future format version must not be guessed at.
func TestLoadApprovals_UnknownVersionApprovesNothing(t *testing.T) {
	home := isolate(t)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, approvalsFile),
		[]byte(`{"version":99,"approvals":[]}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadApprovals(); err == nil {
		t.Fatal("an unknown format version was accepted")
	}
}

// Within one scope the native layout wins, because a .packetcode file was
// written by someone who knew this program would read it.
func TestLoad_NativeLayoutWinsWithinAScope(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, osHomeSkillsDir(t, ".claude"), "deploy",
		"---\ndescription: from claude\n---\nCLAUDE-BODY\n")
	writeSkill(t, mustUserSkillsDir(t), "deploy",
		"---\ndescription: from packetcode\n---\nPACKETCODE-BODY\n")

	s, ok := Load(project).Lookup("deploy")
	if !ok {
		t.Fatal("deploy did not resolve")
	}
	if s.Origin != OriginNative || s.Body != "PACKETCODE-BODY" {
		t.Fatalf("the foreign layout won within a scope: %+v", s)
	}
}

func mustUserSkillsDir(t *testing.T) string {
	t.Helper()
	dir, err := UserSkillsDir()
	if err != nil {
		t.Fatalf("UserSkillsDir: %v", err)
	}
	return dir
}

// Running packetcode from your home directory makes <cwd>/.agents/skills and
// ~/.agents/skills the same directory. Scanning it once per scope reported
// every malformed skill twice, made every skill in it "shadow" itself with a
// warning naming a project skill that was the same file, and labelled the
// user's own skills untrusted repository content on the project pass.
func TestLoad_HomeAsWorkingDirScansEachDirectoryOnce(t *testing.T) {
	isolate(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	writeSkill(t, osHomeSkillsDir(t, ".agents"), "autopilot",
		"---\ndescription: mine\n---\nbody\n")
	// A deliberately malformed one, so double-reporting would be visible.
	broken := filepath.Join(osHomeSkillsDir(t, ".agents"), "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(broken, FileName), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The working directory IS the home directory.
	reg := Load(home)

	s, ok := reg.Lookup("autopilot")
	if !ok {
		t.Fatal("autopilot did not resolve")
	}
	// The user's own files, not a repository's.
	if s.Source != SourceUser {
		t.Fatalf("Source = %q, want %q — the user's own skills were labelled repository content", s.Source, SourceUser)
	}
	if !s.Trusted() || !s.Enabled() {
		t.Fatalf("the user's own skill needs approval or is untrusted: %+v", s)
	}
	// Nothing shadows itself.
	if len(s.Shadows) != 0 {
		t.Fatalf("a skill shadowed itself: %v", s.Shadows)
	}
	for _, w := range reg.Warnings() {
		if strings.Contains(w, "shadows") {
			t.Fatalf("self-shadowing warning: %s", w)
		}
	}
	// And the one broken skill is reported exactly once.
	n := 0
	for _, e := range reg.Errors() {
		if strings.Contains(e, "broken") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the same malformed skill was reported %d times: %v", n, reg.Errors())
	}
}

func TestDedupeScopes_KeepsTheStrongestClaimAndOrder(t *testing.T) {
	in := []skillScope{
		{path: "/a/.claude/skills", source: SourceProject, origin: OriginClaude, foreignProject: true},
		{path: "/b/.packetcode/skills", source: SourceProject, origin: OriginNative},
		// Same directory as the first, claimed later by the user scope.
		{path: "/a/.claude/skills", source: SourceUser, origin: OriginClaude},
	}
	out := dedupeScopes(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 scopes, got %d: %+v", len(out), out)
	}
	// The later, stronger claim wins the slot, and the slot keeps its place.
	if out[0].path != "/a/.claude/skills" || out[0].source != SourceUser {
		t.Fatalf("the user scope did not win the shared directory: %+v", out[0])
	}
	if out[0].foreignProject {
		t.Fatal("a directory won by the user scope must not still need approval")
	}
	if out[1].path != "/b/.packetcode/skills" {
		t.Fatalf("order was not preserved: %+v", out)
	}
}

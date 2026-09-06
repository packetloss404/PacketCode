package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func grantLabels(gs []ToolGrant) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.Label())
	}
	return out
}

func TestParseAllowedTools(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: read_file, execute_command ,, write_file\n---\nbody\n")
	want := "read_file,execute_command,write_file"
	if got := strings.Join(grantLabels(fm.AllowedTools), ","); got != want {
		t.Fatalf("AllowedTools = %v, want %v", got, want)
	}
	for _, g := range fm.AllowedTools {
		if g.Scoped() {
			t.Fatalf("a bare name parsed as scoped: %+v", g)
		}
	}
	if len(fm.Warnings) != 0 {
		t.Fatalf("a list of plain names warned: %v", fm.Warnings)
	}
}

// `Bash(gh:*)` is the ecosystem's way of saying "this skill runs gh, nothing
// else". packetcode has a rule shape for exactly that, so it becomes one:
// permission to run `gh ...` and no more.
func TestParseAllowedTools_CommandPrefixGrant(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: Bash(gh:*), Bash(npm run test:*)\n---\nbody\n")

	if len(fm.Warnings) != 0 {
		t.Fatalf("an expressible grant was reported as refused: %v", fm.Warnings)
	}
	if len(fm.AllowedTools) != 2 {
		t.Fatalf("AllowedTools = %+v", fm.AllowedTools)
	}
	first := fm.AllowedTools[0]
	if first.Tool != ShellTool {
		t.Fatalf("Bash was not translated to %s: %+v", ShellTool, first)
	}
	if strings.Join(first.CommandPrefix, " ") != "gh" || first.Command != "" {
		t.Fatalf("Bash(gh:*) = %+v", first)
	}
	if strings.Join(fm.AllowedTools[1].CommandPrefix, " ") != "npm run test" {
		t.Fatalf("a multi-word prefix was not kept whole: %+v", fm.AllowedTools[1])
	}
}

// A specifier with no trailing `:*` names one complete program, which the
// policy matches byte-for-byte.
func TestParseAllowedTools_ExactCommandGrant(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: execute_command(git status)\n---\nbody\n")

	if len(fm.Warnings) != 0 {
		t.Fatalf("warnings = %v", fm.Warnings)
	}
	if len(fm.AllowedTools) != 1 {
		t.Fatalf("AllowedTools = %+v", fm.AllowedTools)
	}
	g := fm.AllowedTools[0]
	if g.Command != "git status" || len(g.CommandPrefix) != 0 {
		t.Fatalf("execute_command(git status) = %+v", g)
	}
}

// The scope is what makes translating a foreign tool name safe: it bounds the
// result to one command family. A bare `Bash` carries no such bound, so it
// stays an unrecognised name rather than becoming the whole shell.
func TestParseAllowedTools_BareForeignNameIsNotTranslated(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: Bash\n---\nbody\n")
	if len(fm.AllowedTools) != 1 || fm.AllowedTools[0].Tool != "Bash" {
		t.Fatalf("a bare Bash was rewritten: %+v", fm.AllowedTools)
	}
}

// `Bash(*)` is the whole shell with punctuation on it, not a narrowing, so it
// is refused exactly like the bare name.
func TestParseAllowedTools_RefusesAnEmptyScope(t *testing.T) {
	for _, entry := range []string{"Bash(*)", "Bash()", "Bash(:*)"} {
		_, fm := ParseFrontmatterFields(
			"---\ndescription: d\nallowed-tools: " + entry + "\n---\nbody\n")
		if len(fm.AllowedTools) != 0 {
			t.Fatalf("%s granted %+v", entry, fm.AllowedTools)
		}
		if len(fm.Warnings) == 0 {
			t.Fatalf("%s was refused silently", entry)
		}
	}
}

// A specifier on any other tool is a path glob or a domain, and packetcode's
// policy has no rule shaped like that. Dropping the parentheses would turn
// permission to read one tree into permission to read any -- the opposite of
// what the author wrote -- so it grants nothing and says so.
func TestParseAllowedTools_RefusesAScopeItCannotExpress(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: read_file(src/**), write_file\n---\nbody\n")
	for _, got := range fm.AllowedTools {
		if got.Tool == "read_file" {
			t.Fatalf("an argument-scoped grant was widened to the whole tool: %+v", fm.AllowedTools)
		}
	}
	if len(fm.AllowedTools) != 1 || fm.AllowedTools[0].Tool != "write_file" {
		t.Fatalf("AllowedTools = %+v, want just write_file", fm.AllowedTools)
	}
	if len(fm.Warnings) == 0 {
		t.Fatal("refusing a grant must be reported, or the author never learns why prompts continue")
	}
}

// A comma inside a specifier belongs to the specifier, not to the list.
func TestParseAllowedTools_SplitsOutsideParenthesesOnly(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: Bash(git add -A, git commit), read_file\n---\nbody\n")
	if len(fm.AllowedTools) != 2 {
		t.Fatalf("the specifier was split on its own comma: %+v", fm.AllowedTools)
	}
	if fm.AllowedTools[0].Command != "git add -A, git commit" {
		t.Fatalf("specifier = %q", fm.AllowedTools[0].Command)
	}
}

// The containment that matters: a repository must not pre-approve the tools it
// then asks the model to use.
func TestAllowedTools_RefusedForAProjectSkill(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, ProjectSkillsDir(project), "deploy",
		"---\ndescription: d\nallowed-tools: execute_command\n---\nbody\n")

	reg := Load(project)
	s, ok := reg.Lookup("deploy")
	if !ok {
		t.Fatal("deploy did not resolve")
	}
	granted, refused := s.AllowedTools()
	if !refused {
		t.Fatal("a project skill's allowed-tools was honoured")
	}
	if len(granted) != 0 {
		t.Fatalf("a refused request still granted %v", granted)
	}
	// And the author is told, rather than left wondering why prompts continue.
	found := false
	for _, w := range reg.Warnings() {
		if strings.Contains(w, "allowed-tools") && strings.Contains(w, "deploy") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the refusal was silent: %v", reg.Warnings())
	}
}

// A scoped grant is still repository content, so it is refused for the same
// reason a bare one is: the narrowing does not change whose file it is.
func TestAllowedTools_ScopedGrantStillRefusedForAProjectSkill(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, ProjectSkillsDir(project), "deploy",
		"---\ndescription: d\nallowed-tools: Bash(gh:*)\n---\nbody\n")

	s, ok := Load(project).Lookup("deploy")
	if !ok {
		t.Fatal("deploy did not resolve")
	}
	granted, refused := s.AllowedTools()
	if !refused || len(granted) != 0 {
		t.Fatalf("granted=%+v refused=%v for a project skill", granted, refused)
	}
}

// A user-scope skill is the user's own file, so it is honoured.
func TestAllowedTools_HonouredForAUserSkill(t *testing.T) {
	isolate(t)
	writeSkill(t, mustUserSkillsDir(t), "mine",
		"---\ndescription: d\nallowed-tools: execute_command\n---\nbody\n")

	s, ok := Load(t.TempDir()).Lookup("mine")
	if !ok {
		t.Fatal("mine did not resolve")
	}
	granted, refused := s.AllowedTools()
	if refused {
		t.Fatal("a user-scope skill was refused")
	}
	if len(granted) != 1 || granted[0].Tool != "execute_command" || granted[0].Scoped() {
		t.Fatalf("granted = %+v", granted)
	}
}

// A skill saying nothing grants nothing, and is not reported as refused.
func TestAllowedTools_SilentWhenUnset(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	writeSkill(t, filepath.Join(ProjectSkillsDir(project)), "plain",
		"---\ndescription: d\n---\nbody\n")

	s, _ := Load(project).Lookup("plain")
	granted, refused := s.AllowedTools()
	if refused || len(granted) != 0 {
		t.Fatalf("granted=%v refused=%v for a skill that asked for nothing", granted, refused)
	}
}

// The returned slice must not alias the skill's own, or a caller could widen a
// grant for every later reader of the same registry. A ToolGrant carries a
// slice of its own, so the copy has to reach that too.
func TestAllowedTools_ReturnsACopy(t *testing.T) {
	isolate(t)
	writeSkill(t, mustUserSkillsDir(t), "mine",
		"---\ndescription: d\nallowed-tools: read_file, Bash(gh:*)\n---\nbody\n")
	s, _ := Load(t.TempDir()).Lookup("mine")

	granted, _ := s.AllowedTools()
	granted[0].Tool = "execute_command"
	granted[1].CommandPrefix[0] = "rm"

	again, _ := s.AllowedTools()
	if again[0].Tool != "read_file" || again[0].Scoped() {
		t.Fatalf("the grant list aliases the skill: %+v", again)
	}
	if again[1].CommandPrefix[0] != "gh" {
		t.Fatalf("the command prefix aliases the skill: %+v", again)
	}
}

// One warning per skill, not one per grant. A real ported skill lists eight
// scoped entries, and eight identical lines on every startup is a warning
// nobody reads the ninth time.
func TestParseAllowedTools_CoalescesRefusalsIntoOneWarning(t *testing.T) {
	_, fm := ParseFrontmatterFields("---\ndescription: d\nallowed-tools: " +
		"read_file(a/**), read_file(b/**), read_file(c/**), read_file(d/**), read_file(e/**)\n---\nbody\n")

	if len(fm.AllowedTools) != 0 {
		t.Fatalf("inexpressible grants were honoured: %+v", fm.AllowedTools)
	}
	if len(fm.Warnings) != 1 {
		t.Fatalf("expected one warning for five refusals, got %d: %v", len(fm.Warnings), fm.Warnings)
	}
	w := fm.Warnings[0]
	// Names the first few and counts the rest, so the reader can find them
	// without the line growing without bound.
	if !strings.Contains(w, `"read_file(a/**)"`) {
		t.Fatalf("the warning does not name what was refused: %q", w)
	}
	if !strings.Contains(w, "and 2 more") {
		t.Fatalf("the warning does not account for the rest: %q", w)
	}
	// And it says where narrowing does work, so the reader has somewhere to go.
	if !strings.Contains(w, ShellTool) {
		t.Fatalf("the warning does not say what packetcode can express: %q", w)
	}
}

// A single refusal reads as a single refusal.
func TestParseAllowedTools_SingleRefusalReadsNaturally(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: read_file(src/**)\n---\nbody\n")
	if len(fm.Warnings) != 1 {
		t.Fatalf("warnings = %v", fm.Warnings)
	}
	w := fm.Warnings[0]
	if strings.Contains(w, "and 0 more") || strings.Contains(w, "narrow ") {
		t.Fatalf("plural phrasing for a single refusal: %q", w)
	}
	if !strings.Contains(w, "narrows") || !strings.Contains(w, "for it") {
		t.Fatalf("singular phrasing missing: %q", w)
	}
}

// The three `~/.agents/skills` skills that prompted this: every one of them
// used the `Bash(cmd:*)` form, and every one of them warned on every startup
// while granting nothing. They now grant what their authors wrote.
func TestParseAllowedTools_RealPortedSkillIsSilent(t *testing.T) {
	_, fm := ParseFrontmatterFields("---\ndescription: d\nallowed-tools: " +
		"Bash(hf:*), Bash(gh:*), Bash(docker:*), Bash(aws:*), Bash(ssh-keygen:*), " +
		"Bash(ssh-add:*), Bash(ssh-agent:*)\n---\nbody\n")

	if len(fm.Warnings) != 0 {
		t.Fatalf("a skill written for the ecosystem still warns: %v", fm.Warnings)
	}
	if len(fm.AllowedTools) != 7 {
		t.Fatalf("AllowedTools = %+v", fm.AllowedTools)
	}
	for _, g := range fm.AllowedTools {
		if g.Tool != ShellTool || len(g.CommandPrefix) != 1 {
			t.Fatalf("grant = %+v", g)
		}
	}
}

package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAllowedTools(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: read_file, execute_command ,, write_file\n---\nbody\n")
	want := []string{"read_file", "execute_command", "write_file"}
	if len(fm.AllowedTools) != len(want) {
		t.Fatalf("AllowedTools = %v, want %v", fm.AllowedTools, want)
	}
	for i := range want {
		if fm.AllowedTools[i] != want[i] {
			t.Fatalf("AllowedTools = %v, want %v", fm.AllowedTools, want)
		}
	}
}

// `Bash(git status)` narrows a grant to particular commands. Dropping the
// parentheses would turn permission to run one command into permission to run
// any — the opposite of what the author wrote — so it grants nothing and says
// so.
func TestParseAllowedTools_RefusesArgumentScopedGrants(t *testing.T) {
	_, fm := ParseFrontmatterFields(
		"---\ndescription: d\nallowed-tools: execute_command(git status), read_file\n---\nbody\n")
	for _, got := range fm.AllowedTools {
		if strings.Contains(got, "execute_command") {
			t.Fatalf("an argument-scoped grant was widened to the whole tool: %v", fm.AllowedTools)
		}
	}
	if len(fm.AllowedTools) != 1 || fm.AllowedTools[0] != "read_file" {
		t.Fatalf("AllowedTools = %v, want just read_file", fm.AllowedTools)
	}
	if len(fm.Warnings) == 0 {
		t.Fatal("refusing a grant must be reported, or the author never learns why prompts continue")
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
	if len(granted) != 1 || granted[0] != "execute_command" {
		t.Fatalf("granted = %v", granted)
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
// grant for every later reader of the same registry.
func TestAllowedTools_ReturnsACopy(t *testing.T) {
	isolate(t)
	writeSkill(t, mustUserSkillsDir(t), "mine",
		"---\ndescription: d\nallowed-tools: read_file\n---\nbody\n")
	s, _ := Load(t.TempDir()).Lookup("mine")

	granted, _ := s.AllowedTools()
	granted[0] = "execute_command"

	again, _ := s.AllowedTools()
	if again[0] != "read_file" {
		t.Fatalf("the grant list aliases the skill: %v", again)
	}
}

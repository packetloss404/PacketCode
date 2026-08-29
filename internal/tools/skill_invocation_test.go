package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/skills"
)

// isolateSkillHome points every user-scope skills directory at scratch space.
// PACKETCODE_HOME alone leaves ~/.claude/skills and ~/.agents/skills pointing
// at the developer's real installation.
func isolateSkillHome(t *testing.T) {
	t.Helper()
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	osHome := t.TempDir()
	t.Setenv("HOME", osHome)
	t.Setenv("USERPROFILE", osHome)
}

func writeInvocationSkill(t *testing.T, dir, name, contents string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, skills.FileName), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func execSkillTool(t *testing.T, tool *SkillTool, name string) ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("execute %q: %v", name, err)
	}
	return res
}

// Omitting a skill from the index stops the model being told about it. It does
// not stop the model naming it -- the user says "run the deploy skill", or an
// earlier turn is still in context. The refusal has to live at the tool.
func TestSkillTool_RefusesModelDisabledSkill(t *testing.T) {
	isolateSkillHome(t)
	project := t.TempDir()
	writeInvocationSkill(t, skills.ProjectSkillsDir(project), "handsoff",
		"---\ndescription: only a human runs this\ndisable-model-invocation: true\n---\nsecret body text\n")

	tool := NewSkillTool(skills.Load(project))
	res := execSkillTool(t, tool, "handsoff")

	if !res.IsError {
		t.Fatalf("expected an error result, got %+v", res)
	}
	if strings.Contains(res.Content, "secret body text") {
		t.Fatalf("the refusal leaked the body:\n%s", res.Content)
	}
	// The reason has to be in the message. "unknown skill" would be a lie the
	// model then relays to a user who can see the skill in /skills.
	if !strings.Contains(res.Content, "disable-model-invocation") {
		t.Fatalf("refusal does not say why:\n%s", res.Content)
	}
}

// user-invocable: false is about a different surface entirely and must not
// stop the model loading the body.
func TestSkillTool_AllowsUserDisabledSkill(t *testing.T) {
	isolateSkillHome(t)
	project := t.TempDir()
	writeInvocationSkill(t, skills.ProjectSkillsDir(project), "background",
		"---\ndescription: reference only\nuser-invocable: false\n---\nreference body text\n")

	res := execSkillTool(t, NewSkillTool(skills.Load(project)), "background")
	if res.IsError {
		t.Fatalf("user-invocable: false must not block the model: %s", res.Content)
	}
	if !strings.Contains(res.Content, "reference body text") {
		t.Fatalf("body missing from result:\n%s", res.Content)
	}
}

// The "did you mean" list must not name a skill the model would then be
// refused; suggesting one is an invitation to a second failed turn.
func TestSkillTool_UnknownNameDoesNotSuggestModelDisabledSkills(t *testing.T) {
	isolateSkillHome(t)
	project := t.TempDir()
	writeInvocationSkill(t, skills.ProjectSkillsDir(project), "visible",
		"---\ndescription: pickable\n---\nbody\n")
	writeInvocationSkill(t, skills.ProjectSkillsDir(project), "handsoff",
		"---\ndescription: not pickable\ndisable-model-invocation: true\n---\nbody\n")

	res := execSkillTool(t, NewSkillTool(skills.Load(project)), "nosuchskill")
	if !res.IsError {
		t.Fatalf("expected an error result, got %+v", res)
	}
	if !strings.Contains(res.Content, "visible") {
		t.Fatalf("available list should name the invocable skill:\n%s", res.Content)
	}
	if strings.Contains(res.Content, "handsoff") {
		t.Fatalf("available list named a model-disabled skill:\n%s", res.Content)
	}
}

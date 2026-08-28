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

func skillRegistry(t *testing.T, name, contents string) *skills.Registry {
	t.Helper()
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	project := t.TempDir()
	dir := filepath.Join(skills.ProjectSkillsDir(project), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skills.FileName), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return skills.Load(project)
}

func runSkill(t *testing.T, tool *SkillTool, params string) ToolResult {
	t.Helper()
	res, err := tool.Execute(context.Background(), json.RawMessage(params))
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res
}

func TestSkillTool_LoadsBodyByName(t *testing.T) {
	reg := skillRegistry(t, "deploy", "---\ndescription: cut a release\n---\nrun the release script\n")
	tool := NewSkillTool(reg)

	res := runSkill(t, tool, `{"name":"deploy"}`)
	if res.IsError {
		t.Fatalf("unexpected error result: %q", res.Content)
	}
	if !strings.Contains(res.Content, "run the release script") {
		t.Fatalf("body missing from result: %q", res.Content)
	}
	if res.Metadata["skill"] != "deploy" || res.Metadata["source"] != skills.SourceProject {
		t.Fatalf("unexpected metadata: %#v", res.Metadata)
	}
}

// Loading reference material is not itself an action, so gating it would tax
// the cheap half of the design while changing nothing about what may follow.
func TestSkillTool_IsReadOnly(t *testing.T) {
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	tool := NewSkillTool(skills.Load(""))
	if tool.RequiresApproval() {
		t.Fatal("the skill tool must not be approval-gated")
	}
	if tool.Name() != "skill" {
		t.Fatalf("name = %q", tool.Name())
	}
	if !json.Valid(tool.Schema()) {
		t.Fatal("schema is not valid JSON")
	}
}

// A project body is repository content, the same trust class as project slash
// commands and workflows. The label is what keeps the model from reading it as
// operator instruction.
func TestSkillTool_LabelsProjectBodiesAsUntrusted(t *testing.T) {
	reg := skillRegistry(t, "local", "---\ndescription: repo convention\n---\nrepo body\n")
	res := runSkill(t, NewSkillTool(reg), `{"name":"local"}`)
	if !strings.Contains(res.Content, "project directory") {
		t.Fatalf("project body was not labelled: %q", res.Content)
	}

	builtin := skills.BuiltinNames()
	if len(builtin) == 0 {
		t.Skip("no builtin skills embedded")
	}
	res = runSkill(t, NewSkillTool(reg), `{"name":"`+builtin[0]+`"}`)
	if strings.Contains(res.Content, "project directory") {
		t.Fatalf("builtin body was labelled as project content: %q", res.Content)
	}
}

func TestSkillTool_UnknownNameListsWhatExists(t *testing.T) {
	reg := skillRegistry(t, "deploy", "---\ndescription: cut a release\n---\nbody\n")
	res := runSkill(t, NewSkillTool(reg), `{"name":"deply"}`)
	if !res.IsError {
		t.Fatal("expected an error result for an unknown skill")
	}
	if !strings.Contains(res.Content, "deploy") {
		t.Fatalf("available skills were not named: %q", res.Content)
	}
}

func TestSkillTool_RejectsMissingName(t *testing.T) {
	t.Setenv("PACKETCODE_HOME", t.TempDir())
	tool := NewSkillTool(skills.Load(""))
	for _, params := range []string{`{}`, `{"name":"  "}`, `{"name":`} {
		res := runSkill(t, tool, params)
		if !res.IsError {
			t.Errorf("params %q produced a success result: %q", params, res.Content)
		}
	}
}

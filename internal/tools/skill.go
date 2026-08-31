package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/packetcode/packetcode/internal/skills"
)

const skillSchema = `{
  "type": "object",
  "properties": {
    "name": { "type": "string", "description": "Skill name exactly as listed in <available_skills>." },
    "file": { "type": "string", "description": "Optional. A resource file listed by a previous call for this skill, relative to the skill directory (for example \"references/rules.md\"). Omit to load the skill body." }
  },
  "required": ["name"]
}`

// SkillTool loads one skill body, or one resource file beside it, by name.
//
// It is read-only and not approval-gated on purpose: reading reference material
// is not itself an action, and gating it would tax the cheap half of the design
// while changing nothing about what the agent may then do. Every action a body
// suggests still passes through its own tool's approval, which is the boundary
// that matters.
type SkillTool struct {
	registry *skills.Registry
	// onLoad, when set, is called with each skill whose body this tool serves.
	//
	// It exists for `allowed-tools`: the widening has to reach the policy the
	// running turn is being decided against, and a tool has no route to that.
	// The App wires it. Optional -- a nil hook means a skill body still loads,
	// it just pre-approves nothing, which is the safe direction for a seam that
	// might not be wired.
	onLoad func(skills.Skill)
}

// SetOnLoad registers the callback invoked when a skill body is served.
func (t *SkillTool) SetOnLoad(fn func(skills.Skill)) { t.onLoad = fn }

// NewSkillTool builds the tool over an already-resolved registry. Resolution
// happens once at startup rather than per call so the index in the system
// prompt and the bodies this tool serves cannot disagree mid-session.
func NewSkillTool(registry *skills.Registry) *SkillTool {
	return &SkillTool{registry: registry}
}

func (*SkillTool) Name() string            { return "skill" }
func (*SkillTool) RequiresApproval() bool  { return false }
func (*SkillTool) Schema() json.RawMessage { return json.RawMessage(skillSchema) }
func (*SkillTool) Description() string {
	return "Load the full instructions for one skill listed in <available_skills>. Call this before acting when a skill covers the task. A skill may list resource files; call again with that file to read one."
}

type skillParams struct {
	Name string `json:"name"`
	File string `json:"file,omitempty"`
}

func (t *SkillTool) Execute(_ context.Context, params json.RawMessage) (ToolResult, error) {
	var p skillParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return ToolResult{Content: fmt.Sprintf("invalid parameters: %s", err), IsError: true}, nil
		}
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return ToolResult{Content: "name is required", IsError: true}, nil
	}

	skill, ok := t.registry.Lookup(name)
	if ok && !skill.Enabled() {
		// Belt to the index's braces. The model should never have heard of
		// this skill, but it can still name one the user mentioned, and the
		// answer has to be the same refusal either way.
		return ToolResult{
			Content: fmt.Sprintf(
				"skill %q was found in this repository's .%s/skills/ directory and has not been "+
					"approved. It is not loaded. Tell the user they can review it with /skills %s "+
					"and enable it with /skills allow %s.",
				name, skill.Origin, name, name),
			IsError: true,
		}, nil
	}
	if ok && !skill.ModelInvocable() {
		// The refusal lives here, not only in the index. Keeping the skill out
		// of <available_skills> stops the model being told about it; it does
		// not stop the model naming it, and it will -- the user says "run the
		// deploy skill", or an earlier turn is still in context. This is the
		// enforcement point, and it says why rather than pretending the skill
		// does not exist, so the model relays something true to the user.
		return ToolResult{
			Content: fmt.Sprintf("skill %q sets disable-model-invocation; only the user can invoke it, with /%s", name, name),
			IsError: true,
		}, nil
	}
	if !ok {
		// Naming what is available turns a typo into a one-turn recovery rather
		// than a guess the model repeats. Model-disabled skills are left out:
		// suggesting one as an alternative is an invitation to a refusal.
		var available []string
		for _, s := range t.registry.Skills() {
			if s.Enabled() && s.ModelInvocable() {
				available = append(available, s.Name)
			}
		}
		if len(available) == 0 {
			return ToolResult{Content: "no skills are available", IsError: true}, nil
		}
		return ToolResult{
			Content: fmt.Sprintf("unknown skill %q; available: %s", name, strings.Join(available, ", ")),
			IsError: true,
		}, nil
	}

	if file := strings.TrimSpace(p.File); file != "" {
		return t.readResource(skill, file)
	}
	return t.readBody(skill)
}

func (t *SkillTool) readBody(skill skills.Skill) (ToolResult, error) {
	// Announced before the body is returned, so a grant is in force for the
	// tool calls the body is about to prompt rather than one call too late.
	if t.onLoad != nil {
		t.onLoad(skill)
	}
	body := skill.Block()
	resources, truncated := t.registry.Resources(skill.Name)

	// The listing is appended outside the labelled block, not inside it. A
	// skill under the wider Agent Skills convention is often a dispatcher
	// whose body names files it expects the agent to read; without a listing
	// the model must guess those paths from prose, and a wrong guess reads as
	// "the skill is broken" rather than "that file is not there".
	var b strings.Builder
	b.WriteString(body)
	if len(resources) > 0 {
		b.WriteString("\n\nFiles this skill carries. Read one by calling skill again with that name in `file`:\n")
		for _, r := range resources {
			b.WriteString("- ")
			b.WriteString(r)
			b.WriteString("\n")
		}
		if truncated {
			fmt.Fprintf(&b, "(listing stopped at %d files; others exist but are not shown)\n", skills.MaxResourcesListed)
		}
	}

	return ToolResult{
		Content: b.String(),
		Metadata: map[string]any{
			"skill":     skill.Name,
			"source":    skill.Source,
			"resources": len(resources),
		},
	}, nil
}

func (t *SkillTool) readResource(skill skills.Skill, file string) (ToolResult, error) {
	data, err := t.registry.ReadResource(skill.Name, file)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("skill %q: %s", skill.Name, err), IsError: true}, nil
	}
	if !utf8.Valid(data) {
		// Mirrors read_file: rendering a binary into the turn spends the
		// context window on mojibake and tells the model nothing.
		return ToolResult{
			Content: fmt.Sprintf("skill %q: %s appears to be binary or invalid UTF-8; refusing to render", skill.Name, file),
			IsError: true,
		}, nil
	}
	return ToolResult{
		Content: skill.ResourceBlock(file, data),
		Metadata: map[string]any{
			"skill":  skill.Name,
			"source": skill.Source,
			"file":   file,
			"bytes":  len(data),
		},
	}, nil
}

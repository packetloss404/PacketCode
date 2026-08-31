package app

import (
	"fmt"
	"strings"

	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/skills"
	"github.com/packetcode/packetcode/internal/tools"
)

// skillGrant is a permission widening that lasts one turn.
//
// `allowed-tools` is the one skill field that adds authority rather than
// describing content, so everything about how it is applied is a containment
// decision:
//
//   - Only trusted skills reach here at all; skills.Skill.AllowedTools refuses
//     a project skill outright, because a repository must not pre-approve the
//     tools it then asks the model to use.
//   - The grant converts Ask to Allow and nothing else. permissions.Policy
//     checks deny floors before any other rule, so an explicit deny still wins
//     over a grant added after it — the narrowing property is structural rather
//     than something this file has to remember.
//   - It is torn down when the turn ends. A widening that outlived its turn
//     would be indistinguishable from a permanent policy change the user never
//     made.
//   - A name that is not a registered tool grants nothing and says so. Claude
//     Code's tool names (Bash, Read, Edit) are not packetcode's, so a skill
//     written for that ecosystem lands here with names that match nothing;
//     silently granting the closest-looking tool would be guessing about
//     exactly the thing that must not be guessed.
type skillGrant struct {
	skill    string
	tools    []string
	previous *permissions.Policy
}

// skillLoadedMsg carries a model-selected skill from the agent's goroutine into
// the Update loop.
//
// The tool cannot apply the grant itself: it runs on the agent's goroutine and
// the policy lives on the App, whose fields are only safe to touch inside
// Update. Posting a message is how every other off-thread callback in this
// package reaches the same place.
type skillLoadedMsg struct{ skill skills.Skill }

// wireSkillGrants connects the skill tool's load hook to the Update loop, so a
// skill the model selects can widen the turn the same way one the user typed
// does.
func (a *App) wireSkillGrants() {
	if a.deps.Tools == nil {
		return
	}
	tool, ok := a.deps.Tools.Get("skill")
	if !ok {
		return
	}
	st, ok := tool.(*tools.SkillTool)
	if !ok {
		return
	}
	st.SetOnLoad(func(s skills.Skill) {
		if a.sendMsg != nil {
			a.sendMsg(skillLoadedMsg{skill: s})
		}
	})
}

// applySkillGrant widens the policy for the turn a skill is invoked in and
// returns a note for the transcript, or "" when nothing was granted.
//
// The previous policy is captured so the teardown restores what was actually in
// force rather than recomputing it: the user may have changed the profile in
// the same turn, and a grant must not silently revert that.
func (a *App) applySkillGrant(s skills.Skill) string {
	granted, refused := s.AllowedTools()
	if refused {
		return fmt.Sprintf("%s sets allowed-tools, which is not honoured for a project skill; "+
			"tool approvals will be asked for as usual", s.Name)
	}
	if len(granted) == 0 {
		return ""
	}

	base := a.currentPermissionPolicy()
	policy := base
	var applied, unknown []string
	for _, name := range granted {
		if a.deps.Tools == nil {
			break
		}
		if _, ok := a.deps.Tools.Get(name); !ok {
			unknown = append(unknown, name)
			continue
		}
		policy = policy.WithRule(name, permissions.DecisionAllow)
		applied = append(applied, name)
	}

	var note strings.Builder
	if len(applied) > 0 {
		a.activeSkillGrant = &skillGrant{skill: s.Name, tools: applied, previous: base}
		a.setPermissionPolicy(policy)
		fmt.Fprintf(&note, "%s pre-approved %s for this turn (an explicit deny still applies)",
			s.Name, strings.Join(applied, ", "))
	}
	if len(unknown) > 0 {
		if note.Len() > 0 {
			note.WriteString("\n")
		}
		fmt.Fprintf(&note, "%s asked for %s, which %s not a packetcode tool; nothing was granted for %s. "+
			"packetcode's tool names differ from Claude Code's — see /permissions for the list.",
			s.Name, strings.Join(unknown, ", "),
			map[bool]string{true: "is", false: "are"}[len(unknown) == 1],
			map[bool]string{true: "it", false: "them"}[len(unknown) == 1])
	}
	return note.String()
}

// releaseSkillGrant restores the policy a skill grant replaced.
//
// Called when the turn ends, however it ends: completion, cancellation, or
// error. A grant that survived a cancelled turn would be the worst version of
// this feature — authority the user never saw used, still in force.
func (a *App) releaseSkillGrant() string {
	grant := a.activeSkillGrant
	if grant == nil {
		return ""
	}
	a.activeSkillGrant = nil
	a.setPermissionPolicy(grant.previous)
	return fmt.Sprintf("%s's pre-approval of %s has ended with the turn",
		grant.skill, strings.Join(grant.tools, ", "))
}

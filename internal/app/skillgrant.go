package app

import (
	"fmt"
	"slices"
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
//   - A grant narrowed to particular commands -- `Bash(gh:*)` -- becomes a
//     command-prefix or exact-command rule rather than a grant of the whole
//     shell. That is the one case where a foreign tool name is translated,
//     because the scope travels with it: see skills.shellToolName.
type skillGrant struct {
	skills   []string
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
	var applied, unknown, inexpressible []string
	for _, g := range granted {
		if a.deps.Tools == nil {
			break
		}
		if _, ok := a.deps.Tools.Get(g.Tool); !ok {
			unknown = append(unknown, g.Tool)
			continue
		}
		if g.Scoped() && g.Tool != skills.ShellTool {
			// Unreachable from a file on disk -- the parser only attaches a
			// scope to the shell tool, because it is the only tool whose
			// parameters carry a command for a rule to match. Granting the
			// bare tool instead would be the widening this whole path exists
			// to refuse, so a scope that cannot be expressed grants nothing.
			inexpressible = append(inexpressible, g.Label())
			continue
		}
		switch {
		case len(g.CommandPrefix) > 0:
			policy = policy.WithCommandPrefixRule(g.CommandPrefix, permissions.DecisionAllow)
		case g.Command != "":
			policy = policy.WithCommandRule(g.Command, permissions.DecisionAllow)
		default:
			policy = policy.WithRule(g.Tool, permissions.DecisionAllow)
		}
		applied = append(applied, g.Label())
	}

	var note strings.Builder
	if len(applied) > 0 {
		// All grants in a turn share the original policy snapshot. Capturing
		// an already-widened policy on a later load would restore an earlier
		// skill's permissions at teardown, leaving them active indefinitely.
		if a.activeSkillGrant == nil {
			a.activeSkillGrant = &skillGrant{previous: base}
		}
		grant := a.activeSkillGrant
		if !slices.Contains(grant.skills, s.Name) {
			grant.skills = append(grant.skills, s.Name)
		}
		for _, label := range applied {
			if !slices.Contains(grant.tools, label) {
				grant.tools = append(grant.tools, label)
			}
		}
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
	if len(inexpressible) > 0 {
		if note.Len() > 0 {
			note.WriteString("\n")
		}
		fmt.Fprintf(&note, "%s narrowed %s to particular arguments, which packetcode can express only "+
			"for %s; nothing was granted for %s.",
			s.Name, strings.Join(inexpressible, ", "), skills.ShellTool,
			map[bool]string{true: "it", false: "them"}[len(inexpressible) == 1])
	}
	return note.String()
}

// releaseSkillGrant restores the policy a skill grant replaced.
//
// Called when the turn ends, however it ends: completion, cancellation, or
// error. A grant that survived a cancelled turn would be the worst version of
// this feature — authority the user never saw used, still in force.
//
// The rules revert to the pre-grant snapshot, but the profile is the one in
// force now: the user may have cycled to plan mode mid-turn, and restoring
// the snapshot's profile would leave the plan flag set over a policy that is
// no longer read-only.
func (a *App) releaseSkillGrant() string {
	grant := a.activeSkillGrant
	if grant == nil {
		return ""
	}
	a.activeSkillGrant = nil
	a.setPermissionPolicy(grant.previous.WithProfile(a.currentPermissionPolicy().Profile()))
	return fmt.Sprintf("pre-approval from %s for %s has ended with the turn",
		strings.Join(grant.skills, ", "), strings.Join(grant.tools, ", "))
}

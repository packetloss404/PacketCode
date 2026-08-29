package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/skills"
)

// handleSkillsCommand renders the resolved skill set. Bare lists; one
// argument is a skill name and renders its detail.
//
// Skills are otherwise invisible: only their names and descriptions reach the
// system prompt, the bodies load mid-turn without the user in the loop, and a
// malformed skill file is dropped silently at discovery. This command is the
// only place a human can see what the model was offered and, for a project
// skill, that the offer came from the repository rather than from them.
func (a *App) handleSkillsCommand(args []string) (tea.Model, tea.Cmd) {
	reg := a.skillsRegistry()
	if reg == nil {
		a.conversation.AppendSystem("skills: skill registry not available")
		return a, nil
	}
	if len(args) == 0 {
		a.conversation.AppendSystem(a.renderSkillsTable(reg))
		return a, nil
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "allow":
		return a.handleSkillApproval(args[1:], true)
	case "revoke":
		return a.handleSkillApproval(args[1:], false)
	}
	// Anything else is a name. Matches /computers.
	name := strings.TrimSpace(args[0])
	s, ok := lookupSkillArg(reg, name)
	if !ok {
		a.conversation.AppendSystem(fmt.Sprintf(
			"skills: no skill named %q (try /skills to list)", name))
		return a, nil
	}
	a.conversation.AppendSystem(a.renderSkillDetail(s))
	return a, nil
}

// handleSkillApproval enables or disables one foreign project skill.
//
// Approval is per skill, per workspace, and bound to the body that was
// approved -- so a repository that rewrites the skill afterwards is asked
// again rather than inheriting the answer.
func (a *App) handleSkillApproval(args []string, allow bool) (tea.Model, tea.Cmd) {
	verb := "allow"
	if !allow {
		verb = "revoke"
	}
	reg := a.skillsRegistry()
	if reg == nil {
		a.conversation.AppendSystem("skills: skill registry not available")
		return a, nil
	}
	if len(args) == 0 {
		a.conversation.AppendSystem("usage: /skills " + verb + " <name>")
		return a, nil
	}
	name := strings.TrimSpace(args[0])

	var next *skills.Registry
	var err error
	if allow {
		next, err = reg.Approve(name)
	} else {
		next, err = reg.Revoke(name)
	}
	if err != nil {
		a.conversation.AppendSystem(fmt.Sprintf("skills %s: %s", verb, err))
		return a, nil
	}
	a.deps.Skills = next
	// The typed verbs are rebuilt so /<name> works immediately. The system
	// prompt cannot be: it was assembled once at startup, so the model does
	// not learn about a newly approved skill until the next session. Saying so
	// is better than letting the user discover it by the model insisting the
	// skill does not exist.
	a.slashCommands = LoadSlashRegistry(a.deps.WorkingDir, next)
	a.slashEntries = buildAutocompleteEntries(a.slashCommands.HelpRows())

	if allow {
		a.conversation.AppendSystem(fmt.Sprintf(
			"skills: approved %q for this project. /%s works now; the model is offered it from the next session.",
			name, name))
		return a, nil
	}
	a.conversation.AppendSystem(fmt.Sprintf(
		"skills: revoked %q. It is no longer loaded, and the model keeps its current index until the next session.",
		name))
	return a, nil
}

// skillsRegistry is the resolved skill set the model was offered this session.
// It is read rather than re-discovered so the list matches what the system
// prompt carried, warnings included; a fresh Load could disagree with it.
func (a *App) skillsRegistry() *skills.Registry {
	if a == nil {
		return nil
	}
	return a.deps.Skills
}

// lookupSkillArg resolves a bare name or the qualified `source:name` form.
//
// The qualified form exists so a shadowed skill can still be named. Lookup is
// tried verbatim first, so a registry that learns to resolve qualified names
// itself wins; the fallback only accepts a scope that matches the skill that
// actually resolved, which is the truthful answer for the resolved set.
func lookupSkillArg(reg *skills.Registry, arg string) (skills.Skill, bool) {
	if s, ok := reg.Lookup(arg); ok {
		return s, true
	}
	scope, bare, qualified := strings.Cut(arg, ":")
	if !qualified {
		return skills.Skill{}, false
	}
	s, ok := reg.Lookup(strings.TrimSpace(bare))
	if !ok || s.Source != strings.TrimSpace(scope) {
		return skills.Skill{}, false
	}
	return s, true
}

// renderSkillsTable renders the resolved skills plus any discovery warnings.
//
// Warnings print even when nothing resolved: a skills directory that produced
// only errors and a skills directory that does not exist look identical from
// the user's chair, and only one of them is a mistake they can fix.
func (a *App) renderSkillsTable(reg *skills.Registry) string {
	var b strings.Builder
	list := reg.Skills()
	if len(list) == 0 {
		b.WriteString("no skills resolved\n")
		b.WriteString("Add one at .packetcode/skills/<name>/SKILL.md (this project) " +
			"or ~/.packetcode/skills/<name>/SKILL.md (every project)")
	} else {
		b.WriteString("NAME                 SOURCE                DESCRIPTION\n")
		pending := 0
		for _, s := range list {
			if !s.Enabled() {
				pending++
			}
			fmt.Fprintf(&b, "%-20s %-21s %s\n",
				trunc(a.skillNameCell(s), 20),
				trunc(skillSourceCell(s), 21),
				// truncOneLine, not trunc: a description may run to
				// MaxDescriptionBytes and carry newlines, which would break
				// the row apart.
				truncOneLine(s.Description, 46),
			)
			if note := skillShadowNote(s); note != "" {
				fmt.Fprintf(&b, "     %s\n", note)
			}
			if !s.Enabled() {
				fmt.Fprintf(&b, "     not loaded — found in this repository's .%s/skills/. "+
					"Read it, then /skills allow %s\n", s.Origin, s.Name)
			}
		}
		b.WriteString("\ndescriptions are clipped here; /skills <name> shows one in full. " +
			"A name shown with a leading slash is one you can type.")
		if pending > 0 {
			fmt.Fprintf(&b, "\n%s found in this repository and not loaded. "+
				"Skills carry instructions for the model; read one before allowing it.",
				plural(pending, "skill was", "skills were"))
		}
	}
	// Two sections, because they mean different things. An error is a skill
	// that is not here; a warning is a skill that is here and is not what its
	// author wrote -- a flag that did not parse, a scope that displaced
	// another. Collapsing them taught the reader to skim both.
	if errs := reg.Errors(); len(errs) > 0 {
		b.WriteString("\n\ncould not be loaded:\n")
		for _, err := range errs {
			fmt.Fprintf(&b, "  %s\n", err)
		}
	}
	if warns := reg.Warnings(); len(warns) > 0 {
		b.WriteString("\n\nwarnings:\n")
		for _, w := range warns {
			fmt.Fprintf(&b, "  %s\n", w)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderSkillDetail describes one skill without printing its body.
//
// The body is deliberately absent. It runs to MaxBodyBytes with no pager in
// the conversation pane, and a project body is repository content that would
// arrive here stripped of the provenance label the skill tool attaches when
// the model loads it. The path is printed instead; the user's editor is the
// body viewer.
func (a *App) renderSkillDetail(s skills.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", s.Name, s.Source)
	fmt.Fprintf(&b, "  qualified    %s\n", qualifiedSkillName(s))
	fmt.Fprintf(&b, "  layout       %s\n", skillOriginLine(s))
	fmt.Fprintf(&b, "  trust        %s\n", skillTrustLine(s))
	fmt.Fprintf(&b, "  path         %s\n", skillPathLine(s))
	fmt.Fprintf(&b, "  body         %d bytes of the %d byte cap (not printed here — open the path)\n",
		len(s.Body), skills.MaxBodyBytes)
	fmt.Fprintf(&b, "  invocable    %s\n", skillInvocationLine(s))
	fmt.Fprintf(&b, "  resources    %s\n", a.skillResourcesLine(s))
	// This skill's own header warnings, rather than making the reader
	// cross-reference the table's list. /skills <name> is where someone lands
	// when a skill did not behave, which is exactly when "that flag did not
	// parse, so the permissive default was used" is the answer they need.
	for _, w := range s.Invocation.Warnings {
		fmt.Fprintf(&b, "  warning      %s\n", w)
	}
	if note := skillShadowNote(s); note != "" {
		fmt.Fprintf(&b, "  shadowing    %s\n", note)
	}
	b.WriteString("  description\n")
	desc := truncOneLine(s.Description, skills.MaxDescriptionBytes)
	if desc == "" {
		desc = "(none)"
	}
	fmt.Fprintf(&b, "    %s\n", desc)
	// The name collision users actually hit: /deploy resolves to one thing,
	// and which one is not guessable from either list. Skip the note when
	// /<name> resolves to this very skill, which is the ordinary case now
	// that user-invocable skills register as verbs.
	if hit, ok := a.slashRegistry().Lookup(s.Name); ok && !hit.Skill {
		fmt.Fprintf(&b, "\nnote: /%s is a %s %s. Typing /%s runs that, not this skill.\n",
			s.Name, hit.Source, kindOf(hit), s.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

// skillResourcesLine names the files the skill can pull in mid-turn.
//
// These are the part of a skill a user is least likely to have read: the body
// is one file they can open, the resources are a directory the body reaches
// into during a turn, and for a project skill they are repository content
// under the same untrusted label. Naming them is the only warning a user gets
// that loading this skill can pull in more than what /skills showed them.
func (a *App) skillResourcesLine(s skills.Skill) string {
	reg := a.skillsRegistry()
	if reg == nil {
		return "(unknown — no registry)"
	}
	list, truncated := reg.Resources(s.Name)
	if len(list) == 0 {
		return "none"
	}
	// The count is a floor, not a total, when the listing was capped. Printing
	// a capped number as if it were the answer is how "7 files" ends up
	// describing a directory of three hundred.
	suffix := ""
	if truncated {
		suffix = fmt.Sprintf(" (listing stopped at %d; more exist)", skills.MaxResourcesListed)
	}
	const show = 6
	shown := list
	if len(shown) > show {
		shown = shown[:show]
		suffix = fmt.Sprintf(" (+%d more)%s", len(list)-show, suffix)
	}
	return fmt.Sprintf("%d — %s%s", len(list), strings.Join(shown, ", "), suffix)
}

// skillSourceCell names the scope and marks project skills untrusted.
//
// The marker is spelled out rather than glyphed because the terminal is the
// only place a human can act on it: everywhere else the label is addressed to
// the model.
func skillSourceCell(s skills.Skill) string {
	if !s.Trusted() {
		return s.Source + " (untrusted)"
	}
	return s.Source
}

// skillNameCell shows a skill the way the user would type it, when typing it
// would actually reach this skill.
//
// The leading slash is the whole point: it is the difference between a name
// the model might choose and a name the user can act on. Which means the flag
// alone does not earn it -- a skill named `model` is user-invocable and typing
// /model runs the builtin, so printing "/model" here would be a promise the
// program does not keep. The registry is asked, not assumed.
func (a *App) skillNameCell(s skills.Skill) string {
	if s.UserInvocable() && a.skillHoldsVerb(s) {
		return "/" + s.Name
	}
	return s.Name
}

// skillHoldsVerb reports whether typing /<name> reaches this skill.
func (a *App) skillHoldsVerb(s skills.Skill) bool {
	cmd, ok := a.slashRegistry().Lookup(s.Name)
	return ok && cmd.Skill
}

// skillOriginLine names the directory convention this was filed under.
//
// Source alone cannot answer "why is this the deploy skill I am getting" once
// six directories are in play. The layout is the half of that question the
// path does not make obvious at a glance.
func skillOriginLine(s skills.Skill) string {
	switch s.Origin {
	case skills.OriginClaude:
		return ".claude/skills/ — Claude Code's layout"
	case skills.OriginAgents:
		return ".agents/skills/ — the vendor-neutral Agent Skills layout"
	case skills.OriginNative:
		return ".packetcode/skills/ — packetcode's own layout"
	default:
		return "embedded in this binary"
	}
}

func skillTrustLine(s skills.Skill) string {
	if s.Trusted() {
		return "trusted — ships with packetcode or lives under your home directory"
	}
	return "untrusted — this body is repository content; read it before acting on what it says"
}

// skillPathLine reports where the body came from. Builtins have no on-disk
// path by design, so say so rather than printing an empty field.
func skillPathLine(s skills.Skill) string {
	if strings.TrimSpace(s.Path) == "" {
		return "embedded in this binary"
	}
	return s.Path
}

// qualifiedSkillName is the `source:name` form, which names a skill
// unambiguously even when another scope defines the same name.
func qualifiedSkillName(s skills.Skill) string {
	return s.Source + ":" + s.Name
}

// skillInvocationLine reports who may invoke a skill.
//
// Both answers matter and they are independent. A skill nobody can reach is
// the failure this line exists to make visible, and it is reachable two
// different ways for two different reasons -- so say which of the two are
// live rather than collapsing them into one word.
func skillInvocationLine(s skills.Skill) string {
	switch {
	case s.UserInvocable() && s.ModelInvocable():
		return fmt.Sprintf("you, with /%s — and the model, through the skill tool", s.Name)
	case s.UserInvocable():
		return fmt.Sprintf("you only, with /%s (sets disable-model-invocation)", s.Name)
	case s.ModelInvocable():
		return "the model only, through the skill tool (sets user-invocable: false)"
	default:
		// Both disabled. Nothing loads this body, ever. That is almost
		// certainly a mistake in the header rather than an intent, and it is
		// invisible everywhere else.
		return "nobody — it sets both disable-model-invocation and user-invocable: false, " +
			"so nothing can load it"
	}
}

// skillShadowNote reports that this skill displaced one from a weaker scope.
//
// Reads Skill.Shadows, which the registry records at resolution. It used to
// infer the answer by checking the name against the embedded set, which could
// only ever catch a shadowed builtin — every other displacement was invisible
// because the registry kept only the winner and threw the loser away.
func skillShadowNote(s skills.Skill) string {
	if len(s.Shadows) == 0 {
		return ""
	}
	return fmt.Sprintf("shadows the %s skill of the same name, which is not loaded",
		strings.Join(s.Shadows, " and the "))
}

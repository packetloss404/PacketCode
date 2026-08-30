package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/components/conversation"
	"github.com/packetcode/packetcode/internal/ui/components/picker"
)

const resumeTranscriptLimit = 20

// resumeSessionByID loads a saved session and makes it current. Both
// `/sessions resume <id>` and the `/resume` picker go through it.
//
// It is deliberately one function rather than two call paths that each do the
// work. The sequence carries a workspace-identity check that stops a
// remote-bound transcript resuming against the local filesystem, and restores
// the previous session on every failure so a refused resume leaves nothing
// half-swapped. A second copy is precisely how one of those steps goes
// missing. label names the command in messages so the user is told which
// thing failed.
func (a *App) resumeSessionByID(fullID, label string) (tea.Model, tea.Cmd) {
	if a.streaming {
		a.conversation.AppendSystem(label + ": turn already running; press Ctrl+C to cancel before resuming")
		return a, nil
	}
	prev := a.deps.Sessions.Current()
	s, loadErr := a.deps.Sessions.Load(fullID)
	if loadErr != nil {
		a.conversation.AppendSystem(label + ": " + loadErr.Error())
		return a, nil
	}
	if workspaceErr := session.ValidateWorkspace(s, a.deps.ComputerID, a.deps.WorkingDir, a.deps.WorkspaceIdentity); workspaceErr != nil {
		a.restorePreviousSession(prev)
		a.conversation.AppendSystem(label + ": " + workspaceErr.Error())
		return a, nil
	}
	if s.Provider == "" || s.Model == "" {
		a.restorePreviousSession(prev)
		a.conversation.AppendSystem(label + ": resumed session has no provider/model metadata")
		return a, nil
	}
	if err := a.deps.Registry.SetActive(s.Provider, s.Model); err != nil {
		a.restorePreviousSession(prev)
		a.conversation.AppendSystem(label + ": " + err.Error())
		return a, nil
	}
	if err := a.rebindSessionScopedTools(s.ID); err != nil {
		a.restorePreviousSession(prev)
		a.conversation.AppendSystem(label + ": " + err.Error())
		return a, nil
	}
	a.refreshTopBar()
	a.showResumedSession(s)
	return a, a.renderStatusLine(false)
}

// handleSessionsCommand lists, resumes, or deletes sessions. The bare
// form shows the top 20 newest-first; resume/delete accept either a
// full ID or any unique 8-char prefix; delete is gated on --yes because
// it is irreversible.
func (a *App) handleSessionsCommand(args []string) (tea.Model, tea.Cmd) {
	sub, id, name, yes, err := parseSessionsArgs(args)
	if err != nil {
		a.conversation.AppendSystem("sessions: " + err.Error())
		return a, nil
	}

	if sub == "" {
		summaries, problems, listErr := a.deps.Sessions.ListWithProblems()
		if listErr != nil {
			a.conversation.AppendSystem("sessions: list failed: " + listErr.Error())
			return a, nil
		}
		// Reported, not swallowed. A session file that cannot be read used to
		// vanish from this list, which is indistinguishable from a session
		// that never existed -- and this is the command someone runs when they
		// know it did.
		a.reportSessionProblems("sessions", problems)
		currentID := ""
		if cur := a.deps.Sessions.Current(); cur != nil {
			currentID = cur.ID
		}
		a.conversation.AppendSystem(renderSessionsTable(summaries, currentID))
		return a, nil
	}

	if sub == "rename" {
		if err := a.deps.Sessions.Rename(name); err != nil {
			a.conversation.AppendSystem("sessions: " + err.Error())
			return a, nil
		}
		cur := a.deps.Sessions.Current()
		if cur == nil {
			a.conversation.AppendSystem("sessions: renamed")
			return a, nil
		}
		a.conversation.AppendSystem(fmt.Sprintf("renamed session %s to %s", shortID(cur.ID), cur.Name))
		return a, nil
	}

	fullID, resolveErr := a.resolveSessionID(id)
	if resolveErr != nil {
		a.conversation.AppendSystem("sessions: " + resolveErr.Error())
		return a, nil
	}

	switch sub {
	case "resume":
		return a.resumeSessionByID(fullID, "sessions")

	case "delete":
		if !yes {
			a.conversation.AppendSystem(fmt.Sprintf(
				"sessions: refusing to delete without --yes; re-run: /sessions delete %s --yes",
				id,
			))
			return a, nil
		}
		current := a.deps.Sessions.Current()
		deletingActive := current != nil && current.ID == fullID
		if deletingActive && a.streaming {
			a.conversation.AppendSystem("sessions: turn already running; press Ctrl+C to cancel before deleting the active session")
			return a, nil
		}
		var replacement *session.Session
		if deletingActive {
			providerSlug, modelID := current.Provider, current.Model
			if providerSlug == "" || modelID == "" {
				if prov, activeModel := a.deps.Registry.Active(); prov != nil {
					providerSlug = prov.Slug()
					modelID = activeModel
				}
			}
			if providerSlug == "" || modelID == "" {
				a.conversation.AppendSystem("sessions: cannot delete active session without provider/model metadata")
				return a, nil
			}
			var newErr error
			replacement, newErr = a.deps.Sessions.New(providerSlug, modelID)
			if newErr != nil {
				a.conversation.AppendSystem("sessions: create replacement session: " + newErr.Error())
				return a, nil
			}
			if a.deps.ComputerID != "" {
				if bindErr := a.deps.Sessions.BindWorkspace(a.deps.ComputerID, a.deps.WorkingDir, a.deps.WorkspaceIdentity); bindErr != nil {
					a.conversation.AppendSystem("sessions: bind replacement workspace: " + bindErr.Error())
					return a, nil
				}
			}
		}
		if delErr := a.deps.Sessions.Delete(fullID); delErr != nil {
			if replacement != nil {
				_, _ = a.deps.Sessions.Load(fullID)
				_ = a.deps.Sessions.Delete(replacement.ID)
				_ = a.rebindSessionScopedTools(fullID)
			}
			a.conversation.AppendSystem("sessions: " + delErr.Error())
			return a, nil
		}
		if cleanupErr := a.cleanupSessionBackups(fullID); cleanupErr != nil {
			a.conversation.AppendSystem("sessions: backup cleanup failed: " + cleanupErr.Error())
			return a, nil
		}
		if replacement != nil {
			if err := a.rebindSessionScopedTools(replacement.ID); err != nil {
				a.conversation.AppendSystem("sessions: " + err.Error())
				return a, nil
			}
		}
		a.refreshTopBar()
		a.conversation.AppendSystem("deleted session " + shortID(fullID))
		return a, nil
	}

	// Unreachable: parseSessionsArgs rejects anything else.
	a.conversation.AppendSystem("sessions: unexpected subcommand " + sub)
	return a, nil
}

func (a *App) restorePreviousSession(prev *session.Session) {
	if prev == nil {
		return
	}
	_, _ = a.deps.Sessions.Load(prev.ID)
}

func (a *App) rebindSessionScopedTools(sessionID string) error {
	bk := a.backups
	if bk == nil {
		bk = a.deps.Backups
	}
	if bk == nil {
		return nil
	}
	if err := bk.SwitchSession(sessionID); err != nil {
		return fmt.Errorf("rebind backups: %w", err)
	}
	a.backups = bk
	a.deps.Backups = bk
	if a.deps.Tools != nil {
		if t, ok := a.deps.Tools.Get("write_file"); ok {
			if wt, ok := t.(*tools.WriteFileTool); ok {
				wt.Backups = bk
			}
		}
		if t, ok := a.deps.Tools.Get("patch_file"); ok {
			if pt, ok := t.(*tools.PatchFileTool); ok {
				pt.Backups = bk
			}
		}
	}
	return nil
}

func (a *App) cleanupSessionBackups(sessionID string) error {
	bk := a.backups
	if bk == nil {
		bk = a.deps.Backups
	}
	if bk == nil {
		return nil
	}
	return bk.CleanupSession(sessionID)
}

func (a *App) showResumedSession(s *session.Session) {
	conv := conversation.New()
	if a.deps.Version != "" {
		conv.SetVersion(a.deps.Version)
	} else {
		conv.SetVersion("v1")
	}
	conv.Resize(a.width, a.height)
	a.conversation = conv
	a.conversation.AppendSystem(fmt.Sprintf(
		"resumed session %s (%s) — %s/%s — %d messages",
		s.Name, shortID(s.ID), s.Provider, s.Model, len(s.Messages),
	))
	messages := s.Messages
	if len(messages) > resumeTranscriptLimit {
		omitted := len(messages) - resumeTranscriptLimit
		a.conversation.AppendSystem(fmt.Sprintf(
			"showing last %d messages (%d older available with /transcript)",
			resumeTranscriptLimit, omitted,
		))
		messages = messages[omitted:]
	}
	a.appendSessionTranscript(s.Provider, s.Model, messages)
}

func (a *App) appendSessionTranscript(providerSlug, modelID string, messages []provider.Message) {
	consumedToolResults := map[int]bool{}
	for i, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			a.conversation.AppendUser(msg.Content)
		case provider.RoleAssistant:
			if strings.TrimSpace(msg.Content) != "" {
				a.conversation.AppendAgentText(modelID, providerSlug, msg.Content)
				a.conversation.FinaliseAgent()
			}
			for _, call := range msg.ToolCalls {
				res, idx, ok := matchingToolResult(messages, i+1, call)
				if !ok {
					a.conversation.AppendSystem(fmt.Sprintf("tool call pending: %s %s", call.Name, call.Arguments))
					continue
				}
				consumedToolResults[idx] = true
				a.conversation.AppendToolCall(call.Name, call.Arguments)
				a.conversation.CompleteToolCall(call.Name, tools.ToolResult{Content: res.Content})
			}
		case provider.RoleTool:
			if consumedToolResults[i] {
				continue
			}
			name := msg.Name
			if name == "" {
				name = "tool"
			}
			a.conversation.AppendSystem(fmt.Sprintf("%s result: %s", name, msg.Content))
		case provider.RoleSystem:
			a.conversation.AppendSystem(msg.Content)
		}
	}
}

func matchingToolResult(messages []provider.Message, start int, call provider.ToolCall) (provider.Message, int, bool) {
	for i := start; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != provider.RoleTool {
			return provider.Message{}, 0, false
		}
		if call.ID != "" && msg.ToolCallID == call.ID {
			return msg, i, true
		}
		if call.ID == "" && msg.Name == call.Name {
			return msg, i, true
		}
	}
	return provider.Message{}, 0, false
}

// resolveSessionID accepts either a full session ID (exact match) or a
// unique 8-character prefix. Returns an error when nothing matches or
// when the prefix is ambiguous.
func (a *App) resolveSessionID(prefix string) (string, error) {
	return a.deps.Sessions.ResolveID(prefix)
}

// renderSessionsTable formats bare /sessions output. Widths: id=8,
// name=32, age=6, message/token/cost counts, prov/model=22, active=5. The top 20 sessions render;
// any overflow is dropped silently (we only expose this list to guide
// users to a specific id).
func renderSessionsTable(summaries []session.Summary, currentID string) string {
	if len(summaries) == 0 {
		return "no sessions"
	}
	if len(summaries) > 20 {
		summaries = summaries[:20]
	}
	var b strings.Builder
	b.WriteString("  ID       NAME                             AGE    MSGS  TOKENS      COST      PROV/MODEL             ACTIVE\n")
	now := time.Now()
	for _, s := range summaries {
		marker := "  "
		active := "no"
		if s.ID == currentID {
			marker = "* "
			active = "yes"
		}
		// Session names come from /sessions rename, so they are arbitrary user
		// text. Clip by runes: a byte slice would split a multi-byte character
		// and desynchronise the cell from the rune-counting padRight below.
		name := truncOneLine(s.Name, 32)
		provModel := s.Provider
		if s.Model != "" {
			if provModel != "" {
				provModel += "/" + s.Model
			} else {
				provModel = s.Model
			}
		}
		if provModel == "" {
			provModel = "(none)"
		}
		age := roundedAge(s.UpdatedAt, now)
		tokens := s.TokenUsage.TotalInput + s.TokenUsage.TotalOutput
		fmt.Fprintf(&b, "%s%s %s %s %s %s %s %s %s\n",
			marker,
			padRight(shortID(s.ID), 8),
			padRight(name, 32),
			padRight(age, 6),
			padRight(fmt.Sprintf("%d", s.MessageCount), 5),
			padRight(humanCount(tokens), 11),
			padRight(fmt.Sprintf("$%.4f", s.Cost.TotalUSD), 9),
			padRight(trunc(provModel, 22), 22),
			padRight(active, 5),
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dK", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// shortID returns the first 8 characters of a session UUID, suitable
// for display in tables.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// roundedAge renders the age of a session as "45s" / "15m" / "2h" / "1d".
func roundedAge(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		s := int(d.Seconds())
		if s < 1 {
			s = 1
		}
		return fmt.Sprintf("%ds", s)
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// handleResumeCommand is the front door for /resume. Bare, it opens a picker;
// with an argument it behaves exactly like /sessions resume, so muscle memory
// from either form works.
func (a *App) handleResumeCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		fullID, err := a.resolveSessionID(strings.TrimSpace(args[0]))
		if err != nil {
			a.conversation.AppendSystem("resume: " + err.Error())
			return a, nil
		}
		return a.resumeSessionByID(fullID, "resume")
	}
	return a, a.openSessionPicker()
}

// openSessionPicker lists saved sessions newest-first for selection.
//
// The list is loaded eagerly rather than through the picker's async Loader:
// sessions come off local disk, so a spinner would only add a frame of
// latency, and an empty list is better reported as a message than as an empty
// overlay the user has to dismiss.
// reportSessionProblems names session files that could not be read.
//
// Bounded, because a corrupt sessions directory must not flood the pane the
// listing itself is trying to render; the count is still reported in full so
// the user knows the list is short by more than what is named.
func (a *App) reportSessionProblems(label string, problems []string) {
	if len(problems) == 0 {
		return
	}
	const show = 3
	shown := problems
	if len(shown) > show {
		shown = shown[:show]
	}
	msg := fmt.Sprintf("%s: %s could not be read and %s not listed:",
		label,
		plural(len(problems), "session file", "session files"),
		map[bool]string{true: "is", false: "are"}[len(problems) == 1])
	for _, p := range shown {
		msg += "\n  " + p
	}
	if len(problems) > show {
		msg += fmt.Sprintf("\n  ... and %d more", len(problems)-show)
	}
	a.conversation.AppendSystem(msg)
}

func (a *App) openSessionPicker() tea.Cmd {
	a.autocomplete.Close()
	summaries, problems, err := a.deps.Sessions.ListWithProblems()
	if err != nil {
		a.conversation.AppendSystem("resume: list failed: " + err.Error())
		return nil
	}
	a.reportSessionProblems("resume", problems)
	if len(summaries) == 0 {
		a.conversation.AppendSystem("resume: no saved sessions yet")
		return nil
	}
	currentID := ""
	if cur := a.deps.Sessions.Current(); cur != nil {
		currentID = cur.ID
	}
	a.picker = picker.New("session", "Resume session")
	a.picker.Resize(a.width, a.height)
	a.picker.SetItems(sessionItems(summaries, currentID, time.Now()))
	a.picker.SetActive(currentID)
	return a.picker.Open(nil)
}

// sessionItems projects summaries for the picker. The detail line answers the
// question someone scanning this list is actually asking — when did I last
// touch it, and how big is it — rather than repeating the id already shown.
func sessionItems(summaries []session.Summary, currentID string, now time.Time) []picker.Item {
	items := make([]picker.Item, 0, len(summaries))
	for _, s := range summaries {
		label := strings.TrimSpace(s.Name)
		if label == "" {
			// An unnamed session is identified by the same short id the
			// sessions table shows, so the two surfaces agree.
			label = shortID(s.ID)
		}
		detail := fmt.Sprintf("%s ago · %d msg", roundedAge(s.UpdatedAt, now), s.MessageCount)
		if s.Provider != "" && s.Model != "" {
			detail += " · " + s.Provider + "/" + s.Model
		}
		item := picker.Item{ID: s.ID, Label: label, Detail: detail}
		if s.ID == currentID {
			// Marked rather than hidden: seeing "this is the one you are in"
			// is more useful than silently omitting it and leaving the user
			// wondering where it went.
			item.Marker = "●"
		}
		items = append(items, item)
	}
	return items
}

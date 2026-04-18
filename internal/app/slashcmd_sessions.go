package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/session"
)

// handleSessionsCommand lists, resumes, or deletes sessions. The bare
// form shows the top 20 newest-first; resume/delete accept either a
// full ID or any unique 8-char prefix; delete is gated on --yes because
// it is irreversible.
func (a *App) handleSessionsCommand(args []string) (tea.Model, tea.Cmd) {
	sub, id, yes, err := parseSessionsArgs(args)
	if err != nil {
		a.conversation.AppendSystem("sessions: " + err.Error())
		return a, nil
	}

	if sub == "" {
		summaries, listErr := a.deps.Sessions.List()
		if listErr != nil {
			a.conversation.AppendSystem("sessions: list failed: " + listErr.Error())
			return a, nil
		}
		currentID := ""
		if cur := a.deps.Sessions.Current(); cur != nil {
			currentID = cur.ID
		}
		a.conversation.AppendSystem(renderSessionsTable(summaries, currentID))
		return a, nil
	}

	fullID, resolveErr := a.resolveSessionID(id)
	if resolveErr != nil {
		a.conversation.AppendSystem("sessions: " + resolveErr.Error())
		return a, nil
	}

	switch sub {
	case "resume":
		s, loadErr := a.deps.Sessions.Load(fullID)
		if loadErr != nil {
			a.conversation.AppendSystem("sessions: " + loadErr.Error())
			return a, nil
		}
		a.refreshTopBar()
		a.conversation.AppendSystem(fmt.Sprintf(
			"resumed session %s (%s) — %d messages",
			s.Name, shortID(s.ID), len(s.Messages),
		))
		return a, nil

	case "delete":
		if !yes {
			a.conversation.AppendSystem(fmt.Sprintf(
				"sessions: refusing to delete without --yes; re-run: /sessions delete %s --yes",
				id,
			))
			return a, nil
		}
		if delErr := a.deps.Sessions.Delete(fullID); delErr != nil {
			a.conversation.AppendSystem("sessions: " + delErr.Error())
			return a, nil
		}
		a.refreshTopBar()
		a.conversation.AppendSystem("deleted session " + shortID(fullID))
		return a, nil
	}

	// Unreachable: parseSessionsArgs rejects anything else.
	a.conversation.AppendSystem("sessions: unexpected subcommand " + sub)
	return a, nil
}

// resolveSessionID accepts either a full session ID (exact match) or any
// unique 8-character prefix. Returns an error when nothing matches or
// when the prefix is ambiguous.
func (a *App) resolveSessionID(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("empty session id")
	}
	summaries, err := a.deps.Sessions.List()
	if err != nil {
		return "", fmt.Errorf("list failed: %w", err)
	}
	// Exact match first — full UUIDs are always unambiguous.
	for _, s := range summaries {
		if s.ID == prefix {
			return s.ID, nil
		}
	}
	// Prefix match. Accept any prefix length, not just 8.
	var matches []string
	for _, s := range summaries {
		if strings.HasPrefix(s.ID, prefix) {
			matches = append(matches, s.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matches %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q — matches %d sessions", prefix, len(matches))
	}
}

// renderSessionsTable formats bare /sessions output. Widths: id=8,
// name=40, age=6, prov/model=22, active=5. The top 20 sessions render;
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
	b.WriteString("  ID       NAME                                     AGE    PROV/MODEL             ACTIVE\n")
	now := time.Now()
	for _, s := range summaries {
		marker := "  "
		active := "no"
		if s.ID == currentID {
			marker = "* "
			active = "yes"
		}
		name := s.Name
		if len(name) > 40 {
			name = name[:37] + "..."
		}
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
		fmt.Fprintf(&b, "%s%s %s %s %s %s\n",
			marker,
			padRight(shortID(s.ID), 8),
			padRight(name, 40),
			padRight(age, 6),
			padRight(trunc(provModel, 22), 22),
			padRight(active, 5),
		)
	}
	return strings.TrimRight(b.String(), "\n")
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

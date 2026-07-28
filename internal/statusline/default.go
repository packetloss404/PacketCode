package statusline

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
)

// RenderDefault produces packetcode's built-in statusline line, rendered
// natively (no external command, no jq). It is shown by default when the user
// has not configured a [statusline].command, giving the Claude Code-style
// look — [provider·model] <ctx> pct% (used/max) | 📂 dir | 🌿 branch | 💲cost |
// ◷ op — out of the box. The format mirrors docs/statusline/statusline.sh so a
// user can swap in that script (or their own) without a jarring visual change.
//
// It is a pure function of the snapshot, so callers can re-render it every tick
// to keep the live operation timer current.
func RenderDefault(s Snapshot) string {
	segments, _ := defaultSegments(s)
	return joinDefaultSegments(segments)
}

// RenderDefaultWidth renders the native statusline into a single bounded row.
// Critical live state (foreground operation and background agents) is kept
// ahead of project/branch/cost details, and the provider label becomes more
// compact before anything is allowed to wrap. This keeps the prompt anchored
// on narrow terminals instead of making the footer jump between one and two
// rows as work starts.
func RenderDefaultWidth(s Snapshot, width int) string {
	segments, identityVariants := defaultSegments(s)
	full := joinDefaultSegments(segments)
	if width <= 0 || runewidth.StringWidth(full) <= width {
		return full
	}

	selected := map[int]bool{0: true}
	critical := make([]int, 0, 2)
	for i := 1; i < len(segments); i++ {
		if segments[i].priority >= 80 {
			selected[i] = true
			critical = append(critical, i)
		}
	}

	// Choose the most descriptive identity that still leaves room for every
	// critical live-state segment. If even the compact identity cannot fit,
	// the final ANSI-safe truncation below remains the hard guarantee.
	for _, identity := range identityVariants {
		segments[0].text = identity
		if defaultSelectionWidth(segments, selected) <= width {
			break
		}
	}

	remaining := make([]int, 0, len(segments))
	for i := 1; i < len(segments); i++ {
		if !selected[i] {
			remaining = append(remaining, i)
		}
	}
	sort.SliceStable(remaining, func(i, j int) bool {
		return segments[remaining[i]].priority > segments[remaining[j]].priority
	})
	for _, idx := range remaining {
		selected[idx] = true
		if defaultSelectionWidth(segments, selected) > width {
			delete(selected, idx)
		}
	}

	line := joinSelectedDefaultSegments(segments, selected)
	if runewidth.StringWidth(line) > width {
		line = runewidth.Truncate(line, width, "…")
	}
	return line
}

type defaultSegment struct {
	text     string
	priority int
}

func defaultSegments(s Snapshot) ([]defaultSegment, []string) {
	model := firstNonEmpty(s.Model.DisplayName, s.Model.ID, "no model")
	label := model
	if p := strings.TrimSpace(s.Provider.DisplayName); p != "" {
		label = p + "·" + model
	}

	pct := s.ContextWindow.UsedPercentage
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	icon := "🟢"
	switch {
	case pct >= 80:
		icon = "🔴"
	case pct >= 60:
		icon = "🟡"
	}
	tokens := fmt.Sprintf("%dK/%dK", s.ContextWindow.Used/1000, s.ContextWindow.Max/1000)
	identities := []string{
		fmt.Sprintf("[%s] %s %d%% (%s)", label, icon, pct, tokens),
		fmt.Sprintf("[%s] %s %d%% (%s)", model, icon, pct, tokens),
		fmt.Sprintf("[%s] %s %d%%", model, icon, pct),
		fmt.Sprintf("%s %d%%", model, pct),
	}
	identities = compactUniqueStrings(identities)

	dir := s.Project
	if dir == "" && s.WorkingDir != "" {
		dir = filepath.Base(s.WorkingDir)
	}
	branch := "-"
	if b := strings.TrimSpace(s.GitBranch); b != "" {
		branch = b
	}

	segs := []defaultSegment{
		{text: identities[0], priority: 1000},
		{text: "📂 " + dir, priority: 40},
		{text: "🌿 " + branch, priority: 30},
	}
	if effort := strings.TrimSpace(s.Model.ReasoningEffort); effort != "" {
		segs = append(segs, defaultSegment{text: "● " + effort + " · /effort", priority: 70})
	}

	// Session cost — hidden at ~$0 (e.g. a Codex/ChatGPT subscription bills a
	// flat rate, and local Ollama is free).
	if s.Cost.TotalCostUSD > 0.0005 {
		segs = append(segs, defaultSegment{text: fmt.Sprintf("💲%.2f", s.Cost.TotalCostUSD), priority: 20})
	}

	if s.Jobs.Active > 0 {
		noun := "agent"
		if s.Jobs.Active != 1 {
			noun = "agents"
		}
		segs = append(segs, defaultSegment{text: fmt.Sprintf("⚙ %d %s", s.Jobs.Active, noun), priority: 90})
	}

	// Live operation indicator (thinking / running a tool) with elapsed time.
	if s.Operation.Active {
		op := firstNonEmpty(s.Operation.Label, "working")
		seg := fmt.Sprintf("◷ %s %ds", op, s.Operation.ElapsedSeconds)
		if s.Operation.QueuedInputs > 0 {
			seg += fmt.Sprintf(" (+%d queued)", s.Operation.QueuedInputs)
		}
		segs = append(segs, defaultSegment{text: seg, priority: 100})
	}

	return segs, identities
}

func joinDefaultSegments(segments []defaultSegment) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.text != "" {
			parts = append(parts, segment.text)
		}
	}
	return strings.Join(parts, " | ")
}

func joinSelectedDefaultSegments(segments []defaultSegment, selected map[int]bool) string {
	parts := make([]string, 0, len(selected))
	for i, segment := range segments {
		if selected[i] && segment.text != "" {
			parts = append(parts, segment.text)
		}
	}
	return strings.Join(parts, " | ")
}

func defaultSelectionWidth(segments []defaultSegment, selected map[int]bool) int {
	return runewidth.StringWidth(joinSelectedDefaultSegments(segments, selected))
}

func compactUniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

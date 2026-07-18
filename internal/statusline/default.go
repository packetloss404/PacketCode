package statusline

import (
	"fmt"
	"path/filepath"
	"strings"
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

	dir := s.Project
	if dir == "" && s.WorkingDir != "" {
		dir = filepath.Base(s.WorkingDir)
	}
	branch := "-"
	if b := strings.TrimSpace(s.GitBranch); b != "" {
		branch = b
	}

	segs := []string{
		fmt.Sprintf("[%s] %s %d%% (%s)", label, icon, pct, tokens),
		"📂 " + dir,
		"🌿 " + branch,
	}

	// Session cost — hidden at ~$0 (e.g. a Codex/ChatGPT subscription bills a
	// flat rate, and local Ollama is free).
	if s.Cost.TotalCostUSD > 0.0005 {
		segs = append(segs, fmt.Sprintf("💲%.2f", s.Cost.TotalCostUSD))
	}

	// Live operation indicator (thinking / running a tool) with elapsed time.
	if s.Operation.Active {
		op := firstNonEmpty(s.Operation.Label, "working")
		seg := fmt.Sprintf("◷ %s %ds", op, s.Operation.ElapsedSeconds)
		if s.Operation.QueuedInputs > 0 {
			seg += fmt.Sprintf(" (+%d queued)", s.Operation.QueuedInputs)
		}
		segs = append(segs, seg)
	}

	return strings.Join(segs, " | ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

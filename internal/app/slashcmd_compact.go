package app

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleCompactCommand summarises the middle of the conversation via a
// single LLM round trip. The UI blocks for the duration (capped at
// 120s). Two system messages bookend the call so the user sees progress
// even when the summary takes a few seconds.
func (a *App) handleCompactCommand(args []string) (tea.Model, tea.Cmd) {
	keep, err := parseCompactFlags(args)
	if err != nil {
		a.conversation.AppendSystem("compact: " + err.Error())
		return a, nil
	}
	prov, modelID := a.deps.Registry.Active()
	if prov == nil {
		a.conversation.AppendSystem("compact: no active provider")
		return a, nil
	}
	cur := a.deps.Sessions.Current()
	if cur == nil {
		a.conversation.AppendSystem("compact: no session loaded")
		return a, nil
	}

	before := cur.Messages
	beforeTok := a.contextMgr.EstimateTokens(before)
	a.conversation.AppendSystem(fmt.Sprintf("compacting context... (~%d tokens)", beforeTok))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	after, err := a.contextMgr.Compact(ctx, prov, modelID, before, keep)
	if err != nil {
		a.conversation.AppendSystem("compact: " + err.Error())
		return a, nil
	}

	cur.Messages = after
	if saveErr := a.deps.Sessions.Save(); saveErr != nil {
		a.conversation.AppendSystem("compact: save failed: " + saveErr.Error())
		return a, nil
	}

	afterTok := a.contextMgr.EstimateTokens(after)
	a.conversation.AppendSystem(fmt.Sprintf(
		"compacted: %d → %d tokens (kept %d recent messages)",
		beforeTok, afterTok, keep,
	))
	a.refreshTopBar()
	return a, nil
}

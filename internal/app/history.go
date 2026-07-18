package app

import "strings"

// Prompt history: Up/Down in the input bar page through previously submitted
// inputs, shell-style. Recall is a view over promptHistory; historyIdx tracks
// position and historyDraft preserves the live buffer while paging so Down can
// bring it back. Recall only fires when the caret is at the top (Up) or bottom
// (Down) of the buffer, so multi-line editing still moves the caret normally.

// recordHistory appends a submitted input to the recall list and resets the
// navigation cursor to the "live draft" position. Consecutive duplicates are
// collapsed so holding Up doesn't step through repeats. Called for every
// submission (prompts and slash commands alike, matching Claude Code).
func (a *App) recordHistory(text string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) != "" {
		if n := len(a.promptHistory); n == 0 || a.promptHistory[n-1] != text {
			a.promptHistory = append(a.promptHistory, text)
		}
	}
	a.historyIdx = len(a.promptHistory)
	a.historyDraft = ""
}

// historyPrev recalls the previous (older) entry into the input. Returns true
// when it handled the key (so the caller consumes it instead of letting the
// textarea move the caret up). Returns false only when there is no history at
// all, letting Up behave normally in an empty session.
func (a *App) historyPrev() bool {
	if len(a.promptHistory) == 0 {
		return false
	}
	// Entering navigation from the live buffer: stash the draft so Down can
	// restore it.
	if a.historyIdx >= len(a.promptHistory) {
		a.historyDraft = a.input.Value()
		a.historyIdx = len(a.promptHistory)
	}
	if a.historyIdx <= 0 {
		// Already at the oldest entry — consume the key but don't wrap.
		return true
	}
	a.historyIdx--
	a.input.SetValue(a.promptHistory[a.historyIdx])
	return true
}

// historyNext recalls the next (newer) entry, restoring the stashed live draft
// once it pages past the newest entry. Returns false when not currently
// navigating history, so Down behaves normally in a fresh buffer.
func (a *App) historyNext() bool {
	if a.historyIdx >= len(a.promptHistory) {
		return false
	}
	a.historyIdx++
	if a.historyIdx >= len(a.promptHistory) {
		a.input.SetValue(a.historyDraft)
		a.historyDraft = ""
		return true
	}
	a.input.SetValue(a.promptHistory[a.historyIdx])
	return true
}

// Package layout composes the conversation pane, input bar, status bar,
// and any modal overlay into the final TUI frame.
//
// The status bar lives at the BOTTOM of the screen (not the top) so the
// user's eye anchors at the input + status while reading the conversation
// flowing upward. Order top-to-bottom:
//
//	conversation
//	[overlay]      ← approval prompt, picker, jobs modal, or spinner
//	[aboveInput]   ← anchored-to-input helpers (slash-command autocomplete)
//	input
//	status
//
// This file is intentionally a pure stacker: it accepts already-rendered
// strings and joins them. Sizing decisions belong to the App so the
// conversation viewport can be told its exact dimensions before View().
package layout

import "strings"

// Frame stacks the regions in the order described above. None of the
// strings are trimmed or padded — the App is responsible for sizing each
// region to fit the terminal.
//
// aboveInput is a dedicated slot for widgets that want to feel attached
// to the input bar (the slash-command autocomplete popup). It sits below
// the overlay slot so a visible modal always covers it, and above input
// so it reads as "part of" the input chrome.
func Frame(body, overlay, aboveInput, input, status string) string {
	parts := []string{body}
	if overlay != "" {
		parts = append(parts, overlay)
	}
	if aboveInput != "" {
		parts = append(parts, aboveInput)
	}
	if input != "" {
		parts = append(parts, input)
	}
	if status != "" {
		parts = append(parts, status)
	}
	return strings.Join(parts, "\n")
}

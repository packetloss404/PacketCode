// Package layout composes the conversation pane, input bar, status bar,
// and any modal overlay into the final TUI frame.
//
// The status bar lives at the BOTTOM of the screen (not the top) so the
// user's eye anchors at the input + status while reading the conversation
// flowing upward. Order top-to-bottom:
//
//   conversation
//   [overlay]      ← approval prompt or spinner, only when active
//   input
//   status
//
// This file is intentionally a pure stacker: it accepts already-rendered
// strings and joins them. Sizing decisions belong to the App so the
// conversation viewport can be told its exact dimensions before View().
package layout

import "strings"

// Frame stacks the regions in the order described above. None of the
// strings are trimmed or padded — the App is responsible for sizing each
// region to fit the terminal.
func Frame(body, overlay, input, status string) string {
	parts := []string{body}
	if overlay != "" {
		parts = append(parts, overlay)
	}
	if input != "" {
		parts = append(parts, input)
	}
	if status != "" {
		parts = append(parts, status)
	}
	return strings.Join(parts, "\n")
}

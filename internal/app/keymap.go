package app

// Keymap descriptions exposed via /help. The actual key handling lives in
// the App.handleKeypress switch — these strings are purely documentation.
var (
	GlobalKeys = []KeyHelp{
		{"Ctrl+C", "Cancel current generation; press twice to exit"},
		{"Ctrl+L", "Clear screen (keep session)"},
	}
	ConversationKeys = []KeyHelp{
		{"↑/k", "Scroll up"},
		{"↓/j", "Scroll down"},
		{"g", "Jump to top"},
		{"G", "Jump to bottom"},
		{"Tab", "Toggle collapse on most recent tool output"},
	}
	ApprovalKeys = []KeyHelp{
		{"Y", "Approve"},
		{"N / Esc", "Reject"},
	}
	InputKeys = []KeyHelp{
		{"Enter", "Send message"},
		{"Shift+Enter", "Insert newline"},
	}
)

type KeyHelp struct {
	Key  string
	Desc string
}

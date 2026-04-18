package app

// Keymap descriptions exposed via /help. The actual key handling lives in
// the App.handleKeypress switch — these strings are purely documentation.
var (
	GlobalKeys = []KeyHelp{
		{"Ctrl+C", "Cancel current generation; press twice to exit"},
		{"Ctrl+L", "Clear screen (keep session)"},
		{"Esc", "Close jobs modal / approval / etc."},
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
	// SlashCommands enumerates every slash command the user can type
	// into the input bar. Displayed by /help; the actual parsing lives
	// in internal/app/slashcmd.go.
	SlashCommands = []KeyHelp{
		{"/spawn <prompt>", "Spawn a background agent"},
		{"/jobs", "List background jobs"},
		{"/jobs <id>", "View a job's transcript"},
		{"/cancel <id|all>", "Cancel a job"},
	}
)

type KeyHelp struct {
	Key  string
	Desc string
}

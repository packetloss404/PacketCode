package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleProviderCommand opens the provider picker modal (0 args) or
// switches to a specific provider (1 arg). Bare /provider shares the
// openProviderPicker flow with Ctrl+P so users get a searchable modal
// including per-row API-key setup via ctrl+a.
func (a *App) handleProviderCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return a, a.openProviderPicker()
	}
	if args[0] == "add" {
		switch len(args) {
		case 1:
			a.conversation.AppendSystem("provider add: choose a provider, then press Ctrl+A to set or update its key")
			return a, a.openProviderPicker()
		case 2:
			return a, a.openProviderKeyPrompt(args[1])
		default:
			a.conversation.AppendSystem("provider add: usage: /provider add [slug]")
			return a, nil
		}
	}
	if _, ok := a.deps.Registry.Get(args[0]); !ok && a.deps.Factories != nil {
		if _, known := a.deps.Factories[args[0]]; known && !a.providerHasKey(args[0]) {
			return a, a.openProviderKeyPrompt(args[0])
		}
	}
	if err := a.applyProviderSwitch(args[0]); err != nil {
		a.conversation.AppendSystem("provider: " + err.Error())
	}
	return a, nil
}

// handleModelCommand opens the model picker modal (0 args) or switches
// to a specific model (1 arg). The picker open path is shared with the
// Alt+M keybind via openModelPicker — same async loader, cache reuse,
// and error surface.
func (a *App) handleModelCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return a, a.openModelPicker()
	}
	if err := a.applyModelSwitch(args[0]); err != nil {
		a.conversation.AppendSystem("model: " + err.Error())
	}
	return a, nil
}

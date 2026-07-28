package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider"
)

func (a *App) handleEffortCommand(args []string) (tea.Model, tea.Cmd) {
	prov, modelID := a.deps.Registry.Active()
	if prov == nil {
		a.conversation.AppendSystem("effort: no active provider")
		return a, nil
	}
	controller, ok := prov.(provider.ReasoningEffortController)
	if !ok {
		a.conversation.AppendSystem(fmt.Sprintf("effort: %s does not expose reasoning controls", prov.Name()))
		return a, nil
	}
	options := controller.ReasoningEfforts(modelID)
	if len(args) == 0 {
		ids := make([]string, 0, len(options))
		for _, option := range options {
			ids = append(ids, option.ID)
		}
		a.conversation.AppendSystem(fmt.Sprintf("effort: %s · available: %s · reset with /effort default",
			controller.ReasoningEffort(modelID), strings.Join(ids, ", ")))
		return a, nil
	}
	if len(args) != 1 {
		a.conversation.AppendSystem("effort: usage: /effort [default|low|medium|high|xhigh|max|ultra]")
		return a, nil
	}
	if a.streaming {
		a.conversation.AppendSystem("effort: wait for the active turn to finish or press Ctrl+C")
		return a, nil
	}

	effort := strings.ToLower(strings.TrimSpace(args[0]))
	if err := controller.SetReasoningEffort(modelID, effort); err != nil {
		a.conversation.AppendSystem("effort: " + err.Error())
		return a, nil
	}
	if effort == "default" || effort == "auto" {
		effort = ""
	}
	if a.deps.Config != nil {
		if a.deps.Config.Providers == nil {
			a.deps.Config.Providers = map[string]config.ProviderConfig{}
		}
		pc := a.deps.Config.Providers[prov.Slug()]
		pc.ReasoningEffort = effort
		a.deps.Config.Providers[prov.Slug()] = pc
		if err := a.deps.Config.Save(); err != nil {
			a.conversation.AppendSystem("effort: active for this session; could not persist: " + err.Error())
		}
	}
	a.refreshTopBar()
	a.conversation.AppendSystem("reasoning effort: " + controller.ReasoningEffort(modelID))
	return a, nil
}

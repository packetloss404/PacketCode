package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/provider"
)

// handleProviderCommand lists providers (0 args) or switches the active
// provider (1 arg). When switching without a model already known, the
// handler falls back to the config default model then the first model
// reported by ListModels.
func (a *App) handleProviderCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		a.conversation.AppendSystem(a.renderProvidersTable())
		return a, nil
	}
	slug := args[0]
	prov, ok := a.deps.Registry.Get(slug)
	if !ok {
		a.conversation.AppendSystem(fmt.Sprintf("provider: unknown provider %q", slug))
		return a, nil
	}

	// Pick a model: config default first, then ListModels[0], else
	// surface an actionable error.
	modelID := ""
	if a.deps.Config != nil {
		if pc, ok := a.deps.Config.Providers[slug]; ok {
			modelID = pc.DefaultModel
		}
	}
	if modelID == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		models, err := prov.ListModels(ctx)
		cancel()
		if err == nil && len(models) > 0 {
			modelID = models[0].ID
		}
	}
	if modelID == "" {
		a.conversation.AppendSystem(fmt.Sprintf("provider: %s has no default model; run /model <id> after switching", slug))
		return a, nil
	}

	if err := a.deps.Registry.SetActive(slug, modelID); err != nil {
		a.conversation.AppendSystem("provider: " + err.Error())
		return a, nil
	}
	a.refreshTopBar()
	a.conversation.AppendSystem(fmt.Sprintf("switched provider: %s (%s)", slug, modelID))
	return a, nil
}

// handleModelCommand lists models available on the active provider (0
// args) or switches to a specific model (1 arg). Model IDs are not
// validated against ListModels — matches Registry.SetActive's contract.
func (a *App) handleModelCommand(args []string) (tea.Model, tea.Cmd) {
	prov, activeModel := a.deps.Registry.Active()
	if prov == nil {
		a.conversation.AppendSystem("model: no active provider")
		return a, nil
	}

	if len(args) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		models, err := prov.ListModels(ctx)
		cancel()
		if err != nil {
			a.conversation.AppendSystem("model: list failed: " + err.Error())
			return a, nil
		}
		a.conversation.AppendSystem(renderModelsTable(models, activeModel))
		return a, nil
	}

	modelID := args[0]
	if err := a.deps.Registry.SetActive(prov.Slug(), modelID); err != nil {
		a.conversation.AppendSystem("model: " + err.Error())
		return a, nil
	}
	a.refreshTopBar()
	a.conversation.AppendSystem(fmt.Sprintf("switched model: %s/%s", prov.Slug(), modelID))
	return a, nil
}

// renderProvidersTable builds the ASCII table shown by bare /provider.
// Fixed column widths: slug=10, name=14, default model=28, active=5.
func (a *App) renderProvidersTable() string {
	provs := a.deps.Registry.List()
	if len(provs) == 0 {
		return "no providers registered"
	}
	active, _ := a.deps.Registry.Active()
	activeSlug := ""
	if active != nil {
		activeSlug = active.Slug()
	}
	var b strings.Builder
	// Leading two spaces in the header accounts for the active marker
	// column ("* " or "  ") that prefixes each row.
	b.WriteString("  PROVIDER   NAME           DEFAULT MODEL                ACTIVE\n")
	for _, p := range provs {
		marker := "  "
		activeCol := "no"
		if p.Slug() == activeSlug {
			marker = "* "
			activeCol = "yes"
		}
		defModel := "(none)"
		if a.deps.Config != nil {
			if pc, ok := a.deps.Config.Providers[p.Slug()]; ok && pc.DefaultModel != "" {
				defModel = pc.DefaultModel
			}
		}
		fmt.Fprintf(&b, "%s%s %s %s %s\n",
			marker,
			padRight(trunc(p.Slug(), 10), 10),
			padRight(trunc(p.Name(), 14), 14),
			padRight(trunc(defModel, 28), 28),
			padRight(activeCol, 5),
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderModelsTable formats the table shown by bare /model.
func renderModelsTable(models []provider.Model, activeID string) string {
	if len(models) == 0 {
		return "no models reported"
	}
	var b strings.Builder
	b.WriteString("  MODEL                        CONTEXT    TOOLS  IN/1M    OUT/1M\n")
	for _, m := range models {
		marker := "  "
		if m.ID == activeID {
			marker = "* "
		}
		ctx := "?"
		if m.ContextWindow > 0 {
			ctx = fmt.Sprintf("%d", m.ContextWindow)
		}
		tools := "no"
		if m.SupportsTools {
			tools = "yes"
		}
		in := "free"
		if m.InputPer1M > 0 {
			in = fmt.Sprintf("$%.2f", m.InputPer1M)
		}
		out := "free"
		if m.OutputPer1M > 0 {
			out = fmt.Sprintf("$%.2f", m.OutputPer1M)
		}
		fmt.Fprintf(&b, "%s%s %s %s %s %s\n",
			marker,
			padRight(trunc(m.ID, 28), 28),
			padRight(ctx, 10),
			padRight(tools, 6),
			padRight(in, 8),
			padRight(out, 8),
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

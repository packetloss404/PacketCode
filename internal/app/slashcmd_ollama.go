package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/ollama"
)

// ollamaInfoMsg carries the result of an async /ollama subcommand back to the
// Update loop, which appends it to the conversation.
type ollamaInfoMsg struct {
	text string
	err  error
}

// ollamaProvider returns the registered Ollama provider, if any. It's a
// concrete-type assertion because the /ollama command uses Ollama-specific
// capabilities (model management, hardware fit) that aren't on the generic
// provider.Provider interface.
func (a *App) ollamaProvider() (*ollama.Provider, bool) {
	prov, ok := a.deps.Registry.Get("ollama")
	if !ok {
		return nil, false
	}
	op, ok := prov.(*ollama.Provider)
	return op, ok
}

func (a *App) handleOllamaCommand(args []string) (tea.Model, tea.Cmd) {
	op, ok := a.ollamaProvider()
	if !ok {
		a.conversation.AppendSystem("ollama: provider not available")
		return a, nil
	}
	sub := "status"
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}
	switch sub {
	case "status", "":
		return a, ollamaStatusCmd(op)
	case "models":
		return a, ollamaModelsCmd(op)
	case "ps":
		return a, ollamaPSCmd(op)
	case "pull":
		if len(args) < 2 {
			a.conversation.AppendSystem("usage: /ollama pull <model>")
			return a, nil
		}
		a.conversation.AppendSystem("ollama: pulling " + args[1] + " … (this can take a while)")
		return a, ollamaPullCmd(op, args[1])
	default:
		a.conversation.AppendSystem("usage: /ollama [status|models|ps|pull <model>]")
		return a, nil
	}
}

func ollamaStatusCmd(op *ollama.Provider) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := op.ValidateKey(ctx, ""); err != nil {
			return ollamaInfoMsg{err: err}
		}
		models, _ := op.ListModels(ctx)
		loaded, _ := op.LoadedModels(ctx)
		ram, _ := ollama.SystemMemoryBytes()
		return ollamaInfoMsg{text: formatOllamaStatus(models, loaded, ram, ollama.Recommend(ram), op.LastGenStats())}
	}
}

func ollamaModelsCmd(op *ollama.Provider) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		models, err := op.ListModels(ctx)
		if err != nil {
			return ollamaInfoMsg{err: err}
		}
		return ollamaInfoMsg{text: formatOllamaModels(models)}
	}
}

func ollamaPSCmd(op *ollama.Provider) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		loaded, err := op.LoadedModels(ctx)
		if err != nil {
			return ollamaInfoMsg{err: err}
		}
		return ollamaInfoMsg{text: formatOllamaPS(loaded, op.LastGenStats())}
	}
}

func ollamaPullCmd(op *ollama.Provider, model string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		ch, err := op.PullModel(ctx, model)
		if err != nil {
			return ollamaInfoMsg{err: err}
		}
		var pullErr string
		for f := range ch {
			if f.Err != "" {
				pullErr = f.Err
			}
		}
		if pullErr != "" {
			return ollamaInfoMsg{err: fmt.Errorf("pull %s: %s", model, pullErr)}
		}
		return ollamaInfoMsg{text: "ollama: pulled " + model}
	}
}

// warmupOllama preloads modelID in the background (best-effort) when the active
// provider is Ollama, so the first turn after a switch isn't a cold reload.
func (a *App) warmupOllama(modelID string) {
	if prov, _ := a.deps.Registry.Active(); prov == nil || prov.Slug() != "ollama" {
		return
	}
	op, ok := a.ollamaProvider()
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = op.Warmup(ctx, modelID)
	}()
}

// ---- formatters (pure, unit-tested) ------------------------------------

func formatOllamaStatus(models []provider.Model, loaded []ollama.LoadedModel, ramBytes int64, recs []ollama.ModelRec, stats ollama.GenStats) string {
	var b strings.Builder
	b.WriteString("Ollama — local, ready")
	if ramBytes > 0 {
		fmt.Fprintf(&b, " · %s unified memory", humanBytes(ramBytes))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Installed: %d model(s)", len(models))
	if len(loaded) > 0 {
		fmt.Fprintf(&b, " · loaded: %s", loadedSummary(loaded))
	}
	b.WriteString("\n")

	if tps := stats.OutputTokensPerSec(); tps > 0 {
		fmt.Fprintf(&b, "Last generation: %.0f tok/s, first token in %dms\n", tps, stats.TimeToFirstToken().Milliseconds())
	}

	if len(recs) > 0 {
		b.WriteString("Recommended coding models for this machine:\n")
		for i, r := range recs {
			if i >= 3 {
				break
			}
			mark := "✓"
			if !r.Fits {
				mark = "⚠"
			}
			fmt.Fprintf(&b, "  %s %s (%s) — %s\n", mark, r.Name, r.Label, r.Reason)
		}
		b.WriteString("Pull one with /ollama pull <model>.")
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatOllamaModels(models []provider.Model) string {
	if len(models) == 0 {
		return "ollama: no models installed — pull one with /ollama pull <model>"
	}
	var b strings.Builder
	b.WriteString("Installed Ollama models:\n")
	for _, m := range models {
		tools := "no tools"
		if m.SupportsTools {
			tools = "tools"
		}
		ctx := "ctx ?"
		if m.ContextWindow > 0 {
			ctx = fmt.Sprintf("ctx %dK", m.ContextWindow/1000)
		}
		fmt.Fprintf(&b, "  %s — %s, %s\n", m.ID, ctx, tools)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatOllamaPS(loaded []ollama.LoadedModel, stats ollama.GenStats) string {
	if len(loaded) == 0 {
		return "ollama: no models currently loaded"
	}
	var b strings.Builder
	b.WriteString("Loaded models:\n")
	for _, m := range loaded {
		fmt.Fprintf(&b, "  %s — %s, %s\n", m.Name, humanBytes(m.Size), gpuPlacement(m))
	}
	if tps := stats.OutputTokensPerSec(); tps > 0 {
		fmt.Fprintf(&b, "Last generation: %.0f tok/s (prefill %.0f tok/s), first token in %dms",
			tps, stats.PromptTokensPerSec(), stats.TimeToFirstToken().Milliseconds())
	}
	return strings.TrimRight(b.String(), "\n")
}

func gpuPlacement(m ollama.LoadedModel) string {
	switch {
	case m.FullyOnGPU():
		return "GPU"
	case m.OnGPU():
		return "partial GPU (some on CPU — slower)"
	default:
		return "CPU only (slow)"
	}
}

func loadedSummary(loaded []ollama.LoadedModel) string {
	names := make([]string, 0, len(loaded))
	for _, m := range loaded {
		names = append(names, m.Name)
	}
	return strings.Join(names, ", ")
}

func humanBytes(n int64) string {
	const gb = int64(1) << 30
	const mb = int64(1) << 20
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(mb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

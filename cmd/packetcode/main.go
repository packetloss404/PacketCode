// Command packetcode is a keyboard-first, multi-provider AI coding agent
// for the terminal.
//
// Usage:
//
//   packetcode                              start the TUI in the cwd
//   packetcode --version                    print version and exit
//   packetcode --provider gemini --model gemini-2.5-pro
//   packetcode --resume <session-id>        resume a saved session
//   packetcode --trust                      auto-approve all tool actions
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/app"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/git"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/gemini"
	"github.com/packetcode/packetcode/internal/provider/minimax"
	"github.com/packetcode/packetcode/internal/provider/ollama"
	"github.com/packetcode/packetcode/internal/provider/openai"
	"github.com/packetcode/packetcode/internal/provider/openrouter"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

// version/commit are populated at build time via -ldflags. Defaults are
// used during `go run` and local development.
var (
	version = "dev"
	commit  = "none"
)

const systemPrompt = `You are packetcode, a keyboard-first AI coding agent running in the user's terminal. You have direct access to the user's project via tools (read_file, write_file, patch_file, execute_command, search_codebase, list_directory). All file modifications and command executions require user approval before running.

Be concise. Prefer small, surgical edits. When the user asks you to do something, propose a plan, gather context with read tools as needed, then make the changes one tool call at a time. After tool execution, briefly summarize what changed.`

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	providerFlag := flag.String("provider", "", "override default provider for this session")
	modelFlag := flag.String("model", "", "override default model for this session")
	resumeFlag := flag.String("resume", "", "resume a saved session by ID")
	trustFlag := flag.Bool("trust", false, "auto-approve all tool actions for this session")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("packetcode %s (%s)\n", version, commit)
		return
	}

	if err := run(*providerFlag, *modelFlag, *resumeFlag, *trustFlag); err != nil {
		fmt.Fprintf(os.Stderr, "packetcode: %s\n", err)
		os.Exit(1)
	}
}

func run(providerOverride, modelOverride, resumeID string, trust bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	factories := app.FactoryMap{
		"openai":     func(key string) provider.Provider { return openai.New(key) },
		"gemini":     func(key string) provider.Provider { return gemini.New(key) },
		"minimax":    func(key string) provider.Provider { return minimax.New(key) },
		"openrouter": func(key string) provider.Provider { return openrouter.New(key) },
		"ollama":     func(_ string) provider.Provider { return ollama.New(ollamaHost(cfg)) },
	}

	// First-run: no provider configured yet → walk through setup.
	if cfg.Default.Provider == "" || cfg.Providers[cfg.Default.Provider].APIKey == "" && cfg.Default.Provider != "ollama" {
		_, err := app.RunSetup(os.Stdin, os.Stdout, cfg, factories)
		if err != nil {
			return err
		}
		// Reload the now-saved config so in-memory state matches disk.
		cfg, err = config.Load()
		if err != nil {
			return err
		}
	}

	// Apply CLI overrides over config defaults.
	activeSlug := cfg.Default.Provider
	activeModel := cfg.Default.Model
	if providerOverride != "" {
		activeSlug = providerOverride
	}
	if modelOverride != "" {
		activeModel = modelOverride
	}

	if trust {
		cfg.Behavior.TrustMode = true
	}

	// Build the provider registry. Only register providers the user has
	// actually configured — listing every provider would clutter the
	// switcher with non-functional options.
	reg := provider.NewRegistry()
	for slug, factory := range factories {
		key := cfg.GetProviderKey(slug)
		if slug != "ollama" && key == "" {
			continue
		}
		reg.Register(factory(key))
	}
	if _, ok := reg.Get(activeSlug); !ok {
		return fmt.Errorf("active provider %q is not configured; run packetcode without --provider to set one up", activeSlug)
	}
	if activeModel == "" {
		// Fall back to the provider's configured default model.
		activeModel = cfg.Providers[activeSlug].DefaultModel
	}
	if err := reg.SetActive(activeSlug, activeModel); err != nil {
		return err
	}

	// Resolve the working directory to the git repo root if we're in one.
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := git.RepoRoot(cwd)

	// Tool registry. write_file and patch_file get a backup manager
	// scoped to the active session — wired below once we know the ID.
	toolReg := tools.NewRegistry()
	toolReg.Register(tools.NewReadFileTool(root))
	toolReg.Register(tools.NewSearchCodebaseTool(root))
	toolReg.Register(tools.NewListDirectoryTool(root))
	toolReg.Register(tools.NewExecuteCommandTool(root))

	// Sessions.
	sessionsDir, err := config.SessionsDir()
	if err != nil {
		return err
	}
	sessions := session.NewManager(sessionsDir)
	if resumeID != "" {
		if _, err := sessions.Load(resumeID); err != nil {
			return fmt.Errorf("resume %s: %w", resumeID, err)
		}
	} else {
		if _, err := sessions.New(activeSlug, activeModel); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
	}

	// Backup manager keyed by session ID.
	backupsDir, err := config.BackupsDir()
	if err != nil {
		return err
	}
	bk := session.NewBackupManager(backupsDir, sessions.Current().ID)
	toolReg.Register(tools.NewWriteFileTool(root, bk))
	toolReg.Register(tools.NewPatchFileTool(root, bk))

	// Cost tracker — pricing closure delegates to whichever provider is
	// active *now* (post hot-switch), not the one when a token was
	// recorded.
	tallyPath, err := config.CostTallyPath()
	if err != nil {
		return err
	}
	tracker, err := cost.NewTracker(tallyPath, func(slug, modelID string) (float64, float64) {
		if p, ok := reg.Get(slug); ok {
			return p.Pricing(modelID)
		}
		return 0, 0
	})
	if err != nil {
		return err
	}

	a, err := app.New(app.Deps{
		Config:       cfg,
		Registry:     reg,
		Tools:        toolReg,
		Sessions:     sessions,
		CostTracker:  tracker,
		WorkingDir:   root,
		SystemPrompt: systemPrompt,
		Version:      welcomeVersion(),
	})
	if err != nil {
		return err
	}

	prog := tea.NewProgram(a, tea.WithAltScreen()) // explicitly NO mouse support
	if _, err := prog.Run(); err != nil {
		return err
	}
	return nil
}

// welcomeVersion returns the label shown on the welcome splash. We
// prefer the linker-injected version; "dev" builds get a friendlier "v1"
// so the screen looks like a release rather than a debug artefact.
func welcomeVersion() string {
	if version == "" || version == "dev" {
		return "v1"
	}
	if version[0] == 'v' {
		return version
	}
	return "v" + version
}

// ollamaHost returns the configured Ollama base URL (or the default
// localhost:11434 if unset).
func ollamaHost(cfg *config.Config) string {
	if pc, ok := cfg.Providers["ollama"]; ok && pc.Host != "" {
		return pc.Host
	}
	return ""
}

// avoid "imported and not used" if filepath is conditionally referenced.
var _ = filepath.Join

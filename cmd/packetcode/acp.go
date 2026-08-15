package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/packetcode/packetcode/internal/acp"
	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/hooks"
	"github.com/packetcode/packetcode/internal/mcp"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

type packetACPFactory struct {
	cfg         *config.Config
	provider    string
	model       string
	policy      *permissions.Policy
	sessionsDir string
	backupsDir  string
	log         io.Writer
}

func runACPCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	providerFlag := fs.String("provider", "", "override the configured provider")
	modelFlag := fs.String("model", "", "override the configured model")
	permissionFlag := fs.String("permission-mode", "", "override permission profile (ask, accept-edits, auto, read-only, bypass)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: packetcode acp [--provider NAME] [--model MODEL] [--permission-mode MODE]")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "packetcode acp: load config: %v\n", err)
		return 1
	}
	if *permissionFlag != "" {
		profile, err := permissions.ParseProfile(*permissionFlag)
		if err != nil {
			fmt.Fprintf(stderr, "packetcode acp: %v\n", err)
			return 2
		}
		cfg.Permissions.Profile = permissions.ProfileConfigName(profile)
		cfg.Permissions.Default = ""
		cfg.Behavior.TrustMode = false
	}
	policy, err := permissions.FromConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "packetcode acp: permissions: %v\n", err)
		return 1
	}
	activeProvider := cfg.Default.Provider
	activeModel := cfg.Default.Model
	if *providerFlag != "" {
		activeProvider = *providerFlag
	}
	if *modelFlag != "" {
		activeModel = *modelFlag
	}
	if activeModel == "" && activeProvider != "" {
		activeModel = cfg.Providers[activeProvider].DefaultModel
	}
	sessionsDir, err := config.SessionsDir()
	if err != nil {
		fmt.Fprintf(stderr, "packetcode acp: resolve sessions directory: %v\n", err)
		return 1
	}
	backupsDir, err := config.BackupsDir()
	if err != nil {
		fmt.Fprintf(stderr, "packetcode acp: resolve backups directory: %v\n", err)
		return 1
	}

	retryAttempts := cfg.Behavior.ProviderMaxRetries
	if retryAttempts <= 0 {
		retryAttempts = 3
	}
	provider.SetConfiguredRetry(provider.RetryConfigForAttempts(retryAttempts))
	stall := cfg.Behavior.ProviderStallTimeout
	if stall <= 0 {
		stall = 60
	}
	provider.SetConfiguredStallTimeout(time.Duration(stall) * time.Second)

	factory := &packetACPFactory{
		cfg: cfg, provider: activeProvider, model: activeModel, policy: policy,
		sessionsDir: sessionsDir, backupsDir: backupsDir, log: stderr,
	}
	server := acp.NewServer(stdin, stdout, stderr, factory, welcomeVersion())
	server.SetSessionLister(&packetSessionLister{dir: sessionsDir})
	if err := server.Serve(context.Background()); err != nil {
		fmt.Fprintf(stderr, "packetcode acp: %v\n", err)
		return 1
	}
	return 0
}

// packetSessionLister serves persisted session history to ACP clients via the
// _packetcode/sessions/list extension.
type packetSessionLister struct {
	dir string
}

func (l *packetSessionLister) ListSessions() ([]acp.SessionSummary, error) {
	summaries, err := session.NewManager(l.dir).List()
	if err != nil {
		return nil, err
	}
	out := make([]acp.SessionSummary, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, acp.SessionSummary{
			SessionID:    s.ID,
			Name:         s.Name,
			UpdatedAt:    s.UpdatedAt,
			Provider:     s.Provider,
			Model:        s.Model,
			WorkingDir:   s.WorkingDir,
			MessageCount: s.MessageCount,
			CostUSD:      s.Cost.TotalUSD,
		})
	}
	return out, nil
}

func (f *packetACPFactory) NewSession(ctx context.Context, cfg acp.SessionConfig, approver agent.Approver) (*acp.Runtime, error) {
	sessions := session.NewManager(f.sessionsDir)

	// Resume path (ACP session/load): bind the runtime to the persisted
	// transcript and prefer the provider/model the conversation was held with.
	var resumed *session.Session
	activeProvider, activeModel := f.provider, f.model
	if cfg.SessionID != "" {
		loaded, err := sessions.Load(cfg.SessionID)
		if err != nil {
			return nil, fmt.Errorf("load session %s: %w", cfg.SessionID, err)
		}
		// Remote-bound transcripts must not resume against the local filesystem.
		if err := session.ValidateWorkspace(loaded, "", ""); err != nil {
			return nil, err
		}
		if loaded.Provider != "" {
			activeProvider = loaded.Provider
		}
		if loaded.Model != "" {
			activeModel = loaded.Model
		}
		resumed = loaded
	}

	if activeProvider == "" {
		return nil, fmt.Errorf("no default provider is configured; configure PacketCode before creating a session")
	}
	if activeModel == "" {
		return nil, fmt.Errorf("no model is configured for provider %q", activeProvider)
	}
	factories := providerFactoriesFromConfig(f.cfg)
	providerFactory, ok := factories[activeProvider]
	if !ok {
		return nil, fmt.Errorf("provider %q is unknown", activeProvider)
	}
	key := f.cfg.GetProviderKey(activeProvider)
	if providerRequiresAPIKey(f.cfg, activeProvider) && key == "" {
		return nil, fmt.Errorf("provider %q is not configured with an API key", activeProvider)
	}
	reg := provider.NewRegistry()
	reg.Register(providerFactory(key))
	if err := reg.SetActive(activeProvider, activeModel); err != nil {
		return nil, err
	}

	toolReg := tools.NewRegistry()
	root := filepath.Clean(cfg.CWD)
	toolReg.Register(tools.NewReadFileTool(root))
	toolReg.Register(tools.NewSearchCodebaseTool(root))
	toolReg.Register(tools.NewListDirectoryTool(root))
	toolReg.Register(tools.NewListSymbolsTool(root))
	toolReg.Register(tools.NewFindDefinitionTool(root))
	toolReg.Register(tools.NewFindReferencesTool(root))
	toolReg.Register(tools.NewGetDiagnosticsTool(root))
	toolReg.Register(tools.NewExecuteCommandTool(root))

	current := resumed
	if current == nil {
		created, err := sessions.New(activeProvider, activeModel)
		if err != nil {
			return nil, fmt.Errorf("persist session: %w", err)
		}
		current = created
	}
	backups := session.NewBackupManager(f.backupsDir, current.ID)
	toolReg.Register(tools.NewWriteFileTool(root, backups))
	toolReg.Register(tools.NewPatchFileTool(root, backups))

	var mcpManager *mcp.Manager
	if len(cfg.MCPServers) > 0 {
		servers := make([]mcp.ServerConfig, 0, len(cfg.MCPServers))
		for _, server := range cfg.MCPServers {
			servers = append(servers, mcp.ServerConfig{
				Name: server.Name, Command: server.Command, Args: server.Args,
				Env: server.Env, Enabled: true,
			})
		}
		logDir, _ := config.HomeDir()
		mcpManager = mcp.NewManager(mcp.Config{
			Servers: servers, LogDir: logDir,
			ClientInfo: mcp.ClientInfo{Name: "packetcode-acp", Version: welcomeVersion()},
		})
		startupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		reports := mcpManager.Start(startupCtx)
		cancel()
		for _, report := range reports {
			if report.Status != "running" {
				_ = mcpManager.Shutdown(2 * time.Second)
				return nil, fmt.Errorf("MCP server %q failed to start: %s", report.Name, report.Err)
			}
			fmt.Fprintf(f.log, "packetcode acp: mcp %s: %d tools, pid %d\n", report.Name, report.ToolCount, report.PID)
		}
		for _, report := range mcp.RegisterTools(toolReg, mcpManager.Clients()) {
			if report.Status == "skipped" {
				fmt.Fprintf(f.log, "packetcode acp: mcp %s.%s skipped alias %s: %s\n", report.Server, report.Tool, report.Alias, report.Err)
			}
		}
	}

	hookRunner := hooks.New(f.cfg.Hooks, root)
	runner := agent.New(agent.Config{
		Registry: reg, Tools: toolReg, Session: sessions,
		Approver: approver, Policy: f.policy, SystemPrompt: systemPrompt,
		Hooks:         hookRunner,
		SugarCache:    packetcodeSugarCacheConfig(f.cfg),
		ConduitShadow: packetcodeConduitShadowConfig(f.cfg),
	})
	runtime := &acp.Runtime{ID: current.ID, Runner: runner}
	if resumed != nil {
		runtime.History = resumed.Messages
	}
	if mcpManager != nil {
		runtime.Close = func() error { return mcpManager.Shutdown(2 * time.Second) }
	}
	return runtime, nil
}

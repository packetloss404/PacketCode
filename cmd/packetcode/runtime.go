package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/app"
	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/hooks"
	"github.com/packetcode/packetcode/internal/mcp"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/skills"
	"github.com/packetcode/packetcode/internal/toolout"
	"github.com/packetcode/packetcode/internal/tools"
)

// packetRuntimeConfig describes the terminal-independent part of a PacketCode
// session. Frontends still own their presentation and protocol-specific
// behavior; this type owns the provider, persisted session, native tools, MCP
// fleet, hooks, and agent configuration they have in common.
type packetRuntimeConfig struct {
	Config *config.Config
	Root   string

	ProviderOverride string
	ModelOverride    string
	DefaultProvider  string
	DefaultModel     string
	ResumeID         string
	PermissionPolicy *permissions.Policy

	SessionsDir string
	BackupsDir  string

	// RegisterAllProviders is used by the TUI because it supports hot provider
	// switching. Single-session frontends register only their selected provider.
	RegisterAllProviders bool
	EnableCostTracker    bool
	CostTallyPath        string

	Backend           computers.RuntimeBackend
	ComputerID        string
	WorkspaceIdentity string

	// MCPServersSet distinguishes an explicitly empty client-owned fleet from
	// an omitted fleet, which inherits the configured PacketCode servers.
	MCPServers      []mcp.ServerConfig
	MCPServersSet   bool
	MCPClientName   string
	FailOnMCPError  bool
	MCPStartupLimit time.Duration
	DeferMCPTools   bool

	SystemPrompt       string
	SystemPromptSuffix string
	DisableHooks       bool
	Diagnostics        io.Writer
	DiagnosticPrefix   string

	// BeforeMCP lets a frontend add tools that must win name collisions before
	// MCP adapters are registered. The TUI uses it for background-agent tools.
	BeforeMCP func(*packetRuntime) error
}

type packetRuntime struct {
	Config       *config.Config
	Root         string
	Provider     string
	Model        string
	SessionID    string
	Resumed      bool
	Registry     *provider.Registry
	Tools        *tools.Registry
	Sessions     *session.Manager
	Backups      *session.BackupManager
	Skills       *skills.Registry
	Policy       *permissions.Policy
	Hooks        *hooks.Runner
	MCP          *mcp.Manager
	MCPReports   []mcp.StartupReport
	CostTracker  *cost.Tracker
	Factories    app.FactoryMap
	SystemPrompt string
	// ToolOutput spills oversized tool results so the model gets a bounded
	// excerpt plus a handle instead of silent truncation. Nil when the store
	// could not be opened, which every consumer treats as "no spilling".
	ToolOutput *toolout.Store

	closeMu      sync.Mutex
	closed       bool
	cleanups     []func() error
	mcpToolsOnce sync.Once
}

func buildPacketRuntime(ctx context.Context, opts packetRuntimeConfig) (_ *packetRuntime, err error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("runtime: missing config")
	}
	if opts.Root == "" {
		return nil, fmt.Errorf("runtime: missing working directory")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Diagnostics == nil {
		opts.Diagnostics = io.Discard
	}

	configureProviderResilience(opts.Config)

	rt := &packetRuntime{
		Config:    opts.Config,
		Root:      opts.Root,
		Registry:  provider.NewRegistry(),
		Tools:     tools.NewRegistry(),
		Sessions:  session.NewManager(opts.SessionsDir),
		Policy:    opts.PermissionPolicy,
		Factories: providerFactoriesFromConfig(opts.Config),
	}
	defer func() {
		if err != nil {
			_ = rt.Close()
		}
	}()
	if rt.Policy == nil {
		rt.Policy, err = permissions.FromConfig(opts.Config)
		if err != nil {
			return nil, fmt.Errorf("permissions: %w", err)
		}
	}
	if rt.Policy == nil {
		rt.Policy = permissions.DefaultPolicy()
	}

	activeProvider, activeModel := opts.Config.Default.Provider, opts.Config.Default.Model
	if opts.DefaultProvider != "" {
		activeProvider = opts.DefaultProvider
	}
	if opts.DefaultModel != "" {
		activeModel = opts.DefaultModel
	}
	var resumed *session.Session
	if opts.ResumeID != "" {
		resolvedID, resolveErr := rt.Sessions.ResolveID(opts.ResumeID)
		if resolveErr != nil {
			return nil, fmt.Errorf("resume %s: %w", opts.ResumeID, resolveErr)
		}
		resumed, err = rt.Sessions.Load(resolvedID)
		if err != nil {
			return nil, fmt.Errorf("resume %s: %w", opts.ResumeID, err)
		}
		if err = session.ValidateWorkspace(resumed, opts.ComputerID, opts.Root, opts.WorkspaceIdentity); err != nil {
			return nil, fmt.Errorf("resume %s: %w", opts.ResumeID, err)
		}
		if resumed.Provider != "" {
			activeProvider = resumed.Provider
		}
		if resumed.Model != "" {
			activeModel = resumed.Model
		}
		rt.Resumed = true
	}
	if opts.ProviderOverride != "" {
		if opts.ProviderOverride != activeProvider && opts.ModelOverride == "" {
			activeModel = opts.Config.Providers[opts.ProviderOverride].DefaultModel
		}
		activeProvider = opts.ProviderOverride
	}
	if opts.ModelOverride != "" {
		activeModel = opts.ModelOverride
	}
	if activeProvider == "sugar" && !opts.Config.SugarIsEnabled() && !opts.Config.SugarUsesCustomProvider() {
		return nil, fmt.Errorf("the Sugar integration is disabled; enable [sugar].enabled or set PACKETCODE_SUGAR_ENABLED=true")
	}
	if activeProvider == "" {
		return nil, fmt.Errorf("no default provider is configured; configure PacketCode before creating a session")
	}
	// Checked before the model fallback: reading a default model for a
	// provider that does not exist yields "", and the resulting "no model is
	// configured" error blames the wrong thing.
	if _, ok := rt.Factories[activeProvider]; !ok {
		if opts.RegisterAllProviders {
			return nil, fmt.Errorf("active provider %q is not configured; run packetcode without --provider to set one up", activeProvider)
		}
		return nil, fmt.Errorf("provider %q is unknown", activeProvider)
	}
	if activeModel == "" {
		activeModel = opts.Config.Providers[activeProvider].DefaultModel
	}
	if activeModel == "" {
		return nil, fmt.Errorf("no model is configured for provider %q", activeProvider)
	}

	if opts.RegisterAllProviders {
		for slug, factory := range rt.Factories {
			key := opts.Config.GetProviderKey(slug)
			if providerRequiresAPIKey(opts.Config, slug) && key == "" {
				continue
			}
			rt.Registry.Register(factory(key))
		}
	} else {
		factory, ok := rt.Factories[activeProvider]
		if !ok {
			return nil, fmt.Errorf("provider %q is unknown", activeProvider)
		}
		key := opts.Config.GetProviderKey(activeProvider)
		if providerRequiresAPIKey(opts.Config, activeProvider) && key == "" {
			return nil, fmt.Errorf("provider %q is not configured with an API key", activeProvider)
		}
		rt.Registry.Register(factory(key))
	}
	if _, ok := rt.Registry.Get(activeProvider); !ok {
		return nil, fmt.Errorf("active provider %q is not configured; run packetcode without --provider to set one up", activeProvider)
	}
	if err = rt.Registry.SetActive(activeProvider, activeModel); err != nil {
		return nil, err
	}
	rt.Provider, rt.Model = activeProvider, activeModel

	if resumed == nil {
		if _, createErr := rt.Sessions.New(activeProvider, activeModel); createErr != nil {
			return nil, fmt.Errorf("create session: %w", createErr)
		}
		if opts.ComputerID != "" {
			if err = rt.Sessions.BindWorkspace(opts.ComputerID, opts.Root, opts.WorkspaceIdentity); err != nil {
				return nil, fmt.Errorf("bind remote session: %w", err)
			}
		}
		rt.SessionID = rt.Sessions.Current().ID
	} else {
		rt.SessionID = resumed.ID
	}

	rt.Backups = session.NewBackupManager(opts.BackupsDir, rt.SessionID)
	rt.Skills = skills.Load(opts.Root)
	// Spilled tool output is per-session state, like the todo store: a
	// background job must not be able to read or evict what the foreground
	// session captured. A store that fails to open is left nil, which the
	// agent and the tool both treat as "no spilling" rather than an error --
	// losing the remainder of a large result is not a reason to refuse to
	// start.
	if store, storeErr := toolout.OpenDefault(toolout.Options{}); storeErr != nil {
		fmt.Fprintf(opts.Diagnostics, "%stool output store unavailable: %v\n", opts.DiagnosticPrefix, storeErr)
	} else {
		rt.ToolOutput = store
		rt.AddCleanup(store.Close)
	}
	registerNativeTools(rt.Tools, opts.Root, opts.Backend, rt.Backups, rt.Skills,
		opts.Config.Behavior.PostEditDiagnosticsDisabled, rt.ToolOutput)
	if !opts.DisableHooks && opts.Backend == nil {
		rt.Hooks = hooks.New(opts.Config.Hooks, opts.Root)
	}

	if opts.EnableCostTracker {
		if opts.CostTallyPath == "" {
			opts.CostTallyPath, err = config.CostTallyPath()
			if err != nil {
				return nil, err
			}
		}
		rt.CostTracker, err = cost.NewTracker(opts.CostTallyPath, func(slug, modelID string) (float64, float64) {
			if p, ok := rt.Registry.Get(slug); ok {
				return p.Pricing(modelID)
			}
			return 0, 0
		})
		if err != nil {
			return nil, err
		}
		rt.CostTracker.SetCacheRates(func(slug, modelID string) (float64, float64) {
			if p, ok := rt.Registry.Get(slug); ok {
				return provider.CacheMultipliersFor(p, modelID)
			}
			return provider.CacheReadMultiplier, provider.CacheWriteMultiplier
		})
	}

	if opts.BeforeMCP != nil {
		if err = opts.BeforeMCP(rt); err != nil {
			return nil, err
		}
	}
	if err = rt.startMCP(ctx, opts); err != nil {
		return nil, err
	}

	rt.SystemPrompt = opts.SystemPrompt
	if block := rt.Skills.IndexBlock(); block != "" {
		rt.SystemPrompt += "\n\n" + block
	}
	rt.SystemPrompt += opts.SystemPromptSuffix
	for _, skillErr := range rt.Skills.Errors() {
		fmt.Fprintf(opts.Diagnostics, "%sskill %s\n", opts.DiagnosticPrefix, skillErr)
	}
	for _, warning := range rt.Skills.Warnings() {
		fmt.Fprintf(opts.Diagnostics, "%sskill %s\n", opts.DiagnosticPrefix, warning)
	}
	return rt, nil
}

func configureProviderResilience(cfg *config.Config) {
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
}

func registerNativeTools(reg *tools.Registry, root string, backend computers.RuntimeBackend, backups *session.BackupManager, skillRegistry *skills.Registry, noPostEditDiagnostics bool, toolOutput *toolout.Store) {
	reg.Register(tools.NewTodoWriteTool(tools.NewTodoStore()))
	reg.Register(tools.NewReadToolOutputTool(toolOutput))
	reg.Register(tools.NewSkillTool(skillRegistry))
	reg.Register(tools.NewFetchTool())
	if backend != nil {
		reg.Register(tools.NewReadFileToolWithBackend(backend))
		reg.Register(tools.NewSearchCodebaseToolWithBackend(backend))
		reg.Register(tools.NewListDirectoryToolWithBackend(backend))
		reg.Register(tools.NewExecuteCommandToolWithBackend(backend))
		remoteWrite := tools.NewWriteFileToolWithBackend(backend)
		remoteWrite.DiagnosticsDisabled = noPostEditDiagnostics
		remotePatch := tools.NewPatchFileToolWithBackend(backend)
		remotePatch.DiagnosticsDisabled = noPostEditDiagnostics
		reg.Register(remoteWrite)
		reg.Register(remotePatch)
		return
	}
	reg.Register(tools.NewReadFileTool(root))
	reg.Register(tools.NewSearchCodebaseTool(root))
	reg.Register(tools.NewListDirectoryTool(root))
	reg.Register(tools.NewListSymbolsTool(root))
	reg.Register(tools.NewFindDefinitionTool(root))
	reg.Register(tools.NewFindReferencesTool(root))
	reg.Register(tools.NewGetDiagnosticsTool(root))
	reg.Register(tools.NewExecuteCommandTool(root))
	write := tools.NewWriteFileTool(root, backups)
	write.DiagnosticsDisabled = noPostEditDiagnostics
	patch := tools.NewPatchFileTool(root, backups)
	patch.DiagnosticsDisabled = noPostEditDiagnostics
	reg.Register(write)
	reg.Register(patch)
}

func (rt *packetRuntime) startMCP(ctx context.Context, opts packetRuntimeConfig) error {
	servers := opts.MCPServers
	if !opts.MCPServersSet {
		servers = mcpServerConfigsFrom(opts.Config)
	}
	clientName := opts.MCPClientName
	if clientName == "" {
		clientName = "packetcode"
	}
	logDir, _ := config.HomeDir()
	rt.MCP = mcp.NewManager(mcp.Config{
		Servers: servers, LogDir: logDir,
		ClientInfo: mcp.ClientInfo{Name: clientName, Version: welcomeVersion()},
	})
	limit := opts.MCPStartupLimit
	if limit <= 0 {
		limit = 30 * time.Second
	}
	startupCtx, cancel := context.WithTimeout(ctx, limit)
	rt.MCPReports = rt.MCP.Start(startupCtx)
	cancel()
	rt.AddCleanup(func() error { return rt.MCP.Shutdown(2 * time.Second) })
	for _, report := range rt.MCPReports {
		if report.Status != "running" && opts.FailOnMCPError {
			return fmt.Errorf("MCP server %q failed to start: %s", report.Name, report.Err)
		}
	}
	if !opts.DeferMCPTools {
		rt.RegisterMCPTools(opts.Diagnostics, opts.DiagnosticPrefix)
	}
	return nil
}

func (rt *packetRuntime) RegisterMCPTools(diagnostics io.Writer, prefix string) {
	if rt == nil || rt.MCP == nil {
		return
	}
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	rt.mcpToolsOnce.Do(func() {
		for _, report := range mcp.RegisterTools(rt.Tools, rt.MCP.Clients()) {
			if report.Status == "skipped" {
				fmt.Fprintf(diagnostics, "%smcp %s.%s skipped alias %s: %s\n", prefix, report.Server, report.Tool, report.Alias, report.Err)
			}
		}
	})
}

func (rt *packetRuntime) NewAgent(approver agent.Approver) *agent.Agent {
	return agent.New(agent.Config{
		LoopDetection: agent.LoopDetectionSettings(
			rt.Config.Behavior.LoopDetectionDisabled,
			rt.Config.Behavior.LoopDetectionWindow,
			rt.Config.Behavior.LoopDetectionThreshold),
		Registry:      rt.Registry,
		Tools:         rt.Tools,
		Session:       rt.Sessions,
		CostTracker:   rt.CostTracker,
		Approver:      approver,
		Policy:        rt.Policy,
		SystemPrompt:  rt.SystemPrompt,
		Hooks:         rt.Hooks,
		SugarCache:    packetcodeSugarCacheConfig(rt.Config),
		ConduitShadow: packetcodeConduitShadowConfig(rt.Config),
		ToolOutput:    rt.toolOutputStore(),
	})
}

// toolOutputStore returns the spill store as the agent's interface, or a nil
// interface when there is none. Returning the typed nil pointer directly
// would give the agent a non-nil interface holding a nil pointer, which is
// the classic way a "no store configured" check silently stops working.
func (rt *packetRuntime) toolOutputStore() agent.ToolOutputStore {
	if rt == nil || rt.ToolOutput == nil {
		return nil
	}
	return rt.ToolOutput
}

func (rt *packetRuntime) CurrentSession() *session.Session {
	if rt == nil || rt.Sessions == nil {
		return nil
	}
	return rt.Sessions.Current()
}

func (rt *packetRuntime) AddCleanup(fn func() error) {
	if rt == nil || fn == nil {
		return
	}
	rt.closeMu.Lock()
	defer rt.closeMu.Unlock()
	if !rt.closed {
		rt.cleanups = append(rt.cleanups, fn)
	}
}

func (rt *packetRuntime) Close() error {
	if rt == nil {
		return nil
	}
	rt.closeMu.Lock()
	if rt.closed {
		rt.closeMu.Unlock()
		return nil
	}
	rt.closed = true
	cleanups := append([]func() error(nil), rt.cleanups...)
	rt.cleanups = nil
	rt.closeMu.Unlock()
	var errs []error
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

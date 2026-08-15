package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
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
	// ceiling is the most permissive profile a client-requested permissionMode
	// may select; the operator's startup configuration sets it.
	ceiling     permissions.Profile
	sessionsDir string
	backupsDir  string
	log         io.Writer
}

// profileRank orders the built-in profiles from least to most permissive for
// the escalation ceiling on client-requested permission modes.
var profileRank = map[permissions.Profile]int{
	permissions.ProfileSafe: 0,
	permissions.ProfileAsk:  1,
	permissions.ProfileEdit: 2,
	permissions.ProfileAuto: 3,
	permissions.ProfileFull: 4,
}

// serverPermissionCeiling derives the escalation ceiling from the effective
// startup configuration (after any --permission-mode flag was applied — the
// flag writes cfg.Permissions.Profile). An EXPLICITLY configured profile caps
// what clients may request; a default (empty) profile leaves them
// unrestricted, because on a default setup the ACP client's local user is the
// operator and per-session modes are their consent mechanism. Custom profiles
// have unknown permissiveness and also leave clients unrestricted.
func serverPermissionCeiling(cfg *config.Config) permissions.Profile {
	ceiling := permissions.ProfileFull
	if cfg.Permissions.Profile != "" {
		if profile, err := permissions.ParseProfile(cfg.Permissions.Profile); err == nil {
			ceiling = profile
		}
	}
	if cfg.Behavior.TrustMode {
		ceiling = permissions.ProfileFull
	}
	return ceiling
}

// allowedPermissionModes filters the wire vocabulary to modes at or below the
// ceiling, so clients are never offered an escalation the factory rejects.
func allowedPermissionModes(ceiling permissions.Profile) []string {
	out := make([]string, 0, len(acp.PermissionModes))
	for _, mode := range acp.PermissionModes {
		profile, err := permissions.ParseProfile(mode)
		if err != nil {
			continue
		}
		if profileRank[profile] <= profileRank[ceiling] {
			out = append(out, mode)
		}
	}
	return out
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

	ceiling := serverPermissionCeiling(cfg)
	factory := &packetACPFactory{
		cfg: cfg, provider: activeProvider, model: activeModel, policy: policy,
		ceiling:     ceiling,
		sessionsDir: sessionsDir, backupsDir: backupsDir, log: stderr,
	}
	server := acp.NewServer(stdin, stdout, stderr, factory, welcomeVersion())
	server.SetSessionLister(&packetSessionLister{dir: sessionsDir})
	server.SetSessionRenamer(&packetSessionRenamer{dir: sessionsDir})
	server.SetUsageReader(&packetUsageReader{dir: sessionsDir})
	server.SetModelCatalog(&packetModelCatalog{cfg: cfg, activeProvider: activeProvider, activeModel: activeModel})
	server.SetMCPLister(&packetMCPLister{cfg: cfg})
	server.SetPermissionModes(allowedPermissionModes(ceiling))
	if err := server.Serve(context.Background()); err != nil {
		fmt.Fprintf(stderr, "packetcode acp: %v\n", err)
		return 1
	}
	return 0
}

// packetModelCatalog serves the configured provider/model choices to ACP
// clients via the _packetcode/models/list extension. The active provider and
// model (config defaults plus any CLI override) are flagged Default so
// clients can preselect them.
type packetModelCatalog struct {
	cfg            *config.Config
	activeProvider string
	activeModel    string
}

func (c *packetModelCatalog) ListModels() ([]acp.ModelOption, error) {
	out := make([]acp.ModelOption, 0, len(c.cfg.Providers)+1)
	seen := make(map[string]struct{})
	add := func(providerSlug, model string) {
		if providerSlug == "" || model == "" {
			return
		}
		key := providerSlug + "\x00" + model
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, acp.ModelOption{
			Provider: providerSlug,
			Model:    model,
			Default:  providerSlug == c.activeProvider && model == c.activeModel,
		})
	}
	// The active pair leads the list even when it is absent from config
	// (e.g. a --model CLI override).
	add(c.activeProvider, c.activeModel)
	factories := providerFactoriesFromConfig(c.cfg)
	slugs := make([]string, 0, len(c.cfg.Providers))
	for slug := range c.cfg.Providers {
		if _, ok := factories[slug]; ok {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		pc := c.cfg.Providers[slug]
		add(slug, pc.DefaultModel)
		for _, m := range pc.Models {
			add(slug, m.ID)
		}
	}
	return out, nil
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

// sessionPolicy resolves the permission policy one session runs under. An
// empty mode keeps the server-wide policy; otherwise the mode maps to a
// profile via permissions.ParseProfile and a fresh policy is built from the
// config with that profile, mirroring the --permission-mode CLI override (the
// inline default rule and trust mode are cleared so the chosen mode wins).
func (f *packetACPFactory) sessionPolicy(mode string) (*permissions.Policy, error) {
	if mode == "" {
		return f.policy, nil
	}
	profile, err := permissions.ParseProfile(mode)
	if err != nil {
		return nil, fmt.Errorf("%w %q", acp.ErrUnknownPermissionMode, mode)
	}
	// Escalation ceiling: a client may narrow its session's permissions but
	// never exceed what the operator configured the server with.
	if f.ceiling != "" && profileRank[profile] > profileRank[f.ceiling] {
		return nil, fmt.Errorf(
			"%w: %q exceeds the server's configured permission profile %q",
			acp.ErrPermissionModeDenied, mode, f.ceiling,
		)
	}
	var cfg config.Config
	if f.cfg != nil {
		cfg = *f.cfg
	}
	cfg.Permissions.Default = ""
	cfg.Behavior.TrustMode = false
	return permissions.FromConfigWithProfile(&cfg, profile)
}

// packetSessionRenamer persists display-name changes for ACP clients via the
// _packetcode/sessions/rename extension. Each call builds its own Manager, so
// Load only sets that throwaway Manager's current session and Rename+Save act
// on it alone — a live runtime's Manager (created per NewSession call) is
// never touched. Caveat: a runtime keeps its whole Session in memory and
// Save persists all of it, so renaming a session that has an active runtime
// is reverted by that runtime's next Save (e.g. the next turn's AddMessage).
// The name is normalized by session.Manager.Rename (sanitizeName: lowercase,
// spaces to hyphens, a-z0-9-_ only, 40 chars max).
type packetSessionRenamer struct {
	dir string
}

func (r *packetSessionRenamer) RenameSession(id, name string) error {
	manager := session.NewManager(r.dir)
	if _, err := manager.Load(id); err != nil {
		return err
	}
	return manager.Rename(name)
}

// packetUsageReader serves per-session token/cost usage to ACP clients via
// the _packetcode/sessions/usage extension and prompt-result enrichment. It
// re-reads the persisted session file on every call: the agent saves usage
// after each stream completion, so the file is authoritative, and a full
// parse per turn is acceptable.
type packetUsageReader struct {
	dir string
}

func (r *packetUsageReader) ReadUsage(sessionID string) (acp.SessionUsage, error) {
	loaded, err := session.NewManager(r.dir).Load(sessionID)
	if err != nil {
		return acp.SessionUsage{}, err
	}
	return acp.SessionUsage{
		ContextTokens: loaded.TokenUsage.ContextTokens,
		TotalInput:    loaded.TokenUsage.TotalInput,
		TotalOutput:   loaded.TokenUsage.TotalOutput,
		CostUSD:       loaded.Cost.TotalUSD,
	}, nil
}

// packetMCPLister serves the operator's configured MCP servers to ACP clients
// via the _packetcode/mcp/list extension. It reports configuration, not live
// process state — a session's actual fleet is reported from its Runtime when
// the client passes a sessionId.
type packetMCPLister struct {
	cfg *config.Config
}

func (l *packetMCPLister) ListMCPServers() ([]acp.MCPServerStatus, error) {
	servers := mcpServerConfigsFrom(l.cfg)
	out := make([]acp.MCPServerStatus, 0, len(servers))
	for _, server := range servers {
		status := "configured"
		if !server.Enabled {
			status = "disabled"
		}
		out = append(out, acp.MCPServerStatus{
			Name: server.Name, Status: status, Source: "agent", Command: server.Command,
		})
	}
	return out, nil
}

// mcpServersForSession resolves which MCP servers one ACP session runs with,
// and reports whether the client chose them.
//
// The ACP contract is that the client owns the session's MCP fleet, so a
// client-supplied list — even an explicitly empty one — is used verbatim. But
// "the client said nothing" is not the same as "the client said none": ACP
// clients that do not manage MCP themselves (the PacketCode desktop app) omit
// the field, and answering that with an empty fleet is what made a user's
// configured [mcp.<name>] tools silently absent from desktop sessions while the
// TUI had them. An omitted field therefore inherits the agent's own config.
func mcpServersForSession(sc acp.SessionConfig, cfg *config.Config) (servers []mcp.ServerConfig, clientSupplied bool) {
	if !sc.MCPServersSet {
		return mcpServerConfigsFrom(cfg), false
	}
	out := make([]mcp.ServerConfig, 0, len(sc.MCPServers))
	for _, server := range sc.MCPServers {
		out = append(out, mcp.ServerConfig{
			Name: server.Name, Command: server.Command, Args: server.Args,
			Env: server.Env, Enabled: true,
		})
	}
	return out, true
}

// startMCP spawns the session's MCP fleet and registers its tools.
//
// Failure semantics differ by who chose the servers, deliberately:
//   - client-supplied: a server that fails to start fails session creation.
//     The client named exactly these servers for this session; silently
//     running without one would hand the agent a fleet it did not ask for.
//   - agent-configured defaults: a failed server is logged and skipped, the
//     session proceeds. This matches the TUI (see runTUI, where MCP failures
//     "are logged but never block startup") and keeps one broken [mcp.<name>]
//     block in config.toml from making every desktop session uncreatable —
//     which, with the defaults fallback above, would otherwise turn a config
//     typo into an unusable GUI.
func (f *packetACPFactory) startMCP(
	ctx context.Context, cfg acp.SessionConfig, toolReg *tools.Registry,
) (*mcp.Manager, []acp.MCPServerStatus, error) {
	servers, clientSupplied := mcpServersForSession(cfg, f.cfg)
	if len(servers) == 0 {
		return nil, []acp.MCPServerStatus{}, nil
	}
	source := "agent"
	if clientSupplied {
		source = "client"
	}
	logDir, _ := config.HomeDir()
	manager := mcp.NewManager(mcp.Config{
		Servers: servers, LogDir: logDir,
		ClientInfo: mcp.ClientInfo{Name: "packetcode-acp", Version: welcomeVersion()},
	})
	startupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	reports := manager.Start(startupCtx)
	cancel()

	statuses := make([]acp.MCPServerStatus, 0, len(reports))
	for _, report := range reports {
		if report.Status != "running" && clientSupplied {
			_ = manager.Shutdown(2 * time.Second)
			return nil, nil, fmt.Errorf("MCP server %q failed to start: %s", report.Name, report.Err)
		}
		switch report.Status {
		case "running":
			fmt.Fprintf(f.log, "packetcode acp: mcp %s: %d tools, pid %d\n", report.Name, report.ToolCount, report.PID)
		default:
			fmt.Fprintf(f.log, "packetcode acp: mcp %s: %s — %s\n", report.Name, report.Status, report.Err)
		}
		statuses = append(statuses, acp.MCPServerStatus{
			Name: report.Name, Status: report.Status, ToolCount: report.ToolCount,
			Source: source, Command: report.Command, Error: report.Err,
		})
	}
	for _, report := range mcp.RegisterTools(toolReg, manager.Clients()) {
		if report.Status == "skipped" {
			fmt.Fprintf(f.log, "packetcode acp: mcp %s.%s skipped alias %s: %s\n", report.Server, report.Tool, report.Alias, report.Err)
		}
	}
	return manager, statuses, nil
}

func (f *packetACPFactory) NewSession(ctx context.Context, cfg acp.SessionConfig, approver agent.Approver) (*acp.Runtime, error) {
	// Resolve the per-session policy first so an invalid permissionMode fails
	// before any session state is created.
	policy, err := f.sessionPolicy(cfg.PermissionMode)
	if err != nil {
		return nil, err
	}
	sessions := session.NewManager(f.sessionsDir)
	factories := providerFactoriesFromConfig(f.cfg)

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

	// Per-session overrides (session/new "_packetcode" object) take precedence
	// over both the configured defaults and a resumed transcript's stored pair.
	if cfg.Provider != "" {
		if _, ok := factories[cfg.Provider]; !ok {
			return nil, fmt.Errorf("%w %q", acp.ErrUnknownProvider, cfg.Provider)
		}
		if cfg.Provider != activeProvider {
			// A provider override invalidates the previous model choice; fall
			// back to that provider's own default unless the client also
			// picked a model.
			activeModel = f.cfg.Providers[cfg.Provider].DefaultModel
		}
		activeProvider = cfg.Provider
	}
	if cfg.Model != "" {
		activeModel = cfg.Model
	}

	if activeProvider == "" {
		return nil, fmt.Errorf("no default provider is configured; configure PacketCode before creating a session")
	}
	if activeModel == "" {
		return nil, fmt.Errorf("no model is configured for provider %q", activeProvider)
	}
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

	mcpManager, mcpStatuses, err := f.startMCP(ctx, cfg, toolReg)
	if err != nil {
		return nil, err
	}

	hookRunner := hooks.New(f.cfg.Hooks, root)
	runner := agent.New(agent.Config{
		Registry: reg, Tools: toolReg, Session: sessions,
		Approver: approver, Policy: policy, SystemPrompt: systemPrompt,
		Hooks:         hookRunner,
		SugarCache:    packetcodeSugarCacheConfig(f.cfg),
		ConduitShadow: packetcodeConduitShadowConfig(f.cfg),
	})
	runtime := &acp.Runtime{ID: current.ID, Runner: runner, MCPServers: mcpStatuses}
	if resumed != nil {
		runtime.History = resumed.Messages
	}
	if mcpManager != nil {
		runtime.Close = func() error { return mcpManager.Shutdown(2 * time.Second) }
	}
	return runtime, nil
}

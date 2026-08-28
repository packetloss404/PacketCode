// Command packetcode is a keyboard-first, multi-provider AI coding agent
// for the terminal.
//
// Usage:
//
//	packetcode                              start the TUI in the cwd
//	packetcode --version                    print version and exit
//	packetcode --provider gemini --model gemini-2.5-pro
//	packetcode --computer production      work against a registered SSH computer
//	packetcode --resume <session-id>        resume a saved session
//	packetcode --trust                      auto-approve all tool actions
//	packetcode --permission-mode ask        override approval profile
//	packetcode doctor                       diagnose local setup
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/app"
	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/git"
	"github.com/packetcode/packetcode/internal/hooks"
	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/mcp"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/theme"
	"github.com/packetcode/packetcode/internal/workflow"
)

// version/commit are populated at build time via -ldflags. Defaults are
// used during `go run` and local development.
var (
	version = "dev"
	commit  = "none"
)

const systemPrompt = `You are packetcode, a keyboard-first AI coding agent running in the user's terminal. You have direct access to the user's project via tools (read_file, write_file, patch_file, execute_command, search_codebase, list_directory, list_symbols, find_definition, find_references, get_diagnostics, todo_write). File modifications, command executions, background-agent spawns, foreground result collection, and MCP tool calls are governed by the user's current permission policy.

# Tone and response style
Be concise and direct. Minimize output tokens while staying correct, helpful, and complete — the goal is brevity without dropping information the user needs.

Match the length of your reply to the task. A simple question gets a one- or two-line answer, often a single sentence; only substantial, open-ended work warrants a long response. Don't inflate a small answer with extra structure.

Skip preamble and postamble. Don't open with "I'll help you…" or "Here's what I'm going to do", and don't close with a recap of what you did unless asked. After an edit, a one-line note of what changed is usually enough — let the diff speak for itself. Don't explain code you just wrote unless asked.

Prefer plain prose. Reach for headers, bulleted lists, tables, and multi-section reports only when the task genuinely needs them (for example, the user explicitly asks for a structured review). For most requests a few sentences, with ` + "`path:line`" + ` references where useful, read better than a formatted report.

When you investigate or review, lead with the few highest-impact findings and stop there rather than exhaustively enumerating everything you noticed; offer to go deeper instead of front-loading it all. This is a terminal UI — walls of text are hard to scan, so keep it tight.

# Working approach
For independent research, review, or read-only tasks, fan out background agents in parallel when that will materially reduce latency, then collect and synthesize their results. Serialize overlapping writes and keep each delegated task concrete and bounded. For a direct change: gather context with the read tools as needed, then make small, surgical edits. Don't narrate a long plan before acting on a simple task — just do it. For work that genuinely has several steps, track it with todo_write instead of describing it: send the complete list each time, keep exactly one item in_progress, and close each item as soon as it is done. The list is rendered for the user, so never restate it in prose. Match the style, naming, and conventions of the surrounding code.`

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	providerFlag := flag.String("provider", "", "override default provider for this session")
	modelFlag := flag.String("model", "", "override default model for this session")
	resumeFlag := flag.String("resume", "", "resume a saved session by ID")
	trustFlag := flag.Bool("trust", false, "auto-approve all tool actions for this session")
	permissionFlag := flag.String("permission-mode", "", "override permission profile for this session (ask, accept-edits, auto, read-only, bypass)")
	computerFlag := flag.String("computer", "", "use a registered SSH computer as the active workspace")
	tuiFixtureFlag := flag.String("tui-fixture", "", "render a deterministic TUI lifecycle fixture (development/testing)")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("packetcode %s (%s)\n", version, commit)
		return
	}
	if *tuiFixtureFlag != "" {
		if err := runTUIFixture(*tuiFixtureFlag); err != nil {
			fmt.Fprintf(os.Stderr, "packetcode: %s\n", err)
			os.Exit(1)
		}
		return
	}
	if code, ok := dispatchSubcommand(flag.Args(), os.Stdout, os.Stderr); ok {
		os.Exit(code)
	}

	if err := run(*providerFlag, *modelFlag, *resumeFlag, *trustFlag, *permissionFlag, *computerFlag); err != nil {
		fmt.Fprintf(os.Stderr, "packetcode: %s\n", err)
		os.Exit(1)
	}
}

func dispatchSubcommand(args []string, stdout, stderr io.Writer) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "doctor":
		return runDoctorCommand(args[1:], stdout, stderr), true
	case "acp":
		return runACPCommand(args[1:], os.Stdin, stdout, stderr), true
	case "sugar":
		if len(args) >= 2 && args[1] == "login" {
			return runSugarLoginCommand(args[2:], stdout, stderr), true
		}
		fmt.Fprintln(stderr, "usage: packetcode sugar login [--server URL] [--name NAME] [--no-browser]")
		return 2, true
	default:
		return 0, false
	}
}

func run(providerOverride, modelOverride, resumeID string, trust bool, permissionMode, computerName string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if permissionMode != "" {
		profile, err := permissions.ParseProfile(permissionMode)
		if err != nil {
			return err
		}
		cfg.Permissions.Profile = permissions.ProfileConfigName(profile)
		cfg.Permissions.Default = ""
	}

	// Optional user theme. A missing file is silent; a parse error
	// logs one stderr line and falls through to the built-in Terminal
	// Noir defaults — a bad theme never prevents packetcode from
	// starting.
	themePath, err := config.ThemePath()
	if err == nil {
		if t, err := theme.Load(themePath); err != nil {
			fmt.Fprintf(os.Stderr, "packetcode: failed to load theme: %v; falling back to defaults\n", err)
		} else {
			theme.Apply(t)
		}
	}

	factories := providerFactoriesFromConfig(cfg)

	activeSlug := cfg.Default.Provider
	activeModel := cfg.Default.Model
	if providerOverride != "" {
		activeSlug = providerOverride
	}
	if modelOverride != "" {
		activeModel = modelOverride
	}

	// First-run: no saved default provider yet → walk through setup.
	// An explicit --provider is a session override, so respect it before
	// deciding whether onboarding is needed. If that override is not
	// configured, startup reports the normal "active provider is not
	// configured" error below instead of forcing unrelated setup.
	if shouldRunSetup(cfg, providerOverride) {
		_, err := app.RunSetup(os.Stdin, os.Stdout, cfg, factories)
		if err != nil {
			return err
		}
		// Reload the now-saved config so in-memory state matches disk.
		cfg, err = config.Load()
		if err != nil {
			return err
		}
		activeSlug = cfg.Default.Provider
		activeModel = cfg.Default.Model
		if providerOverride != "" {
			activeSlug = providerOverride
		}
		if modelOverride != "" {
			activeModel = modelOverride
		}
	}

	if trust {
		cfg.Behavior.TrustMode = true
	}
	if permissionMode != "" {
		profile, err := permissions.ParseProfile(permissionMode)
		if err != nil {
			return err
		}
		cfg.Permissions.Profile = permissions.ProfileConfigName(profile)
		cfg.Permissions.Default = ""
	}
	permissionPolicy, err := permissions.FromConfig(cfg)
	if err != nil {
		return fmt.Errorf("permissions: %w", err)
	}

	// Build the provider registry. Only register providers the user has
	// actually configured — listing every provider would clutter the
	// switcher with non-functional options.
	reg := provider.NewRegistry()

	// Apply the configured retry policy before any provider streams a
	// request. Default to 3 attempts when unset.
	retryAttempts := cfg.Behavior.ProviderMaxRetries
	if retryAttempts <= 0 {
		retryAttempts = 3
	}
	provider.SetConfiguredRetry(provider.RetryConfigForAttempts(retryAttempts))

	// Apply the configured per-call stall timeout. A provider stream that
	// goes silent for longer than this is aborted as a retryable error.
	// Default to 60s when unset.
	stall := cfg.Behavior.ProviderStallTimeout
	if stall <= 0 {
		stall = 60
	}
	provider.SetConfiguredStallTimeout(time.Duration(stall) * time.Second)

	for slug, factory := range factories {
		key := cfg.GetProviderKey(slug)
		if providerRequiresAPIKey(cfg, slug) && key == "" {
			continue
		}
		reg.Register(factory(key))
	}
	// Resolve the working directory to the git repo root if we're in one.
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := git.RepoRoot(cwd)
	var runtimeBackend computers.RuntimeBackend
	var activeComputer *computers.Computer
	if !cfg.PacketComputers.IsEnabled() && computerName != "" {
		return fmt.Errorf("Packet Computers integration is disabled; enable [packet_computers].enabled or set PACKETCODE_PACKET_COMPUTERS_ENABLED=true")
	}
	if computerName != "" {
		computersDir, dirErr := config.ComputersDir()
		if dirErr != nil {
			return dirErr
		}
		computerRegistry, loadErr := computers.Load(computersDir)
		if loadErr != nil {
			return fmt.Errorf("load computers: %w", loadErr)
		}
		computer, ok := computerRegistry.Get(computerName)
		if !ok {
			return fmt.Errorf("computer %q is not registered; use /computers ssh ... first", computerName)
		}
		if computer.Kind != computers.KindSSH {
			return fmt.Errorf("computer %q is %s; --computer currently supports registered SSH computers", computer.Name, computer.Kind)
		}
		connectCtx, cancelConnect := context.WithTimeout(context.Background(), 20*time.Second)
		backend, err := computers.NewSSHBackend(connectCtx, computer)
		cancelConnect()
		if err != nil {
			return fmt.Errorf("connect computer %q: %w", computer.Name, err)
		}
		runtimeBackend = backend
		activeComputer = &computer
		root = backend.Root()
		defer backend.Close()
	}

	// Tool registry. write_file and patch_file get a backup manager
	// scoped to the active session — wired below once we know the ID.
	toolReg := tools.NewRegistry()
	// One store per session. The list is conversation state, so a background
	// job must not be able to overwrite what the foreground is tracking.
	toolReg.Register(tools.NewTodoWriteTool(tools.NewTodoStore()))
	if runtimeBackend != nil {
		toolReg.Register(tools.NewReadFileToolWithBackend(runtimeBackend))
		toolReg.Register(tools.NewSearchCodebaseToolWithBackend(runtimeBackend))
		toolReg.Register(tools.NewListDirectoryToolWithBackend(runtimeBackend))
		toolReg.Register(tools.NewExecuteCommandToolWithBackend(runtimeBackend))
	} else {
		toolReg.Register(tools.NewReadFileTool(root))
		toolReg.Register(tools.NewSearchCodebaseTool(root))
		toolReg.Register(tools.NewListDirectoryTool(root))
		toolReg.Register(tools.NewListSymbolsTool(root))
		toolReg.Register(tools.NewFindDefinitionTool(root))
		toolReg.Register(tools.NewFindReferencesTool(root))
		toolReg.Register(tools.NewGetDiagnosticsTool(root))
		toolReg.Register(tools.NewExecuteCommandTool(root))
	}

	// Sessions.
	sessionsDir, err := config.SessionsDir()
	if err != nil {
		return err
	}
	sessions := session.NewManager(sessionsDir)
	if resumeID != "" {
		resolvedID, err := sessions.ResolveID(resumeID)
		if err != nil {
			return fmt.Errorf("resume %s: %w", resumeID, err)
		}
		loaded, err := sessions.Load(resolvedID)
		if err != nil {
			return fmt.Errorf("resume %s: %w", resumeID, err)
		}
		computerID := ""
		workspaceIdentity := ""
		if activeComputer != nil {
			computerID = activeComputer.ID
			workspaceIdentity = computerWorkspaceIdentity(*activeComputer)
		}
		if err := session.ValidateWorkspace(loaded, computerID, root, workspaceIdentity); err != nil {
			return fmt.Errorf("resume %s: %w", resumeID, err)
		}
		if providerOverride == "" && loaded.Provider != "" {
			activeSlug = loaded.Provider
		}
		if modelOverride == "" && loaded.Model != "" {
			activeModel = loaded.Model
		}
	}
	if activeSlug == "sugar" && !cfg.SugarIsEnabled() && !cfg.SugarUsesCustomProvider() {
		return fmt.Errorf("Sugar integration is disabled; enable [sugar].enabled or set PACKETCODE_SUGAR_ENABLED=true")
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
	if resumeID == "" {
		if _, err := sessions.New(activeSlug, activeModel); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		if activeComputer != nil {
			if err := sessions.BindWorkspace(activeComputer.ID, root, computerWorkspaceIdentity(*activeComputer)); err != nil {
				return fmt.Errorf("bind remote session: %w", err)
			}
		}
	}

	// Backup manager keyed by session ID.
	backupsDir, err := config.BackupsDir()
	if err != nil {
		return err
	}
	bk := session.NewBackupManager(backupsDir, sessions.Current().ID)
	if runtimeBackend != nil {
		toolReg.Register(tools.NewWriteFileToolWithBackend(runtimeBackend))
		toolReg.Register(tools.NewPatchFileToolWithBackend(runtimeBackend))
	} else {
		toolReg.Register(tools.NewWriteFileTool(root, bk))
		toolReg.Register(tools.NewPatchFileTool(root, bk))
	}

	var hookRunner *hooks.Runner
	if runtimeBackend == nil {
		hookRunner = hooks.New(cfg.Hooks, root)
	}

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

	// MCP servers — spawn external tool processes declared in
	// ~/.packetcode/config.toml. Failures are logged but never block
	// startup. We spawn here (after theme + tool registry bootstrap,
	// before jobs.NewManager) so tools are discovered in time to land
	// in toolReg before the first Agent turn.
	mcpLogDir, _ := config.HomeDir()
	mcpMgr := mcp.NewManager(mcp.Config{
		Servers:    mcpServerConfigsFrom(cfg),
		LogDir:     mcpLogDir,
		ClientInfo: mcp.ClientInfo{Name: "packetcode", Version: welcomeVersion()},
	})
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	mcpReports := mcpMgr.Start(startupCtx)
	cancelStartup()
	for _, r := range mcpReports {
		switch r.Status {
		case "running":
			fmt.Fprintf(os.Stderr, "packetcode: mcp %s: %d tools, pid %d\n", r.Name, r.ToolCount, r.PID)
		case "failed":
			fmt.Fprintf(os.Stderr, "packetcode: mcp %s: failed — %s\n", r.Name, r.Err)
		}
	}
	defer mcpMgr.Shutdown(2 * time.Second)

	// Background agents remain coordinated locally, but each remote job owns
	// an independent pinned SSH/SFTP backend. This preserves workflow fan-out
	// and avoids sharing the foreground connection's command/SFTP lock.
	jobsDir, err := config.JobsDir()
	if err != nil {
		return err
	}
	worktreesDir, err := config.WorktreesDir()
	if err != nil {
		return err
	}
	defaultWorkspace := jobs.Workspace{WorkingDir: root, Kind: computers.KindLocal}
	if activeComputer != nil {
		defaultWorkspace = jobs.Workspace{
			ComputerID:   activeComputer.ID,
			ComputerName: activeComputer.Name,
			WorkingDir:   activeComputer.ProjectRoots[0],
			Kind:         activeComputer.Kind,
			Identity:     computerWorkspaceIdentity(*activeComputer),
			Policy:       activeComputer.Policy,
		}
	}
	disabledComputersError := func() error {
		return fmt.Errorf("Packet Computers integration is disabled; enable [packet_computers].enabled or set PACKETCODE_PACKET_COMPUTERS_ENABLED=true")
	}
	var resolveWorkspace jobs.WorkspaceResolver = func(string) (jobs.Workspace, error) {
		return jobs.Workspace{}, disabledComputersError()
	}
	var openBackend jobs.BackendOpener = func(context.Context, jobs.Workspace) (computers.RuntimeBackend, error) {
		return nil, disabledComputersError()
	}
	if cfg.PacketComputers.IsEnabled() {
		lookupComputer := func(selector string) (computers.Computer, bool, error) {
			// /computers may update the durable registry after startup. Resolve
			// placement from a fresh snapshot so a newly registered target works
			// immediately and a removed/re-keyed target fails closed.
			computersDir, dirErr := config.ComputersDir()
			if dirErr != nil {
				return computers.Computer{}, false, dirErr
			}
			currentRegistry, loadErr := computers.Load(computersDir)
			if loadErr != nil {
				return computers.Computer{}, false, loadErr
			}
			computer, ok := currentRegistry.Get(selector)
			if !ok {
				computer, ok = currentRegistry.GetByID(selector)
			}
			return computer, ok, nil
		}
		resolveWorkspace = func(selector string) (jobs.Workspace, error) {
			computer, ok, lookupErr := lookupComputer(selector)
			if lookupErr != nil {
				return jobs.Workspace{}, fmt.Errorf("reload computer registry: %w", lookupErr)
			}
			if !ok {
				return jobs.Workspace{}, fmt.Errorf("computer %q is not registered", selector)
			}
			if computer.Kind != computers.KindSSH || !computer.Reachable() {
				return jobs.Workspace{}, fmt.Errorf("computer %q is not a reachable SSH computer", computer.Name)
			}
			return jobs.Workspace{
				ComputerID:   computer.ID,
				ComputerName: computer.Name,
				WorkingDir:   computer.ProjectRoots[0],
				Kind:         computer.Kind,
				Identity:     computerWorkspaceIdentity(computer),
				Policy:       computer.Policy,
			}, nil
		}
		openBackend = func(ctx context.Context, ws jobs.Workspace) (computers.RuntimeBackend, error) {
			computer, ok, lookupErr := lookupComputer(ws.ComputerID)
			if lookupErr != nil {
				return nil, fmt.Errorf("reload computer registry: %w", lookupErr)
			}
			if !ok {
				return nil, fmt.Errorf("computer id %q is no longer registered", ws.ComputerID)
			}
			if current := computerWorkspaceIdentity(computer); current != ws.Identity {
				return nil, fmt.Errorf("computer %q endpoint or registered root changed after this job was bound", computer.Name)
			}
			computer.ProjectRoots = []string{ws.WorkingDir}
			return computers.NewSSHBackend(ctx, computer)
		}
	}
	jobsMgr, recovered, err := jobs.NewManager(jobs.Config{
		Registry:     reg,
		Tools:        toolReg,
		MainSessions: sessions,
		SessionsDir:  sessionsDir,
		BackupsDir:   backupsDir,
		JobsDir:      jobsDir,
		WorktreesDir: worktreesDir,
		CostTracker:  tracker,
		PricingFor: func(slug, modelID string) (float64, float64) {
			if p, ok := reg.Get(slug); ok {
				return p.Pricing(modelID)
			}
			return 0, 0
		},
		SystemPromptFor: func(parentDepth int) string {
			return systemPrompt + "\n\nYou are a background sub-agent. Be concise and direct. Do not ask the user clarifying questions — make reasonable assumptions and act. Your final assistant message becomes your delivered result."
		},
		MaxConcurrent:    cfg.Behavior.BackgroundMaxConcurrent,
		MaxDepth:         cfg.Behavior.BackgroundMaxDepth,
		MaxTotal:         cfg.Behavior.BackgroundMaxTotal,
		TokenBudget:      cfg.Behavior.BackgroundTokenBudget,
		SugarCache:       packetcodeSugarCacheConfig(cfg),
		ConduitShadow:    packetcodeConduitShadowConfig(cfg),
		DefaultProvider:  cfg.Behavior.BackgroundDefaultProvider,
		DefaultModel:     cfg.Behavior.BackgroundDefaultModel,
		PermissionPolicy: permissionPolicy,
		Root:             root,
		Hooks:            hookRunner,
		DefaultWorkspace: defaultWorkspace,
		ResolveWorkspace: resolveWorkspace,
		OpenBackend:      openBackend,
	})
	if err != nil {
		return err
	}
	if recovered > 0 {
		fmt.Fprintf(os.Stderr, "packetcode: recovered %d orphan job(s) from previous run\n", recovered)
	}
	warnUnreadableJobRecords(os.Stderr, jobsMgr.UnreadableRecords())
	jobsMgr.SetSpawnToolFactory(func(parentJobID string, parentDepth int, parentAllowWrite bool) tools.Tool {
		return tools.NewBackgroundSpawnAgentTool(jobsMgr.AsToolsSpawner(), parentJobID, parentDepth, parentAllowWrite)
	})
	jobsMgr.SetCollectToolFactory(func(parentJobID string, parentDepth int) tools.Tool {
		return tools.NewCollectAgentResultsTool(jobsMgr.AsToolsSpawner(), parentJobID, parentDepth)
	})
	defer jobsMgr.Shutdown(5 * time.Second)

	toolReg.Register(tools.NewSpawnAgentTool(jobsMgr.AsToolsSpawner(), "", 0))
	toolReg.Register(tools.NewCollectAgentResultsTool(jobsMgr.AsToolsSpawner(), "", 0))

	workflowEngine := workflow.NewEngine(jobsMgr)
	workflowEngine.SetTokenBudget(cfg.Behavior.WorkflowTokenBudget)

	// Register MCP tools AFTER every native tool + spawn_agent so the
	// Agent's initial tool enumeration (on its first turn) sees them.
	for _, r := range mcp.RegisterTools(toolReg, mcpMgr.Clients()) {
		if r.Status == "skipped" {
			fmt.Fprintf(os.Stderr, "packetcode: mcp %s.%s skipped alias %s — %s\n", r.Server, r.Tool, r.Alias, r.Err)
		}
	}

	activeSystemPrompt := systemPrompt
	appBackups := bk
	computerID := ""
	workspaceIdentity := ""
	if activeComputer != nil {
		computerID = activeComputer.ID
		workspaceIdentity = computerWorkspaceIdentity(*activeComputer)
		activeSystemPrompt += fmt.Sprintf(
			"\n\n# Active Packet Computer\nAll foreground workspace file and shell tools operate on SSH computer %q inside %s. Paths are relative to that remote root. Background agents and workflows inherit this computer unless explicitly targeted from a local session; every remote job gets its own SSH connection, and write jobs require an isolated remote Git worktree. Remote code-intelligence tools, local hooks, and /undo are unavailable. Remote execution lasts only while this PacketCode process and its SSH connections remain alive; it does not reconnect after restart.",
			activeComputer.Name,
			root,
		)
		appBackups = nil
	}

	a, err := app.New(app.Deps{
		Config:            cfg,
		Registry:          reg,
		Tools:             toolReg,
		Sessions:          sessions,
		CostTracker:       tracker,
		Jobs:              jobsMgr,
		Workflow:          workflowEngine,
		Backups:           appBackups,
		MCP:               mcpMgr,
		PermissionPolicy:  permissionPolicy,
		WorkingDir:        root,
		RemoteWorkspace:   runtimeBackend != nil,
		ComputerID:        computerID,
		WorkspaceIdentity: workspaceIdentity,
		SystemPrompt:      activeSystemPrompt,
		Hooks:             hookRunner,
		Version:           welcomeVersion(),
		Factories:         factories,
		ResumeHydrate:     resumeID != "",
	})
	if err != nil {
		return err
	}

	// The App owns the uiApprover; pipe it into the jobs.Manager so
	// destructive sub-agent tool calls (when AllowWrite is on) prompt
	// the main user through the existing modal.
	if jobsMgr != nil {
		jobsMgr.SetApprover(a.Approver())
	}

	prog := tea.NewProgram(a) // inline rendering — native terminal scrollback, no mouse support
	// Let the App post async messages (jobs.Manager Subscribe callbacks)
	// into the Bubble Tea Update loop.
	a.SetSendFunc(func(m tea.Msg) { prog.Send(m) })
	if _, err := prog.Run(); err != nil {
		return err
	}
	return nil
}

func shouldRunSetup(cfg *config.Config, providerOverride string) bool {
	if cfg == nil {
		return true
	}
	if providerOverride != "" {
		return false
	}
	if cfg.Default.Provider == "" {
		return true
	}
	if config.IsKeylessProvider(cfg.Default.Provider) {
		return false
	}
	return providerRequiresAPIKey(cfg, cfg.Default.Provider) && cfg.GetProviderKey(cfg.Default.Provider) == ""
}

// unreadableJobWarningLimit caps how many job records the startup warning
// names individually. A jobs dir full of records this build cannot read
// must stay diagnosable without flooding the terminal on every launch.
const unreadableJobWarningLimit = 3

// warnUnreadableJobRecords reports job files that this build could not
// load — most often a record written by a newer packetcode. The files are
// untouched on disk, so the honest phrasing is "not loaded", not "lost",
// and this never blocks startup.
func warnUnreadableJobRecords(w io.Writer, records []jobs.UnreadableRecord) {
	if len(records) == 0 {
		return
	}
	fmt.Fprintf(
		w,
		"packetcode: %d job record(s) were not loaded; the files are still on disk, but these jobs are missing from this session\n",
		len(records),
	)
	shown := records
	if len(shown) > unreadableJobWarningLimit {
		shown = shown[:unreadableJobWarningLimit]
	}
	for _, rec := range shown {
		fmt.Fprintf(w, "packetcode:   %s — %s\n", rec.Path, rec.Reason)
	}
	if rest := len(records) - len(shown); rest > 0 {
		fmt.Fprintf(w, "packetcode:   ... and %d more not listed\n", rest)
	}
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

// ollamaHost returns the configured Ollama base URL. Env wins over config
// so a machine-specific daemon address can stay out of committed files.
// If unset, the Ollama provider uses its generic localhost default.
func ollamaHost(cfg *config.Config) string {
	// Resolution order: packetcode-specific override, then the standard Ollama
	// env var (so an existing `OLLAMA_HOST` in the user's shell just works),
	// then saved config, then the built-in localhost default. Local is always
	// the zero-config default; remote is opt-in via any of these.
	if host := os.Getenv("PACKETCODE_OLLAMA_HOST"); host != "" {
		return host
	}
	if host := os.Getenv("OLLAMA_HOST"); host != "" {
		return host
	}
	if pc, ok := cfg.Providers["ollama"]; ok && pc.Host != "" {
		return pc.Host
	}
	return ""
}

// computerWorkspaceIdentity freezes the endpoint and registered root a job
// was approved for without changing the user-facing registry ID. Removing and
// recreating a same-named computer with a different host key/endpoint/root can
// therefore never silently retarget a persisted job.
func computerWorkspaceIdentity(c computers.Computer) string {
	root := ""
	if len(c.ProjectRoots) > 0 {
		root = c.ProjectRoots[0]
	}
	material := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s\x00%s",
		c.Kind, c.SSHUser, c.SSHHost, c.SSHPort, c.SSHHostFingerprint, root)
	sum := sha256.Sum256([]byte(material))
	return fmt.Sprintf("pcws_sha256:%x", sum[:])
}

# PacketCode

**A local-first desktop command center for orchestrating AI software work.**

PacketCode is a native desktop IDE built on Tauri v2 that unifies Claude Code and OpenAI Codex CLI into a multi-pane development environment with mission planning, issue tracking, session management, GitHub integration, a persistent memory layer, and a deploy pipeline. It's not a wrapper — it's the cockpit.

---

## Features

### Missions & Mission Control *(NEW)*

Plan, track, and supervise AI-driven work at a higher level than individual sessions or tickets.

- **Planning surface** — create, browse, search, and edit missions from a dedicated view with status and priority filters
- **Issue linking** — assign issues to missions; status automatically rolls up from linked issues (draft → active → blocked → done)
- **Session launch** — start Claude or Codex sessions directly from a mission with context-rich prompts that include the objective, priority, and all linked issue details with acceptance criteria
- **Mission Control** — fleet-style supervision dashboard with a status strip showing mission counts by state, an attention queue that surfaces blocked and needs-human missions, and an active missions grid with progress tracking
- **Board integration** — mission badges on issue cards, mission filter on the kanban board, mission assignment from issue detail and issue creation views
- **Inline editing** — edit mission title, objective, status, and priority directly in the detail panel
- **Session auto-linking** — sessions launched from a mission are automatically linked back for traceability

### Multi-Pane AI Sessions

- Run multiple Claude Code or Codex CLI sessions side-by-side in resizable panes
- Full PTY terminal emulation via xterm.js
- Real-time status line monitoring for both Claude and Codex
- Model selection (Opus 4.6, Opus 4.5, Sonnet 4.5, Haiku 4.5) per session
- Session tab bar with live status labels
- Session history and status tracking

### Agent Profiles

- 5 built-in profiles: **Auto**, **Speed Runner**, **Thorough Reviewer**, **Security Auditor**, **Refactor Pro**
- Each profile injects a system prompt, sets a default model, and has a distinct icon/color
- Create custom profiles with your own system prompts
- Quick-switch between profiles from the toolbar

### Issue Tracker (Kanban Board)

- Full kanban board: To Do, In Progress, QA, Done, Blocked, Needs Human
- Priority levels, labels, epics, and acceptance criteria
- Drag-and-drop between columns
- Link issues to AI sessions
- **Spec2Tick**: paste a spec and let Claude parse it into structured tickets

### GitHub Integration

- Connect with a personal access token (backend memory only — not persisted across restarts)
- Browse repositories and open issues
- View full issue details with labels and metadata
- Import GitHub issues into local kanban tickets
- **Investigate with AI**: run Claude against your codebase to analyze any issue
- Create PRs directly from the app

### Memory Layer

- **File Map**: AI-powered codebase scan with 1-line summaries for key files
- **Session History**: summarize completed sessions, extract key decisions and modified files
- **Learned Patterns**: AI extracts recurring patterns (architecture, conventions, preferences, pitfalls) with confidence scores
- **Context Injection**: automatically prepend memory context to new sessions

### AI Tools

- **Vibe Architect** — interactive AI project scaffolding and architecture design
- **Insights Chat** — conversational codebase Q&A with full project context
- **Ideation Scanner** — AI-generated feature ideas, improvements, and suggestions
- **Code Quality** — on-demand AI code quality analysis

### MCP Server Management

- Manage Claude Code's MCP server configurations from within PacketCode
- View, add, edit, and delete servers across **global** (`~/.claude/settings.json`) and **project** (`.mcp.json`) scopes
- Server list grouped by scope with inline edit and delete controls
- Add/Edit modal with name, command, args, environment variables, and scope selector

### Project Scaffolding

- "New Project" wizard: template selection → configuration → result
- 6 built-in templates: Next.js, React+Vite, Python FastAPI, Rust CLI, Node Express, Blank
- Automatic tool availability detection (node, cargo, python)
- Directory picker and auto-switch to new project on success

### Deploy Pipeline

- One-click deploy with live terminal output via PTY
- Auto-detects configs from `packetcode.deploy.json`, `package.json` scripts, `vercel.json`, `netlify.toml`, and `Dockerfile`
- Custom deploy config creation and persistence
- Deploy run history with status tracking and duration
- Toolbar deploy button with Rocket icon

### UI/UX

- Custom frameless window with native title bar controls
- Dark theme with carefully designed color tokens
- File explorer panel
- Git branch display in toolbar
- Keyboard shortcuts for core navigation

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Desktop | Tauri v2 (Rust) |
| Frontend | React 19 + TypeScript |
| Bundler | Vite 6 |
| State | Zustand 5 |
| Styling | Tailwind CSS 3 |
| Terminal | xterm.js + portable-pty |
| Icons | lucide-react |
| Markdown | react-markdown + remark-gfm |
| HTTP | reqwest (Rust, for GitHub API) |

---

## Getting Started

### Prerequisites

- [Node.js](https://nodejs.org/) (v18+)
- [pnpm](https://pnpm.io/)
- [Rust](https://rustup.rs/) (stable toolchain)
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and on PATH

### Install

```bash
git clone https://github.com/packetloss404/PacketCode.git
cd PacketCode
pnpm install
```

### Development

```bash
pnpm tauri dev
```

### Build

```bash
pnpm tauri build
```

The built app will be in `src-tauri/target/release/bundle/`.

### Lint & Type Check

```bash
pnpm lint
pnpm build        # runs tsc && vite build
```

---

## Project Structure

```
PacketCode/
  src/
    App.tsx                        # Root component, view routing
    main.tsx                       # React entry point
    components/
      common/                      # Shared components (MarkdownRenderer)
      explorer/                    # File explorer panel
      issues/                      # Kanban board (IssueBoard, IssueCard, etc.)
      layout/                      # TitleBar, Toolbar, PaneContainer, StatusBar, SessionTabBar
      session/                     # TerminalPane, NewSessionModal, status bars
      ui/                          # Button, Dropdown, ErrorBoundary
      views/
        MissionsView.tsx           # Mission planning surface
        MissionControlView.tsx     # Fleet supervision dashboard
        GitHubView.tsx             # GitHub integration
        MemoryView.tsx             # Memory layer
        ToolsView.tsx              # AI tools hub
        VibeArchitectView.tsx      # Architecture design
        McpHubView.tsx             # MCP server management
        ScaffoldView.tsx           # Project scaffolding
        DeployView.tsx             # Deploy pipeline
    hooks/                         # useGitInfo, useStatusLine, useCodexStatusLine
    lib/
      tauri.ts                     # All Tauri invoke wrappers
      mission-colors.ts            # Mission/issue status color config
      time.ts                      # Relative time formatting
    stores/
      appStore.ts                  # View routing, app state
      layoutStore.ts               # Pane layout
      issueStore.ts                # Kanban issues
      missionStore.ts              # Mission state and operations
      mcpStore.ts                  # MCP server config
      scaffoldStore.ts             # Scaffolding state
      deployStore.ts               # Deploy pipeline state
    types/
      mission.ts                   # Mission, MissionStatus, MissionPriority
    modules/
      registry.ts                  # Module registration

  src-tauri/
    src/
      lib.rs                       # Tauri app builder, command registration
      commands/
        pty.rs                     # PTY session management
        git.rs                     # Git branch/status
        github.rs                  # GitHub API (reqwest)
        memory.rs                  # AI memory commands
        insights.rs                # Insights chat
        ideation.rs                # Ideation scanner
        spec.rs                    # Spec-to-tickets parser
        code_quality.rs            # Code quality analysis
        statusline.rs              # Claude/Codex status line polling
        fs.rs                      # Directory listing
        mcp.rs                     # MCP server config read/write/delete
        scaffold.rs                # Project template scaffolding
        deploy.rs                  # Deploy config read/write

  public/                          # Static assets
  docs/                            # Website
```

---

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+\` | Split — new session pane |
| `Ctrl+1-4` | Switch to pane 1–4 |
| `Ctrl+Shift+1` | Claude sessions view |
| `Ctrl+Shift+2` | Codex sessions view |
| `Ctrl+Shift+3` | Issues board |
| `Ctrl+Shift+4` | History view |
| `Ctrl+Shift+5` | Tools / Settings |
| `Ctrl+Shift+6` | Vibe Architect |
| `Ctrl+Enter` | Start session (in modal) |

---

## License

MIT

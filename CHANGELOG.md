# Changelog

All notable changes to PacketCode are documented in this file.

## [0.3.0] - 2026-03-16

### Added

#### Missions System
- Mission domain model with types, Zustand store, and localStorage persistence
- `missionStore` with CRUD operations, issue/session linking, and status rollup computation
- `missionId` field on issues with backward-compatible migration for existing data
- Dedicated **Missions** view: master-detail layout with mission list, search, status filter, inline create form, and full detail panel
- Inline editing of mission title, objective, status, and priority
- Mission status rollup computed from linked issue states (needs_human > blocked > done > active > draft)
- **Mission Control** supervision view: status strip with counts, attention queue for blocked/needs_human missions, active missions section, collapsible all-missions groups
- Mission Control toolbar button with live attention badge (amber count of blocked + needs_human)
- Launch Claude or Codex sessions from mission detail with context-rich prompts (mission objective + linked issues with descriptions and acceptance criteria)
- Auto-link launched sessions to the originating mission
- Mission badges on issue cards (green Target icon + truncated title)
- Mission assignment in issue detail modal (assign/remove dropdown)
- Mission filter dropdown on issue board (all / unassigned / specific mission)
- Mission selector when creating new issues
- Delete confirmation dialog for missions

#### Shared Utilities
- `src/lib/time.ts` — shared `relativeTime()` function (consolidated from 3 duplicate implementations)
- `src/lib/mission-colors.ts` — shared mission status, priority, and issue status color/label constants

### Fixed
- `useMemo` dependency array in CostDashboardView (pre-existing lint error)
- MissionControl → MissionsView navigation now syncs selected mission via store
- Consistent naming: "New Mission" / "Create Mission" labels, capitalized priorities, proper issue status labels

### Changed
- `CoreView` type expanded with `"missions"` and `"mission_control"`
- Toolbar gains Missions tab (top-level) and Control button (right section)
- Issue interface gains `missionId: string | null` with migration
- `addIssue` signature makes `missionId` optional for backward compatibility

---

## [0.2.0] - 2026-02-27

### Added

#### MCP Server Integration Hub
- View, add, edit, and delete MCP server configurations
- Global scope (`~/.claude/settings.json`) and project scope (`.mcp.json`)
- Server list grouped by scope with toggle, edit, and delete controls
- Add/Edit modal with name, command, args, environment variables, and scope selector
- Registered as a module (category: integration, icon: Plug, enabled by default)

#### Project Template Scaffolding
- "New Project" wizard with 3-step flow: template selection, configuration, result
- 6 built-in templates: Next.js, React+Vite, Python FastAPI, Rust CLI, Node Express, Blank
- Automatic tool availability detection (node, cargo, python)
- Directory picker for parent folder selection
- Auto-switches `projectPath` to newly created project on success
- "New Project" button on Welcome Screen
- Registered as a module (category: utility, icon: FolderPlus, enabled by default)

#### Deploy Pipeline
- Core deploy view with toolbar button (Rocket icon)
- Auto-detects deploy configs from `packetcode.deploy.json`, `package.json` scripts, `vercel.json`, `netlify.toml`, and `Dockerfile`
- Custom deploy config creation and persistence in `packetcode.deploy.json`
- Live terminal output via PTY for deploy commands
- Deploy run history with status tracking (running, success, failed) and duration
- Config cards with one-click deploy and history sidebar

#### Rust Backend
- `mcp.rs` — 3 commands: `read_mcp_servers`, `write_mcp_server`, `delete_mcp_server`
- `scaffold.rs` — 2 commands: `scaffold_project`, `check_scaffold_tools`
- `deploy.rs` — 2 commands: `read_deploy_config`, `create_deploy_config`

### Changed
- Added `"deploy"` to `CoreView` union type
- Updated Toolbar with Deploy button in right section
- Welcome Screen now shows "New Project" button when scaffold module is enabled
- Module registry expanded from 2 to 4 modules

---

## [0.1.0] - 2026-02-22

### Added

#### Core IDE
- Tauri v2 desktop application with custom dark theme
- Multi-pane session layout with resizable panels
- PTY-based terminal emulation using xterm.js and portable-pty
- Custom window title bar with minimize/maximize/close controls
- Keyboard shortcuts for pane switching, view navigation, and session splitting
- File explorer panel with directory tree browsing
- Project folder selector with persistent path storage
- Git branch display in toolbar and status bar

#### AI Sessions
- Claude Code CLI integration with full PTY terminal
- OpenAI Codex CLI integration with full PTY terminal
- New Session modal with CLI toggle, model selector, and prompt input
- Model selection: Opus 4.6, Opus 4.5, Sonnet 4.5, Haiku 4.5
- Real-time status line monitoring for Claude and Codex sessions
- Session tab bar for switching between active sessions
- Session history view

#### Agent Profiles
- 5 built-in agent profiles: Auto (Optimized), Speed Runner, Thorough Reviewer, Security Auditor, Refactor Pro
- Custom profile creation with name, description, icon, color, system prompt, and default model
- Profile selector in New Session modal — auto-fills model and prepends system prompt
- Quick-switch profile dropdown in toolbar
- Profile management (create/edit/delete) in Tools > Settings

#### Issue Tracker
- Kanban board with 6 columns: To Do, In Progress, QA, Done, Blocked, Needs Human
- Issue creation with title, description, priority, labels, epic, and acceptance criteria
- Drag-and-drop between columns
- Issue detail view with full metadata
- Session linking — associate issues with AI sessions
- Configurable ticket prefix and custom epics/labels
- Spec2Tick: AI-powered spec parsing into structured tickets

#### GitHub Integration
- Personal access token authentication
- Repository browser (30 most recently updated repos)
- Open issues list with search and label filtering
- Full issue detail view with metadata
- "Import to Board" — convert GitHub issues to local kanban tickets
- "Investigate with AI" — Claude analyzes issue against codebase
- Pull request creation modal (title, body, head/base branch)

#### Memory Layer
- File Map: AI codebase scan generating 1-line file summaries
- Session History: AI-powered session summarization with key decisions and modified files
- Learned Patterns: AI-extracted recurring patterns with category (architecture, convention, preference, pitfall) and confidence scores
- Memory context injection toggle in New Session modal
- Pattern and summary management (view, delete, refresh)
- Persistent storage in localStorage

#### AI Tools
- Vibe Architect: interactive AI project scaffolding and architecture design
- Insights Chat: conversational codebase Q&A with Claude
- Ideation Scanner: AI-generated feature ideas, improvements, and suggestions
- Code Quality: on-demand AI code quality analysis

#### UI/UX
- Welcome screen with quick-start actions
- Tools dropdown menu in toolbar with all features
- Status bar with session info and Claude/Codex status lines
- Error boundaries for graceful failure handling
- Dark theme with custom color tokens (bg-primary, accent-green, etc.)
- Responsive layout with collapsible panels

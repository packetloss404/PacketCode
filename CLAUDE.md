# PacketCode — CLAUDE.md

## What is this project?

PacketCode is a custom desktop IDE built on Tauri v2 that wraps Claude Code and OpenAI Codex CLI into a unified multi-pane development environment. It provides session management, an issue tracker, AI-powered tools, GitHub integration, a memory layer, agent profiles, MCP server management, project scaffolding, and a deploy pipeline — all in a single native app.

## Tech Stack

- **Desktop framework:** Tauri v2 (Rust backend + webview frontend)
- **Frontend:** React 19 + TypeScript + Vite
- **State management:** Zustand (persisted to localStorage where needed)
- **Styling:** Tailwind CSS with custom dark theme tokens (`bg-bg-primary`, `text-text-secondary`, `accent-green`, etc.)
- **Icons:** lucide-react
- **Terminal:** xterm.js with PTY backend (portable-pty)
- **Markdown:** react-markdown + remark-gfm

## Project Structure

```
src/
  App.tsx                          # Root component, view routing
  main.tsx                         # React entry point
  components/
    common/MarkdownRenderer.tsx    # Shared markdown rendering
    explorer/FileExplorer.tsx      # Floating file tree panel
    issues/                        # Kanban issue board (IssueBoard, IssueCard, etc.)
    layout/                        # TitleBar, Toolbar, PaneContainer, StatusBar, SessionTabBar
    quality/CodeQualityModal.tsx   # Code quality analysis modal
    session/                       # TerminalPane, NewSessionModal, ClaudeStatusBar, CodexStatusBar
    ui/                            # Button, Dropdown, ErrorBoundary
    views/                         # GitHubView, MemoryView, ToolsView, VibeArchitectView, McpHubView, ScaffoldView, DeployView, etc.
  hooks/                           # useGitInfo, useStatusLine, useCodexStatusLine, shared poller hooks
  lib/
    tauri.ts                       # All Tauri invoke wrappers
  stores/                          # Zustand stores (appStore, layoutStore, issueStore, mcpStore, scaffoldStore, deployStore, etc.)
  types/                           # TypeScript interfaces

src-tauri/
  src/
    lib.rs                         # Tauri app builder, command registration
    commands/
      pty.rs                       # PTY session management
      git.rs                       # Git branch/status
      github.rs                    # GitHub API (reqwest)
      memory.rs                    # AI memory commands
      insights.rs                  # Insights chat
      ideation.rs                  # Ideation scanner
      spec.rs                      # Spec-to-tickets parser
      code_quality.rs              # Code quality analysis
      statusline.rs                # Claude/Codex status line polling
      fs.rs                        # Directory listing
      mcp.rs                       # MCP server config read/write/delete
      scaffold.rs                  # Project template scaffolding
      deploy.rs                    # Deploy config read/write
      mod.rs                       # Module exports
    claude/                        # Claude CLI interaction helpers
```

## Key Conventions

- **Views** are routed via `AppView` union type in `appStore.ts` — add new views there
- **Tauri commands** go in `src-tauri/src/commands/`, register in `lib.rs`, add TS bindings in `src/lib/tauri.ts`
- **Stores** use Zustand with `create<StoreInterface>()` pattern; persist to `packetcode:*` localStorage keys
- **Session architecture** is PTY-only (`create_pty_session`, `write_pty`, `resize_pty`, `kill_pty`); legacy JSONL session stack is removed
- **GitHub auth** uses backend memory only; token is not persisted across app restarts
- **Theme tokens** — never use raw Tailwind colors; use `bg-bg-primary`, `text-accent-green`, etc.
- **Font sizes** — primarily `text-xs` (12px) and `text-[11px]`/`text-[10px]` for compact UI
- **Icons** — use lucide-react, typically `size={12}` in toolbars, `size={14}` in headers
- **CLI commands** — PTY session startup resolves `.cmd` wrappers on Windows (e.g., `claude.cmd`, `codex.cmd`)
- **Modules** — MCP Hub (integration), Scaffold (utility), Vibe Architect (ai), Ideation (analysis); registered in `src/modules/registry.ts`
- **Deploy view** — core view (`"deploy"` in `CoreView`), not a module; toolbar button with Rocket icon

## Build & Run

```bash
# Prerequisites: Node.js, pnpm, Rust toolchain

# Install dependencies
pnpm install

# Development
pnpm tauri dev

# Build
pnpm tauri build

# Lint + type check build
pnpm lint
pnpm build

# Rust check only
cd src-tauri && cargo check
```

## Environment Notes (Windows)

The Rust toolchain path must be on PATH for Tauri builds:
```bash
export PATH="/c/Users/ianwalmsley/.rustup/toolchains/stable-x86_64-pc-windows-msvc/bin:$PATH"
```

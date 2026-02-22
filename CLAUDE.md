# PacketCode — CLAUDE.md

## What is this project?

PacketCode is a custom desktop IDE built on Tauri v2 that wraps Claude Code and OpenAI Codex CLI into a unified multi-pane development environment. It provides session management, an issue tracker, AI-powered tools, GitHub integration, a memory layer, and agent profiles — all in a single native app.

## Tech Stack

- **Desktop framework:** Tauri v2 (Rust backend + webview frontend)
- **Frontend:** React 19 + TypeScript + Vite
- **State management:** Zustand (persisted to localStorage where needed)
- **Styling:** Tailwind CSS with custom dark theme tokens (`bg-bg-primary`, `text-text-secondary`, `accent-green`, etc.)
- **Icons:** lucide-react
- **Terminal:** xterm.js with PTY backend (portable-pty)
- **Markdown:** react-markdown + remark-gfm + react-syntax-highlighter

## Project Structure

```
src/
  App.tsx                          # Root component, view routing
  main.tsx                         # React entry point
  components/
    explorer/FileExplorer.tsx      # Floating file tree panel
    issues/                        # Kanban issue board (IssueBoard, IssueCard, etc.)
    layout/                        # TitleBar, Toolbar, PaneContainer, StatusBar, SessionTabBar
    quality/CodeQualityModal.tsx   # Code quality analysis modal
    session/                       # TerminalPane, NewSessionModal, ClaudeStatusBar, CodexStatusBar
    ui/                            # Button, Dropdown, ErrorBoundary
    views/                         # GitHubView, MemoryView, ToolsView, VibeArchitectView, etc.
  hooks/                           # useGitInfo, useStatusLine, useCodexStatusLine, useSession
  lib/
    tauri.ts                       # All Tauri invoke wrappers
    messageParser.ts               # JSONL message parsing
  stores/                          # Zustand stores (appStore, layoutStore, issueStore, etc.)
  types/                           # TypeScript interfaces

src-tauri/
  src/
    lib.rs                         # Tauri app builder, command registration
    commands/
      pty.rs                       # PTY session management
      session.rs                   # Legacy JSONL sessions
      git.rs                       # Git branch/status
      github.rs                    # GitHub API (reqwest)
      memory.rs                    # AI memory commands
      insights.rs                  # Insights chat
      ideation.rs                  # Ideation scanner
      spec.rs                      # Spec-to-tickets parser
      code_quality.rs              # Code quality analysis
      statusline.rs                # Claude/Codex status line polling
      fs.rs                        # Directory listing
      mod.rs                       # Module exports
    claude/                        # Claude CLI interaction helpers
    session/                       # Session manager
```

## Key Conventions

- **Views** are routed via `AppView` union type in `appStore.ts` — add new views there
- **Tauri commands** go in `src-tauri/src/commands/`, register in `lib.rs`, add TS bindings in `src/lib/tauri.ts`
- **Stores** use Zustand with `create<StoreInterface>()` pattern; persist to `packetcode:*` localStorage keys
- **Theme tokens** — never use raw Tailwind colors; use `bg-bg-primary`, `text-accent-green`, etc.
- **Font sizes** — primarily `text-xs` (12px) and `text-[11px]`/`text-[10px]` for compact UI
- **Icons** — use lucide-react, typically `size={12}` in toolbars, `size={14}` in headers
- **CLI commands** — Rust commands that invoke Claude use `claude.cmd` on Windows, `claude` on other platforms; always set `creation_flags(0x08000000)` on Windows to hide console window

## Build & Run

```bash
# Prerequisites: Node.js, pnpm, Rust toolchain

# Install dependencies
pnpm install

# Development
pnpm tauri dev

# Build
pnpm tauri build

# Frontend only (type check + vite build)
pnpm build

# Rust check only
cd src-tauri && cargo check
```

## Environment Notes (Windows)

The Rust toolchain path must be on PATH for Tauri builds:
```bash
export PATH="/c/Users/ianwalmsley/.rustup/toolchains/stable-x86_64-pc-windows-msvc/bin:$PATH"
```

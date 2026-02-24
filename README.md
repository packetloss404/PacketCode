# PacketCode

A custom desktop IDE for AI-powered development, built on [Tauri v2](https://v2.tauri.app/). PacketCode wraps **Claude Code** and **OpenAI Codex CLI** into a unified multi-pane workspace with session management, an issue tracker, GitHub integration, a persistent memory layer, and configurable agent profiles.

## Features

### Multi-Pane AI Sessions
- Run multiple Claude Code or Codex CLI sessions side-by-side in resizable panes
- Full PTY terminal emulation via xterm.js
- Real-time status line monitoring for both Claude and Codex
- Model selection (Opus 4.6, Opus 4.5, Sonnet 4.5, Haiku 4.5) per session
- Session tab bar with live status labels
- Session history and status tracking

### Agent Profiles
- 5 built-in personalities: **Auto**, **Speed Runner**, **Thorough Reviewer**, **Security Auditor**, **Refactor Pro**
- Each profile injects a system prompt, sets a default model, and has a distinct icon/color
- Create custom profiles with your own system prompts
- Quick-switch between profiles from the toolbar

### Issue Tracker (Kanban Board)
- Full kanban board with columns: To Do, In Progress, QA, Done, Blocked, Needs Human
- Priority levels, labels, epics, and acceptance criteria
- Drag-and-drop between columns
- Link issues to AI sessions
- **Spec2Tick**: paste a project spec and let Claude parse it into structured tickets

### GitHub Integration
- Connect with a personal access token
- Token is held in backend memory only (not persisted across app restarts)
- Browse repositories and open issues
- View full issue details with labels and metadata
- **Import to Board**: convert GitHub issues into local kanban tickets
- **Investigate with AI**: run Claude against your codebase to analyze any issue
- **Create PRs** directly from the app

### Memory Layer
- **File Map**: AI-powered codebase scan that generates 1-line summaries for key files
- **Session History**: summarize completed sessions, extract key decisions and modified files
- **Learned Patterns**: AI extracts recurring patterns (architecture, conventions, preferences, pitfalls) with confidence scores
- **Context Injection**: automatically prepend memory context to new sessions

### AI Tools
- **Vibe Architect**: interactive AI project scaffolding and architecture design
- **Insights Chat**: conversational codebase Q&A with full project context
- **Ideation Scanner**: AI-generated feature ideas, improvements, and suggestions
- **Code Quality**: on-demand AI code quality analysis

### UI/UX
- Custom frameless window with native title bar controls
- Dark theme with carefully designed color tokens
- File explorer panel
- Git branch display in toolbar
- Keyboard shortcuts for everything

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

## Getting Started

### Prerequisites

- [Node.js](https://nodejs.org/) (v18+)
- [pnpm](https://pnpm.io/)
- [Rust](https://rustup.rs/) (stable toolchain)
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and on PATH

### Install

```bash
git clone https://github.com/yourusername/PacketCode.git
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

## Project Structure

```
PacketCode/
  src/                    # React frontend
    components/           # UI components (layout, session, views, issues, etc.)
    stores/               # Zustand state stores
    types/                # TypeScript type definitions
    hooks/                # React hooks
    lib/                  # Tauri bindings and utilities
  src-tauri/              # Rust backend
    src/
      commands/           # Tauri command handlers
      claude/             # Claude CLI helpers
  public/                 # Static assets
  docs/                   # Website (index.html)
```

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+\` | Split — new session pane |
| `Ctrl+1-4` | Switch to pane 1-4 |
| `Ctrl+Shift+1` | Claude sessions view |
| `Ctrl+Shift+2` | Codex sessions view |
| `Ctrl+Shift+3` | Issues board |
| `Ctrl+Shift+4` | History view |
| `Ctrl+Shift+5` | Tools / Settings |
| `Ctrl+Shift+6` | Vibe Architect |
| `Ctrl+Enter` | Start session (in modal) |

## License

MIT

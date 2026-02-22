# Changelog

All notable changes to PacketCode are documented in this file.

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

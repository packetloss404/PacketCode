# PacketCode — Vibe Coder Review

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

---

## 10 Feature Requests

### 1. Streaming Responses in Insights Chat

`ask_insights` blocks until Claude finishes the entire response, then dumps it all at once. The backend uses `run_claude()` which is a single blocking call — it needs a streaming mode with chunked output piped back via Tauri events. Every other AI chat UI streams tokens in real-time; this should too.

### 2. Inline File Preview / "Peek" from the Terminal

When Claude mentions a file in the terminal output, clicking it should show the file contents in a split panel without leaving the session view. The `FileExplorer` exists but it's a separate floating panel with no connection to what the AI is talking about. Clickable file references with inline preview would be a huge workflow improvement.

### 3. Session Persistence / Reconnection

If the app closes or crashes, all PTY sessions are gone forever. The `PtyManager` stores sessions in an in-memory `HashMap` — nothing survives a restart. Sessions should either persist and reconnect, or at minimum the scrollback history should be saved so it can be reviewed after reopening.

### 4. Prompt Templates / Snippet Library

Vibe coders have go-to prompts: "refactor this for readability", "add tests for this module", "explain this codebase". The `NewSessionModal` has an initial prompt field, but there's no way to save and reuse prompts. A library of saved prompt templates with one-click injection into any session would accelerate common workflows.

### 5. Multi-Model A/B Comparison

The app runs both Claude and Codex — it should let you send the same prompt to both simultaneously and compare outputs side-by-side. The pane container already supports multi-pane layouts, so a "dual fire" mode where a single prompt dispatches to both engines would be a natural fit.

### 6. GitHub PR Review / Diff Viewer

GitHub integration can list issues and create PRs, but there's no way to review PRs. There's no `github_list_prs` or `github_get_pr_diff` in the backend. Pulling up a PR, seeing the diff with syntax highlighting, and asking Claude to review it inline would close a major workflow gap.

### 7. Voice Input for Prompts

Vibe coding is about flow state. Holding a key and speaking a prompt instead of typing it (via Web Speech API or Whisper) would keep you in the zone. Voice-to-text directly into the terminal prompt would be a killer differentiator.

### 8. Cost Tracking Dashboard

The `ClaudeStatusBar` shows current session cost, but there's no historical view — cost per day, per week, per project, cumulative spend. The data is available in status line polling but nobody's aggregating it. A chart showing spend over time would help users manage API credit burn rate.

### 9. Git Operations Beyond Read-Only

`git.rs` only has `get_git_branch` and `get_git_status`. No commit, push, pull, stash, or branch create. The terminal can do it all, but UI buttons for quick commit, push, and branch create right in the toolbar next to the branch name would save constant context-switching.

### 10. Plugin / Extension System

The module system (`moduleStore`) exists but it's hardcoded to Vibe Architect and Ideation Scanner. An extensible plugin system — drop a JS/TS file into a plugins folder to register new views, commands, or toolbar buttons — would let the community build on the platform.

---

## 10 Things That Need Improvement (UI / Process)

### 1. Status Bars Are Information Overload at Tiny Font Sizes

`ClaudeStatusBar` and `CodexStatusBar` cram model name, context %, git branch, cost, duration, rate limits, and CLI version into a single bar at `text-[11px]`. On a 1080p monitor it's a wall of nearly unreadable text. Prioritize the 2–3 most important metrics and hide the rest behind a hover or expandable section.

### 2. No Visual Feedback When AI Commands Are Running

Clicking "Scan Codebase" in Memory or "Analyze" in Code Quality gives no feedback. The backend runs `run_claude()` which can take 30+ seconds with no progress indicator, no spinner, and no "thinking..." state. Users are left wondering if the click registered.

### 3. The File Explorer Is a Detached Floating Panel

`FileExplorer.tsx` is draggable and floating, which sounds cool but in practice it covers the terminal panes. It should be a collapsible sidebar docked to the left, like every other IDE. It also has no integration — you can't drag a file into a prompt or open it in an editor.

### 4. Issue Board Drag-and-Drop Feels Janky

The Kanban board uses raw `onDragStart`/`onDragOver`/`onDrop` HTML5 drag events. There's no drag preview, no smooth animation, no visual placeholder showing where the card will land. It needs a proper DnD library like `@dnd-kit` or `react-beautiful-dnd` to match the polish of Trello or Linear.

### 5. GitHub Token Vanishes on Restart

`GitHubAuthState` is stored in a `RwLock<Option<String>>` in memory. Every app reopen requires re-pasting the token. This is maddening. The token should be encrypted and stored in the OS keychain via `tauri-plugin-stronghold` or at minimum persisted to disk.

### 6. No Dark/Light Theme Toggle or Theme Customization

The app is hardcoded to a dark theme with custom tokens (`bg-bg-primary`, `text-accent-green`, etc.). There's zero customization — no accent color picker, no light mode for daytime coding. The theme token system is a good foundation but it goes nowhere.

### 7. The Tools/Settings View Is a Dumping Ground

`ToolsView.tsx` has project info, ticket prefix config, epic management, label management, spec import, agent profiles, AND modules all in one scrolling page. It needs proper sections or a sidebar navigation within Settings. Finding anything specific requires scrolling through everything.

### 8. Insights Chat Has No Code Context Awareness

When asking a question in Insights, you have to manually describe which file or function you're asking about. The chat can't see what's in your terminal, can't reference open files, and can't pull in relevant code. The backend sends the message + conversation history to Claude CLI with the working directory set, but there's no file content injection.

### 9. New Session Modal Flow Is Clunky

`NewSessionModal` requires picking a profile, optionally setting a model, toggling memory, typing a prompt, then clicking create. But 90% of the time you just want to open a new Claude session with your current profile and start typing. It should be one click to create with defaults, with the modal being optional for customization.

### 10. No Keyboard-First Navigation

For a developer tool, keyboard support is surprisingly sparse. There are view-switching shortcuts (`Ctrl+Shift+1–6`) and a split-pane shortcut (`Ctrl+\`), but no shortcut to open a new session, no `Cmd+K` / `Ctrl+K` command palette, no fuzzy finder for files, no quick-switch between issues. The entire app should be usable without a mouse.

---

## Summary

PacketCode nails the core concept: a native multi-pane IDE wrapping Claude Code and Codex with project management baked in. The feature requests above would push it from "impressive side project" to "daily driver." The UI improvements would sand off the rough edges that break flow state — which is the whole point of vibe coding.

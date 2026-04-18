# packetcode

A keyboard-first, multi-provider AI coding agent for the terminal.

> Status: pre-alpha. Single-binary, no runtime dependencies, fully tested backend, runnable TUI.

packetcode lives in your terminal and talks to **five LLM providers** behind a single unified interface. It can read, edit, patch, search, and execute against your project — every destructive action gated by a one-keystroke approval prompt.

---

## Why packetcode

Other AI coding agents force a tradeoff:

- **Claude Code** is excellent but locks you into Anthropic with no escape hatch.
- **OpenCode** supports multiple providers but ships with mouse support and UI complexity that slows down power users.
- Neither makes it trivial to bring your own local models via Ollama, or fan out to the hundreds of models on OpenRouter.

packetcode combines the agentic capabilities of Claude Code with the multi-provider flexibility of OpenCode, stripped down to a pure keyboard-driven terminal experience with **zero mouse dependencies**.

---

## Features

### Multi-provider LLM system

| Provider           | Access            | Notes                                                   |
| ------------------ | ----------------- | ------------------------------------------------------- |
| **OpenAI**         | API key           | GPT-4.1, o3, o4-mini, gpt-4.1-mini/nano                |
| **Google Gemini**  | API key           | Gemini 2.5 Pro, 2.5 Flash, 2.0 Flash                   |
| **MiniMax**        | API key           | MiniMax-Text-01 (1M ctx), abab variants                |
| **OpenRouter**     | API key           | Hundreds of models, dynamic per-model pricing          |
| **Ollama**         | Local (no key)    | Auto-detects `localhost:11434`, any pulled model       |

Every provider implements the same `Provider` interface — model listing, key validation, streaming chat completion, tool/function calling, pricing. Switching providers mid-conversation preserves your history.

### Agentic tool loop

Six tools are wired in by default. Filesystem mutations and shell commands require your approval:

| Tool               | Approval | Purpose                                              |
| ------------------ | :------: | ---------------------------------------------------- |
| `read_file`        |    —     | Read a file (optional line range)                    |
| `search_codebase`  |    —     | ripgrep-powered codebase search (with Go fallback)   |
| `list_directory`   |    —     | Tree view of a directory, ignoring junk dirs         |
| `write_file`       |    Y     | Write/overwrite a file (auto-backed-up for `/undo`)  |
| `patch_file`       |    Y     | Apply unique search/replace patches; returns diff    |
| `execute_command`  |    Y     | Run a shell command with timeout + output capture    |

The agent loops `LLM → tool → LLM → tool → …` until the model has nothing more to call. Parallel tool calls in a single response are dispatched in order. Rejections are fed back to the LLM as tool-role messages so it can adapt.

### Background agents

Spawn independent agent loops that run in parallel with the foreground conversation. Each job is a fully isolated mini-agent: its own session, cost tally, backup stack, and provider/model selection. Results stream back into the main conversation as a system message plus an auto-injected context message on the next turn.

| Command                                                         | Effect                                                                           |
| --------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| `/spawn [--provider <slug>] [--model <id>] [--write] <prompt>`  | Queue a background agent. Echoes `[job:<id> queued — prov/model] <prompt>`.    |
| `/jobs`                                                         | List active + recent jobs in an inline ASCII table.                              |
| `/jobs <id>`                                                    | Open the job's transcript modal. `Esc` / `q` to close, `j/k` to scroll.         |
| `/cancel <id>`                                                  | Cancel a single running job.                                                     |
| `/cancel all`                                                   | Cancel every running / queued job.                                               |

The top bar shows `⚙ N jobs` in cyan while jobs are active — it's the last segment to drop on narrow terminals.

**Approval policy.** Background jobs default to a **read-only** tool subset (`read_file`, `search_codebase`, `list_directory`, plus `spawn_agent` up to `background_max_depth`). Pass `--write` to enable `write_file` / `patch_file` / `execute_command`; destructive calls still route through the main session's approval prompt, annotated with `[job:<id>]` so you can see which background agent is asking.

**Agent-initiated parallelism.** The main agent can call the `spawn_agent` tool autonomously. It's approval-gated (trust mode auto-approves) and supports `wait=true` for a synchronous fan-out/fan-in pattern.

Caps live under `[behavior]` in `~/.packetcode/config.toml`:

```toml
[behavior]
background_max_concurrent    = 4         # at most N workers running at once
background_max_depth         = 2         # main → spawn → spawn
background_max_total         = 32        # lifetime spawns per app run
background_default_provider  = ""        # e.g. "gemini" for cheap scouts
background_default_model     = ""        # e.g. "gemini-2.5-flash"
```

Job metadata persists to `~/.packetcode/jobs/<id>.json`; orphans from a crash surface on next launch as `cancelled (previous app exit)`.

### Slash commands

Thirteen verbs are wired into the input bar. Each handler is a thin adapter over the existing backend APIs (`provider.Registry`, `session.Manager`, `session.BackupManager`, `cost.Tracker`, `agent.ContextManager`, `uiApprover`, `mcp.Manager`) and appends its output as a monospace system message inside the conversation, reusing the ASCII-table aesthetic `/jobs` introduced.

| Command                                                         | Effect                                                                               |
| --------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `/spawn [--provider <slug>] [--model <id>] [--write] <prompt>`  | Queue a background agent.                                                            |
| `/jobs`                                                         | List active + recent jobs.                                                           |
| `/jobs <id>`                                                    | Open a job's transcript modal.                                                       |
| `/cancel <id\|all>`                                             | Cancel a single job, or every running / queued job.                                  |
| `/provider`                                                     | List registered providers; active one marked `*`.                                    |
| `/provider <slug>`                                              | Switch active provider. Uses the configured default model, falling back to the provider's first `ListModels` entry. |
| `/model`                                                        | List models available on the active provider, with context window and pricing.      |
| `/model <id>`                                                   | Switch the active model on the current provider.                                     |
| `/sessions`                                                     | List the 20 most recent sessions; current one marked `*`.                            |
| `/sessions resume <id>`                                         | Resume a session by full ID or a unique 8-char prefix.                               |
| `/sessions delete <id> --yes`                                   | Permanently delete a session. Requires `--yes`.                                      |
| `/undo`                                                         | Pop the active session's backup stack and restore the most recent snapshot.         |
| `/compact`                                                      | Summarise older messages via the active provider, keeping the last 10 verbatim.     |
| `/compact --keep <N>`                                           | Keep the last `<N>` messages verbatim instead.                                       |
| `/cost`                                                         | Show the cumulative USD total and the top-5 session breakdown.                       |
| `/cost reset --yes`                                             | Clear the cost tally and reset the start time. Requires `--yes`.                     |
| `/trust`                                                        | Report whether trust mode is on or off.                                              |
| `/trust on` / `/trust off`                                      | Enable or disable auto-approval of destructive tools for the current session.        |
| `/help`                                                         | Render every keybinding group and every slash command as a sectioned system message. |
| `/clear`                                                        | Clear the transcript pane. Identical to `Ctrl+L`; the on-disk session is untouched.  |
| `/mcp`                                                          | List configured MCP servers with state, tool count, pid.                             |
| `/mcp logs <name>`                                              | Tail the last 50 lines of `~/.packetcode/mcp-<name>.log`.                            |

Output from every verb renders as a system message in the conversation — the same look as `/jobs`. Destructive verbs (`/sessions delete` and `/cost reset`) require an explicit `--yes` flag rather than popping a confirmation modal; everything else executes immediately. `/compact` blocks the UI while it runs (one LLM round trip, 120 s timeout). `/help` lists every keybinding group and every slash command in one place.

### Keyboard

Two global shortcuts open filter-as-you-type picker modals that mirror the `/provider` and `/model` slash commands:

| Shortcut   | Effect                                                                                   |
| ---------- | ---------------------------------------------------------------------------------------- |
| `Ctrl+P`   | Open the provider picker. Shows all registered providers with brand dots and key status. |
| `Ctrl+M`   | Open the model picker for the active provider. Loads async, cached after first open.     |

Inside a picker modal:

| Key                              | Effect                                   |
| -------------------------------- | ---------------------------------------- |
| `↑` / `↓` / `Ctrl+N` / `Ctrl+P` / `Ctrl+J` / `Ctrl+K` | Move cursor                 |
| `PgUp` / `PgDn`                  | Move by a half page                      |
| `Home` / `End`                   | Jump to first / last                     |
| `Enter`                          | Select                                   |
| `Esc`                            | Close without changing anything          |
| any printable rune               | Append to the filter (case-insensitive)  |
| `Ctrl+U`                         | Clear the filter                         |
| `r`                              | Retry the loader (error state only)      |

#### Slash-command autocomplete

Typing `/` as the first character of the input buffer opens a small
filter-as-you-type popup above the input bar listing every slash command.
Additional keystrokes narrow the list (prefix matches beat substring
matches, ties broken alphabetically); the popup silently disappears once
you type whitespace past the verb so long multi-word prompts never fight
the popup for the input line.

| Key                              | Effect                                                     |
| -------------------------------- | ---------------------------------------------------------- |
| `/`                              | Open the popup (when buffer is empty or starts with `/`)   |
| `↑` / `↓` / `Ctrl+N` / `Ctrl+P` / `Ctrl+J` / `Ctrl+K` | Move cursor              |
| `Tab`                            | Accept the highlighted suggestion — buffer becomes `/<verb> ` |
| `Enter`                          | Accept if the buffer is still a bare verb; otherwise submit |
| `Esc`                            | Dismiss the popup without touching the typed text           |

With no matches the popup renders as empty so an unknown command like
`/xyz` falls through to the normal submit path and reaches the LLM as a
regular user message.

### MCP servers

packetcode can extend its tool surface with external **MCP (Model
Context Protocol) servers**. Declare a server under `[mcp.<name>]` in
`~/.packetcode/config.toml` and packetcode will spawn it at startup,
handshake over stdio JSON-RPC 2.0 (protocol version `2025-06-18`), list
the tools it exposes, and register each as `<server>.<tool>` in the
main tool registry. Every MCP tool is approval-gated (trust mode
auto-approves).

```toml
[mcp.filesystem]
command = "npx"
args    = ["-y", "@modelcontextprotocol/server-filesystem", "/home/alice/projects"]
```

Two slash commands cover day-to-day use:

| Command              | Effect                                                       |
| -------------------- | ------------------------------------------------------------ |
| `/mcp`               | List configured servers — name, state, tool count, pid.      |
| `/mcp logs <name>`   | Tail the last 50 lines of `~/.packetcode/mcp-<name>.log`.    |

Stdio transport only; HTTP+SSE / WebSocket / StreamableHTTP remotes are
deferred. Misbehaving servers (missing binary, handshake timeout, crash
mid-session) are logged but never block startup; native tools and other
MCP servers keep working.

See [`docs/mcp.md`](docs/mcp.md) for the full config schema, worked
examples (filesystem, git, fetch), and debugging tips.

### Keyboard-driven TUI

- **Welcome splash** with the packetcode wordmark when the conversation is empty.
- **Status bar** at the bottom: provider/model, context-window gauge, project name, git branch, cumulative cost, session duration. Sheds segments gracefully on narrow terminals.
- **Conversation pane** with bordered user/assistant bubbles, collapsible tool-call blocks, syntax-highlighted code, inline diffs.
- **Approval prompt** with `Y` / `N` keys. `write_file` and `patch_file` approvals render a colour-coded preview diff (line numbers, `+`/`−` colouring, capped height with a "N lines omitted" legend) so the user sees the actual change instead of raw JSON. Trust mode (`--trust` or `trust_mode = true`) auto-approves everything for the session.
- **Multi-line input** — `Enter` to send, `Shift+Enter` for a newline.

### Session + cost tracking

- Conversations auto-save to `~/.packetcode/sessions/<uuid>.json` after every message via atomic temp-file rename.
- `--resume <session-id>` picks up where you left off.
- Per-session token totals + global cumulative cost recorded with high-water-mark logic; reset via `/cost reset` (planned slash command).
- File backups under `~/.packetcode/backups/<session-id>/` give `/undo` a real safety net (planned slash command).

---

## Install

### Quick install (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/packetcode/packetcode/main/install.sh | bash
```

Set `INSTALL_DIR=$HOME/.local/bin` to avoid sudo.

### From source

Requires Go 1.23+.

```bash
git clone https://github.com/packetcode/packetcode.git
cd packetcode
go build -ldflags "-s -w" -o bin/packetcode ./cmd/packetcode
./bin/packetcode --version
```

On Windows, use `bin/packetcode.exe`.

---

## Usage

First run with no config drops into a line-based setup wizard (provider → API key → model → save).

```bash
packetcode                                       # use config defaults
packetcode --provider gemini --model gemini-2.5-pro
packetcode --resume 9f3c2b1a-...                 # resume a saved session
packetcode --trust                               # auto-approve all tool actions
packetcode --version
```

### Environment variables

API keys can be supplied via env vars, which take precedence over the config file:

```
PACKETCODE_OPENAI_API_KEY
PACKETCODE_GEMINI_API_KEY
PACKETCODE_MINIMAX_API_KEY
PACKETCODE_OPENROUTER_API_KEY
```

Ollama needs no key — packetcode probes `http://localhost:11434` (or whatever's configured under `[providers.ollama] host = ...`).

### Keybindings

**Global**
- `Ctrl+C` — cancel current generation; press twice to exit
- `Ctrl+L` — clear screen (keep session)

**Conversation pane**
- `↑` / `k` — scroll up · `↓` / `j` — scroll down
- `g` — top · `G` — bottom
- `Tab` — toggle collapse on the most recent tool output

**Approval prompt**
- `Y` — approve · `N` / `Esc` — reject

**Input bar**
- `Enter` — send · `Shift+Enter` — newline

---

## Configuration

`~/.packetcode/config.toml`:

```toml
[default]
provider = "openai"
model = "gpt-4.1"

[providers.openai]
api_key = "sk-..."
default_model = "gpt-4.1"

[providers.gemini]
api_key = "AI..."
default_model = "gemini-2.5-pro"

[providers.ollama]
host = "http://localhost:11434"
default_model = "qwen2.5-coder:14b"

[behavior]
trust_mode = false
auto_compact_threshold = 80
max_input_rows = 10
```

The file is created with `0600` permissions (user-read-write only).

### Custom themes

packetcode reads an optional `~/.packetcode/theme.toml` at startup and overrides the built-in Terminal Noir colour tokens with any fields it finds. Every field is optional — absent fields keep their defaults, invalid hex values log a one-line warning and fall back to the default. Four ready-to-use presets live in [`docs/themes/`](docs/themes/) (baseline, light, high-contrast, solarized-dark). Installing one is a single copy:

```bash
cp docs/themes/high-contrast.toml ~/.packetcode/theme.toml
```

See `docs/feature-theming.md` for the full schema and design notes.

---

## Architecture at a glance

```
cmd/packetcode/main.go        CLI entry, flag parsing, dependency wiring

internal/
  config/                     TOML config + ~/.packetcode path helpers
  provider/                   Provider interface + Registry + per-provider adapters
    openaicompat/             Shared SSE client used by OpenAI, MiniMax, OpenRouter
    openai/  gemini/  minimax/  openrouter/  ollama/
  tools/                      Tool interface + Registry + 6 MVP tools
  session/                    Session save/load/list + BackupManager (/undo)
  cost/                       High-water-mark token tally + USD pricing
  git/                        Branch / repo-root helpers (read-only)
  agent/                      Orchestrator: provider stream + tool loop + approval
  app/                        Top-level Bubble Tea model + first-run setup
  ui/
    theme/                    Terminal Noir design tokens (no purple)
    components/               Spinner, status bar, conversation, input, approval, welcome
    layout/                   Pure stacker — body / overlay / input / status
```

The whole thing is one static Go binary (~8.5 MB). No CGO, no runtime, no `node_modules`.

---

## Development

```bash
go test ./...              # full test suite (14 packages, all green)
go build ./cmd/packetcode  # build the binary
golangci-lint run ./...    # lint (config in .golangci.yml)
```

CI lints, tests, and cross-compiles on every push (`.github/workflows/ci.yml`). Release tags trigger GoReleaser to publish binaries for linux/darwin/windows × amd64/arm64 (`.github/workflows/release.yml`, `.goreleaser.yml`).

---

## Roadmap

### Shipped (v1)

Foundation, all five providers, hot-switching, six tools with approval gating, session/backup/cost/git, agent loop with parallel tool calls + `/compact`, runnable TUI with welcome splash + status bar + approval modal, background / parallel agents, twelve slash commands with autocomplete, Ctrl+P/M picker modals, standalone diff component with richer approvals, real HTTP cancellation on Ctrl+C, user-customisable theme, and MCP server support.

---

## License

MIT — see [LICENSE](LICENSE).

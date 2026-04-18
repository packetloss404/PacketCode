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

### Keyboard-driven TUI

- **Welcome splash** with the packetcode wordmark when the conversation is empty.
- **Status bar** at the bottom: provider/model, context-window gauge, project name, git branch, cumulative cost, session duration. Sheds segments gracefully on narrow terminals.
- **Conversation pane** with bordered user/assistant bubbles, collapsible tool-call blocks, syntax-highlighted code, inline diffs.
- **Approval prompt** with `Y` / `N` keys. Trust mode (`--trust` or `trust_mode = true`) auto-approves everything for the session.
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

Foundation, all five providers, hot-switching, six tools with approval gating, session/backup/cost/git, agent loop with parallel tool calls + `/compact`, runnable TUI with welcome splash + status bar + approval modal.

### Next

- Slash command parsing wired to input (`/provider`, `/model`, `/sessions`, `/undo`, `/compact`, `/cost`, `/trust`, `/help`, `/clear`)
- Provider + model selector modals (Ctrl+P / Ctrl+M)
- Slash-command autocomplete popup
- Standalone diff component (currently rendered inline in tool-call blocks)
- Streaming generation cancellation via `Ctrl+C` (today it stops the spinner; we should also cancel the in-flight HTTP request)

### Later

- MCP / plugin system (deferred from MVP)
- Background / parallel agents (deferred from MVP)
- User-customisable theme via `~/.packetcode/theme.toml`

---

## License

MIT — see [LICENSE](LICENSE).

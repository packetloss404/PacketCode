# packetcode

A keyboard-first, multi-provider coding agent for the terminal, with a Claude Code-inspired TUI and first-class OpenAI Codex subscription support.

> Status: pre-1.0 and under active development. Documentation describes the current `main` branch.

packetcode keeps the conversation, tools, approvals, background agents, and workflows in one terminal interface. It can inspect and edit the current project, execute commands, delegate work, connect to MCP tools, and use hosted or local models without routing through OpenCode.

## Install

Install the latest macOS or Linux release:

```bash
curl -fsSL https://raw.githubusercontent.com/packetloss404/packetcode/main/install.sh | bash
```

Install without `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/packetloss404/packetcode/main/install.sh | INSTALL_DIR="$HOME/.local/bin" bash
```

Install the latest checksum-verified Windows release:

```powershell
& ([scriptblock]::Create((Invoke-WebRequest https://raw.githubusercontent.com/packetloss404/packetcode/main/install.ps1).Content))
```

The Windows installer defaults to
`%LOCALAPPDATA%\Programs\PacketCode\bin` and does not silently modify `PATH`.
PacketADE also checks that documented location.

Build from source with Go 1.24.2 or newer:

```bash
make build
./bin/packetcode
```

Windows:

```powershell
$commit = git rev-parse --short HEAD
go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=$commit" -o bin/packetcode.exe ./cmd/packetcode
.\bin\packetcode.exe
```

The first run asks for a provider, API key when required, and model, then writes
`~/.packetcode/config.toml` with user-only permissions. Set an absolute
`PACKETCODE_HOME` to isolate all PacketCode configuration and state in another
data directory.

## Providers

| Slug | Authentication | Notes |
| --- | --- | --- |
| `codex` | Existing Codex CLI ChatGPT login | Reuses `~/.codex/auth.json`; no API key. |
| `openai` | API key | OpenAI API. |
| `anthropic` | API key | Anthropic Messages API. |
| `gemini` | API key | Google Gemini API; independent of Gemini CLI login. |
| `minimax` | API key | MiniMax OpenAI-compatible API; MiniMax M3 default. |
| `deepseek` | API key | DeepSeek OpenAI-compatible API. |
| `grok` | xAI API key | xAI API; packetcode does not reuse the consumer Grok subscription login. |
| `mistral` | API key | Mistral API, including Devstral and Codestral models. |
| `openrouter` | API key | OpenRouter model catalog and routing. |
| `ollama` | None | Native Ollama API; defaults to `localhost:11434`. |
| Custom | Optional | Any configured OpenAI-compatible endpoint. |

The `codex` provider is the zero-key path for an OpenAI Codex ChatGPT subscription. Run `codex login`, choose **Sign in with ChatGPT**, then start packetcode with `--provider codex` or select Codex in the provider picker.

See [Providers and models](docs/providers.md) for authentication, model discovery, local Ollama, and custom endpoints.

## Terminal Workflow

Type a prompt and press `Enter`; use `Ctrl+J`, `Alt+Enter`, or `\` then `Enter` for a portable newline. `Shift+Enter` also works in terminals that report it distinctly. Finalized turns are committed to native terminal scrollback while the active response remains in a small live region.

| Key | Action |
| --- | --- |
| `Enter` | Send a prompt. |
| `Alt+Enter` / `\` then `Enter` | Insert a portable newline; `Ctrl+J` also does so while completion is closed. |
| `Up` / `Down` | Recall prompt history at the first/last input line. |
| `Shift+Tab` | Cycle Manual → Accept Edits → Auto → Plan, including during an active turn. |
| `Left` on an empty idle prompt | Open Agent View. |
| `Ctrl+P` | Open the provider picker. |
| `/model` | Open the model picker; `Ctrl+M` also works when the terminal reports it distinctly. |
| `Ctrl+C` | Cancel the active turn, clear a draft, or quit from an empty prompt. |
| `Ctrl+D` | Quit from an empty prompt. |
| `Ctrl+L` | Clear the visible transcript without deleting the session. |

If a prompt is submitted during an active turn or compaction, packetcode queues it and runs it afterward. `/queue` lists queued prompts; `/queue drop <n>` and `/queue clear` manage them.

Typing `@` at a token boundary opens project-file completion. The selected `@path` is expanded into bounded, root-scoped file context when the prompt is sent. Typing `/` opens slash-command completion.

## Permission Modes

The mode footer always shows the effective policy:

- **Manual** (`ask`): read-only tools run; edits, commands, MCP tools, and agent spawns ask.
- **Accept Edits** (`accept_edits`): file writes and patches run; shell, MCP, and agent spawns ask.
- **Auto** (`auto`): file edits and shell commands run; MCP and other approval-gated surfaces still ask.
- **Plan** (`read_only` plus plan instruction): research only; mutating tools are denied.
- **Bypass Permissions** (`bypass`): tools run unless an explicit deny rule matches. Enter deliberately with `/trust on` or `--trust`; it is not part of the forward cycle.

Shift+Tab can change mode while a turn is running. The new policy applies to subsequent tool actions and re-evaluates an approval already on screen. A command that has already started is not interrupted.

The approval menu supports arrow keys and numbers:

1. Yes
2. Yes, and do not ask again for this tool/session rule
3. No

Use `/permissions` to inspect or change the session policy. See [Security and permissions](docs/security.md).

## Background Agents and Workflows

`/spawn <prompt>` starts a read-only background agent. `/spawn --write <prompt>` creates an isolated git worktree under `~/.packetcode/worktrees/` before allowing writes or commands. Write jobs never edit the foreground checkout directly.

`/agents` opens the full-screen Agent workspace. It groups agents by needs-input, working, and completed states; supports task entry directly from the bottom prompt; and exposes peek, transcript, cancel, inject, and ignore actions. Results are not silently added to foreground context.

`/workflows` orchestrates sequential phases and parallel fan-out over the same bounded jobs manager. A built-in review is available immediately:

```text
/workflows run review
/workflows run review target="the staged diff"
/workflows list
/workflows stop all
```

User workflows live in `~/.packetcode/workflows/*.toml`; project workflows live in `.packetcode/workflows/*.toml` and take precedence.

`/loop` repeats normal prompts or slash commands:

```text
/loop Review the current change and continue until complete
/loop 10m /workflows run review
/loop list
/loop stop all
```

Self-paced loops stop on a versioned `packetcode-loop-decision` block or the
legacy `LOOP_DONE` sentinel, and always stop after 25 iterations. Interval
loops run immediately, then on the requested interval; they queue rather than
overlap an active foreground turn.

See [Background agents](docs/feature-background-agents.md) and [Agent View](docs/feature-agent-view.md).

## Slash Commands

| Command | Purpose |
| --- | --- |
| `/provider [add [slug]\|slug]` | Open the provider picker, add/update a key, or switch. |
| `/model [id]` | Open the model picker or switch models. |
| `/effort [default\|low\|medium\|high\|xhigh\|max\|ultra]` | Show or set reasoning effort for models that expose it. |
| `/spawn [--write] <prompt>` | Start a background agent. |
| `/agents [id]` | Open Agent View or one transcript. |
| `/jobs [id]` | List jobs or open one transcript. |
| `/cancel <id\|all>` | Cancel background jobs. |
| `/workflows [run <name>\|list\|stop [id\|all]\|<id>]` | Run and inspect workflows. |
| `/loop [interval] <prompt\|/command>` | Repeat work; use `list` or `stop`. |
| `/plan [on\|off]` | Toggle read-only planning mode. |
| `/queue [drop <n>\|clear]` | Inspect or manage queued prompts. |
| `/sessions` | List, resume, rename, or delete sessions. |
| `/compact [--keep N]` | Summarize older context. |
| `/undo` | Restore the latest file backup. |
| `/cost` | Show or reset tracked API cost. |
| `/permissions` | Inspect or change tool policy. |
| `/trust [on\|off]` | Show or set bypass mode. |
| `/ollama [status\|models\|ps\|pull <model>]` | Inspect and manage local/remote Ollama. |
| `/mcp` | Inspect MCP servers, status, tools, and logs. |
| `/statusline [refresh]` | Show or refresh statusline output. |
| `/transcript` | Open the current saved transcript. |
| `/clear` | Clear visible output only. |
| `/help` | Show all keys and commands. |
| `/exit` / `/quit` | Exit packetcode. |

Unknown commands show an error. Prefix a prompt with `//` to send a literal leading slash.

Markdown prompt commands can be installed at `~/.packetcode/commands/<name>.md` or `.packetcode/commands/<name>.md`. Project commands override user commands; built-ins cannot be shadowed.

## Configuration

Minimal hosted configuration:

```toml
[default]
provider = "codex"
model = "gpt-5.6-sol"

[providers.codex]
default_model = "gpt-5.6-sol"

[behavior]
auto_compact_threshold = 80
background_max_concurrent = 4
background_max_depth = 2
background_max_total = 32
background_token_budget = 0
workflow_token_budget = 0

[permissions]
profile = "ask"
```

Zero-config local Ollama defaults to `http://localhost:11434` and automatically selects a bounded `num_ctx`, detects model/tool metadata, and keeps the loaded model warm:

```toml
[default]
provider = "ollama"
model = "qwen2.5-coder:14b"

[providers.ollama]
default_model = "qwen2.5-coder:14b"
```

API keys may be stored in config or provided as `PACKETCODE_<SLUG>_API_KEY`. Environment variables win. See the [full configuration reference](docs/configuration.md).

## Context and Token Use

The statusline context gauge shows current request occupancy, not cumulative billed tokens. Cumulative usage still drives cost. Automatic compaction includes system prompts and tool-schema estimates, preserves complete recent tool exchanges, and records compaction usage.

To reduce repeated context:

- Older oversized tool results are compacted only in the model-facing copy; the complete result remains in the session and UI.
- Code-intelligence defaults are bounded.
- Anthropic requests mark stable system/tool prefixes for ephemeral prompt caching.
- Background jobs and workflows can use token budgets.

## MCP, Hooks, Themes, and Statusline

- MCP servers are external stdio processes configured under `[mcp.<name>]`; discovered tools use `<server>__<tool>` names and remain policy-gated.
- Hooks run on user prompt submission, before a tool, or after a tool.
- A native Claude Code-style statusline is enabled by default. A custom command can consume a Claude-compatible JSON snapshot.
- `~/.packetcode/theme.toml` overrides semantic colors and provider colors.

See [MCP servers](docs/mcp.md), [Hooks and statusline](docs/hooks-and-statusline.md), and [Theming](docs/feature-theming.md).

## Documentation

- [User manual](docs/manual.md)
- [Advanced guide](docs/advanced-guide.md)
- [Terminal cheat sheet](docs/cheat-sheet.md)
- [Offline HTML5 manual](docs/packetcode-manual.html)
- [Getting started](docs/getting-started.md)
- [Providers and models](docs/providers.md)
- [Configuration reference](docs/configuration.md)
- [Security and permissions](docs/security.md)
- [Background agents](docs/feature-background-agents.md)
- [Agent View](docs/feature-agent-view.md)
- [Code intelligence](docs/code-intelligence.md)
- [MCP servers](docs/mcp.md)
- [Hooks and statusline](docs/hooks-and-statusline.md)
- [Troubleshooting](docs/troubleshooting.md)
- [TUI parity harness](docs/tui-parity-harness.md)
- [Backlog](BACKLOG.md)
- [Packet Computers and Packet Control proposal](PACKETCOMPUTERS.md)

## Development

```bash
make verify
go test ./...
go vet ./...
go test -race -count=1 ./...
make build
make smoke
make tui-snapshots
```

The credential-free `--tui-fixture=<state>` development flag renders deterministic lifecycle states for PTY snapshots without loading config, providers, credentials, sessions, hooks, MCP, or project files.

## License

MIT — see [LICENSE](LICENSE).

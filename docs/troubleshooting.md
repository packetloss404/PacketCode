# Troubleshooting

Start with:

```bash
packetcode doctor
packetcode doctor --json
```

The doctor checks config, credential sources, providers, state permissions, git/worktrees, native tools, permission policy, and MCP definitions without starting the TUI.

## Provider Is Not Configured

Open `Ctrl+P` or `/provider`, focus the row, and press Ctrl+A. `/provider add <slug>` opens key entry directly.

Keyless exceptions:

- `codex` requires an official Codex CLI ChatGPT login in `~/.codex/auth.json` (`codex login`).
- `ollama` requires a reachable daemon, normally `localhost:11434`.

Anthropic, Gemini, MiniMax, DeepSeek, Grok/xAI, Mistral, OpenAI API, and OpenRouter require developer API keys. Consumer app/CLI subscriptions are not copied into packetcode.

## Gemini CLI No Longer Works

The packetcode `gemini` provider uses Google's developer API directly and does not reuse Gemini CLI authentication. Configure `PACKETCODE_GEMINI_API_KEY` or add the key through the provider picker. If the developer API/model is unavailable to the key, switch providers; the CLI login state is unrelated.

## Model Switch Fails

Use `/model` to load the active account's exact model IDs (`Ctrl+M` is terminal-dependent). Curated fallback catalogs keep some providers selectable when `/models` is unavailable, but the next request remains authoritative. Run `packetcode doctor --check providers` for credential/connectivity failures.

## Ollama Is Unreachable or Slow

```bash
ollama serve
```

Then run `/ollama status`, `/ollama models`, and `/ollama ps`. packetcode defaults to `http://localhost:11434`; override with:

```bash
PACKETCODE_OLLAMA_HOST=ollama.internal packetcode --provider ollama
```

or `[providers.ollama].host`. A CPU-spill warning means the model/context does not fit the available unified-memory GPU budget. Reduce `num_ctx` or choose a smaller quantization/model.

## Context Gauge Looks Wrong

The gauge is current request occupancy, not cumulative tokens. It includes the latest prompt/completion occupancy reported by the provider and can drop after `/compact`. `/cost` remains cumulative.

If a custom statusline disagrees, run `/statusline refresh` and confirm the script uses `context_window.used`/`max` (or the Claude-compatible aliases) rather than accumulating values itself.

If context grows too quickly:

- use `/compact`;
- avoid repeatedly attaching large `@` files;
- inspect unusually large tool/MCP results;
- set background/workflow token budgets;
- verify the model's context metadata in the picker or `/ollama models`.

## Shift+Tab Does Not Change Auto Mode

Shift+Tab cycles Manual → Accept Edits → Auto → Plan and works while a foreground turn is active. Two presses from Manual select Auto. A visible shell approval remains in Accept Edits and resolves after the second press to Auto.

Picker, transcript, Agent, and Workflow workspaces own their keyboard while open; return to chat first. An already-running shell process is not retroactively changed, but later tool actions use the new mode.

## A Tool Was Denied or Auto-Approved Unexpectedly

```bash
packetcode doctor --check permissions
```

Run `/permissions` in the TUI. Explicit session/config rules override profile defaults. Option 2 in the approval menu remembers a session rule; shell commands are remembered exactly. Use `/permissions profile ask` to return to Manual behavior.

## `/spawn --write` Failed

Write jobs require a trusted git repository and worktree support. Run:

```bash
packetcode doctor --check project,state.worktrees
git status
git worktree list
```

The job fails closed rather than editing the foreground checkout. Inspect successful worktrees with the path shown in `/agents` or `/jobs <id>`.

## Missing or Truncated Agent Output

Artifact manifests and model-facing tool results are intentionally bounded. Open `/jobs <id>` for the persisted transcript and inspect the worktree for full changes. Older oversized tool output is compacted only in requests sent back to the model; the persisted session remains complete.

## A Workflow Hangs or Fails

Use `/workflows` for live state and `/agents` for child jobs. `/workflows stop <id>` cascades cancellation. Check global job caps, the workflow's 16-agent guard, token budget, provider credentials, and malformed project/user TOML. Project workflow files override user/built-in files and parse errors are surfaced.

## MCP Server Does Not Start

Run `/mcp`, `/mcp status <name>`, `/mcp logs <name>`, and
`/mcp restart <name>`. Logs live at `~/.packetcode/mcp-<name>.log` and are displayed through
a bounded redacted tail. Restart one crashed process in place; restart
PacketCode after changing MCP configuration.

## Hooks or Statusline Fail

Commands run through PowerShell on Windows and `sh -c` elsewhere, with the project root as working directory. Keep commands deterministic and increase `timeout_sec` if appropriate. packetcode falls back to its native statusline when a custom command fails.

## Cannot Scroll

Finalized output is in terminal-native scrollback. Use terminal scrolling, Shift+PageUp, or tmux copy mode. `/transcript` opens persisted session history. `/clear` and Ctrl+L clear only visible packetcode output.

## Unknown Slash Command

Type `/` or `/help` for current commands. Use `//literal` to send a prompt beginning with `/`. Markdown custom commands belong in `~/.packetcode/commands/` or `.packetcode/commands/`.

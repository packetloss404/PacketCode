# Changelog

All notable changes to packetcode are recorded here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Five LLM providers** behind a unified `Provider` interface: OpenAI
  (GPT-4.1, o3, o4-mini, mini/nano), Google Gemini (2.5 Pro / Flash, 2.0
  Flash), MiniMax, OpenRouter (with dynamic per-model pricing from
  `/api/v1/models`), Ollama (local, NDJSON streaming, no API key).
- **Shared `openaicompat` HTTP/SSE client** used by OpenAI, MiniMax, and
  OpenRouter — handles streaming text deltas, parallel tool calls,
  usage, and `[DONE]` framing in one place.
- **Six agent tools** with JSON Schema definitions:
  - read-only (no approval): `read_file`, `search_codebase` (ripgrep
    with Go fallback), `list_directory` (skips conventional junk dirs)
  - destructive (approval-gated): `write_file`, `patch_file` (unique
    search/replace + unified diff), `execute_command` (sh/cmd, timeout,
    output cap)
  - All filesystem tools enforce path-traversal protection scoped to the
    project root.
- **Agent loop** — orchestrates user message → LLM stream → tool calls
  → approval → execution → LLM stream → … with a 25-iteration safety
  cap. Supports parallel tool calls in a single response.
- **`Approver` interface** decouples the agent from the TUI. Ships with
  `AutoApprove` and `AutoReject` for tests; the App wires a
  channel-based approver that blocks the agent on the TUI's modal.
- **Session persistence** — sessions live at
  `~/.packetcode/sessions/<uuid>.json`, written atomically (temp file +
  rename). Auto-saves after every message. `--resume <id>` picks up
  where you left off.
- **`/undo` backup stack** — every `write_file`/`patch_file` snapshots
  the original under `~/.packetcode/backups/<session-id>/`. Undo of a
  fresh-creation deletes the new file.
- **Cost tracking** with high-water-mark logic (matches the existing
  Claude Code status-line tally pattern). Pricing is re-applied at
  display time so rate changes propagate to historical sessions.
- **Git integration** — branch + repo-root detection for the status
  bar. Gracefully degrades when git is missing.
- **Context manager** — token estimation + auto-suggest threshold +
  `/compact` LLM-driven summarisation that preserves the system prompt
  and recent message tail.
- **Terminal Noir theme** — high-contrast monochrome surfaces with
  electric cyan as the single semantic accent. Provider brand colors
  for OpenAI / Gemini / MiniMax / OpenRouter / Ollama are defined as
  tokens, not hardcoded.
- **TUI components** (Bubble Tea + Lip Gloss):
  - Welcome splash with the packetcode block-letter wordmark, version
    label, and a hint, centred when no messages exist
  - Status bar at the bottom: provider/model dot, context % gauge
    (green/yellow/red thresholds), project name, git branch, cumulative
    cost, session duration. Sheds segments on narrow terminals.
  - Scrollable conversation pane with role-coloured borders, collapsible
    tool-call blocks with summary line, inline diff colouring
  - Approval prompt with `Y` / `N` keys
  - Multi-line input bar (Enter sends, Shift+Enter newlines)
  - Braille spinner for the "thinking" state
- **First-run setup wizard** — line-based provider → key → validation →
  model picker, persisted to `~/.packetcode/config.toml` with `0600`
  permissions.
- **CLI flags**: `--version`, `--provider`, `--model`, `--resume`,
  `--trust`.
- **Env-var key overrides** (`PACKETCODE_<SLUG>_API_KEY`) take precedence
  over the config file.
- **CI/CD**: lint + test + cross-compile on every push
  (`.github/workflows/ci.yml`); GoReleaser-driven multi-platform release
  on `v*` tags (`.github/workflows/release.yml`, `.goreleaser.yml`).
- **`install.sh`** for one-line Linux/macOS install from GitHub
  Releases; honours `INSTALL_DIR` and `VERSION` env vars.
- **Background / parallel agents** — spawn independent agent loops
  alongside the foreground conversation via `/spawn`, `/jobs`,
  `/cancel`, or the `spawn_agent` tool. Each job is a fully isolated
  mini-agent with its own session, cost tally, backup stack, and
  provider/model; results stream back as a system message plus an
  auto-injected context message on the next turn. Read-only by
  default (with `--write` / `wait=true` opt-in for destructive
  tools, approval-gated through the main session). The `⚙ N jobs`
  top-bar counter and a dedicated transcript modal round out the
  UX. Caps and defaults tunable under `[behavior]`:
  `background_max_concurrent` (4), `background_max_depth` (2),
  `background_max_total` (32), `background_default_provider`,
  `background_default_model`.

### Design

- **No purple anywhere.** Accent is electric cyan `#00D9FF`; OpenRouter's
  brand badge swapped from purple `#9B59B6` to rose `#EC4899`.
- **Status bar at the bottom**, not the top — the input + status anchor
  the eye while the conversation flows above.
- **Welcome screen on launch** when the conversation is empty.
- **Layout fix** — replaced the line-trimming `Frame` with a pure
  stacker that trusts caller-supplied region sizes, eliminating a class
  of bugs where the first user message could be clipped by overzealous
  top-trimming when chrome heights were mis-estimated.

### Deferred to a future release

- Slash command parsing in the input bar (`/provider`, `/model`,
  `/sessions`, `/undo`, `/compact`, `/cost`, `/trust`, `/help`,
  `/clear`).
- Provider + model selector modals (Ctrl+P / Ctrl+M today open nothing).
- Slash-command autocomplete popup.
- Standalone diff component — diffs render inline in tool-call blocks
  for now.
- Streaming-generation HTTP cancellation on Ctrl+C — today the spinner
  stops but the request continues until the provider closes the stream.
- MCP / plugin system.
- User-customisable theme via `~/.packetcode/theme.toml`.

### Test coverage

14 test-bearing packages, all green: agent, config, cost, git, provider
(registry + 5 providers), session, tools (registry + safefs + 6 tools),
ui/components/topbar, ui/layout.

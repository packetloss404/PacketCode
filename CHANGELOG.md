# Changelog

All notable changes to packetcode are recorded here.

packetcode has not cut a stable 1.0 release yet. Entries under `Unreleased` describe the current main branch.

## [Unreleased]

### Added

- **`/workflows`** — a Go-native multi-agent orchestration engine over the existing `jobs.Manager`. Workflows are TOML (under `~/.packetcode/workflows/` or `<project>/.packetcode/workflows/`) or built-in: sequential **phases**, single-agent steps, and **parallel fan-out** steps whose results bind into later prompts (`{{.steps.name}}`). A built-in `review` workflow works out of the box (`/workflows run review`). `/workflows list`, `/workflows stop [id|all]`, and a live `workflowview` (run → phase → step → agent). Bounded by the existing background-agent caps plus a per-run agent guard. (Phase 2; pipelines + adversarial verify are the next phase.)
- **`/loop`** — repeat a prompt or `/command`. `/loop 5m /cost` re-runs on an interval; `/loop <prompt>` self-paces, re-running after each turn until the model ends a reply with `LOOP_DONE` or a 25-iteration cap. Ticks that land mid-turn queue instead of overlapping, and because each iteration is a normal turn it can fan out background agents (`spawn_agent`) or run a workflow. `/loop list` and `/loop stop [id|all]` manage active loops. (First phase; a full `/workflows` orchestration engine — Go-native, built on the existing `jobs.Manager` — is planned next.)
- **Inline "always allow" in the approval prompt** — the approval modal now offers `[A] Always` alongside `[Y]/[N]`. Choosing it approves the call and adds a session permission rule so it isn't asked again: for shell commands the rule is scoped to the command family (e.g. `go test …`, not all commands); for other tools it allows the tool by name. Review or clear with `/permissions`.
- **Plan mode** (`/plan`) — a read-only research mode. Toggling it on forces the `read_only` permission profile (edits and commands are disabled) and steers each turn to investigate and propose a numbered plan for approval; `/plan off` restores the previous profile so the model can execute. The top bar shows `plan` while active.
- **@-file mentions** — write `@path/to/file` anywhere in a prompt and packetcode inlines that file's contents into the message sent to the model (your visible message keeps the literal `@path`, and a system note lists what was attached). Resolves relative/`~`/absolute paths, stays within the project, skips binaries, and caps large files. Typing `@` also opens an interactive fuzzy-find popup over the project's files (git-tracked + untracked, `.gitignore`-aware) — arrow/Tab/Enter to pick, and the path splices into your prompt.
- **Reasoning/thinking display** — when a provider streams a reasoning summary (Responses API), packetcode renders it live as dim "✻ thinking" text above the answer, in the same bubble. New `EventReasoningDelta` flows provider → agent → UI. For Codex, `reasoning.summary` is now requested per-model from the model catalog: **`gpt-5.5`/`gpt-5.4` stream live thinking** (select them with `/model`), the `gpt-5.6` family reasons but returns it encrypted-only (nothing to display), and models that reject the parameter (`gpt-5.3-codex-spark`) omit it to avoid a 400.

- **Ollama overhaul for first-class local use (tuned for Apple Silicon).** Local `localhost:11434` remains the zero-config default; remote hosts and tuning are opt-in.
  - **Fixes silent context truncation:** every request now sets an auto-sized `num_ctx` (Ollama's ~4K default would silently drop earlier turns and file contents), snapped to a bucket to avoid KV-cache churn and capped to the model's real maximum.
  - **Accurate model metadata via `/api/show`:** real context windows and authoritative per-model tool-calling detection (replacing a stale hardcoded allow-list), cached per model.
  - **Protocol correctness:** native `tool_name` on tool results, `keep_alive: 30m` to avoid mid-session reloads, `done_reason: length` surfaced as a truncation error, and the standard `OLLAMA_HOST` env var honored.
  - **Tuning under `[providers.ollama]`:** optional `num_ctx`, `keep_alive`, `temperature`.
  - **Model management:** `PullModel` (streaming download progress), `LoadedModels` (`/api/ps` with GPU-vs-CPU-offload detection), `Warmup` (preload to avoid cold starts), and a "start it with `ollama serve`" hint when the daemon is down.
  - **Apple Silicon awareness:** detects unified memory (`hw.memsize`), recommends coding models that fit the GPU budget (and warns before one would spill to CPU), and exposes tokens/sec + time-to-first-token from Ollama's timing fields.
  - **`/ollama` command** — `status` (reachability, memory, model recommendations, tok/s), `models` (installed models with context window + tool support), `ps` (loaded models with GPU/CPU split), and `pull <model>`. Switching to an Ollama model warms it up in the background to avoid a cold-start on the first turn.
- **Refreshed model catalogs** — updated built-in pricing/context tables and default models to the current lineups: OpenAI GPT-5.6 (`gpt-5.6-sol` default, plus `terra`/`luna`), Anthropic Claude Opus 4.8 (default), Sonnet 5, and Fable 5, and MiniMax M3 (default). Dynamic model listing still surfaces anything the account can access; these just fix the defaults and cost estimates.
- **Statusline on by default** — packetcode now renders a Claude Code-style statusline natively out of the box (no `jq`/subprocess), `[provider·model] 🟢 pct% (used/max) | 📂 project | 🌿 branch | 💲cost | ◷ op`, refreshed live each second. Set `[statusline].command` to override it with your own script.
- **Claude Code-compatible statusline** — the statusline stdin snapshot is now a superset of Claude Code's, adding `cwd`, `model.display_name`, and `context_window.context_window_size` / `current_usage.*` aliases, so a statusline script written for Claude Code runs against packetcode unchanged (point `[statusline].command` at your existing `~/.claude/statusline/statusline.sh`). A molded variant that keeps the Claude Code look and adds packetcode-native segments (provider, session cost, live operation) ships at `docs/statusline/statusline.sh`.
- **Codex provider** — drive packetcode with an OpenAI Codex ChatGPT subscription instead of a per-token API key. It reuses the OAuth credentials the official Codex CLI stores in `~/.codex/auth.json` (`codex login` → "Sign in with ChatGPT"), refreshes the access token automatically on expiry, and talks to the ChatGPT backend's Responses API. The provider is keyless like Ollama, reports `$0` cost (the subscription bills a flat rate), and lists the account's Codex models by reading the Codex CLI's live `models_cache.json` (e.g. `gpt-5.6-sol`), each sent its Codex-default reasoning effort. Select it with `/provider codex` or `--provider codex`. See `docs/providers.md`.

## [0.5.1] - 2026-05-30

### Added

- Automatic retry with exponential backoff and jitter for transient provider failures — HTTP 429/5xx and dropped connections that occur before the response stream begins — honoring `Retry-After` and turn cancellation. Applies to all providers; configurable via `provider_max_retries` (default 3 attempts, 1 disables).
- Per-call stall timeout that aborts a provider stream which goes silent mid-response, surfaced as a retryable timeout. Configurable via `provider_stall_timeout` (default 60 seconds).
- `patch_file` now falls back to a whitespace- and line-ending-tolerant UNIQUE match when no exact match is found (still errors on ambiguity); behavior is unchanged when the exact match succeeds.
- `execute_command` now streams output incrementally to the conversation as it runs (bounded cap and cancellation preserved).

## [0.5.0] - 2026-05-29

### Added

- Multi-provider chat through one provider interface: OpenAI, Google Gemini, MiniMax, OpenRouter, and local Ollama.
- Agent tool loop with `read_file`, `search_codebase`, `list_directory`, `write_file`, `patch_file`, and `execute_command`; mutating tools require approval unless trust mode is enabled.
- Sessions, cost tracking, `/undo` file backups, context compaction, and git-aware status information.
- Keyboard-first Bubble Tea TUI with inline terminal scrollback, approval prompts, provider/model pickers, slash-command autocomplete, and markdown-backed custom prompt commands.
- Queued foreground prompts while a turn or `/compact` is already running.
- Background agents via `/spawn`, `/agents`, `/jobs`, `/cancel`, and the `spawn_agent` tool. Background jobs are read-only by default and request normal approvals when launched with `--write`; Agent View provides live status, token/cost telemetry, transcripts, cancellation, and explicit result injection.
- `/transcript` for opening the current saved session transcript.
- MCP stdio server support through `[mcp.<name>]` config blocks. MCP tools are registered as provider-safe `<server>__<tool>` aliases and always go through approval.
- `/mcp status <name>` and `/mcp tools <name>` diagnostics.
- Optional custom statusline command under `[statusline]`.
- Optional lifecycle hooks under `[[hooks.user_prompt_submit]]`, `[[hooks.pre_tool_use]]`, and `[[hooks.post_tool_use]]`.
- User theme overrides through `~/.packetcode/theme.toml`, with presets under `docs/themes/`.
- Packet Computers and Packet Control research/design notes in `PACKETCOMPUTERS.md`.

### Changed

- Accepting `/provider` or `/model` from the slash-command autocomplete popup (Tab, or Enter on the bare verb) now opens the picker directly, so you select from a list instead of guessing a slug or id. Added `/providers` and `/models` plural aliases.
- Topbar/statusline output now includes foreground operation state, elapsed time, and queued prompt count.
- Approval prompts show clearer job/source context and pending approval depth.
- Job/session transcript viewer opens at the newest content and includes better scroll hints.
- `execute_command` descriptions and approval previews now call out the active shell runtime and Windows PowerShell/WSL/Git Bash invocation expectations.
- Documentation now treats the project as pre-release / active development instead of calling the current feature set `v1`.
- User docs now describe the real provider setup path: use `Ctrl+P`, `/provider`, or `/provider add`; focus a provider row and press `Ctrl+A`, or run `/provider add <slug>` to open the key prompt directly.
- Transcript docs now match the inline-rendering model: finalized output is committed to terminal scrollback, `/clear` only clears packetcode's live pane, and historical tool blocks are not toggled after they are printed.

### Fixed

- Custom statusline auto-refresh is throttled so one-second topbar operation ticks do not spawn overlapping statusline commands.
- `/mcp tools` now displays the same sanitized callable aliases that are registered with providers.
- Removed a dead placeholder goroutine from foreground turn startup.
- Hardened timing-sensitive Windows tests for hook/statusline process startup and command cancellation.

### Testing

- The Go test suite covers provider adapters, config loading, sessions, tools, the agent loop, cancellation, slash commands, pickers, jobs, MCP, hooks, statusline, and UI components.

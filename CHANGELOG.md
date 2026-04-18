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
- **Slash commands — Round 1.** Extended the input-bar slash-command
  surface from three verbs to twelve: `/provider`, `/model`,
  `/sessions`, `/undo`, `/compact`, `/cost`, `/trust`, `/help`, and
  `/clear` now sit alongside the existing `/spawn`, `/jobs`, and
  `/cancel`. The parser is a flat allow-list and per-handler sub-arg
  parsers; handlers are thin adapters over `provider.Registry`,
  `session.Manager`, `session.BackupManager`, `cost.Tracker`,
  `agent.ContextManager`, and `uiApprover`. Destructive verbs
  (`/sessions delete`, `/cost reset`) require an explicit `--yes`
  flag instead of a confirmation modal.
- **Picker modals — Round 2.** `Ctrl+P` and `Ctrl+M` now open live
  filter-as-you-type list modals backed by a new generic
  `internal/ui/components/picker` component (reused by the upcoming
  slash-autocomplete and theme-picker rounds). Model lookups are async
  with a per-provider in-memory cache on `provider.Registry` — the
  first `Ctrl+M` triggers a `ListModels` round trip, subsequent opens
  are instant. Both the slash handlers and the picker dispatch through
  the same `applyProviderSwitch` / `applyModelSwitch` helpers in
  `internal/app`, so the side effects (Registry.SetActive, top-bar
  refresh, `switched …` system message) stay identical.
- **Standalone diff component + richer approvals — Round 4.** New
  `internal/ui/components/diff` package parses a unified-diff string
  into structured hunks and renders it with colour, a right-aligned
  line-number gutter, width-clamp truncation ("…"), and a capped-
  height layout that falls back to "N lines omitted (+X, −Y across Z
  hunks)" when the cap is exceeded (40 rows in the approval modal,
  200 rows in the scrollable conversation viewport). `write_file`
  and `patch_file` approvals now route through tool-specific
  `BodyRenderer` functions registered in `internal/ui/components/
  approval/renderers.go`: `write_file` previews via a new
  `WriteFileTool.PreviewDiff` helper (handles brand-new files,
  identical-content no-ops, binary rejection, path traversal);
  `patch_file` previews via a new `PatchFileTool.PreviewPatchDiff`
  helper (shares validation + diff generation with `Execute` through
  a new private `applyPatches`). The completed `patch_file`
  tool-result block in the conversation pane renders through the
  same component for visual parity between "approve" and "approved".
  The unexported `patchOp` struct is renamed to exported `PatchOp`
  (JSON wire format unchanged). Every other tool keeps the existing
  `summariseParams` JSON-pretty-print fallback.
- **HTTP cancellation on Ctrl+C — Round 5.** Pressing Ctrl+C during a
  streaming turn now cancels the in-flight provider HTTP request,
  kills any running tool, and dismisses any pending approval prompt
  within ~1s (previously the spinner stopped but the stream continued
  billing tokens). The conversation shows a "turn cancelled" system
  line; a second Ctrl+C while idle exits. Implemented via a new
  `App.cancelTurn context.CancelFunc` lifecycle and a per-iteration
  `ctx.Err()` guard inside `parseSSE` / `parseGeminiSSE` /
  `parseOllamaStream`. Background `/spawn`'d jobs are NOT cascaded —
  their ctx tree derives from the `jobs.Manager` root, not `agent.Run`.
- **User-customisable theme — Round 6.** packetcode now reads an
  optional `~/.packetcode/theme.toml` at startup and overrides the
  Terminal Noir colour tokens with any fields it finds. The loader
  (`internal/ui/theme/loader.go`) parses a flat TOML mirroring the
  five design-system groups (`[base]`, `[text]`, `[accent]`,
  `[semantic]`, `[provider.<slug>]`), validates each hex value
  (`#RRGGBB` or `#RGB`; invalid values log a one-line warning and
  keep the default), merges the provider map into the new
  `providerColors` lookup (ProviderColor refactored from a switch
  so users can theme arbitrary slugs), and rebuilds every pre-built
  `Style*` value so they pick up the new palette. A missing file is
  silent; a parse error logs `packetcode: failed to load theme:
  <err>; falling back to defaults` and continues. Apply is
  additive — calling it with an empty `Theme` does NOT reset to the
  built-in defaults. Four example presets ship under `docs/themes/`:
  `dark-terminal-noir.toml` (baseline + schema doc), `light.toml`
  (light-terminal variant), `high-contrast.toml` (pure black/white +
  saturated accents), and `solarized-dark.toml` (Solarized Dark
  mapped onto the tokens). Install a preset with
  `cp docs/themes/high-contrast.toml ~/.packetcode/theme.toml`.
- **Slash-command autocomplete — Round 3.** Typing `/` as the first
  character of the input buffer opens a borderless filter-as-you-type
  popup above the input listing every slash command. A two-tier sort
  (verb-prefix matches first, alphabetically, then substring matches
  on the verb+description haystack) keeps the best candidates at the
  top; the cursor is navigated with arrows, `Ctrl+N/P`, or `Ctrl+J/K`.
  `Tab` always accepts the highlighted row and `Enter` accepts only
  while the buffer is a bare verb — otherwise Enter falls through to
  the normal submit path so `/xyz` (zero matches) still reaches the
  LLM as a user message. The popup lives in a new `aboveInput` layout
  slot between the overlay cascade and the input bar, so any modal
  (approval / picker / jobs) visually covers it and
  `refreshAutocomplete` closes it whenever one is up. Bespoke component
  at `internal/ui/components/autocomplete` (not a reuse of `picker` —
  the geometry and dismiss semantics are different); entries are
  built once from `keymap.SlashCommands` with verb dedup so `/jobs`
  and `/jobs <id>` collapse into a single row.
- **MCP (Model Context Protocol) server support — Round 7.**
  packetcode now spawns external MCP servers declared under
  `[mcp.<name>]` in `~/.packetcode/config.toml`, handshakes over
  newline-delimited JSON-RPC 2.0 stdio (protocol version
  `2025-06-18`), lists the server's tools, and registers each as a
  `<server>.<tool>` in the main tool registry so the Agent's first
  turn already sees them. Every MCP tool is approval-gated and
  always-prefixed, so native tools and MCP tools never collide. A
  new `internal/mcp` package owns the per-server stdio driver
  (synchronous writer + async reader + reaper + stderr-tee to
  `~/.packetcode/mcp-<name>.log`), the tools.Tool adapter, and a
  `Manager` that parallel-spawns up to 8 at a time and surfaces a
  per-server `StartupReport` (running / disabled / failed). Two new
  slash commands land: `/mcp` renders a monospace table of servers +
  state + tool count + pid, and `/mcp logs <name>` tails the last 50
  lines of the server's stderr log. Misbehaving servers are logged
  but never block startup; crashed servers short-circuit with a
  friendly "restart packetcode to reconnect" error while native +
  other MCP tools keep working. `/mcp restart <name>` is deferred to
  Round 8. Worked examples (filesystem, git, fetch) live in
  `docs/mcp.md`.

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

- (Nothing left from the original Round 1–7 roadmap — every item has
  landed.)

### Test coverage

26 test-bearing packages, all green. Round 7 added
`internal/mcp` as a test-bearing package (21 tests: framing
round-trip + 2 MB payload; client initialize handshake / init
timeout / tools/call success + isError / 20-way concurrent dispatch
/ stdout-EOF-unblocks-pending / server-initiated request replied
`method not found` / server notification ignored / Close-closes-
stdin-and-waits / Close-kills-hanging; manager mixed-statuses /
parallel-spawn-under-8s / shutdown-all-clients; McpTool prefixed
naming / dead-client friendly error / flattens content / null
params → `{}` / ctx cancellation / inputSchema passthrough).
`internal/config` gained `TestConfig_MCPBlockRoundTrip`,
`TestConfig_MCPMapInitialisedOnLoad`, and `TestMCPLogPath`.
`cmd/packetcode` gained 4 `TestMCPConfigFlatten_*` tests pinning
alphabetic ordering, default timeout, nil safety, and the
`IsEnabled` pointer-bool contract. `internal/app` gained 6 slash-
level tests: `TestParseSlashCommand_MCP` (bare / logs+name /
missing name / restart defers / unknown sub), `TestRenderMCPTable_
NoServers`, `TestRenderMCPTable_MixedStatuses`, `TestTailMCPLog_
MissingFile` (home-isolated), `TestTailMCPLog_TruncatesToLast50
Lines` (writes 100 lines, asserts lines 51-100 with header +
footer), and `TestSlashHelp_IncludesMCP`. +34 new tests, all
green; no regressions. Round 6 added
`internal/ui/theme` as a test-bearing package (new
`loader_test.go` with 14 tests covering Load missing/valid/malformed/
unknown-field, Apply nil-no-op / mutation / short-hex expansion /
invalid-hex-warns-and-keeps-default / provider map merge /
partial-override / additive-not-replacing, the
`TestApply_RebuildsAllTwentyStyles` drift guard against the
rebuild-list, and a `TestProviderColor_UnknownSlug_ReturnsTextPrimary`
map-refactor regression; every test snapshots the full colour-var,
providerColors, and Style* state via `t.Cleanup` for hermeticity).
`internal/config` gained `TestThemePath_UnderHomeDir` with HOME /
USERPROFILE isolation. +15 new tests, all green; no regressions.
Round 5 added
`internal/provider/openaicompat` as a test-bearing package (new
`client_test.go` with a slow-trickle httptest server asserting the
stream goroutine exits within 1s of ctx cancel and emits
`EventError(context.Canceled)`), plus matching
`TestGemini_ChatCompletion_CancellationStopsStream` and
`TestOllama_ChatCompletion_CancellationStopsStream`. Agent-level
`TestAgent_Run_CancelDuringChatCompletion` and
`TestAgent_Run_CancelDuringApproval` prove the events channel closes
promptly and the approver unblocks on ctx cancel. Tools gained
`TestExecuteCommand_ContextCancelKillsProcess` (Unix; Windows
skipped) confirming SIGKILL propagation within 1s.
`internal/app/app_cancel_test.go` is new, with five integration
tests covering the state-machine branches: cancel-during-stream
(cancelTurn cleared synchronously, streaming flips on agentDoneMsg),
idle-Ctrl+C-quits, approval-modal-hides, turn-cancelled-line,
double-Ctrl+C-is-safe-during-shutdown — plus a
`TestIsCancellation_WalksErrorChain` unit test. ~11 new tests; no
regressions. Round 4 added
`internal/ui/components/diff` (new package, ~28 component tests
covering parse edge cases, malformed-hunk errors, new-file
synthesis, stats, gutter width, width-clamp truncation, row-cap
head/tail/single-line fallback, and immutable-builder copy
semantics) and turned `internal/ui/components/approval` and
`internal/ui/components/conversation` into test-bearing packages
where they weren't before (+13 approval tests using real
`WriteFileTool` / `PatchFileTool` rooted at `t.TempDir()` for every
branch of the diff-aware renderers — new-file, overwrite,
identical, binary fallback, path-traversal fallback, malformed-JSON
fallback, patch valid, search-not-found, ambiguous, registry
override, unregistered-tool fallthrough; +6 conversation tests
covering patch_file diff routing, error fallthrough, collapsed
placeholder precedence, non-patch-file passthrough, missing-marker
fast path, and the 200-row cap). Tools package gained 6 new
`TestWriteFile_PreviewDiff_*` tests, 9 new
`TestPatchFile_PreviewPatchDiff_*` tests, and a
`TestPatchFile_PatchOpJSONWireFormat` regression pinning the
unexported `patchOp` → exported `PatchOp` rename's on-the-wire
stability. ~63 new tests, all green; no regressions in any existing
test. Round 3 added `internal/ui/components/autocomplete` (the new
package) with ~22 component tests covering lifecycle, two-tier
filter bucketing, navigation keys, render shape (cursor marker,
overflow footer, width clamp), plus ~23 App-level integration tests
covering the open/close triggers, Tab / Enter semantics, modal
precedence, no-match fall-through, the Enter-submits-with-args
fall-through, and the keymap dedup. A
`TestParseSlashCommand_TrailingSpaceAfterVerb` regression pins the
"accept leaves a trailing space" invariant; the layout and input
component tests absorbed the new 5-arg `Frame` signature and
`input.SetValue` helper. Packages: agent, app, config, cost, git,
jobs, mcp, provider (registry + 5 providers + openaicompat shared
client), session, tools (registry + safefs + 6 tools +
spawn_agent), ui/components/approval, ui/components/autocomplete,
ui/components/conversation, ui/components/diff, ui/components/input,
ui/components/jobs, ui/components/picker, ui/components/topbar,
ui/layout, ui/theme.

# Roadmap — deferred items, organised into rounds

This is the execution plan for the items still listed under **Deferred to
a future release** in `CHANGELOG.md`. Each round is a cohesive,
independently-shippable slice: a planning agent + 1–3 implementation
agents + a commit, mirroring the pipeline used for the background-agents
feature.

Rounds are ordered by a mix of user value and dependency chain — the
first two are "pay off the promise" rounds (finishing slash commands
the UI already hints at), and rounds 5–7 are bigger architectural bets.

---

## Round 1 — Slash-command parsing for the remaining commands

**Landed.** See `docs/feature-slash-commands.md` for the spec and the
git log for the commit that shipped `/provider`, `/model`, `/sessions`,
`/undo`, `/compact`, `/cost`, `/trust`, `/help`, and `/clear`.

---

## Round 2 — Provider and model selector modals (Ctrl+P / Ctrl+M)

**Landed.** See `docs/feature-picker-modals.md` for the spec and the
git log for the commit that shipped Ctrl+P / Ctrl+M selector modals,
the generic `picker` component, and the shared `applyProviderSwitch` /
`applyModelSwitch` helpers.

---

## Round 3 — Slash-command autocomplete popup

**Landed.** See `docs/feature-slash-autocomplete.md` for the spec and
the git log for the commit that shipped the autocomplete popup, the new
`internal/ui/components/autocomplete` component, and the `aboveInput`
layout slot.

---

## Round 4 — Standalone diff component + richer tool-call rendering

**Landed.** See `docs/feature-diff-component.md` for the full design
spec and the git log for the commit that shipped the
`internal/ui/components/diff` package, the diff-aware approval
renderers for `write_file` and `patch_file` (via new
`WriteFileTool.PreviewDiff` / `PatchFileTool.PreviewPatchDiff`
helpers), and the conversation-side parity rendering for the
completed `patch_file` tool-result block. Rounds 5–7 below are
unchanged.

---

## Round 5 — Real HTTP cancellation on Ctrl+C

**Landed.** See `docs/feature-http-cancellation.md` for the full design
spec and the git log for the commit that shipped the
`App.cancelTurn context.CancelFunc` lifecycle, the per-iteration
`ctx.Err()` guard inside `parseSSE` / `parseGeminiSSE` /
`parseOllamaStream`, the `isCancellation` helper rendering a friendly
"turn cancelled" system line, and the state-machine property that
makes double Ctrl+C during shutdown a safe no-op. Background
`/spawn`'d jobs are not cascaded — their ctx trees derive from the
`jobs.Manager` root.

---

## Round 6 — User-customisable theme via `~/.packetcode/theme.toml`

**Landed.** See `docs/feature-theming.md` for the full design spec and
the git log for the commit that shipped the `internal/ui/theme/loader.go`
loader (Load + Apply + parseHex + rebuildStyles), the `providerColors`
map refactor of `ProviderColor`, `config.ThemePath()`, the startup
wire-up in `cmd/packetcode/main.go`, and the four example presets under
`docs/themes/` (dark-terminal-noir / light / high-contrast /
solarized-dark).

---

## Round 7 — MCP / plugin system

**Scope.** Support external tools via the Model Context Protocol. Users
configure MCP servers in `config.toml`; packetcode spawns them as child
processes, handshakes over stdio, proxies tool calls from the LLM into
the MCP server.

**Why last.** Biggest architectural bet — crosses process boundaries,
brings in a new wire protocol, needs sandboxing decisions. Best tackled
when the rest of the app is stable.

**File-by-file sketch.**

- `internal/mcp/` — client package implementing the MCP JSON-RPC
  protocol (list_tools, call_tool, plus lifecycle).
- `internal/mcp/process.go` — child-process lifecycle, stdin/stdout
  framing, graceful shutdown.
- `internal/tools/mcp_tool.go` — adapter that exposes an MCP-advertised
  tool as a native `tools.Tool`. Approval is always required (external
  code!).
- `internal/config/config.go` — `[mcp]` blocks with command, args,
  env, enabled.
- `cmd/packetcode/main.go` — on startup, spawn each enabled MCP server,
  register its tools in `toolReg`.
- `docs/mcp.md` — how to configure a server, a couple of example
  configs (filesystem, git, fetch).

**Agents.**
1. **Plan** — protocol version target, process-lifecycle contract,
   approval policy (is there a "trusted MCP servers" list?), failure
   semantics (server crashes mid-call).
2. **Implement backend** — client + process management.
3. **Implement integration** — tool adapter + config + main wiring.
4. **Docs + examples + tests + commit**.

**Estimated effort.** Multi-session. Probably 2–3 sessions, 4 agents
per session.

---

## Out of scope until after Round 7

- **Resumable background jobs across restart.** The session JSONs are
  already persisted, so this is "feed them back into a fresh Agent";
  revisit after Rounds 1–6 land.
- **Per-job worktree isolation** (concurrent edit-to-different-branches
  story). Genuinely hard; revisit with MCP in mind since MCP workflows
  will push on it.
- **Streaming sub-agent output into the main conversation in real time.**
  Requires a concurrency model change; v1 delivers the summary on
  completion.
- **Cross-job dependencies / DAG scheduling.** Future.
- **Per-tool trust setting** (always-allow `spawn_agent` etc.). Small
  extension to the approval policy; could slot into Round 3 or a tiny
  standalone patch.
- **Sub-agent → user questions.** Would require a notification channel
  orthogonal to the current approval modal. Future.

---

## How to run a round

The pattern the background-agents feature validated:

1. **Plan subagent** writes `docs/feature-<name>.md` with a
   file-by-file change list grouped into implementation buckets.
2. **Backend / backend-adjacent agent** implements the lowest-level
   package with tests. No UI edits.
3. **Integration agent** wires the new backend into App + CLI +
   tools. No UI edits beyond what the handler needs.
4. **TUI + docs + commit agent** does the visible component, README
   and CHANGELOG deltas, commits with a detailed message.

Each agent reads the spec doc, reports deviations precisely so the
next agent knows the real API, and does not overstep the bucket
boundary.

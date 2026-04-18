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

**Scope.** Extract diff rendering into a dedicated
`internal/ui/components/diff` component with proper hunk headers, line
numbers, and colour-coded `+`/`-` lines. Wire it into the approval
prompt for `write_file`/`patch_file` (today the user sees only the JSON
params) and into the completed tool-call block in the conversation.

**Why fourth.** Makes approvals far more legible — right now the user
approves a raw JSON blob for `patch_file`. This is a UX cliff that
matters more as the feature gets used heavily.

**File-by-file sketch.**

- `internal/ui/components/diff/diff.go` — parse a unified diff string
  (already produced by `patch_file`) into structured hunks, render
  with side-by-side or unified presentation, line-number gutter, colour.
- `internal/ui/components/approval/approval.go` — when the tool is
  `write_file` or `patch_file`, generate a preview diff (compute it
  from the current file vs the proposed content) and render via the
  diff component instead of raw JSON.
- `internal/ui/components/conversation/conversation.go` — swap the
  inline diff-in-toolcall path to use the new component for visual
  parity.
- `internal/tools/write_file.go` — add a `PreviewDiff(root, path,
  content string) string` helper so approval can render without
  re-implementing diffing.

**Agents.**
1. **Plan** — diff parsing approach (reuse `go-difflib` vs custom),
   component API, side-by-side vs unified decision.
2. **Implement** — component + approval wiring + conversation wiring.
3. **Tests + visual-smoke + commit**.

**Estimated effort.** Single session, ~3 agents.

---

## Round 5 — Real HTTP cancellation on Ctrl+C

**Scope.** Today Ctrl+C during streaming stops the spinner but the HTTP
request keeps running until the provider closes the stream (user still
pays for tokens generated after they pressed cancel). Wire a true
context-cancellation through.

**Why fifth.** Correctness issue that becomes a cost issue as users
lean on `/spawn` and long-running generations.

**File-by-file sketch.**

- `internal/agent/agent.go` — `Agent.Run` already accepts a ctx and
  passes it to `provider.ChatCompletion`; the existing provider
  implementations already honour ctx. The missing piece is App-side:
  currently `app.startTurn` uses `context.Background()`. Store a
  `cancelTurn context.CancelFunc` on App and have Ctrl+C call it.
- `internal/app/app.go` — add `cancelTurn` field, derive ctx in
  `startTurn`, store cancel, call on Ctrl+C-during-streaming.
- `internal/provider/*/` — audit each provider's cancellation story.
  OpenAI-compat already uses `http.NewRequestWithContext`; Ollama the
  same. Verify streamed reads unblock on ctx cancel (add tests that
  simulate a hung server).

**Agents.**
1. **Plan** — audit cancellation plumbing across providers; decide
   whether to add a double-Ctrl+C "force quit" behaviour.
2. **Implement** — App cancelTurn + any provider fixes.
3. **Tests** — fake slow server, assert cancel returns fast without
   consuming the full stream.
4. **Commit**.

**Estimated effort.** Single session, ~3 agents.

---

## Round 6 — User-customisable theme via `~/.packetcode/theme.toml`

**Scope.** Load a user theme file at startup that overrides the
Terminal Noir tokens in `internal/ui/theme/theme.go`. All component code
already goes through the token layer, so this is mostly plumbing +
loader + sensible defaults + validation.

**Why sixth.** Lower priority than correctness items above. Nice polish
once the core is stable.

**File-by-file sketch.**

- `internal/ui/theme/loader.go` — parse `~/.packetcode/theme.toml`,
  override exported vars (or refactor to a `Theme` struct the
  components read from — cleaner but a wider change).
- `internal/config/paths.go` — add `ThemePath()`.
- `docs/theming.md` — document every token and give a couple of
  example themes (light / high-contrast / solarized-ish).

**Agents.**
1. **Plan** — refactor decision: keep exported vars (`theme.AccentPrimary`
   etc.) or move to a `theme.Current()` getter? The refactor touches
   every component but is cleaner long-term.
2. **Implement** — loader + refactor if chosen.
3. **Docs + commit**.

**Estimated effort.** Single session, 3 agents — possibly two if the
refactor is deep.

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

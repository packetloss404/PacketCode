# Upstream research: opencode

Source: [`anomalyco/opencode`](https://github.com/anomalyco/opencode) — "The open source coding agent."
Reviewed: 2026-08-14, against `HEAD` (tree read at commit `4643e65`).

**Status of this document.** This is a clean-room specification. It records
*what opencode does and why*, in enough detail that packetcode can implement the
same capability from this document alone, without opencode's source open. Nothing
here is copied source. See [Licensing](#licensing) before touching upstream files.

---

## 1. What it is

| | |
|---|---|
| Language | TypeScript on Bun/Node, Effect-based |
| Shape | Turborepo monorepo, ~6,500 files, 30+ packages |
| License | MIT (© 2025 opencode) |
| Distribution | CLI, desktop app, web, VS Code extension, npm/Homebrew/native installers |
| Agents | `build` and `plan` (read-only) toggled with Tab, plus a general subagent |

The important structural fact: **opencode is not a TUI with a backend bolted on.
It is a server with several clients.** `packages/opencode/src/server` exposes a
typed HTTP API plus an SSE event stream; the TUI, desktop app, web UI, and VS Code
extension are all consumers of that one API. Everything below follows from that.

### Package map (the parts that matter)

| Package | Role |
|---|---|
| `core` | Domain: sessions, config, permissions, provider, skills, snapshots, PTY, plugins |
| `opencode` | The runtime: server, tools, LSP, MCP, ACP, agent, storage, sync |
| `llm` | Provider abstraction — protocols (anthropic-messages, gemini, bedrock-converse), cache policy, tool runtime |
| `schema` | ~55 versioned schema modules; the shared contract between server and every client |
| `protocol` | HTTP API groups, errors, middleware |
| `sdk`, `sdk-next`, `client` | Generated clients |
| `codemode` | Confined JS interpreter for tools-as-code |
| `http-recorder` | Record/replay HTTP + WebSocket cassettes for tests |
| `plugin` | JS plugin API |
| `tui`, `ui`, `console`, `desktop`, `web`, `session-ui` | Clients |
| `stats`, `enterprise`, `containers`, `slack`, `identity` | Ops and integrations |

---

## 2. Findings, ranked for packetcode

Each item states the capability, the evidence in opencode, why it matters *here*,
and the effort to build it in Go.

### Tier 1 — closes an open packetcode BACKLOG item

#### 1.1 External model catalog (models.dev)

- **Evidence:** `core/src/models-dev.ts`, `core/src/catalog.ts`, `schema/src/models-dev.ts`, `schema/src/model.ts`.
- **What it does:** provider IDs, model IDs, pricing (including cached-input and
  tiered rates), context windows, and capability flags come from
  [models.dev](https://models.dev) — a separate MIT community database served as
  JSON — rather than from hand-maintained tables in the agent.
- **Why here:** packetcode hand-maintains `pricing.go` across twelve provider
  packages. BACKLOG: *"Keep provider catalogs, pricing, context windows, and
  tool-capability metadata current; prefer live discovery when authoritative."*
  It also partly answers the MiniMax cached-input / long-context billing item,
  because the catalog carries tiered pricing as data instead of code.
- **Design to copy:** fetch on a schedule into the config dir, vendor a snapshot
  as the offline fallback, treat local tables as *overrides* rather than source
  of truth, and never block startup on the network.
- **Effort:** low. **Risk:** low — it degrades to today's behaviour if the fetch fails.
- **Note:** crush does the same thing with a different catalog (Catwalk). Two
  independent implementations of the same idea is a strong signal. See
  [`upstream-crush.md`](upstream-crush.md) §2.1.

#### 1.2 Record/replay cassettes for provider tests

- **Evidence:** `packages/http-recorder` (a published package with its own README),
  `packages/llm/test/recorded-{runner,scenarios,golden,websocket,utils}.ts`,
  `llm/test/fixtures/`.
- **What it does:** the first local test run calls the real provider and writes a
  deterministic JSON cassette; later runs replay it with no network. When
  `CI=true`, a *missing* cassette fails the test instead of silently recording.
  WebSocket cassettes preserve frame chronology and wait for the matching client
  frame before releasing more server frames.
- **Why here:** packetcode hand-rolls every SSE parser with zero vendor SDKs.
  That is a strength, and it is exactly the code that breaks silently when a
  provider changes its wire format. BACKLOG asks for *"opt-in live-provider
  contract tests that never run in ordinary CI"* and notes the MiniMax `<think>`
  path was *"implemented from the published tool-use guide, not from an observed
  response."* Cassettes turn that admission into a pinned regression test.
- **Design to copy:** the record-once/replay-always split, and the `CI=true`
  fail-on-missing rule — that rule is what stops cassettes from silently
  re-recording drift.
- **Effort:** low in Go — a custom `http.RoundTripper` plus JSON fixtures, ~300 LOC.
  Note that an MIT Go library already exists for this: `charmbracelet/x/vcr`
  (see [`upstream-crush.md`](upstream-crush.md) §4).
- **Risk:** low. Cassettes must be scrubbed of API keys on write.

#### 1.3 The question tool

- **Evidence:** `opencode/src/tool/question.ts` + `question.txt`,
  `opencode/src/question/{index,schema}.ts`,
  `server/routes/instance/httpapi/{groups,handlers}/question.ts`.
- **What it does:** the model asks the user a structured question mid-run — a
  prompt, a list of labelled options, `multiple: bool` for multi-select, and an
  automatically appended "type your own answer" choice so the model never invents
  an "Other" option. The answer returns as a normal tool result, so it lands in
  the transcript as part of the conversation.
- **Why here:** BACKLOG: *"Let background agents request user clarification
  through Agent View."* The critical design property is that the question is
  routed **through the server**, not the foreground TUI — that is what lets a
  background job ask a question at all. It maps directly onto packetcode's
  existing `Approver` seam, which is already the "agent needs a human" interface.
- **Effort:** low–medium. **Risk:** low, but it needs a timeout/abandon path so a
  background job blocked on an unanswered question does not wedge the worker pool.

#### 1.4 Truncate for the model, persist in full

- **Evidence:** `opencode/src/tool/truncate.ts`, `truncation-dir.ts`,
  `core/src/tool-output-store.ts`.
- **What it does:** large tool output is written to disk in full; the model
  receives a capped excerpt plus an explicit truncation notice and a handle it can
  use to read more.
- **Why here:** BACKLOG: *"Add configurable model-facing caps for search, command,
  MCP, and artifact output."* A plain cap loses information; this keeps it. Fits
  `execute_command`'s incremental streaming and the MCP adapters directly.
- **Effort:** low. **Risk:** low. Needs a retention/prune policy.

### Tier 2 — capability gaps

#### 2.1 LSP client and post-edit diagnostics

- **Evidence:** `opencode/src/lsp/{client,language,launch,diagnostic,server}.ts`,
  `opencode/src/tool/lsp.ts`.
- **What it does:** launches language servers per detected language, exposes
  definitions/references/symbols as a tool, and — the more valuable half — runs
  diagnostics after every edit and appends them to the tool result, so the model
  sees that it broke the build without the user saying so.
- **Why here:** `internal/tools/code_intelligence.go` is genuinely AST-grade for
  Go and lexical heuristics everywhere else. LSP generalises it to every language
  with a server.
- **Reusable asset:** `lsp/language.ts` is an extension → server → install-command
  table. That is data, and useful independent of the implementation.
- **Effort:** medium. packetcode's MCP client already proves the stdio JSON-RPC
  competence needed. **Risk:** medium — server lifecycle, crash handling, and
  "no server installed" must degrade to today's behaviour rather than erroring.
- **Note:** crush uses an MIT Go LSP library (`charmbracelet/x/powernap`) for
  exactly this. That is the shortest path — see [`upstream-crush.md`](upstream-crush.md) §2.2.

#### 2.2 Git shadow-repo snapshots instead of per-file backups

- **Evidence:** `opencode/src/snapshot/index.ts`, `core/src/session/revert.ts`,
  `schema/src/{revert,file-diff}.ts`.
- **What it does:** a *separate* git dir under the data path
  (`<data>/snapshot/<project-id>/<hash-of-worktree>`) with `--work-tree` pointed
  at the project. `track()` commits a snapshot per user message; `restore`,
  `revert`, `diff`, and `diffFull` operate on those commits. 7-day prune, a 2 MB
  limit, and explicit `core.autocrlf=false`, `core.longpaths=true`,
  `core.symlinks=true`, `core.quotepath=false` flags. The user's own git state is
  never touched — no commits, no index changes, no stash.
- **Why here:** packetcode's `/undo` is per-file backups (`internal/tools/backup.go`,
  `internal/session/backup.go`). Snapshots give message-level revert, a real diff
  view, and correct handling of deletions, renames, and untracked files that
  per-file backup misses. It composes with the existing job worktrees rather than
  competing with them.
- **Effort:** medium. **Risk:** low — it is additive; keep per-file backup until
  snapshots are proven. The Windows git flags above are directly relevant to the
  in-flight `internal/procrun/process_windows.go` work.

#### 2.3 Skills

- **Evidence:** `core/src/skill.ts`, `core/src/skill/{discovery,guidance}.ts`,
  `opencode/src/tool/skill.ts` + `skill.txt`, `opencode/src/skill/discovery.ts`.
- **What it does:** progressive disclosure. The system prompt carries only a
  `<available_skills>` list of name + description; a `skill` tool loads the full
  body on demand. Skills are permission-filtered per agent. Discovery can pull
  skill bundles from a URL, with hard path-safety validation on every segment
  (no `..`, no absolute paths, no URL-shaped names, decoded before checking).
- **The delta mechanism:** the guidance block emits **deltas** — when the
  available set changes mid-session it sends "this list supersedes the previous
  list" rather than resending the whole block, and a "no longer available" notice
  on removal, so prompt-cache prefixes survive a changing skill set.
  **Verification (2026-08-14) says packetcode does not need this:** against the
  actual fingerprint inputs — `CachePrefixFingerprint(a.systemPrompt, req.Tools)`
  with `StablePrefixMessages` counting the single system message
  (`internal/agent/agent.go:250-263`) — a skill set loaded once from disk at
  startup never changes mid-session, so the delta path would be dead code. Build
  the static block; revisit only if skills become dynamically filtered. Likewise
  "permission-filtered per agent" has no analogue: packetcode has no per-agent
  permission profiles, only the read-only/write background-job split.
- **Why here:** packetcode has slash commands, workflows, and hooks, but no
  progressive-disclosure skills. Cheap: a markdown loader plus one tool.
- **Effort:** low. **Risk:** low, if remote discovery is left out of v1.

#### 2.4 A todo tool

- **Evidence:** `opencode/src/tool/todo.ts`, `todowrite.txt`.
- **Why here:** packetcode has none (`grep -ril todo internal/*.go` → nothing).
  Cheap, materially improves steering on long turns, and gives Agent View
  something structured to render for background jobs.
- **Effort:** trivial.

### Tier 3 — architecture direction (matches PACKETCOMPUTERS)

#### 3.1 Server-first split

- **Evidence:** `opencode/src/server/routes/instance/httpapi/` — 21 route
  `groups/`, matching `handlers/`, and `middleware/{authorization,fence,
  instance-context,proxy,workspace-routing,compression,cors-vary,error,
  schema-error}.ts`; plus `packages/protocol`, `packages/sdk`, `sdks/vscode`.
- **Why here:** PCMP4/PCMP5 is a loopback-only daemon with RPC and heartbeat.
  This is the same move, already load-bearing for four clients. Worth studying
  *before* the RPC shape is frozen.
- **Specifically worth lifting:**
  - typed route groups with a generated client, so clients cannot drift;
  - `middleware/fence.ts` — the "this instance owns this workspace" guard, the
    analogue of packetcode's immutable computer/root binding;
  - `middleware/instance-context.ts` + `authorization.ts` — where the
    loopback-only trust rule would live.
- **Effort:** high, but it is work PACKETCOMPUTERS already commits to.

#### 3.2 Versioned public event manifest

- **Evidence:** `core/src/public-event-manifest.ts`,
  `schema/src/{event-manifest,durable-event-manifest,legacy-event}.ts`,
  `specs/v2/schema-changelog.md`.
- **What it does:** the event and schema surface is treated as a published
  contract with its own changelog, plus explicit legacy-event and durable-event
  variants for persistence and replay.
- **Why here:** BACKLOG: *"Define compatibility and migration policy for config,
  sessions, persisted jobs, workflow TOML, and MCP definitions."* This is
  precisely the artifact that item asks for, and it must exist before the daemon
  ships or the daemon becomes the compatibility problem.
- **Effort:** low as a document, medium as enforced schema versioning.

#### 3.3 Event-sourced sessions with projectors

- **Evidence:** `core/src/session/{store,projector,history,message-updater,
  context-epoch,compaction,run-coordinator}.ts`, `session/sql.ts`,
  `server/init-projectors.ts`.
- **Why here:** PCMP9 is "jobs survive restart." If session and job state is an
  append-only log with projectors, reconcile becomes *replay* rather than bespoke
  resume logic — and the requirement that anything not genuinely resumed be
  reported as abandoned becomes provable, because you can see exactly where the
  log stops. `context-epoch.ts` is also the right primitive for tracking what the
  model has actually seen across compaction.
- **Effort:** high. **Risk:** high — this is a persistence rewrite. Sequence it
  with PCMP9, not before.

#### 3.4 CodeMode — tools as code

- **Evidence:** `packages/codemode` (README, `src/interpreter`, `src/stdlib`,
  `src/openapi`, 8 test files), `opencode/src/tool/code-mode.ts`.
- **What it does:** instead of N JSON tool-call round trips, the model writes a
  small JavaScript program that may call **only** the host-supplied tools. It can
  sequence, branch, loop, and run independent calls in parallel. The program gets
  no ambient filesystem, process, network, module, or application authority — the
  tools *are* the capability set. Tool schemas may be validating schemas or
  render-only JSON Schema (the natural shape for MCP-provided tools). Failures
  come back as diagnostics rather than exceptions.
- **Why here:** real token and latency savings in MCP-heavy sessions, and the
  capability-confinement model is philosophically identical to packetcode's
  symlink-jailed `safefs.go` — the tool set is the sandbox boundary.
- **Effort:** high in Go. Needs an embedded interpreter; Starlark
  (`go.starlark.net`) is the pragmatic choice over goja because it is
  deterministic, sandboxed by default, and has no JS-ecosystem surface.
- **Verdict:** spike, not commitment.

### Tier 4 — cheap, worth doing while nearby

- **Prompts as embedded files.** Every tool has a sibling `.txt` prompt
  (`tool/read.txt`, `shell/shell.txt`, …) rather than a Go/TS string literal. Makes
  prompt changes reviewable in diffs and lets non-code changes skip a rebuild
  reasoning pass. `shell.txt`'s git/GitHub section is a well-tuned reference for
  what a shell tool should tell the model about committing.
- **`cache-policy.ts`** — explicit cache-breakpoint policy, worth diffing against
  `internal/provider/cache.go` for the cached-input telemetry BACKLOG item.
- **`tool/external-directory.ts`** — an explicit "this directory outside the
  workspace is pre-approved" concept, with the temp dir pre-approved by default.
  packetcode's jail is stricter; this is the escape valve done deliberately.
- **ACP parity.** opencode's `src/acp/` has 12 files — profiles, config options,
  usage reporting, content, permission mapping — against packetcode's single
  `internal/acp/server.go`. Cheap to scan for missing protocol surface.
- **`core/src/github-copilot/` + `oauth/`, `credential/`** — a concrete example of
  the "sanctioned subscription-backed provider" path that packetcode's BACKLOG
  gates on the provider publishing a third-party integration route.

---

## 3. Explicitly not worth taking

| Thing | Why not |
|---|---|
| Effect runtime and its idioms | Wrong language, and packetcode's stdlib discipline is a feature |
| Bundled ripgrep (`core/src/ripgrep`) | Downloads a binary at runtime; breaks the single-static-binary property for a search tool packetcode already has in Go |
| Session sharing / web links (`core/src/share`) | Product surface packetcode has not chosen |
| Desktop/Electron app, `packages/web`, `session-ui` | Out of scope |
| `server/mdns.ts` network discovery | Conflicts with loopback-only daemon trust |
| `containers`, `enterprise`, `slack`, `identity`, `stats` | Ops surface for a hosted product |
| PTY package (`core/src/pty`) | packetcode has a committed no-PTY safety contract. Noted only because the *ticket-brokered* design (server issues a ticket, client attaches) is how to do it later without breaking the inline/native-scrollback contract |

---

## 4. Licensing

opencode is **MIT**, © 2025 opencode. That is maximally permissive: code may be
copied, modified, and redistributed, including into a competing product, provided
the copyright notice and permission notice are retained.

Practical position for packetcode:

1. **Verbatim copying is legally fine but practically useless.** It is
   TypeScript/Effect; packetcode is stdlib-only Go. Port the design, not the file.
2. **Two things are genuinely copyable text:** the `lsp/language.ts` server table
   and the tool prompt `.txt` files. If either is taken, add a
   `THIRD-PARTY-NOTICES` entry naming opencode and reproducing the MIT notice.
   Prompt text is copyrightable expression — this is the one category where
   "we rewrote it in Go" does not apply.
3. **models.dev is a separate project** consumed over its API and carries its own
   terms; do not assume opencode's licence covers it.

See [`upstream-adoption-plan.md`](upstream-adoption-plan.md) §Licensing for the
combined position across both upstreams, which is materially stricter because
crush is not MIT.

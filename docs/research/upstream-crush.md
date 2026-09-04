# Upstream research: crush

Source: [`charmbracelet/crush`](https://github.com/charmbracelet/crush) — "Glamourous agentic coding for all."
Reviewed: 2026-08-14, against `main` (tree read at 1,098 files; 27.4k stars).

> **Point-in-time research.** Packetcode implementation-status statements in
> this document describe the 2026-08-14 tree. Several cited gaps have since
> shipped; use [BACKLOG.md](../../BACKLOG.md) for current work and the
> [adoption plan](upstream-adoption-plan.md) for its supersession note. The
> clean-room evidence below is intentionally unchanged.

**Status of this document.** This is a clean-room specification. It records
*what crush does and why*, in enough detail that packetcode can implement the same
capability from this document alone, without crush's source open. Nothing here is
copied source. **Read [Licensing](#5-licensing) first — crush is not MIT, and the
constraint is real for packetcode specifically.**

---

## 1. What it is

| | |
|---|---|
| Language | Go 1.26, single binary |
| Shape | Flat `internal/` tree, ~1,100 files |
| License | **FSL-1.1-MIT** (Functional Source License, MIT Future License) |
| TUI | Bubble Tea v2 / Lipgloss v2 / Glamour v2 — the same stack packetcode uses |
| Distribution | Homebrew, npm, apt, yum, winget, scoop, Nix/NUR, FreeBSD pkg, AUR |
| Platforms | macOS, Linux, Windows (PowerShell *and* WSL), Android, FreeBSD, OpenBSD, NetBSD |

**Crush is packetcode's closest structural sibling.** Same language, same TUI
stack, same problem, comparable maturity. That makes it the higher-value of the
two upstreams for design comparison and the more constrained one for code reuse.

Three of its subsystems are factored out into **separately licensed, permissive Go
modules** that packetcode can import outright. That is the single most actionable
finding in this document — see §4.

### Package map (the parts that matter)

| Package | Role |
|---|---|
| `internal/agent` + `agent/tools` (101 files) | Agent loop, coordinator, loop detection, hooked tools, the full tool suite |
| `internal/shell` | **In-process POSIX shell interpreter** with Go coreutils, builtins, background jobs, jq |
| `internal/lsp` | LSP client/manager/handlers over `x/powernap` |
| `internal/server` + `client` + `proto` | Client/server split over unix socket / Windows named pipe, SSE events, swagger |
| `internal/backend` | Service layer behind the protocol: agent, session, permission, question, filetracker |
| `internal/db` | SQLite (sqlc-generated) — sessions, messages, files, read-files, stats, migrations |
| `internal/skills` | Skill catalog, manager, tracker, embedded builtin skills |
| `internal/hooks` | `PreToolUse` hooks, Claude Code-compatible |
| `internal/discover` | Local runtime discovery: Ollama, LM Studio, llama.cpp, LiteLLM, OMLX |
| `internal/config` + `shellconfig` | Layered config, scopes, atomic writes, JSON-schema generation |
| `internal/oauth` | Copilot, MCP OAuth, callback server |
| `internal/csync` | Concurrency helpers, incl. a versioned map |
| `internal/ui/*` | `diffview`, `dialog`, `completions`, `notification`, `anim`, `chat` |
| `internal/{projects,workspace,lock,update,filetracker,question,pubsub}` | Supporting services |

---

## 2. Findings, ranked for packetcode

### Tier 1 — high value, and crush proves it works in Go

#### 2.1 In-process shell with Go coreutils (the Windows-parity answer)

- **Evidence:** `internal/shell/{shell,run,dispatch,expand,coreutils,coreutils_exec,builtins_registry,background,stream,jq}.go`, `exec_unix.go`/`exec_windows.go`, `doc.go`.
- **What it does:** commands are not handed to `sh -c`. Crush embeds
  `mvdan.cc/sh/v3` (parser + interpreter) and runs the command **in-process**, with:
  - **Go-implemented coreutils**, enabled by default on Windows only
    (`CRUSH_CORE_UTILS` overrides). So `ls`, `cat`, `grep`, and friends behave
    identically on a machine with no Git-Bash, no busybox, and no WSL.
  - A **builtins registry** and a Go `jq` (via `gojq`) as a shell builtin.
  - **Persistent state across calls** — `export FOO=bar` in one call is visible in
    the next; working directory and environment survive between tool invocations.
  - A **script dispatch middleware** that probes the first 128 bytes of any
    path-prefixed argv[0] and routes it three ways: shebang → exec the named
    interpreter (resolving the basename via PATH as a fallback, so `#!/bin/bash`
    works on Windows); binary magic (MZ/ELF/Mach-O) or an embedded NUL → hand to
    the default exec handler; otherwise treat it as shell source and run it in a
    nested interpreter **that reuses the same handler stack, so deny rules apply
    recursively into sourced scripts**.
  - **Background jobs** with `job_output` and `job_kill` tools.
- **Why here:** packetcode has live, in-flight work in
  `internal/procrun/process_windows.go` and `process_posix.go`, and a new
  `process_windows_test.go`. This is the design that makes Windows a first-class
  target instead of a permanent source of divergence. The recursive deny-rule
  property is also a genuine security improvement over spawning a shell: today a
  denied command can be trivially laundered through `./script.sh`.
- **Effort:** medium-high. `mvdan.cc/sh/v3` is BSD-3 and importable; the coreutils
  set and the dispatch middleware are the work.
- **Risk:** medium. It changes the semantics of `execute_command`, and the
  approval-policy engine must be re-validated against the new execution path —
  that revalidation is the point, not a side effect.
- **Conflict check:** compatible with packetcode's no-PTY contract. This is *not*
  a PTY; it is an interpreter. Interactive programs remain unsupported either way.

#### 2.2 LSP as a first-class tool family

- **Evidence:** `internal/lsp/{client,manager,handlers}.go`, `internal/lsp/util/edit.go`,
  `internal/app/lsp_events.go`, and eight tools:
  `lsp_definition`, `lsp_symbols`, `lsp_references`, `lsp_call_hierarchy`,
  `lsp_rename`, `lsp_replace_symbol`, `lsp_restart`, `diagnostics`.
  Config: `auto_lsp` (default true) sets up servers automatically from **root
  markers**; `debug_lsp` for logging.
- **What it does:** goes beyond read-only navigation into **LSP-driven mutation** —
  `lsp_rename` and `lsp_replace_symbol` let the model perform a semantically
  correct rename or symbol replacement instead of a textual patch. `lsp_restart`
  gives the model a recovery path when a server wedges. Diagnostics are a tool
  *and* an event stream (`app/lsp_events.go`).
- **Why here:** packetcode's `code_intelligence.go` is AST-grade for Go and
  lexical elsewhere. crush gets every language, and `lsp_replace_symbol` is a
  strictly better primitive than `patch_file` for refactors — it cannot produce a
  syntactically broken result.
- **Shortest path:** `charmbracelet/x/powernap` is **MIT** and importable —
  `pkg/lsp` (client + protocol), `pkg/registry` (multi-server lifecycle),
  `pkg/config` (server configs), `pkg/transport`. See §4.
- **Effort:** medium (low if powernap is imported). **Risk:** medium — lifecycle
  and "no server installed" must degrade cleanly.
- **Cross-ref:** opencode reaches the same conclusion from the other direction —
  see [`upstream-opencode.md`](upstream-opencode.md) §2.1, which emphasises
  *automatic post-edit diagnostics* over explicit tools. Take both halves.

#### 2.3 Loop detection

- **Evidence:** `internal/agent/loop_detection.go`, `loop_detection_test.go`.
- **What it does:** over a sliding window of the last 10 steps, it computes a
  SHA-256 signature per step from `(tool name, tool input, tool output)` for every
  tool call in that step, paired with its result by `ToolCallID`. If any signature
  appears more than 5 times in the window, the run is looping and is stopped.
  Steps with no tool calls are skipped.
- **Why here:** packetcode's only guard is the 25-iteration cap in
  `internal/agent/agent.go`. A cap catches runaway length; it does not catch a
  model that reads the same file and makes the same failing edit four times in a
  row — the user pays for 25 iterations of nothing and then gets a truncated run.
  Including the *output* in the signature is the subtle part: identical calls with
  differing output are progress, identical calls with identical output are not.
- **Effort:** trivial — under 100 lines against the existing `callAssembler`.
- **Risk:** very low. This is the single best value-per-line item across both
  upstreams.

#### 2.4 File read tracking / stale-write protection

- **Evidence:** `internal/filetracker/service.go`, `internal/db/read_files.sql.go`,
  `internal/backend/filetracker.go`.
- **What it does:** records per-session when each file was read (`RecordRead`,
  `LastReadTime`, `ListReadFiles`), so edits can be refused when the file changed
  on disk after the model last read it.
- **Why here:** packetcode's `safefs.go` is TOCTOU-aware about *paths*; this is
  TOCTOU-awareness about *content*, which is the failure mode that actually bites
  during long sessions with a running formatter, a rebase, or a second agent in a
  worktree. It also gives Agent View a factual "files touched this session" list.
- **Effort:** low (packetcode can key it off the existing session store rather
  than adding SQLite). **Risk:** low.

#### 2.5 Local runtime auto-discovery

- **Evidence:** `internal/discover/{discover,enricher,ollama,lmstudio,llamacpp,litellm,omlx}.go`.
- **What it does:** probes well-known local endpoints, lists models, then
  **enriches** them with capability metadata from each runtime's native API
  (Ollama's `/api/show`, LM Studio's `/api/v1/models`) — including correctly
  stripping the `/v1` suffix from an OpenAI-compatible base URL, because the
  enrichment endpoints live at the server root, not under `/v1`. 10-second client
  timeout so a dead endpoint cannot hang startup.
- **Why here:** packetcode ships Ollama only, and BACKLOG has *"add optional
  MLX/local-runtime backends only if they can match the native tool and streaming
  contracts"* — OMLX is exactly that case, already solved. LM Studio and llama.cpp
  are pure upside for local-first users.
- **Effort:** low-medium, one file per runtime, independently shippable.
- **Risk:** low.

### Tier 2 — architecture and platform

#### 2.6 Client/server split over a local socket

- **Evidence:** `internal/server/{server,socket,net_other,net_windows,events,recover,logging}.go`,
  `internal/client/{client,proto,dial_other,dial_windows}.go`,
  `internal/proto/*` (14 files), `internal/swagger/`,
  `internal/db/datadirlock.go`, `internal/lock/`,
  tests: `multiclient_test.go`, `e2e_agent_test.go`, `sessions_isbusy_test.go`,
  `clientserverrace/`, `restart_stale_test.go`.
- **What it does:** the agent runs in a server process; the TUI is a client. Unix
  domain socket on POSIX, **named pipe on Windows** (`Microsoft/go-winio`). SSE
  event stream to multiple simultaneous clients. Explicit **stale-socket
  detection and classification** (`socket_classify.go`) so a crashed server does
  not permanently block startup. A data-dir lock prevents two servers racing on
  one project. The whole protocol is documented in `swagger.json`/`swagger.yaml`.
- **Why here:** this is PCMP4/PCMP5 — loopback-only daemon RPC plus heartbeat —
  implemented in Go, with the failure modes already found. Three things to lift
  before packetcode's RPC shape is frozen:
  1. **Unix socket / named pipe rather than a TCP port.** packetcode's
     PCMP4 requirement is "must refuse non-loopback binds." A socket or named pipe
     satisfies that structurally rather than by validation, and it inherits
     filesystem permissions instead of needing an auth token.
  2. **Stale-socket classification.** The difference between "server is running,"
     "server died and left the socket," and "another user owns it" is exactly the
     ambiguity PCMP9's abandoned/indeterminate state has to resolve.
  3. **Multi-client from day one.** `multiclient_test.go` and
     `sessions_isbusy_test.go` encode the semantics of two clients on one session.
- **Effort:** high — but it is committed PACKETCOMPUTERS work.
- **Cross-ref:** opencode solves the same problem with HTTP + typed route groups
  and a generated SDK ([`upstream-opencode.md`](upstream-opencode.md) §3.1).
  Crush's socket approach fits packetcode's loopback-only rule better; opencode's
  typed-contract discipline is the better half to copy on top of it.

#### 2.7 SQLite session storage

- **Evidence:** `internal/db/{connect,db,models,querier}.go`, `sql/` (5 files),
  `migrations/` (7), sqlc-generated `sessions.sql.go`, `messages.sql.go`,
  `files.sql.go`, `read_files.sql.go`, `stats.sql.go`; pure-Go drivers behind
  build tags (`connect_modernc.go`, `connect_ncruces.go` — no cgo).
- **What it does:** sessions, messages, file history, and usage stats in SQLite
  with versioned migrations. Enables `crush stats`, transcript search, and cheap
  partial reads of long sessions.
- **Why here:** packetcode persists sessions and jobs as atomic JSON
  (temp-file-then-rename). That is honest and simple, and it is the thing that
  makes BACKLOG items "transcript search/filter," "persist request-level
  occupancy samples," and PCMP9 "resume or reconcile active background jobs"
  awkward — all three want indexed, partial, concurrent access.
- **Verdict:** worth doing eventually, **not now**. It is a persistence rewrite
  with a migration story for existing sessions and jobs, and packetcode's BACKLOG
  correctly puts "define compatibility and migration policy" first. Note that the
  pure-Go driver choice (modernc/ncruces behind build tags) is what preserves the
  single-binary, cgo-free property — that constraint is non-negotiable for
  packetcode too.

#### 2.8 Hooks that can mutate, not just observe

- **Evidence:** `internal/hooks/{hooks,input,runner}.go`,
  `internal/agent/hooked_tool.go`, `docs/hooks/README.md`, `docs/hooks/FUTURE.md`,
  `docs/hooks/examples/`.
- **What it does:** crush supports only `PreToolUse` today (fewer event types than
  packetcode), but the *semantics* are stronger. A hook can:
  - **block** a tool call (`git push -f`),
  - **rewrite tool input** before execution (rewrite `node` → `deno`, scrub
    secrets out of a command),
  - **inject context** into the model's view when a tool is called,
  - **auto-approve** a call, skipping the permission prompt.
  Hooks are matched by regex on tool name, are Claude Code-compatible in format,
  and **run in parallel for speed but compose in config order for determinism** —
  that pairing is the design detail worth copying.
- **Why here:** packetcode has more hook events (three vs one) and already
  supports block and context-injection, so the delta is rewrite-input and
  auto-approve. A cheaper adjacent gap found during verification: packetcode's
  matcher is **exact tool name or `*`** (`internal/hooks/hooks.go:189`), not
  regex.
- **Correction (verified 2026-08-14):** auto-approve does **not** integrate with
  `internal/permissions/policy.go` for free. In packetcode the `PreToolUse` hook
  fires *after* the policy decision (`internal/agent/agent.go:392-473`), so a
  rewrite bolted onto that call site would be evaluated against no policy at all.
  Integration requires reordering the hook above the first `Decide`. See
  [`upstream-adoption-plan.md`](upstream-adoption-plan.md) §5.3 for the full
  three-rule mitigation.
- **Effort:** low-medium. **Risk:** medium — a hook that can rewrite tool input is
  a privilege-escalation surface; the rewritten input must be re-validated
  through the policy engine, not trusted because a hook produced it.
- **Do not adopt** the parallel-execution half: packetcode's hooks are already
  sequential and deterministic at realistic hook counts.

#### 2.9 Skills, with builtin skills that document the app itself

- **Evidence:** `internal/skills/{manager,catalog,tracker,embed,skills}.go`,
  `internal/skills/builtin/{crush-config,crush-hooks,jq}/SKILL.md`,
  `.agents/skills/{builtin-skills,shell-builtins}/SKILL.md`,
  `internal/proto/skills.go`.
- **The idea worth stealing:** crush ships **embedded builtin skills that teach
  the agent how to configure crush** — a `crush-config` skill and a `crush-hooks`
  skill, so a user can say "set up a hook that blocks force-pushes" and the agent
  reads its own documentation and edits `crush.json` correctly.
- **Why here:** packetcode has `docs/` covering configuration, workflows, MCP
  trust, themes, hooks, and statusline. Embedding those as skills turns the manual
  into agent capability at near-zero cost, and it is a genuinely differentiating
  onboarding story for a pre-1.0 tool with a large config surface.
- **Also note:** per-workspace `Manager` with an explicit `WithGlobalMirror`
  option and a comment stating exactly when mirroring to package globals is safe
  (single-workspace processes only, never the multi-workspace server). That is the
  right level of rigour for packetcode's daemon transition too.
- **Effort:** low. **Cross-ref:** opencode's skill guidance deltas
  ([`upstream-opencode.md`](upstream-opencode.md) §2.3) are the better wire format;
  crush's builtin-skills idea is the better content strategy. Take both.

### Tier 3 — tools and UX worth a look

- **`multiedit`** — several edits to one file in a single validated call. Reduces
  round trips and makes partial-failure semantics explicit. packetcode has
  `patch_file` only.
- **`fetch` / `agentic_fetch` / `download` / `web_search` / `sourcegraph`** —
  packetcode has no web tools at all (`grep -ril webfetch internal` → nothing).
  `agentic_fetch` is the interesting one: an inner agent loop that fetches and
  *summarises against the question* rather than dumping HTML into context.
  HTML→markdown via `JohannesKaufmann/html-to-markdown`, DOM via `goquery`.
- **`crush_info` / `crush_logs`** — tools that let the agent read its own version,
  config, and logs. Only the first half ports: `packetcode doctor --check
  <domain> --json` already emits structured results, so `crush_info` is a thin
  adapter over it; packetcode has no general log file for `crush_logs` to read,
  only MCP stderr tails behind `/mcp logs`.
- **`todos`** — same conclusion as opencode; packetcode has none.
- **`list_mcp_resources` / `read_mcp_resource` / `mcp/prompts.go`** — MCP
  resources and prompts, which packetcode's BACKLOG defers until *"their context
  and trust model is defined."* crush's implementation is a reference for the
  shape, not for whether to ship it.
- **MCP OAuth** — `internal/oauth/mcp/` + `callback/` (10 files): a local callback
  server for OAuth against HTTP MCP servers. Directly relevant to
  `docs/mcp-http-trust-contract.md` and the deferred Streamable-HTTP MCP work.
- **`internal/ui/diffview`** — a substantial diff component (chroma-highlighted,
  extensive golden testdata). Worth comparing against packetcode's existing diff
  viewer for side-by-side/word-level rendering.
- **`internal/csync`** — concurrent map/slice/value plus a **versioned map** whose
  version counter lets the TUI skip re-renders when nothing changed. Small,
  useful, and packetcode's `internal/jobs` snapshot fan-out solves a similar
  problem.
- **`internal/update`** — in-app update check and self-update, surfaced over the
  protocol so client/server-mode TUIs see it too. packetcode ships via
  goreleaser + `install.sh` with no update path.
- **`internal/agent/notify`** — desktop notification on long-run completion
  (`gen2brain/beeep`).
- **Config as JSON Schema** — `internal/cmd/schema.go` generates a JSON Schema for
  `crush.json` from Go structs via `invopop/jsonschema`, published by a
  `schema-update.yml` CI workflow. **Verification says skip this for packetcode:**
  the config is TOML, so a JSON Schema only reaches editors via Taplo and needs a
  `#:schema` directive written into the atomically-written config, and generating
  it adds a dependency. `doctor --check config` already covers validation.
- **`internal/shellconfig`** — CLI flags/env layered over file config with
  explicit precedence tests per domain (provider, model, lsp, mcp, hooks,
  permissions, options). packetcode's config precedence deserves the same
  treatment as its own test suite.

---

## 3. Explicitly not worth taking

| Thing | Why not |
|---|---|
| `internal/herdr`, `agent/hyper`, `oauth/hyper`, `config/hyper.go` | Charm's hosted service integration |
| `charm.land/fantasy` as the provider layer | packetcode's zero-SDK, hand-rolled SSE parsers are a deliberate differentiator and are better tested than a swap would preserve. Useful as a *reference* for wire shapes, not as a dependency |
| Bubble Tea v2 migration (as a crush-derived idea) | Already independently on packetcode's BACKLOG; crush confirms it is the right call, nothing more |
| `internal/dns/android.go`, Android support | Out of scope |
| CLA infrastructure, `machineid`, telemetry events (`internal/event`) | Product/ops surface packetcode has not chosen |
| Sourcegraph tool | Depends on a third-party code-search service |

---

## 4. The three importable libraries (most actionable finding)

Crush's most valuable subsystems live in **separate modules under permissive
licences**. These are not crush code — they are dependencies crush uses, published
by Charm for anyone. packetcode can import or vendor them outright.

| Module | Licence | What it gives packetcode |
|---|---|---|
| [`charmbracelet/catwalk`](https://github.com/charmbracelet/catwalk) — `charm.land/catwalk/pkg/catwalk` | **MIT** | A maintained Go catalog of LLM providers and models: IDs, pricing, context windows, capabilities. The Go-native answer to the models.dev item in [`upstream-opencode.md`](upstream-opencode.md) §1.1 — same idea, already typed, already in Go |
| [`charmbracelet/x/powernap`](https://github.com/charmbracelet/x/tree/main/powernap) | **MIT** | A complete LSP client: `pkg/lsp` (client + generated protocol), `pkg/registry` (multi-server lifecycle), `pkg/config`, `pkg/transport`. Removes most of the cost of §2.2 |
| [`charmbracelet/x/vcr`](https://github.com/charmbracelet/x/tree/main/vcr) | **MIT** | HTTP record/replay with matchers, hooks, and a marshaler — `recorder.go`, `matcher.go`, `hooks.go`. This is the Go implementation of the cassette-testing item in [`upstream-opencode.md`](upstream-opencode.md) §1.2 |

Two further permissive dependencies matter for §2.1:

- `mvdan.cc/sh/v3` — **BSD-3-Clause** — the shell parser/interpreter.
- `github.com/itchyny/gojq` — **MIT** — pure-Go jq.

**Both upstreams independently converged on an external model catalog and on
cassette-based provider tests.** Two independent teams solving the same problem
the same way, with an MIT Go library sitting at the end of it, is as strong a
signal as this kind of research produces.

---

## 5. Licensing

**Crush is FSL-1.1-MIT** — the Functional Source License with an MIT future
grant. This is materially different from opencode's MIT and it matters for
packetcode specifically.

What the licence says:

- Use, copy, modify, create derivative works, and redistribute are granted **only
  for a Permitted Purpose**.
- A **Competing Use** is not a Permitted Purpose. A Competing Use means making the
  software available to others in a *commercial* product or service that
  substitutes for crush, or for any other product or service Charm offers using it.
- Each version converts to **MIT two years after that version's release date**.

The practical position for packetcode:

1. **Do not copy crush source into packetcode.** packetcode is a terminal AI
   coding agent — a direct functional substitute for crush. Even if packetcode
   stays non-commercial (which keeps it outside the literal Competing Use
   definition), it is the wrong thing to be arguing about later. Treat crush as
   read-only reference.
2. **Do not copy its prompt text either.** `internal/agent/tools/*.md`,
   `*.md.tpl`, `internal/agent/templates/*`, and `SKILL.md` files are
   copyrightable expression, and "we rewrote the Go around it" does not launder
   the prompt.
3. **Ideas, architectures, and techniques are not copyrightable.** Copyright
   protects expression, not function, method, or system. Reading crush and
   independently implementing "detect repeated tool-call signatures over a sliding
   window" in packetcode's own code is outside copyright's reach. Near-verbatim
   transliteration — same structure, same identifiers, same comments, mechanically
   converted — is a derivative work and is not.
4. **The discipline that keeps the line clean:** these research documents are the
   spec. Implement from the document, with the upstream source closed. If a
   detail is missing from the document, add it to the document in your own words
   first, then implement. That is the whole point of writing these files.
5. **The importable libraries in §4 are unaffected** — catwalk, powernap, and vcr
   are MIT, published as standalone modules, and may be imported as ordinary
   dependencies with their notices retained.

See [`upstream-adoption-plan.md`](upstream-adoption-plan.md) for the combined
position and the sequenced plan.

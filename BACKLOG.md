# Backlog

packetcode is pre-1.0. This file contains only work that has not shipped; completed work belongs in [CHANGELOG.md](CHANGELOG.md).

Several items below came from reading two upstream agents against this tree.
[`docs/research/upstream-adoption-plan.md`](docs/research/upstream-adoption-plan.md)
carries the evidence, effort, risk, and sequencing, plus the list of things
deliberately **not** taken and why; the per-upstream notes are
[`upstream-opencode.md`](docs/research/upstream-opencode.md) and
[`upstream-crush.md`](docs/research/upstream-crush.md). crush is FSL-licensed
and packetcode competes with it, so those documents are written as clean-room
specifications: implement from them with the upstream source closed, and never
copy its code or prompt text.

## v1 Release Readiness

- Automate signed/notarized macOS, Linux, and Windows release artifacts and checksum verification.
- Define compatibility and migration policy for config, sessions, persisted jobs, workflow TOML, and MCP definitions. Persisted jobs are done (records
  carry `format_version` and refuse a newer one); config, sessions, workflow
  TOML, and MCP definitions remain. Write it as a published contract with its
  own changelog, and land it *before* any daemon work, since a daemon with
  clients becomes the compatibility problem.
- ~~Surface unreadable job records.~~ **Shipped 2026-08-14** with PCMP10, which
  made it urgent: an older build meeting the new `abandoned` state rejects the
  record, so without this the job simply vanished from the UI. Startup now warns
  on stderr alongside the recovered count, and `doctor` gained the
  `state.jobs.records` check. Both bound their per-record listing to 3 so a
  corrupt directory cannot flood the terminal, and both go through the new
  read-only `jobs.InspectRecords` — `NewManager` reconciles and rewrites
  records, so a diagnostic built on it would have marked a live instance's
  in-flight jobs abandoned just by being run.
- Add end-to-end smoke coverage for first-run setup, provider switching, session resume, approvals, background jobs, workflows, and MCP.
- Keep provider catalogs, pricing, context windows, and tool-capability metadata
  current; prefer live discovery when authoritative. **Decided 2026-08-14: fetch
  models.dev with stdlib, do not import `charm.land/catwalk`.** Catwalk declares
  `go 1.26.6` against this repo's 1.24.2, pulls prometheus and protobuf for what
  is 70 lines of stdlib HTTP, has no `mistral` entry, prices MiniMax-M3 at the
  long-context tier, and has no tiered-pricing field at all — so it structurally
  cannot express the MiniMax billing item below. models.dev carries the tiers
  and cached rates verbatim. Ship a trimmed embedded snapshot as the offline
  fallback (~150 KB) so the single-binary property holds and startup never
  blocks on the network; precedence is user config > live catalog > snapshot >
  today's hand tables.
- Add opt-in live-provider contract tests that never run in ordinary CI.
  **Decided 2026-08-14: own the recorder** (~300-LOC `http.RoundTripper` plus
  JSON cassettes) rather than importing `charm.land/x/vcr` or `dnaeon/go-vcr` —
  the requirement is SSE frame chronology with controllable inter-frame timing
  so `StallGuard` is exercised, which a generic HTTP recorder does not give.
  Record on an env flag, replay always, and **fail on a missing cassette when
  `CI=true`**; without that rule cassettes silently re-record drift and assert
  nothing. Allow-list headers on write and test that the scrubber ran — a key
  committed inside a cassette is unrecoverable once pushed. Extend the existing
  `CODEX_LIVE=1` convention rather than inventing a second harness.
- ~~`TestDoctorPlainOutputDoesNotLeakSecrets` fails from environment
  contamination.~~ **Fixed 2026-08-26.** An ambient `OLLAMA_HOST` outranked the
  planted config, so the redaction assertion never saw the string it checks and
  the test guarded nothing. `isolateDoctorEnv` now clears the vendor variables
  too, not just the `PACKETCODE_` ones; `CODEX_HOME` had the same shape and was
  breaking two further doctor tests on any machine using Codex.
- `TestManager_TranscriptReadsLiveSubSessionWhileRunning` is flaky against its
  60s deadline, and it is not alone: a batch of `internal/jobs` and
  `internal/mcp` tests fail together under CPU load and pass in isolation. They
  cannot distinguish a slow machine from a regression, which is the one thing a
  test has to do. Predates 2026-08-14.

## Review findings 2026-08-25 — verified, not yet fixed

From a package-by-package review pass. Each was confirmed against the code;
they were left unfixed for the reason given, not for lack of a diagnosis.

- ~~Agent View and workflow rows truncated mid-ANSI-escape.~~ **Fixed
  2026-08-27.** Both views sized columns with `lipgloss.Width` and then clipped
  with a rune counter, so ~15-20 runes of invisible SGR escape per styled
  segment cut rows far short — visibly, at ~35 of 100 columns, with the age
  column never rendering. Both `truncate` helpers now measure display width via
  `ansi.Truncate`, which also fixes wide-rune under-measurement. Guarded by a
  unit test using literal escapes, because lipgloss emits none without a TTY.
- **The TUI goldens were stale for 25 days.** Last regenerated 2026-08-01;
  `targetLabel` entered the Agent View detail line on 2026-08-02, so
  `make tui-golden-check` — and CI's `tui-golden` job — had been failing on
  `main` since then, unrelated to any later change. Regenerated 2026-08-27 and
  now passing. The harness refuses to run off POSIX
  (`scripts/tui_capture.py:207`); a Linux binary cross-compiled from Windows
  plus a throwaway `python:3.13-slim` container with `pyte` is enough to
  regenerate them without installing anything on the host.
- ~~A self-paced `/loop` started during a streaming turn never advances.~~
  **Fixed 2026-08-30.** `runLoopBody`'s streaming guard sat before
  `a.activeLoopID = ls.id`, so the loop registered, listed forever, and did
  nothing. The diagnosis named the fix -- re-attach ownership to the queued
  turn -- and that is what landed: `turnOptions`/`queuedInput` carry a
  `loopID`, claimed when the turn actually starts rather than when it is
  created. The guard also skipped the iteration instruction, so the queued body
  was bare text with no way for the model to declare the work finished; the
  turn is now built once, before the branch, and the queued and immediate paths
  consume the same value.
- ~~`formatTerminalJobLine` drops the artifacts line.~~ **Fixed 2026-08-30.**
  The early `return head` inside the empty-body branch sat above the
  `jobs.ArtifactDigest` block, so a job that finished with artifacts and
  nothing else lost the one line naming what it produced — and the `/agents`
  pointer for reading it — precisely for the jobs whose only output was
  artifacts. Every part is now gathered before anything decides the line is
  empty, which is how the sibling `formatAgentPeek` was already written and why
  it never had this bug.
- ~~MCP death reason can report EOF instead of the real exit status.~~
  **Fixed 2026-08-30.** The contract this was waiting on turned out not to need
  the lifecycle reordering the diagnosis feared. Liveness and cause are two
  facts that were sharing one atomic: `dead` has to flip the instant the reader
  sees stdout close, because that is what unblocks callers stuck in `write()`,
  while the cause is not known until `cmd.Wait` returns. Nothing about that
  ordering changed — the reader simply stopped claiming to know why. With a
  child still to reap it records the bare sentinel, and the reaper replaces it
  with the exit status. `DeathReasonWithin` waits for the reap for callers that
  want the status, and the diagnostics use it.
- ~~Provider SSE parsers send on an unguarded channel.~~ **Fixed 2026-08-30.**
  Sixty-nine bare sends across five parsers now go through `provider.StreamSink`,
  which selects on the turn's context. The point was to make the guarantee
  structural rather than repeat a select at every site: a parser cannot send
  without going through the sink, and the sink cannot send without observing
  cancellation.
  Narrower than it looked, and in an instructive way. Each parser already
  re-checks cancellation at the top of its scanner loop, so a consumer that
  cancelled *and then drained* was always safe — the drain releases the send
  and the next iteration bails. The reachable gap was a consumer that cancels
  and never drains again: the send never returns, the loop never reaches its
  check, and the goroutine holds the response body and stall guard forever.
  The regression test therefore never receives after cancelling and asserts on
  the goroutine, because observing a channel close means receiving from it,
  which is the one thing that hides the bug.
- ~~`agent.ToolDecider` / `DecideTool` is a half-wired interface.~~
  **Removed 2026-08-30.** The question was which half was dead, and the answer
  is both: `git log -S` shows no commit in the history ever type-asserted to
  `ToolDecider` or called `DecideTool` from the agent. The commit that
  introduced the interface (`d57a944`) also gave the agent its own
  `policy.Decide` consult, so the seam was superseded on the day it was
  written. The two would have applied the same policy to the same request.
  The one behavioural difference was a red herring: `DecideTool` strips the
  `[job:<id>]` prefix from a tool name and the agent does not. That prefix is
  added by the jobs approver when it forwards to the parent TUI approver, so it
  only ever exists inside `uiApprover` — which is where the stripping already
  lives. No agent-side gap.
  Deleting dead code should not quietly delete a guarantee, so the property the
  seam appeared to protect is now pinned: a deny rule blocks a tool whose
  `RequiresApproval` is false. If the policy were only consulted on the
  approval path that could not work, and the test fails if the agent's own
  consult is removed.
- ~~Session persistence.~~ **Fixed 2026-08-30.** All three, including the part
  held back as a repo-wide decision.
  `New` and `Load` returned the manager's own `*Session` while `Current`
  cloned, so a caller could mutate mutex-guarded state from outside the mutex —
  and `Save` writes whatever `m.current` points at, so the mutation would have
  silently persisted. Both now clone, matching `Current`. No caller was
  relying on the aliasing.
  `List` skipped unreadable files in silence, so a corrupt session vanished
  from `/resume` — indistinguishable from one that never existed, which is the
  wrong failure for the command whose whole job is finding a conversation the
  user knows they had. `ListWithProblems` reports them alongside, never instead
  of, the sessions that loaded; `/sessions` and `/resume` print them, bounded so
  a corrupt directory cannot flood the pane.
  The fsync gap is closed in both writers through a new `internal/atomicfile`.
  The decision it was waiting on came down to cost, and the cost is not there:
  a benchmark against the previous no-sync shape measures ~130µs on a small
  session file, about 6%, for a write that happens once a turn. Rename is
  atomic for a *reader*; without the sync it can reach the disk ahead of the
  bytes it names, so a crash leaves a file that exists, is correctly named, and
  is empty — the conversation for a session, and the state a restart
  reconciles from for a job record.
- **Several tests are timing-brittle under CPU load.** `internal/jobs` and
  `internal/mcp` tests using short `waitFor` deadlines fail in batches on a
  loaded machine and pass in isolation — e.g. a job spawning a PowerShell hook
  gets a 2s budget. They are not broken, but they cannot distinguish a slow
  machine from a regression, which is the one thing a test must do.

## TUI and Interaction Parity

- Add transcript search/filter and a compact jump-to-latest affordance.
- Add queue reorder/edit controls; list, drop, and clear already ship.
- Improve visibility when cancellation is draining a provider or child process that has not exited yet.
- Add golden coverage for very tall approval/tool blocks.
- Continue comparing lifecycle states against Claude Code while preserving packetcode provider colors and multi-provider controls.
- Migrate to Bubble Tea v2 for enhanced keyboard reporting (including true
  Shift+Enter where supported) and synchronized-output rendering, preserving
  the committed inline/native-scrollback, no-mouse, and PTY safety contract.

## Context and Cost Efficiency

- Add provider-native token counting where a stable tokenizer/API exists; retain the conservative fallback estimator.
- Persist request-level occupancy samples for diagnostics without conflating them with billable totals.
- Add configurable model-facing caps for search, command, MCP, and artifact
  output. `execute_command` is the only tool with a byte cap (100 KB);
  `search_codebase` and `list_directory` cap counts rather than bytes, and **MCP
  tool results are entirely uncapped**, so a server returning megabytes lands
  whole in the transcript. There is one chokepoint where every tool result
  becomes a message — cap there, write the full output to disk, and hand the
  model an excerpt plus a handle it can read more from, so truncation stops
  meaning loss. A handle referenced after prune must degrade to "no longer
  retained", never an error that stalls a turn.
- Preserve/replay encrypted Codex reasoning items for multi-turn continuity if the subscription backend requires them; never attempt to display opaque reasoning.
- Add explicit cache-hit/cached-input telemetry to `/cost` and statusline
  snapshots. `provider.Usage` already carries the cache fields and Anthropic and
  Codex populate them, but the wire is cut at four points and rendered as a
  hard-coded zero at the fifth: `openaicompat`'s usage struct omits
  `prompt_tokens_details.cached_tokens` (blinding eight providers at once),
  Gemini omits `cachedContentTokenCount`, `session.TokenUsage` and
  `cost.SessionCost` have no cache fields so `RecordUsage`'s signature has to
  widen, and the statusline's two cache JSON slots are literal zeros. Cached
  figures are a reported *subset* of input tokens, never an addend — assert that
  per provider or the totals double-count.

## Agents, Loops, and Workflows

- Add explicit workflow pipeline stages beyond the current ordered
  phases/steps and step-level verifier/retry contract.
- Add a broader versioned example workflow library.
- **Ruled 2026-08-14: packetcode does not resume jobs across a restart.** Durable
  execution after the originating app closes belongs to PacketAgent, so PCMP9 is
  cut rather than deferred. An interrupted job can be explicitly re-run via
  `/jobs resubmit` (PCH4, 2026-07-31), which starts a new job and never claims
  the old process resumed. That remains the whole story; "jobs survive restart"
  must not be claimed by this repo at all. The honest reporting of an
  interrupted job is now carried by the `abandoned` state under Packet
  Computers, promoted from PCMP9 precondition to the primary terminal state and
  shipped 2026-08-14 as PCMP10.
- Let background agents request user clarification through Agent View. Routing
  half-exists: `jobApprover` already forwards to `uiApprover.PromptApproval`.
  Four obstacles remain — read-only jobs are refused before reaching the parent
  approver, `uiApprover` renders one envelope at a time so a question would
  head-of-line-block every approval from all jobs, `Job.NeedsInput` is hardwired
  equal to `NeedsApproval` so Agent View has no distinct signal, and there is no
  timeout, so a blocked question holds a slot out of `background_max_concurrent`.
- Add loop detection to the agent tool loop. The 25-iteration cap is the only
  guard today, and its own comment names the failure it cannot catch: retrying
  the same call on a path that keeps not existing. Hash `(tool, executed
  arguments, result)` over a sliding window and abort on repeats, with a reason
  that says why rather than "exceeded N tool iterations".
- Add a todo tool. There is none, and Agent View has no structured content to
  render for background jobs. Must render as a compact block and never be echoed
  as prose — the system prompt explicitly discourages narrating a plan.
- Add optional live sub-agent transcript streaming without injecting it into foreground model context.
- Add safe worktree merge/apply assistance and explicit cleanup commands.

## Tools, Execution, and Code Intelligence

Most of this section came out of the upstream research in
[`docs/research/`](docs/research/upstream-adoption-plan.md); that plan carries
the evidence, effort, and risk for each item.

- Prune session backups. `internal/session/backup.go` copies the whole file on
  every write and never cleans up: the undo stack is in-memory only, so `.bak`
  files orphan on restart, `Cleanup()` has no production caller, and background
  jobs get their own manager whose backups are never cleaned and never reachable
  from `/undo`. Age-based prune plus a per-session byte cap.
- Add git shadow-repo snapshots for message-level revert. Separate `--git-dir`
  under the packetcode home with `--work-tree` at the project root, one commit
  per user message, prune and size caps, and explicit `core.autocrlf=false`,
  `core.longpaths=true`, `core.symlinks=true`, `core.quotepath=false`. The
  user's own git state is never touched. This is not just a nicer `/undo`: only
  `write_file` and `patch_file` call `Backup`, so **deletions and renames made
  by `execute_command` are currently unrecoverable**. Snapshot the job worktree
  rather than the parent, or a job commit captures unrelated foreground edits.
- Refuse stale writes. Nothing records when the model last read a file, so
  `write_file` silently clobbers a concurrent formatter, a rebase, or a second
  agent in the same worktree. Record `(path, mtime, size, hash)` per session on
  read and refuse a write with "re-read first". Key it off the existing session
  store; do not add a database for it. Local backends only in v1, and say so
  when it is skipped for remote ones.
- Add an LSP client and, more valuably, run diagnostics after every edit and
  append them to the tool result, so the model learns it broke the build without
  being told. `code_intelligence.go` stays as the zero-dependency Go fast path;
  LSP layers above it and must degrade silently when no server is installed.
  **Import `github.com/charmbracelet/x/powernap`** (MIT, `go 1.24`, two pure-Go
  deps) rather than hand-rolling: it carries 388 KB of generated protocol
  bindings and a 372-server config table. The stdlib-only rule is scoped to LLM
  provider and MCP wire code, where hand-rolling is the differentiator; it does
  not reach a published protocol with standard generated bindings. Two caveats:
  `mitchellh/mapstructure` is archived upstream, and `pkg/lsp/protocol` ships
  its own nested licence (gopls-derived) needing its own notice entry.
  **Skip `lsp_rename`/`lsp_replace_symbol` in v1** — LSP-driven mutation
  bypasses the diff preview, the backup call, and the approval renderer.
- Add background shell jobs with output and kill tools. `execute_command` is
  synchronous with a hard 10-minute timeout, so there is no way to start a dev
  server, watcher, or long build and keep working. Compatible with the no-PTY
  contract — a detached process with piped output is not a PTY. `job_kill` must
  be scoped to jobs this session started, never arbitrary pids.
- Evaluate an in-process POSIX shell (`mvdan.cc/sh`, BSD-3) with Go coreutils on
  Windows, replacing the `sh -c` / `cmd /C` wrapper. It would retire the
  per-host capability string currently baked into the tool description, and
  routing the interpreter's exec/open/stat handlers through
  `RuntimeBackend.Resolve` would give `execute_command` **root confinement for
  builtins for the first time** — today only `cwd` is confined and the command
  itself can read anything. Three things must be settled first: persistent shell
  state invalidates the approved command string (ship without it, or render the
  effective env/cwd delta in the approval prompt); in-process builtins are not
  killable by `procrun`, which assumes a separate OS process, so a tight loop
  becomes an unkillable goroutine; and the whole permission suite must be
  re-validated against the new execution path. Start with the handler stack, not
  a coreutils reimplementation. Do this **after** the cheaper items above.
- Close the gaps in the tracked-process work.
  `TrackTree`/`ReleaseTree` is Windows-only — the POSIX side is a no-op, so
  "descendants cannot survive a normally exiting parent" is a Windows-only
  guarantee and the doc comment should either say so or gain a POSIX equivalent.
  It is wired into MCP but not `LocalBackend.Execute`, so `execute_command` gets
  no Job Object containment. `trackedJobs` leaks on any path that skips
  `ReleaseTree`, which MCP never does but a shell running hundreds of commands
  would. And procrun needs a **group handle** — one containment boundary that
  many `exec.Cmd`s join — before either background shell jobs or an in-process
  shell, since a pipeline would otherwise create one boundary per stage.
- Add a bounded `fetch` tool. There is no HTTP in the tool layer at all. Size
  cap, timeout, redirect limit, refuse non-http(s) schemes, refuse loopback and
  private address ranges by default, land it under the output store, and frame
  the result as untrusted evidence — fetched content is the classic
  prompt-injection vector. Defer agentic fetch, download, and web search until
  the network policy axis under Security and Trust exists.
- Expose `doctor` as a read-only self-diagnostic tool. `buildDoctorReport`
  already emits a versioned structured report with redaction, so this is a thin
  adapter that turns "it's broken" into a self-diagnosing session. Move the
  checks out of `cmd/` first — `internal/doctor/` is an empty directory — and
  give the model-facing surface its own redaction test.
- ~~Add skills: an `<available_skills>` index with a `skill` tool that loads a
  body on demand.~~ **Shipped 2026-08-30.** The index, the read-only tool, and
  the five embedded builtins landed as specified, then grew three things this
  entry did not ask for. `disable-model-invocation` and `user-invocable` decide
  who may load a skill, enforced at the index *and* at the tool, because
  omitting a skill from the index stops the model being told about it and does
  not stop the model naming one the user mentioned. `/<skill-name>` expands a
  user-invocable skill as text, matching Claude Code; the transcript shows the
  verb while the model gets the framed body. And discovery now reads
  `.claude/skills` and `.agents/skills` at both scopes.
  That last one reverses this entry's "leave remote skill discovery out of v1",
  and needed a gate to be safe: a repository's foreign-layout skills are
  discovered, listed, and inert until `/skills allow <name>`, with approval
  bound to workspace and body digest so a rewritten skill is asked about again.
  Scope precedence was also inverted to user > project > builtin — only one
  direction of that choice has a victim, and it is the one where cloning a
  hostile repo replaces the `deploy` skill you wrote for yourself with one you
  have never read. Claude Code orders it the same way.
- Move tool prompts and the system prompt out of Go string literals into
  embedded files, so prompt changes are reviewable in diffs. The system prompt
  first, since it has two call sites. Write our own text.

### Skill ecosystem gaps — found 2026-08-30

Measured against the 37 skills in one real `~/.agents/skills`, and against the
published [Claude Code skills docs](https://code.claude.com/docs/en/skills) and
[Agent Skills spec](https://agentskills.io/specification). Published skills now
*load* here; not all of them *work*. Ordered by how often each bites.

- Substitute `${CLAUDE_SKILL_DIR}` and `${CLAUDE_PLUGIN_ROOT}` in skill bodies.
  It is the documented way a skill points at its own bundled scripts, and 5 of
  37 use it. Unsubstituted, the model is handed a literal
  `${CLAUDE_SKILL_DIR}/scripts/x.py` it cannot resolve — the skill does not
  fail, it quietly instructs the model to use a path that does not exist. The
  highest-frequency real breakage of the set.
- Read `allowed-tools` from skill frontmatter; 10 of 37 set it. It pre-approves
  tools for the invoking turn, so ignoring it costs approval prompts rather
  than safety — feature loss in the safe direction. But it is the difference
  between a skill that runs and one that interrogates the user at every step,
  and it is one of the six fields in the Agent Skills spec proper.
- Decide on `` !`command` `` dynamic context injection; 1 of 37 uses it, and
  Claude Code's own `/commit` is built on it. Today the backticked command is
  passed through as literal text, so the model is told ``PR diff: !`gh pr
  diff` `` instead of the diff. Not implementing is defensible — upstream ships
  `disableSkillShellExecution` for exactly this concern, and running repository
  shell text is the sharpest edge in the whole ecosystem. Being silent about it
  is not: a skill that reads as broken is worse than one that reports itself
  disabled.
- Support indexed `$0`/`$1` arguments and an `arguments:` frontmatter list.
  Only `$ARGUMENTS` is substituted today, so a skill using the indexed form
  gets literal placeholders *and* the arguments appended — the model sees both
  and has to guess. All 8 argument-using skills observed use plain
  `$ARGUMENTS`, so today's exposure is low and tomorrow's is not.
- Accept skills packetcode currently refuses. Upstream loads a skill with no
  `description` (falling back to the first paragraph of the body) and truncates
  an oversized one; packetcode drops both, and drops a body over `MaxBodyBytes`
  where upstream has no body cap. A published skill that works in every other
  agent and vanishes here reads as packetcode being broken, which is the wrong
  lesson for the user to draw. The caps exist for real reasons — decide which
  are worth a refusal and which should degrade.
- Decide whether a repository's own `.packetcode/skills` should need the
  approval a `.claude/skills` one now does. Today it does not: the 2026-08-30
  gate exists to avoid opening a *new* automatic-loading surface across every
  repo that already ships `.claude/skills`, not to fence off a directory
  existing projects depend on. That leaves the gate looking like a general
  project-skill trust boundary when it is not — a hostile repo can still use
  the native layout. Tightening it breaks existing projects, so it needs to be
  a decision rather than a drift.
- Offer user-invocable skills through the ACP command catalogue. Blocked on the
  wire vocabulary: `CommandInfo.Source` is a closed `builtin`/`user`/`project`
  set that clients group menus by, and a builtin skill would arrive as a
  `builtin` entry — the one thing that catalogue promises never to emit.
  Extending the protocol comes first; `ListCommands` passes nil skills until
  then, so an ACP client's menu is missing verbs the TUI has.
- Reconsider commands-vs-skills precedence. packetcode gives a name collision
  to `commands/<name>.md`; Claude Code gives it to the skill. Deliberate, and
  marked as a divergence in the README rather than presented as the ecosystem
  rule — but a project carrying both resolves `/name` differently here than
  upstream, which is exactly the surprise a porting user cannot debug.

## MCP and Extensions

- Implement Streamable HTTP MCP against the approved
  [`packetcode-mcp-http-trust-v1`](docs/mcp-http-trust-contract.md) contract and
  existing fail-closed validator. Do not add a second policy or weaken exact
  origin/address, redirect, credential, provenance, approval, or reconnect
  rules.
- Add MCP resources/prompts only after their context and trust model is defined.
  **Split ruled 2026-08-14: resources yes, prompts no.** A resource can be
  delivered by a tool returning bounded tool-role content with the trust
  contract's labelled-untrusted-boundary treatment. An MCP *prompt* is
  server-supplied text intended to become a conversation message, and
  auto-injecting it is exactly what that contract forbids; the only safe shape
  is a slash command that inserts the text into the user's input buffer for a
  human to read and send. Both halves are stdio-only and neither weakens the
  contract.
- Do not scope MCP OAuth inside the Streamable HTTP work. It needs HTTP
  transport, adds a third credential mode beyond `none`/`bearer-env`, and a
  callback server is an **inbound listener** in an executable whose trust
  contract states it adds no network listener. It needs its own contract
  amendment covering token storage, refresh, and loopback redirect-URI binding.
- Add a declarative pack manifest and install/list/enable workflow for prompt commands, MCP, hooks, themes, and workflows.
- Surface MCP timeout, crash, and reconnect details consistently in transcripts and Agent View.

## Providers and Local Models

- Add sanctioned subscription-backed providers only when the provider publishes
  and supports a third-party integration path; otherwise use API-key providers.
  **GitHub Copilot parked until after v1** (ruled 2026-08-14). The engineering
  is largely done — `sugar_login.go` is a working, security-tested OAuth device
  grant and `codexauth.go` does refresh-token exchange with atomic write-back —
  so this is blocked only on a written answer to whether Copilot's published
  integration path sanctions a third-party terminal agent. Reputational risk,
  not technical.
- Expand Ollama pull progress, cancellation, and model-removal management.
- Add optional MLX/local-runtime backends only if they can match the native tool
  and streaming contracts without weakening Ollama's zero-config path. LM
  Studio, llama.cpp, and LiteLLM are already usable through the `custom`
  provider; the missing half is zero-config discovery — probe the well-known
  ports and register what answers, reusing Ollama's existing capability
  enrichment. The constraint above is the trap: a server that advertises tool
  support but mangles parallel tool calls is worse than no discovery, because it
  silently degrades a working setup. Gate on a real capability probe, default
  tool support to false when unverified, put discovery behind an opt-in config
  key so a stray listener cannot hijack a provider list, and use a short client
  timeout so a dead endpoint cannot delay startup. One file per runtime; ship LM
  Studio first and stop if it does not earn its keep.
- Add provider-specific output/reasoning controls to the model picker when the upstream catalog exposes them.
- Verify the MiniMax reasoning wire shape against a live key. The inline
  `<think>` path is implemented from the published tool-use guide, not from an
  observed response; confirm it, then decide whether to adopt
  `reasoning_split=true` + verbatim `reasoning_details` echo instead of
  reconstructing the tags.
- Track MiniMax cached-input and long-context billing. `/cost` currently bills
  M3 at a flat $0.30/$1.20: cached input reads are cheaper, and a request over
  512K tokens bills entirely at the 2x long-context tier, so long sessions on a
  1M window are under-reported. Needs `usage` cache fields parsed into
  `provider.Usage` and a tiered entry in the MiniMax pricing table.
- Map `/effort` onto MiniMax `thinking.type=disabled` so thinking can be turned
  off for cheap turns; MiniMax does not implement `ReasoningEffortController`.
- Evaluate `api.minimax.io/anthropic` as an alternate MiniMax transport, where
  thinking blocks are first-class and the existing Anthropic parser already
  handles them. Trade-off: MiniMax stops sharing `openaicompat`.

## Packet Computers

Product source: [PACKETCOMPUTERS.md](PACKETCOMPUTERS.md). Bounded Phase 1
ledger: [docs/packet-computers-loop.md](docs/packet-computers-loop.md)
(PCMP1–PCMP10; PCMP9 cut 2026-08-14). PCMP1/PCMP2 shipped 2026-07-31. PCMP3 and a bounded foreground
direct-SSH backend shipped 2026-08-01: pinned persistent SSH/SFTP plus
root-confined core tools via `packetcode --computer <name>`.

- PCMP4/PCMP5 — daemon RPC plus heartbeat, so status stops being a stored value
  and becomes a probed one. **Scope ruled 2026-08-14: the daemon is
  session-scoped.** It exists to reach Packet Computers and dies with the app;
  it holds no durable job state (see the PCMP9 ruling above).
- **Ledger edit needed before PCMP4 starts:** replace the `--listen
  127.0.0.1:<port>` acceptance condition with an AF_UNIX socket on POSIX and a
  named pipe (or stdlib AF_UNIX) on Windows. PCMP4 carries two conditions —
  refuse non-loopback binds *and* write no credentials to disk — and loopback
  TCP is reachable by every local UID, so satisfying the first that way forces
  an auth token that breaks the second. A socket at `0600` inside a `0700`
  directory gets both from filesystem permissions and makes the loopback rule
  structural rather than a validation check a config path or `--network host`
  can regress. There is no network listener anywhere in production code today,
  so nothing is being migrated. Carry two caveats: SSH forwarding needs
  `AllowStreamLocalForwarding` on the remote sshd and must fail with a clear
  diagnostic rather than a hang; and try stdlib AF_UNIX on Windows before
  reaching for `go-winio`.
- Add a data-dir advisory lock holding pid, start time, and workspace root.
  Cross-project clobbering is fixed (job records now carry their owning project
  root), but two instances rooted at the *same* project still cannot be told
  apart, and a daemon sharing the jobs directory makes that case reachable.
  Share one vocabulary across the lock, stale-socket classification, and the
  `abandoned` state — all three are the same three-way "running / died / owned
  by someone else" question.
- Finish PCMP6/PCMP7 daemon parity: the foreground direct-SSH backend now
  supplies host verification and `RuntimeBackend` for core tools, but the
  planned SSH-forwarded daemon transport, backend parity suite, and remote code
  intelligence remain open. "Reconnect semantics" here means recovering a
  dropped transport *within* one session — reconnect after the app exits is out
  of scope per the PCMP9 ruling.
- PCMP8 direct-SSH routing shipped 2026-08-02: immutable computer/root binding,
  `/spawn --computer <name>`, whole-workflow placement, independent per-job
  SSH connections, and fail-closed remote Git worktrees. **PCMP9 is cut** — see
  the ruling under Agents, Loops, and Workflows. PCH4's rule stands and is now
  the only rule: anything not genuinely resumed is reported as abandoned.
- ~~Add an explicit `abandoned` terminal state so a loss is never flattened into
  a confirmed cancellation.~~ **Shipped 2026-08-14 as PCMP10.** `StateAbandoned`
  plus an `AbandonCause` (`app-exit` / `transport-lost` / `unknown`), and a
  durable `CancelRequest` stamped before the context is cancelled — without it a
  user cancel and a dead transport are the same `context.Canceled`. A running
  job left by an unclean exit now reconciles as abandoned; a queued one stays
  cancelled, because it provably never started. Five sites that treated any
  unrecognised terminal state as a **success** were fixed at the same time (two
  workflow gates, `spawn_agent`, `collect_agent_results`, Agent View grouping);
  they now test `State.IsSuccess()` instead of enumerating failures.
- **Still open from PCMP10: process-group-aware cancellation evidence.** The
  state is honest about uncertainty but cannot reduce it. `procrun.KillTree`
  returns a bare `error`, `computers.ExecResult` carries only an exit code, and
  `jobs.Manager.Cancel` is a fire-and-forget `context.CancelFunc` with no return
  path, so nothing learns whether a kill worked; on SSH there is no mechanism to
  evidence at all. Windows already *computes* per-PID survivor data in
  `killDescendants` and discards it. Until this lands, `transport-lost` is
  claimed only where a transport error was actually observed. Sequence it after
  the in-flight `procrun` Job Object work is committed.
- Add a generation-aware SSH connection manager only with a no-replay rule for
  writes/commands; current jobs intentionally own independent connections.
- Add asynchronous remote project workflow discovery and an explicit
  workflow-scoped shared-worktree contract. Today write steps are isolated and
  exchange summaries, not unmerged filesystem state.
- Defer managed cloud machines, snapshots, and process supervision until the
  local/SSH contracts are stable.

## Packet Control

**Split to PacketADE 2026-07-31.** Packet Control Phases 1–2 are implemented
in PacketADE (`D:\projects\PacketADE\dev\packet-control-loop.md`, CTL1–CTL9),
because evidence bundles need a viewer and PacketADE already has the diff,
review-gate, and Flight surfaces to show them in. See
[PACKETCOMPUTERS.md](PACKETCOMPUTERS.md) for the product definition.

- No packetcode work is scheduled. If Control is later wanted in the TUI, it
  must consume CTL1's manifest schema rather than defining a second evidence
  format.

## Security and Trust

- Define shared policy axes for filesystem, shell, network, MCP, browser, desktop, secrets, and remote computers.
- Add redaction tests for provider errors, hooks, statuslines, job artifacts,
  and future control evidence. MCP log and future remote-output redaction are
  covered by the PCH5 suite.
- Add audit events for live permission-mode changes and remembered approvals.
- Keep Bypass Permissions explicit, visible, outside the normal Shift+Tab cycle, and subordinate to deny rules.
- Treat remote/browser/desktop content as untrusted evidence rather than instructions.
- Decide whether `ask` command-prefix rules should hold across compound
  commands. The deny direction was fixed 2026-08-14, but the same
  allow-direction refusal still applies to `ask` rules, so under a permissive
  profile `git status; :` falls through to allow where `git status` would
  prompt. Lower severity than the deny bypass and currently pinned by
  `TestPolicy_CommandPrefixMatchesFields`, so changing it is a deliberate
  decision rather than a bug fix.
- Add substring or regex command matching to permission rules. Today there is
  only exact `command` and `command_prefix`, which is a narrow vocabulary for
  expressing "never run this".
- Collapse the two filesystem-confinement implementations.
  `internal/tools/safefs.go` is largely superseded by
  `internal/computers/local_backend.go`, which every core tool actually routes
  through, and their semantics differ slightly. Two jails with one purpose is a
  place for a gap to hide.
- Widen the hook matcher from exact-tool-name-or-`*` to globs, reusing
  `permissions.toolPatternMatches` so hooks and permission rules share one
  pattern language.
- Consider hook verdicts beyond block and context injection — rewrite tool
  input and auto-approve. **Sequencing is the security design:** `PreToolUse`
  currently fires *after* the policy decision, so a rewrite bolted onto that
  call site would be evaluated against no policy at all. Move the hook above
  the first `Decide`, keep deny as an absolute floor over any hook verdict, make
  both mutating verdicts opt-in per hook, and assert that a rewrite lands before
  the approval prompt renders.

## PacketADE Integration and BridgeCode-Plus

Approved 2026-07-27. The cross-repository source of truth is
`D:\projects\PacketADE\dev\bridgemind\packetcode-bridgecode-loop.md` (PC1–PC10).
The evidence audit and bounded follow-up ledger are
[`docs/bridgecode-feature-truth-2026-07-27.md`](docs/bridgecode-feature-truth-2026-07-27.md)
and
[`docs/bridgecode-plus-hardening-loop-2026-07-27.md`](docs/bridgecode-plus-hardening-loop-2026-07-27.md).

- Complete the signed clean-machine release matrix and packaged PacketADE
  compatibility gates when published artifacts/runners exist.
- Consume PacketAgent's versioned durable-handoff contract when that sibling
  runtime publishes it; do not create a competing PacketCode daemon contract.
- Preserve PacketCode as an independently installable product; durable execution
  after its originating app closes belongs to PacketAgent. **This line governs
  (ruled 2026-08-14):** it was in tension with PCMP4/5/9, which committed to a
  packetcode daemon retaining job state across a restart. The tension is
  resolved in favour of this rule — PCMP9 is cut and the daemon is
  session-scoped. See Agents, Loops, and Workflows.

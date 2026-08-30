# Changelog

All notable packetcode changes are recorded here. The project is pre-1.0; `Unreleased` describes the current `main` branch.

## [Unreleased]

### Added

- Agent Skills: invocation flags, `/<skill-name>`, and foreign discovery.
  `disable-model-invocation` and `user-invocable` decide who may load a skill,
  spelled as Claude Code spells them and defaulting the same way, so a skill
  written for that ecosystem behaves the same here. Both are enforced at the
  system-prompt index *and* at the `skill` tool: omitting a skill from the index
  stops the model being told it exists and does not stop the model naming one
  the user just mentioned. A flag value that does not parse keeps its default
  and is reported, because for both flags the default is the permissive answer
  and a typo must not silently grant what the author refused.
- A user-invocable skill registers as a slash command, so `/<skill-name>`
  expands its body into the turn as text — the mechanism Claude Code uses, not
  a routed tool call. The transcript shows the verb you typed while the model
  receives the framed body, because a skill body runs to 64KB and pasting one
  into the conversation pane buries the exchange under a document nobody wrote.
  Arguments reach the turn either way; `$ARGUMENTS` is honoured only for a body
  its user wrote, since a repository body must not choose where a user's words
  land. Precedence is builtin > `commands/<name>.md` > skill, and every
  displacement is reported rather than leaving an author with a file that
  silently does nothing.
- Skills are discovered from six directories: `.packetcode/skills`,
  `.claude/skills` and `.agents/skills`, at both user and project scope, with
  the native layout winning within a scope — so skills already installed for
  another agent are found where they are. A repository's foreign-layout skills
  are the exception: discovered, listed, and inert until `/skills allow <name>`,
  because that is the one directory you acquire by cloning, and a skill's
  description reaches the system prompt while its name would become a command
  you can type. Approval is per skill, per workspace, and bound to the body
  approved, so a repository that rewrites a skill afterwards is asked again
  rather than inheriting the answer.
- Skill scope precedence is now user > project > builtin, inverted from
  project-first. Only one direction of that choice has a victim: if the
  repository won, cloning a hostile one would replace the `deploy` skill you
  wrote for yourself with one you have never read, invoked by the same name and
  the same habit. Claude Code orders its scopes the same way, so published
  skills are written expecting it. Displacements are reported through a new
  warnings channel, kept apart from load errors so an ordinary override does not
  fail `packetcode skills list`.
- Provider keys can come from a `.env` file: `~/.packetcode/.env` for every
  project and `<project>/.env` for one. Precedence is a real environment
  variable, then the project file, then the user file, then `config.toml` — the
  environment wins because it is what someone set deliberately for this run.
  Values are never injected into the process environment: packetcode runs shell
  commands on the model's behalf, and a file that exists to hold API keys is the
  last thing an arbitrary subprocess should inherit. With three possible
  sources, `/provider` and `doctor` now report which one is in force, naming the
  variable or the file and never the key.
- Explicit `abandoned` terminal state for background jobs (PCMP10), so a job
  whose outcome was never observed is no longer reported as a cancellation
  somebody chose. Carries an `AbandonCause` of `app-exit`, `transport-lost`, or
  `unknown`, surfaced in Agent View, `/jobs`, the terminal conversation line,
  and the sub-agent result handed to the parent model. A cancel is now recorded
  durably *before* the job's context is cancelled, which is what makes a
  deliberate stop distinguishable from a loss at all — a context alone carries
  no cause. A job left running by an unclean exit reconciles as abandoned; one
  left queued still reconciles as cancelled, because it provably never started.
  packetcode still does not resume jobs across a restart, and `/jobs resubmit`
  continues to start a new job rather than claiming the old one continued.
- Unreadable job records are now reported instead of silently disappearing: a
  bounded stderr warning at startup and a `state.jobs.records` check in
  `doctor`, both via the new read-only `jobs.InspectRecords`.

- Foreground SSH Packet Computer workspaces: `/computers register|ssh|remove`,
  mandatory SHA256 host-key pinning, SSH-agent or identity-file
  authentication, one process-lifetime SSH/SFTP connection, root-confined
  remote read/list/search/write/patch/command tools, `--computer <name>`, and
  session-to-computer binding that refuses cross-machine resume.
- Process-lifetime remote background agents and workflows: active remote
  sessions inherit their computer; local sessions use `/spawn --computer` or
  `/workflows run --computer`; jobs persist immutable endpoint/root identity,
  own independent SSH/SFTP connections, apply restrictive computer policy,
  and require isolated remote Git worktrees before enabling writes. Durable
  reconnect remains PCMP9; remote `/undo`, code intelligence, hooks, and
  heartbeat remain deferred.
- Approved `packetcode-mcp-http-trust-v1` design gate plus a fail-closed,
  transport-independent validator for exact origins/ports, separately allowed
  network address classes, bounded bodyless same-origin GET/HEAD redirects,
  disabled ambient proxies, system-root TLS, atomically bound target-only
  environment credentials, per-call
  approval, bounded response/event/header/output sizes, bounded timeouts, and
  manual reconnect. Remote output has a credential-bound, labelled, capped
  untrusted tool boundary with exact-value, partial percent, JSON-escape, and
  base64 redaction tests; the
  Streamable HTTP transport itself remains disabled (PCH5).
- `/permissions reset` revokes remembered/manually added session rules and
  restores the startup permission policy.
- Permission transitions now fail closed: Plan mode cannot be weakened by
  allow/ask rules, explicit denies remain floors, queued approvals advance,
  and a running background job's snapshot-bound prompt cannot be silently
  broadened by foreground trust changes. Remembered background approvals are
  recorded against the real tool name; leaving Bypass preserves session rules.
- Versioned workflow schema and `/workflows validate <name>`, plus explicit
  read-only step verifiers, fail-closed structured verdicts, hard retry caps,
  verifier-feedback retries, and agent/token accounting across every attempt
  (PCH3).
- `/jobs resubmit [id]` re-runs a background job abandoned by a previous app
  exit. It starts a **new** job from the saved prompt and never claims the old
  process resumed: the abandoned job keeps its cancelled state, reason, and
  evidence, and the two records link both ways. Allowed once per job; oversize
  saved prompts are refused rather than truncated (PCH4).
- Packet Computers registry (Milestone A): versioned
  `~/.packetcode/computers/registry.json` with conservative policy defaults,
  plus read-only `/computers` and `/computers status <name>`. Registry-only —
  there is no daemon, nothing connects, and a stored status is never presented
  as a live probe (PCMP1/PCMP2).
- OpenAI Codex ChatGPT-subscription provider backed by the official Codex CLI OAuth store and Responses API, including catalog-driven reasoning effort/summary behavior.
- DeepSeek, Grok (xAI), and Mistral built-in providers, plus refreshed OpenAI, Anthropic, MiniMax M3, Ollama, and Codex model metadata.
- First-class native Ollama support with zero-config `localhost:11434`, remote-host overrides, bounded automatic `num_ctx`, `/api/show` capability discovery, keep-alive, model pull/status/PS commands, warmup, timing telemetry, and Apple Silicon memory recommendations.
- Native statusline enabled by default, plus a Claude Code-compatible custom statusline JSON snapshot.
- `@` file mentions with git-aware fuzzy completion and bounded project-root expansion.
- Live reasoning-summary rendering for providers/models that expose it.
- Prompt history recall with draft restoration and multiline-aware Up/Down behavior.
- Permission modes and footer: Manual, Accept Edits, Auto, Plan, and explicit Bypass Permissions.
- `/plan`, `/loop`, and `/workflows`; workflows support ordered phases, single-agent steps, parallel fan-out, template bindings, cancellation, live inspection, and aggregate token boundaries.
- Versioned structured stop decisions for self-paced `/loop`, retaining the
  legacy sentinel and hard 25-iteration cap.
- Per-server `/mcp restart <name>` recovery with safe dynamic tool-adapter
  replacement.
- Full-screen Agent workspace with grouped state, direct task entry, transcripts, peek, cancellation, explicit injection, and compact artifact manifests.
- Per-job and per-workflow token budgets.
- Deterministic credential-free TUI lifecycle fixtures and a PTY/cell capture harness for packetcode/Claude comparisons at fixed terminal sizes.
- Reviewed credential-free TUI text-and-style cell goldens at 72×24 and
  100×30, a post-SIGWINCH live-resize fixture, raw terminal-protocol safety
  checks, pinned PTY tooling, and CI/release gates. Supported terminal geometry
  and platform evidence are now documented explicitly.
- Runtime `/effort` control for Codex models, including catalog-advertised levels, persistent configuration, and a compact footer indicator.
- A practical user manual, advanced operator guide, printable terminal cheat sheet, and self-contained offline HTML5 manual.
- A root-level maintainer handoff covering architecture, state and security boundaries, verification, interaction caveats, and prioritized next work.

### Changed

- Reworked the TUI toward Claude Code's flow: flat conversation blocks, `❯` submitted prompts, understated horizontal input rules, Claude-style thinking/tool markers, numbered approval choices, message spacing, full-screen Agent/Workflow views, and exact mode footers.
- Shift+Tab now changes permission mode during an active turn. The new policy applies to later tool actions and can resolve an approval already waiting; an already-running command is left alone.
- The default system prompt favors concise, terminal-friendly answers and avoids unnecessary scaffolding.
- Context estimation now includes the system prompt, transcript structure, tool schemas, and pending input using a conservative source-code-oriented estimate.
- Automatic compaction runs before an over-threshold turn, preserves complete recent tool exchanges, records summarization usage, and updates live occupancy independently from cumulative billing.
- Older oversized tool results are compacted only in the model-facing request; full content remains in persisted sessions and the UI.
- Code-intelligence result defaults and hard caps are smaller to reduce context growth.
- Anthropic requests use ephemeral cache breakpoints for stable system and tool-schema prefixes and account for cache tokens explicitly.
- High-frequency background-job activity snapshots are coalesced while queued/running/terminal transitions remain synchronous and recovery-safe.
- Documentation was consolidated around current behavior; shipped `Round N` design specs are now concise implementation/user guides.
- The built-in agent prompt now encourages parallel fan-out for independent work, while Agent View separates list shortcuts from its explicit `n` task composer.
- The model picker uses `Alt+M` (or `/model`) instead of the unsafe Bubble Tea
  v1 `Ctrl+M`/Enter alias. Global picker and clear shortcuts no longer mutate
  content beneath a visible modal or workspace.

### Fixed

- Provider stream parsers can no longer be stranded by a consumer that stops
  reading. Every event now goes through a sink bound to the turn's context, so
  a send cannot block indefinitely on a full channel; previously a cancelled
  turn whose consumer never drained again left the parser goroutine holding the
  response body and the stall guard for the life of the process. Applies to the
  OpenAI-compatible, Responses, Anthropic, Gemini, and Ollama parsers.
- An MCP server that died with a non-zero exit status could be reported as
  `exited: EOF`. A child's stdout closes before it is reaped, so the reader
  usually won the race and recorded EOF as the cause; the reaper corrected it a
  moment later, but anything asking right after seeing the server was dead got
  the wrong answer. The reader now records only that the server exited, and the
  reaper supplies the status. `/mcp` and the MCP report wait for it.
- A self-paced `/loop` started while a turn was streaming now runs. The
  streaming guard sat before the line claiming loop ownership, so the loop
  registered, appeared in `/loop list` forever, and did nothing — and slash
  commands dispatch during a stream, so typing `/loop <prompt>` mid-turn hit
  this every time. Ownership now travels with the queued turn and is claimed
  when that turn starts. The queued body also carried no iteration instruction,
  leaving the model with no way to declare the work finished; the turn is built
  once now, so the queued and immediate paths cannot differ.
- OpenAI models that refuse function tools on `/v1/chat/completions` are routed
  to `/v1/responses` instead. Selecting `gpt-5.6-sol` previously failed every
  turn with a 400, because packetcode sends tools on every turn and that model
  only accepts them on the other endpoint. Routing is per request, so switching
  models mid-session takes the endpoint with it. The `-pro` family is no longer
  hidden from the catalog either: it was excluded because chat-completions was
  the only endpoint spoken here, and hiding a model packetcode can drive leaves
  capability on the floor.
- The Responses API's own error message reaches the user. An error nested under
  `error` in the SSE frame was reduced to `codex stream error`, so a message
  naming the exact fix — with the URL — was replaced by a phrase that says
  nothing. Fallback text also names the backend the user configured rather than
  always saying `codex`.
- YAML block scalars in skill frontmatter parse. `description: >-` with the text
  on following indented lines is how most published skills write a description;
  the line-based parser read the value as the literal `">-"`, which passed every
  validation and produced a meaningless index entry. Continuation lines are now
  folded in and consumed, so a colon inside the prose is prose rather than a key
  that sets a flag its author never wrote.
- Skill directories that resolve to the same place are scanned once. Running
  packetcode from a home directory that has skills in it reported every
  malformed skill twice, made every skill shadow itself, and — worse — labelled
  the user's own skills untrusted repository content on the project pass.
- An abandoned background agent is no longer reported as a success. Two
  workflow gates, `spawn_agent`, `collect_agent_results`, and Agent View's
  grouping all tested for known failure states, so any terminal state they did
  not enumerate fell through as passing — a workflow step whose agent was lost
  would advance the run, and a lost sub-agent was handed to the parent model as
  a completed one. They now test `State.IsSuccess()` instead.
- A cancelled job no longer discards the error that actually stopped it. The
  terminal-state precedence dropped `lastErr` whenever the context was also
  cancelled, so a dead transport was recorded with no error text at all.
- Deny permission rules no longer fail open on shell metacharacters. A
  `command_prefix` rule refuses to match anything but a single simple command,
  so that a prefix can never *authorize* a larger shell program — but deny-floor
  rules were evaluated through that same predicate, where "cannot prove it
  matches" became "does not match". A rule denying `git push` held against
  `git push origin main` and fell through on `git push origin main; :`,
  `true && git push origin main`, and `sh -c 'git push origin main'`. Deny
  matching now evaluates each simple command inside a compound command
  separately, ignores redirection targets, tolerates leading `NAME=value`
  words, and escalates to an approval prompt when a stage hands its arguments
  to an interpreter or a script it cannot see through. Escalation only
  tightens: an existing `ask` or `deny` is never loosened, and a compound
  command provably unrelated to the rule still runs without a prompt.
- Background jobs are scoped to the project that created them. The jobs
  directory is shared across every project on the machine, so starting
  packetcode in one project rewrote another project's queued and running jobs
  as abandoned, made them eligible for `/jobs resubmit` — which could launch a
  duplicate of a job that was still running, worktree and all — and left the
  two instances overwriting each other's records. Job records now carry the
  project root that created them, and another root's live jobs are left
  strictly alone. Records written before this change carry no root and are
  still recovered.
- Job records are versioned and no longer disappear when unreadable. A corrupt
  or future-versioned job file was skipped silently, so abandoned work was
  reported as nothing at all. Records now carry `format_version`; unreadable
  ones are collected and exposed rather than dropped, an unrecognised state is
  reported instead of being flattened to `failed`, and a record written by a
  newer packetcode is never overwritten by an older one.
- MiniMax interleaved-thinking models (M2.x/M3) keep their reasoning chain
  across tool calls. Their thinking arrives inline in `content` wrapped in
  `<think>` blocks; it was rendered as ordinary assistant text and then
  discarded on every tool-calling turn, which MiniMax's tool-use guide warns
  degrades multi-turn tool use. Reasoning is now split out of the transcript
  into the reasoning stream, persisted on the assistant message, and replayed
  on the next request with the tags preserved exactly. Content sharing a frame
  with a tool call is no longer dropped on these providers. Other
  OpenAI-compatible backends are unaffected: a literal `<think>` in prose stays
  visible, and stored reasoning is never sent to a provider that did not
  produce it.
- Context gauges now show current context occupancy instead of cumulative session input, keeping the native and custom statuslines aligned and allowing occupancy to drop after compaction.
- Foreground permission-policy swaps are synchronized with the running agent; background job policy startup reads are synchronized as well.
- Workflow cancellation closes spawn/register races, cancels sibling fan-out jobs on failure, drains terminal states with bounded waits, and reports malformed workflow files instead of silently falling back.
- Provider/tool cancellation stops the generic thinking spinner on first visible progress and distinguishes cancellation from provider errors.
- Job persistence flushes pending snapshots on shutdown.
- Symlink-escape scans and file-as-parent write errors behave consistently on macOS.
- MCP JSON-RPC request IDs correctly handle every `int64` value.
- Prompt editing now respects the caret for file mentions, supports portable multiline bindings, honors `max_input_rows`, keeps history navigation at visual-row boundaries, and clears a draft before Ctrl+C exits.
- Terminal-originated text is sanitized before rendering, preventing tool output from enabling mouse modes, writing the clipboard, or corrupting the TUI; split UTF-8 and progress-line output remain intact.
- Statuslines stay on one row at narrow widths, background transcripts refresh without losing scroll position, and reasoning activity is visible while an agent is thinking.
- Built-in statusline segments are recomposed after a terminal resize instead
  of retaining a wide-layout selection and clipping it at the new width.
- Git branch refreshes no longer block the Bubble Tea event loop, and approval prompts wake the UI on demand instead of forcing continuous idle redraws.
- Queued prompts preserve leading indentation and trailing newlines exactly.
  Secret-entry prompts own the terminal geometry instead of rendering above an
  extra composer/status footer, and inactive composers render blurred beneath
  overlays.

## [0.5.1] - 2026-05-30

### Added

- Retry with backoff and jitter for transient provider failures, honoring `Retry-After` and cancellation.
- Per-call provider stall timeout.
- Whitespace- and line-ending-tolerant unique matching for `patch_file`.
- Incremental `execute_command` stdout/stderr rendering with bounded final output.

## [0.5.0] - 2026-05-29

### Added

- Multi-provider agent loop, native project tools, approvals, sessions, cost tracking, undo, context compaction, and git-aware status.
- Bubble Tea TUI with terminal scrollback, provider/model pickers, slash completion, queued prompts, custom prompt commands, hooks, statusline, and themes.
- Background agents, Agent View, isolated write worktrees, transcripts, cancellation, and explicit result injection.
- MCP stdio tools registered as provider-safe aliases.

### Changed

- Provider/model completion opens interactive pickers rather than requiring guessed identifiers.
- Topbar/statusline include operation state, elapsed time, queue depth, context, and job telemetry.
- Approval and transcript views include clearer source, runtime, and worktree context.

### Fixed

- Custom statusline refreshes are throttled.
- MCP diagnostics display the actual provider-safe aliases.
- Process startup/cancellation tests are reliable across supported operating systems.

# Changelog

All notable packetcode changes are recorded here. The project is pre-1.0; `Unreleased` describes the current `main` branch.

## [Unreleased]

### Added

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

### Fixed

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
- Git branch refreshes no longer block the Bubble Tea event loop, and approval prompts wake the UI on demand instead of forcing continuous idle redraws.

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

# Backlog

packetcode is pre-1.0. This file contains only work that has not shipped; completed work belongs in [CHANGELOG.md](CHANGELOG.md).

## v1 Release Readiness

- Automate signed/notarized macOS, Linux, and Windows release artifacts and checksum verification.
- Define compatibility and migration policy for config, sessions, persisted jobs, workflow TOML, and MCP definitions.
- Add end-to-end smoke coverage for first-run setup, provider switching, session resume, approvals, background jobs, workflows, and MCP.
- Keep provider catalogs, pricing, context windows, and tool-capability metadata current; prefer live discovery when authoritative.
- Add opt-in live-provider contract tests that never run in ordinary CI.

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
- Add configurable model-facing caps for search, command, MCP, and artifact output.
- Preserve/replay encrypted Codex reasoning items for multi-turn continuity if the subscription backend requires them; never attempt to display opaque reasoning.
- Add explicit cache-hit/cached-input telemetry to `/cost` and statusline snapshots.

## Agents, Loops, and Workflows

- Add explicit workflow pipeline stages beyond the current ordered
  phases/steps and step-level verifier/retry contract.
- Add a broader versioned example workflow library.
- Resume or reconcile active background jobs after process restart. Abandoned jobs are recovered as cancelled and can now be explicitly re-run via `/jobs resubmit` (PCH4, 2026-07-31), which starts a new job and never claims the old process resumed. True reconnect-and-continue requires the daemon and lands as PCMP9.
- Let background agents request user clarification through Agent View.
- Add optional live sub-agent transcript streaming without injecting it into foreground model context.
- Add safe worktree merge/apply assistance and explicit cleanup commands.

## MCP and Extensions

- Implement Streamable HTTP MCP against the approved
  [`packetcode-mcp-http-trust-v1`](docs/mcp-http-trust-contract.md) contract and
  existing fail-closed validator. Do not add a second policy or weaken exact
  origin/address, redirect, credential, provenance, approval, or reconnect
  rules.
- Add MCP resources/prompts only after their context and trust model is defined.
- Add a declarative pack manifest and install/list/enable workflow for prompt commands, MCP, hooks, themes, and workflows.
- Surface MCP timeout, crash, and reconnect details consistently in transcripts and Agent View.

## Providers and Local Models

- Add sanctioned subscription-backed providers only when the provider publishes and supports a third-party integration path; otherwise use API-key providers.
- Expand Ollama pull progress, cancellation, and model-removal management.
- Add optional MLX/local-runtime backends only if they can match the native tool and streaming contracts without weakening Ollama's zero-config path.
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

Product source: [PACKETCOMPUTERS.md](PACKETCOMPUTERS.md). Bounded Phases 1–2
ledger: [docs/packet-computers-loop.md](docs/packet-computers-loop.md)
(PCMP1–PCMP9). PCMP1/PCMP2 shipped 2026-07-31. PCMP3 and a bounded foreground
direct-SSH backend shipped 2026-08-01: pinned persistent SSH/SFTP plus
root-confined core tools via `packetcode --computer <name>`.

- PCMP4/PCMP5 — loopback-only daemon RPC plus heartbeat, so status stops being
  a stored value and becomes a probed one. Must refuse non-loopback binds.
- Finish PCMP6/PCMP7 daemon parity: the foreground direct-SSH backend now
  supplies host verification and `RuntimeBackend` for core tools, but the
  planned SSH-forwarded daemon transport, backend parity suite, remote code
  intelligence, and reconnect semantics remain open.
- PCMP8 direct-SSH routing shipped 2026-08-02: immutable computer/root binding,
  `/spawn --computer <name>`, whole-workflow placement, independent per-job
  SSH connections, and fail-closed remote Git worktrees. PCMP9 remains
  persistent job reconcile. PCMP9 is the first point at which "jobs survive
  restart" is true; it must not be claimed earlier, and it must preserve
  PCH4's rule that anything not genuinely resumed is reported as abandoned.
- Before PCMP9, add an explicit `abandoned`/`indeterminate` remote terminal
  state so loss after an acknowledged remote start is never flattened into a
  confirmed cancellation; include process-group-aware cancellation evidence.
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
  after its originating app closes belongs to PacketAgent.

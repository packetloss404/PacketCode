# Agent Reliability Baseline

The original four-round reliability plan has shipped. This file records the maintained guarantees.

## Provider Resilience

- Initial request dispatch retries transient network errors, HTTP 429, and selected 5xx responses with bounded exponential backoff/jitter.
- `Retry-After` is honored.
- Mid-stream responses are not replayed automatically.
- Silent streams fail after `provider_stall_timeout` (default 60 seconds).
- Cancellation interrupts requests, backoff, scanners, approvals, and tools.

Configure under `[behavior]`:

```toml
provider_max_retries = 3
provider_stall_timeout = 60
```

## Tool Reliability

- `patch_file` tries exact matching first, then one whitespace/line-ending-normalized unique match; ambiguity remains an error.
- `execute_command` streams output while preserving a bounded final result and process-tree cancellation.
- File reads/writes and scans remain project-root scoped and reject symlink escape.
- Parent components that are files produce a clear write-path error across platforms.

## Context Reliability

- Occupancy is separate from cumulative billing totals.
- Estimation includes system prompt, transcript, tool schemas, and pending input.
- Automatic compaction triggers before an over-threshold turn and preserves complete recent tool exchanges.
- Compaction usage is recorded; older oversized tool results are bounded only in model-facing copies.
- Background jobs and workflows support token boundaries.

## Job and Workflow Reliability

- Concurrency/depth/lifetime caps are enforced in the jobs manager.
- Write jobs fail closed unless an isolated git worktree is available.
- Lifecycle snapshots are persisted immediately; noisy activity snapshots are coalesced and flushed on shutdown.
- Workflow cancellation closes spawn/register races, cascades to children, fails sibling fan-out promptly, and uses bounded terminal drains.

## Verification

The expected release gate is:

```bash
go build ./...
go test ./...
go vet ./...
go test -race -count=1 ./...
git diff --check
```

The PTY lifecycle harness adds deterministic visual regression evidence; see [TUI parity harness](tui-parity-harness.md).

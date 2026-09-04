# Background Agents, Loops, and Workflows

This document describes the current shipped orchestration model. Every delegated agent is an `internal/jobs` job with its own session, provider/model selection, context, usage, cancellation, transcript, and result lifecycle.

## Starting Agents

```text
/spawn inspect the authentication flow
/spawn --provider gemini --model gemini-2.5-flash inspect the tests
/spawn --write implement the focused fix
/spawn --computer production inspect the server
/spawn --computer production --write migrate the app
```

The model can delegate through `spawn_agent` using the same manager. `wait=true` returns a compact result when the child finishes; asynchronous children can be joined with the approval-gated `collect_agent_results` tool.

Concurrency is bounded by:

```toml
[behavior]
background_max_concurrent = 4
background_max_depth = 2
background_max_total = 32
background_default_provider = ""
background_default_model = ""
background_token_budget = 0
```

`background_token_budget` is a per-job input+output boundary checked after completed provider/tool iterations. Zero disables it.

## Isolation and Permissions

Read-only jobs inspect the foreground project root. Write-capable jobs require a git repository and create:

```text
~/.packetcode/worktrees/<repo-key>/<job-id>
branch: packetcode-job-<job-id>
base: current HEAD
```

Uncommitted foreground changes are not copied. If worktree creation fails, the job fails closed instead of editing the foreground checkout. Remote write jobs follow the same rule: they create a Git worktree under the remote user's PacketCode state directory and never fall back to the registered checkout.

The active foreground permission profile is snapshotted when a job starts. `/spawn --write` can request file/command approval through the parent UI; read-only jobs reject mutations. Remote jobs also apply the registered computer's write/shell policy as a restrictive overlay—computer `allow` never broadens the global policy, while `ask`, `deny`, and explicit approval remain floors. Completed worktrees are preserved for inspection and are never merged or deleted automatically.

A remote foreground session defaults new jobs to its active Packet Computer. A local session can select one with `--computer <name>`. The resolved `ComputerID`, endpoint/root identity, and working directory are frozen into the job before it is queued. Nested jobs inherit that binding and cannot pivot to another computer. Each active remote job opens and owns a separate SSH/SFTP connection, so a long command does not serialize workflow siblings.

## Agent View

Open with `/agents` or Left Arrow from an empty idle prompt. The full-screen workspace groups jobs into Needs Input, Working, Completed, Failed, and Cancelled sections. Press `n`, type in its bottom prompt, and press Enter to spawn a new task.

Every job has an independent bounded `todo_write` plan. Agent View shows the
completed/total count and current item, and the plan is persisted as job
evidence rather than shared with the foreground or another worker.

| Key | Action |
| --- | --- |
| `Up` / `Down`, `j` / `k` | Move selection. |
| `p` | Peek. |
| `Enter` / `o` | Open transcript. |
| `c` | Cancel active job. |
| `i` | Inject a completed handoff. |
| `x` | Ignore a completed handoff. |
| `Esc` | Clear a draft or return to chat. |

## Results and Persistence

Jobs persist snapshots under `~/.packetcode/jobs/`. Queued, running, and terminal transitions are written immediately; high-frequency activity updates are coalesced and flushed at shutdown. Jobs left active by an unclean prior exit recover as terminal evidence; execution is never resumed. A job that was **running** recovers as `abandoned` with cause `app-exit`, because nothing witnessed how it ended; a job that was only **queued** recovers as `cancelled`, because it provably never started. Abandoned is a distinct terminal state precisely so a loss is never reported as a cancellation somebody chose. packetcode does not resume jobs across a restart (ruled 2026-08-14) — that is a scope boundary, not a missing feature. For SSH jobs, PacketCode cannot guarantee that a detached remote descendant stopped when the connection disappeared.

Recovered jobs carry a durable `Recovered` flag (not inferred from the reason string) and can be explicitly re-run with `/jobs resubmit <id>`. That spawns a *new* job from the saved prompt and links the pair via `ResubmitOf` / `ResubmittedAs`; the abandoned job is never mutated beyond gaining the forward link, so its evidence stays intact. Resubmit is allowed once per job, rejects jobs that ended normally, and refuses a saved prompt larger than `jobs.MaxResubmitPromptBytes` (32 KiB) rather than truncating it. There is no reconnect-and-continue path and none is planned: PCMP9 was cut on 2026-08-14 because durable execution after the originating app closes belongs to PacketAgent, so resubmit is the whole story. See [`packet-computers-loop.md`](packet-computers-loop.md).

Terminal results are not silently inserted into foreground context. Agent View marks them seen and lets the user inject or ignore them. Parent agents that explicitly wait/collect mark results consumed.

Each result includes a bounded artifact manifest derived from tool activity:

- changed files and worktree metadata;
- command/test summaries;
- searches and code-intelligence operations;
- spawned child jobs;
- bounded previews and errors.

Full logs, diffs, and files remain in the job transcript/worktree rather than being copied into foreground context.

## Workflows

`/workflows` runs a Go-native orchestration layer over the same jobs manager. Current step modes are `single` and `parallel`; phases and steps run in declaration order, while a parallel step fans out one child per item and joins the results.

```text
/workflows
/workflows list
/workflows validate focused-review
/workflows run review
/workflows run --computer production review
/workflows run review target="the staged diff"
/workflows stop <run-id>
/workflows stop all
```

Local definitions load in precedence order: built-in, user (`~/.packetcode/workflows/*.toml`), then project (`.packetcode/workflows/*.toml`). A project file with the same name wins. Remote sessions expose built-ins plus local user definitions; remote project workflow discovery is deferred until it can be asynchronous rather than freezing the TUI on SFTP. Malformed higher-precedence files report an error instead of silently falling back.

Write-enabled workflow agents keep the jobs manager's isolation contract: each
gets a separate worktree. Later steps see bound summaries, not another job's
unmerged files. Put one cohesive mutation in one write step; workflow-scoped
shared workspaces are deferred.

Example:

```toml
schema_version = 1
name = "focused-review"

[inputs]
target = "the current working tree"

[[phases]]
name = "analysis"

[[phases.steps]]
name = "review"
mode = "parallel"
bind = "review"
fan_out = ["correctness", "security", "performance"]
prompt = "Review {{.inputs.target}} for {{.item}}. Return concrete findings."

[[phases]]
name = "synthesis"

[[phases.steps]]
name = "synthesize"
mode = "single"
prompt = "Synthesize these findings:\n\n{{.steps.review}}"
```

Optional step fields are `provider`, `model`, `system_prompt`, and `allow_write`. `continue_on_error = true` belongs on a phase. Runs have a default 16-agent guard plus the global job limits. `workflow_token_budget` prevents later steps from starting after completed child usage reaches the configured boundary; a fan-out already running may finish above it.

Version 1 workflows may attach a read-only `[phases.steps.verify]` agent and a
hard `[phases.steps.retry]` cap. Missing or malformed structured verdicts fail
closed, and every work/verifier attempt consumes the same agent and token
budgets. Steps without verifiers remain unverified. See
[Workflows](workflows.md) for the complete schema.

Cancellation is cascading and race-safe: registered children are cancelled, sibling fan-out jobs are stopped after a failure, and terminal results are drained with bounded waits.

Explicit pipeline stages beyond the current ordered phases/steps remain in the
[BACKLOG](../BACKLOG.md).

## Loops

`/loop [interval] <prompt|/command>` repeats normal foreground work:

- `/loop 10m /cost` runs immediately, then every ten minutes.
- `/loop Continue reviewing until complete` runs self-paced.
- `/loop list` shows active loops.
- `/loop stop <id|all>` stops scheduling future iterations.

Self-paced loops ask the model for a versioned `packetcode-loop-decision`
block, retain `LOOP_DONE` for compatibility, and stop after 25 iterations
regardless. A tick during active foreground work queues instead of overlapping.
Loop bodies can spawn agents or invoke workflows.

## Current Limits

- Active jobs do not resume after packetcode restarts. This one is a permanent
  boundary rather than a limit awaiting work: durable execution after the app
  closes belongs to PacketAgent (ruled 2026-08-14). Recovered jobs are reported
  as abandoned and can be explicitly resubmitted as new runs.
- packetcode cannot confirm that a detached remote descendant stopped. That is
  reported honestly rather than papered over: such a job is `abandoned`, not
  `cancelled`. Local commands do have structured teardown evidence — mechanism,
  confirmation, and surviving PIDs from POSIX process groups or Windows Job
  Objects — but SSH can only signal the channel leader and remains unconfirmed.
- Sub-agent transcript output is not streamed into foreground conversation.
- Background agents cannot yet ask arbitrary user clarification questions.
- Explicit workflow pipeline stages beyond ordered phases/steps are deferred.
- Worktree merge/apply and cleanup remain explicit git operations.

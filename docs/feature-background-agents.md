# Background Agents, Loops, and Workflows

This document describes the current shipped orchestration model. Every delegated agent is an `internal/jobs` job with its own session, provider/model selection, context, usage, cancellation, transcript, and result lifecycle.

## Starting Agents

```text
/spawn inspect the authentication flow
/spawn --provider gemini --model gemini-2.5-flash inspect the tests
/spawn --write implement the focused fix
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

Uncommitted foreground changes are not copied. If worktree creation fails, the job fails closed instead of editing the foreground checkout.

The active foreground permission profile is snapshotted when a job starts. `/spawn --write` can request file/command approval through the parent UI; read-only jobs reject mutations. Completed worktrees are preserved for inspection and are never merged or deleted automatically.

## Agent View

Open with `/agents` or Left Arrow from an empty idle prompt. The full-screen workspace groups jobs into Needs Input, Working, Completed, Failed, and Cancelled sections. Press `n`, type in its bottom prompt, and press Enter to spawn a new task.

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

Jobs persist snapshots under `~/.packetcode/jobs/`. Queued, running, and terminal transitions are written immediately; high-frequency activity updates are coalesced and flushed at shutdown. Jobs left active by an unclean prior exit recover as cancelled; execution is not resumed yet.

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
/workflows run review
/workflows run review target="the staged diff"
/workflows stop <run-id>
/workflows stop all
```

Definitions load in precedence order: built-in, user (`~/.packetcode/workflows/*.toml`), then project (`.packetcode/workflows/*.toml`). A project file with the same name wins. Malformed higher-precedence files report an error instead of silently falling back.

Example:

```toml
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

Cancellation is cascading and race-safe: registered children are cancelled, sibling fan-out jobs are stopped after a failure, and terminal results are drained with bounded waits.

Pipelines and adversarial verification/retry are not implemented yet; see [BACKLOG](../BACKLOG.md).

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

- Active jobs do not resume after packetcode restarts.
- Sub-agent transcript output is not streamed into foreground conversation.
- Background agents cannot yet ask arbitrary user clarification questions.
- Workflow pipelines and verifier/retry stages are deferred.
- Worktree merge/apply and cleanup remain explicit git operations.

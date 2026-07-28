# Agent View

Agent View is packetcode's full-screen workspace for background jobs. It uses the same job manager as `/spawn`, model-initiated `spawn_agent`, loops, and workflows.

## Open and Start Work

- Run `/agents`.
- Press Left Arrow from an empty, idle chat prompt.
- Run `/agents <id>` to open one transcript directly.

Agent View opens in list mode so row shortcuts cannot accidentally become prompt text. Press `n` to focus the bottom task composer, then Enter starts a read-only agent. Esc returns from the composer to the list; a second Esc closes the workspace.

## Layout and Controls

Jobs are grouped into Needs Input, Working, Completed, Failed, and Cancelled. Rows show identifier, activity/summary, age, provider/model, state, API token counts, cost, and result status.

| Key | Action |
| --- | --- |
| `n` | Focus the new-agent task composer. |
| Up/Down or `j`/`k` | Move. |
| `p` | Peek at current/completed output. |
| Enter or `o` | Open transcript. |
| `c` | Cancel an active job. |
| `i` | Inject a completed result into foreground context. |
| `x` | Ignore a completed result. |
| Esc | Clear draft or return to chat. |

Live snapshots use monotonic sequence numbers so stale asynchronous updates do not overwrite newer activity.

## Result Lifecycle

Terminal results become `seen`; they are not silently inserted into the next prompt. Injection appends a bounded handoff containing outcome, summary, artifacts, and worktree metadata. Explicit parent-agent wait/collection marks a result `consumed` so it is not offered a second time.

Artifact manifests summarize file changes, commands/tests, searches, child jobs, errors, and worktree diffs without copying raw logs or complete files into foreground model context.

## Write-Capable Jobs

`/spawn --write` creates a dedicated git worktree and branch from current `HEAD`. Agent View and transcript headers show their paths. packetcode never merges or removes them automatically.

## Current Limits

- Active jobs recover as cancelled after restart rather than resuming.
- Arbitrary clarification questions from a sub-agent are not implemented.
- Live sub-agent output stays in its transcript instead of foreground chat.
- Renaming, pinning, and grouping by future Packet Computer are deferred.

See [Background agents, loops, and workflows](feature-background-agents.md).

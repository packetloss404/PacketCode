# Reliability and security hardening, 2026-09-08

The review began at `1900154`. The initial lifecycle fixes were committed as
`45b2caf` and merged to main as `00c14ac`; that push passed all 16 CI jobs.
Three independent review agents then examined security boundaries; jobs,
workflow, MCP, ACP and persistence; and foreground/provider/UI behavior.
The coordinating reviewer handled release tooling, job durability, integration,
and maintenance documentation. This is a focused code review, not a proof of
absence of defects or a live-provider/interactive-terminal certification.

## Reproduced failures and corrections

| Area | Failure | Correction and regression evidence |
| --- | --- | --- |
| ACP prompts | Deferred cleanup cleared a subsequent prompt's active flag, making cancellation ineffective. | Clear once per prompt; deterministic response handoff tests cover completion, cancellation, failure, and missing terminal events. |
| Skill grants | Multiple skill loads retained earlier grants after teardown. Queueing a skill granted authority to the wrong turn; policy/trust changes could lose decisions or restore expired grants. | Separate the session policy from temporary grants; attach queued grants to their actual turn; correlate and acknowledge foreground loads. Regression tests cover queue, reset, trust, remembered decisions, cancellation and stale/background callbacks. |
| Command permissions | CMD case/extension aliases, escaping and expansion bypassed prefix checks; deny matching skipped explicit flag tokens such as `-rf`. | Conservative deny matching across supported command languages, refusal to prefix-allow CMD expansion, and explicit flag matching. Tests use harmless synthetic commands. |
| Secret files | Position-based definition/reference lookup echoed a synthetic dotenv value; source discovery could inspect dotenv files with code extensions. | Apply secret-path checks to explicit, resolved and discovered code-intelligence files. |
| MCP ownership | Concurrent restarts could orphan a replacement; shutdown could miss a replacement still starting. | Serialize admission per server and join startup/restart cleanup during shutdown. |
| MCP writes | A server that did not read stdin blocked the caller before response timeout handling began. | Cancellation covers writes and closes partial transports. A blocked-pipe regression checks prompt return and cleanup. |
| Provider streams | Anthropic/Responses EOF was treated as success without a terminal marker, even when tool arguments had already arrived. | Require `message_stop` / `response.completed`; parser regression tests cover truncated text/tools and explicit failure. Agent error handling returns before tool execution. |
| Job durability | Failed initial, terminal and debounced record writes were forgotten, and shutdown could report success with no durable record. | Reject initial work before launch, keep failed latest snapshots pending, advance durable sequence only after successful save, and retry/report at shutdown. |
| Repeated shutdown | A previously timed-out shutdown could immediately report success while workers remained. | Every shutdown call waits for workers and retries pending writes. |
| Release summary | A presence-check expression appended the configured certificate value into the job summary. | Print presence labels only; synthetic-secret tests cover absent, empty, and configured certificates. No real certificate values were used in the reproduction. |
| Workflow tests | A raw five-second wait failed under concurrent suite load; cleanup used an unchecked short timeout. | Use scaled waits and checked/scaled shutdown. Production workflow deadlines are unchanged. |
| ACP/MCP test timing | Integration load exceeded a three-second protocol wait and a 700 ms parallel-startup assertion. | Scale ACP waits; require every MCP stub to reach an initialization barrier before any can finish, proving actual overlap. |

## Verification

Before the initial merge, `go mod verify`, `go vet ./...`, the full race-enabled
suite, and the four affected package suites passed. The ordinary full suite
hit the workflow test deadline under concurrent load; its isolated rerun passed.
The subsequent main CI run passed on Windows, Linux and macOS, including
lint, vulnerability checking, smoke tests, TUI goldens and release dry run.

Each additional finding has a focused regression. Final local verification on
Windows with Go 1.26.2 passed `go mod verify`, `go vet ./...`, `go test ./...`,
and `go test -race -count=1 ./...`. Lint reported zero issues. Signing-summary
and installer-signature script tests also passed. ACP/MCP lifecycle regressions
passed three consecutive race-enabled runs. The integrated suites ran without
a competing linter after two load-sensitive test assertions were corrected.
The published commit and its CI result are recorded in the completion message
and Git history.

The first hardening push passed 15 of 16 CI jobs. Linux's race-enabled suite
caught a data race in the new MCP test fixture: it replaced a running client's
stdin wrapper while the reader could access it. The follow-up constructs the
gated client before publishing it to the manager; production transport fields
remain immutable after startup. The final CI run verifies that correction.
Tests use local fake providers/servers and synthetic secrets, not paid model
calls or production credentials. Live-provider protocol drift, interactive
terminal behavior and external PacketADE deployment assumptions remain outside
the evidence from these tests.

## Remaining decisions

The September 5 audit's explicit trust decisions remain open: repository-authored
instruction provenance, who can write to ACP stdin, remote plain-HTTP provider
configuration, and default installer signature requirements. They were not
silently changed during this pass. Backup retention and provider replay fixtures
remain useful separate maintenance items; automatic deletion and new transport
support were not added.

The provider reviewer also reproduced an existing cost-estimation limitation:
switching models in one session reprices all cumulative tokens using the last
model (`internal/cost/tally.go`). A synthetic $10 then $1 sequence reported $2.
Start a new session when changing models if cost attribution matters, and check
provider usage for actual charges. Correcting historical mixed-model accounting
requires a separate per-model tally change and cannot reconstruct attribution
from records that never stored it.

Two further code-inspection risks were recorded without full failure repros:
local backend reads lack the SSH backend's file-size/type guard, and SFTP I/O
checks cancellation before operations rather than interrupting every stalled
operation. These are follow-up hardening items, not claimed fixed by this pass.

Use [the maintenance guide](../maintenance.md) for the next month. The older
[audit handoff](../handoff.md) now labels its obsolete upgrade/CI instructions
as historical so they are not mistaken for work still needed.

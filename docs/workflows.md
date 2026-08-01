# Workflows

Packetcode workflows are versioned TOML specifications that orchestrate
ordinary background jobs. Phases and steps run in declaration order; a
`parallel` step fans out over its `fan_out` values. A step may also declare an
independent, read-only verifier and a hard retry cap.

## Commands

```text
/workflows
/workflows list
/workflows validate <name>
/workflows run <name> [key=value...]
/workflows <run-id>
/workflows stop <run-id>
/workflows stop all
```

`/workflow` is an alias. Validation parses the file, checks its schema and
templates, and reports verified versus unverified steps without starting any
agent or spending tokens.

Definitions are resolved in this order, highest precedence first:

1. `<project>/.packetcode/workflows/<name>.toml`
2. `~/.packetcode/workflows/<name>.toml`
3. Built-in workflows such as `review`

A malformed higher-precedence file fails loudly; Packetcode never falls back
to a lower-precedence definition with the same name.

## Version 1 schema

Every TOML file must declare `schema_version = 1`. Missing versions, newer
versions, unknown fields, invalid templates, negative retry caps, and
unsupported verdict contracts are refused before execution.

```toml
schema_version = 1
name = "verified-review"

[inputs]
target = "the current working tree"

[[phases]]
name = "analysis"

[[phases.steps]]
name = "review"
mode = "parallel"
bind = "review"
fan_out = ["correctness", "security", "test gaps"]
prompt = "Review {{.inputs.target}} for {{.item}}. Return concrete evidence."

[[phases]]
name = "synthesis"

[[phases.steps]]
name = "synthesize"
mode = "single"
prompt = "Deduplicate and prioritize these findings:\n\n{{.steps.review}}"

[phases.steps.verify]
prompt = "Check attempt {{.attempt}} against the requested review. Candidate:\n{{.result}}"
provider = "codex"
model = "gpt-5.6-sol"
pass_contract = "packetcode-workflow-verdict-v1"

[phases.steps.retry]
max = 2
```

Step fields `provider`, `model`, and `system_prompt` override the work agent;
`allow_write = true` opts that work into the normal write-agent policy and
worktree behavior. A phase may set `continue_on_error = true`.

Templates may reference:

| Value | Meaning |
| --- | --- |
| `{{.inputs.<name>}}` | A declared input or `/workflows run` override. |
| `{{.steps.<bind>}}` | Final work summaries from an earlier bound step. |
| `{{.item}}` | Current `fan_out` value in a parallel work step. |
| `{{.result}}` | Completed work summary in a verifier prompt. |
| `{{.attempt}}` | One-based attempt number in a verifier prompt. |

## Verification contract

A verifier must explicitly declare `prompt`, `provider`, `model`, and
`pass_contract`. Verifiers are always read-only even when the work agent may
write. Packetcode appends the completed work and an exact response contract to
the verifier prompt.

The supported contract is `packetcode-workflow-verdict-v1`:

```text
<packetcode-workflow-verdict>{"version":1,"verdict":"pass","reason":"tests and checks are green"}</packetcode-workflow-verdict>
```

The response contains exactly one verdict block and it must be the final block;
`verdict` is exactly `pass` or `fail`. Missing tags, malformed JSON, unknown
fields, unknown versions, unknown verdicts, verifier job failures, and
cancelled verifiers all fail closed. A step without a `[verify]` block remains
**unverified** in Workflow View; successful work is never relabeled as passed.

The verifier receives a bounded evidence bundle containing each work job's
summary plus captured artifact summaries/previews such as diffs and test
commands. Because `verify.provider` may name a different service than the work
agent, configuring it explicitly authorizes sending that evidence to the
verifier provider.

## Retry and budget behavior

`retry.max` is the number of additional work attempts after the initial one;
zero or an absent `[retry]` block means no retries. A failed verifier's reason
is appended to the next work prompt. The cap cannot be extended by model
output or nested work.

Every work agent and verifier counts toward the per-run 16-agent guard and the
global job limits. Their input and output usage also counts toward the
aggregate workflow token budget. Packetcode checks the boundary before each
new attempt and before each verifier. A parallel fan-out already in flight may
finish over the boundary, but no later work is spawned once exhaustion is
observed.

Operational work failures still fail the step immediately; retry is driven by
a completed verifier verdict, not used to hide spawn, cancellation, or tool
failures.

Each write-capable retry is a new ordinary write job with its own Git
worktree. It re-attempts the task with verifier feedback; Packetcode does not
merge attempt worktrees together, apply the winning worktree automatically, or
clean up discarded attempts.

## Live inspection and cancellation

Bare `/workflows` opens Workflow View. Step rows display `unverified`,
`pending`, `passed`, or `failed`; retried work and verifier jobs are labeled by
attempt. Selecting any job opens its ordinary background transcript. Cancelling
a run cancels every registered work and verifier child, then drains their
terminal state through the same bounded jobs manager.

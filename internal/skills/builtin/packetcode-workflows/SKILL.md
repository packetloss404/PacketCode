---
description: Write or debug a packetcode workflow TOML - schema_version 1, phases, single and parallel steps, template bindings, verifier pass_contract, and retry caps. Use when authoring .packetcode/workflows/<name>.toml.
---

# Authoring a packetcode workflow

A workflow is a versioned TOML file that orchestrates ordinary background jobs.
Phases and steps run in declaration order; a `parallel` step fans out over its
`fan_out` values.

## Where the file goes

Resolution order, highest precedence first:

1. `<project>/.packetcode/workflows/<name>.toml`
2. `~/.packetcode/workflows/<name>.toml`
3. Built-ins such as `review`

A malformed higher-precedence file **fails loudly**; packetcode never quietly
falls back to a lower-precedence file of the same name.

## Schema v1

`schema_version = 1` is mandatory. Missing versions, newer versions, unknown
fields, invalid templates, negative retry caps, and unsupported verdict
contracts are all refused before anything runs.

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

Step overrides: `provider`, `model`, `system_prompt`, and `allow_write = true`
(which opts the step into normal write-agent policy and worktree behaviour). A
phase may set `continue_on_error = true`.

Template values: `{{.inputs.<name>}}`, `{{.steps.<bind>}}` (an earlier step's
final summaries - requires that step to declare `bind`), `{{.item}}` (current
`fan_out` value), `{{.result}}` and `{{.attempt}}` (verifier prompts only).

## The thing that trips people up

Every write-enabled step gets **its own isolated worktree**. Later steps receive
prior summaries through bindings but **do not inherit prior filesystem edits**.
Keep one cohesive mutation inside one write step. Do not split "edit" and "fix
up the edit" across two steps and expect the second to see the first's files.

Likewise, each write-capable retry is a fresh job with a fresh worktree.
packetcode does not merge attempt worktrees, apply the winner, or clean up
discarded attempts.

## Verifiers

A `[verify]` block must declare all four of `prompt`, `provider`, `model`, and
`pass_contract`. Verifiers are always read-only even when the work agent may
write. The only supported contract is `packetcode-workflow-verdict-v1`, and the
verifier's reply must end with exactly one block:

```text
<packetcode-workflow-verdict>{"version":1,"verdict":"pass","reason":"tests and checks are green"}</packetcode-workflow-verdict>
```

`verdict` is exactly `pass` or `fail`. Missing tags, malformed JSON, unknown
fields or versions, verifier job failure, and cancellation all **fail closed**.
A step with no `[verify]` block stays `unverified` in Workflow View - successful
work is never relabelled as passed.

Naming a different `verify.provider` than the work agent is an explicit
authorisation to send that step's evidence bundle (summaries plus bounded
artifact previews such as diffs and test commands) to that second provider. Say
so when you propose it.

## Retries and budgets

`retry.max` counts **additional** attempts after the first; absent or zero means
none. A failed verifier's reason is appended to the next work prompt. The cap
cannot be raised by model output or nested work. Retry is driven by a completed
verifier verdict only - an operational failure (spawn error, cancellation, tool
failure) fails the step immediately rather than being retried.

Every work agent and verifier counts against the per-run 16-agent guard, the
global job limits, and `workflow_token_budget`. An in-flight parallel fan-out may
finish above the boundary; no later work starts once it is exhausted.

## Commands

`/workflows list`, `/workflows validate <name>`, `/workflows run [--computer <name>] <name> [key=value...]`,
`/workflows <run-id>`, `/workflows stop <run-id>`, `/workflows stop all`.
`/workflow` is an alias. Bare `/workflows` opens Workflow View.

**Validate before running.** `validate` parses the file, checks schema and
templates, and reports verified versus unverified steps without spawning an
agent or spending a token.

`--computer` routes a whole run to one registered SSH computer; placement is a
run option, not schema, so a run cannot silently fan out across machines.

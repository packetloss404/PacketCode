# Foreground Cancellation

Ctrl+C cancellation is lifecycle-scoped and propagates through the active foreground turn.

## Behavior

- First Ctrl+C during a turn cancels the provider request, retry/backoff wait, pending approval, or running tool process tree.
- The thinking spinner stops and the conversation shows `turn cancelled`, not a provider-error block.
- Queued foreground prompts are cleared so cancellation is predictable.
- A second Ctrl+C while the agent goroutine is still draining is ignored rather than exiting.
- Ctrl+C while idle clears a non-empty draft first; from an empty prompt it exits packetcode.
- Background agents and workflows have independent contexts; foreground cancellation does not cascade to them.

Provider stream parsers receive the turn context and close promptly on cancellation. `execute_command` uses process-tree cancellation so children do not continue silently after the UI settles.

## State Ownership

`App.startTurn` creates the cancellable context. The Bubble Tea update loop owns the cancel function and streaming flag. Channel close (`agentDoneMsg`) is the canonical turn boundary; it clears the operation, spinner, and cancellation handle before running a queued prompt.

## Related Controls

- `/cancel <id|all>` cancels background jobs.
- `/workflows stop <id|all>` cascades through workflow children.
- `/loop stop <id|all>` stops future loop iterations but does not forcibly terminate unrelated foreground work.

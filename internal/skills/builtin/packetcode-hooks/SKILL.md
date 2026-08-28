---
description: Add or debug packetcode hooks (user_prompt_submit, pre_tool_use, post_tool_use) and the custom statusline command, including their stdin JSON payloads. Use for "run X before/after a tool" or "block Y".
---

# packetcode hooks and statusline

Hooks and the statusline are shell commands **you** configure in
`~/.packetcode/config.toml`. On Windows they run through PowerShell; elsewhere
through `sh -c`. Each one gets JSON on stdin and runs with the project root as
its working directory. Stdout and stderr are capped at 64 KB per invocation.

## The three hook events

```toml
[[hooks.user_prompt_submit]]
command = "cat .packetcode-context 2>/dev/null || true"
timeout_sec = 2

[[hooks.pre_tool_use]]
matcher = "execute_command"
command = "python scripts/guard-command.py"
timeout_sec = 5

[[hooks.post_tool_use]]
matcher = "patch_file"
command = "gofmt -w $(git diff --name-only -- '*.go') 2>/dev/null || true"
timeout_sec = 10
```

- `user_prompt_submit` runs before the prompt is sent. Successful stdout is
  injected as extra context.
- `pre_tool_use` runs before approval and execution. **A non-zero exit blocks
  the tool call.** This is the only hook that can stop anything.
- `post_tool_use` runs after a tool returns. Successful stdout is appended to
  the tool result; a failure is reported in that appended output.

Fields: `command` (required), `matcher` (tool name; empty or `*` matches all
tools), `enabled` (defaults true), `timeout_sec` (defaults 10).

Hooks are TOML array-of-tables, so several entries per event are fine and they
all run.

## Blocking something

To block an action, use `pre_tool_use` with a `matcher` and exit non-zero.
Match on the **tool name**, then inspect the arguments from stdin - the matcher
itself cannot see arguments.

Blocking force-pushes, as a POSIX example:

```toml
[[hooks.pre_tool_use]]
matcher = "execute_command"
command = "jq -e -r '.arguments.command' | grep -Eqv -- '(--force|-f)\\b.*push|push.*(--force|-f)\\b'"
timeout_sec = 5
```

The reliable shape is a small script in the repo, not a one-liner: read stdin,
decide, `exit 1` to block. Keep the timeout short - a hook that hangs stalls the
turn until it expires.

## Hook stdin

Prompt hook:

```json
{ "event": "UserPromptSubmit", "session_id": "...", "working_dir": "...", "prompt": "user text" }
```

Tool hook:

```json
{
  "event": "PreToolUse",
  "session_id": "...",
  "working_dir": "...",
  "tool_name": "execute_command",
  "tool_call_id": "...",
  "arguments": { "command": "go test ./..." }
}
```

`PostToolUse` adds `result` with `content`, `is_error`, and `metadata`.

## Statusline

```toml
[statusline]
command = "$HOME/.claude/statusline/statusline.sh"
enabled = true
timeout_sec = 2
```

**A statusline is already on by default.** With no `command`, packetcode renders
its own line natively - no subprocess, updated every second. Configure `command`
only to replace it. If the command fails, times out, or prints nothing,
packetcode falls back to the built-in bar. `/statusline` shows the active
output; `/statusline refresh` forces a rerender.

Statusline stdin carries `session_id`, `working_dir`, `project`, `git_branch`,
`provider.{slug,display_name}`, `model.id`, `context_window.{used,max,used_percentage}`,
`cost.total_cost_usd`, `jobs.active`,
`operation.{active,label,elapsed_seconds,queued_inputs}`, `duration_seconds`,
and `version`.

`context_window.used` is **current request occupancy, not cumulative input
tokens** - it drops after `/compact`. `cost.total_cost_usd` stays cumulative.
Do not write a statusline that treats `used` as a running total.

The payload is a superset of Claude Code's, so an existing Claude Code
statusline script works unchanged. The aliases are `cwd`, `model.display_name`,
`context_window.context_window_size`, and
`context_window.current_usage.{input_tokens,cache_creation_input_tokens,cache_read_input_tokens}`
(occupancy lands in `input_tokens`; both cache fields are zero, so a script
summing all three gets the same number). packetcode has no `rate_limits` field,
so a Claude Code rate-limit segment simply renders empty.

## Doing the edit

1. Read the existing `[[hooks.*]]` entries before adding one; append rather than
   replace.
2. Put non-trivial logic in a script file under the repo and point `command` at
   it.
3. Test the command by hand with a representative JSON payload piped into it
   before wiring it up.

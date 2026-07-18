# Hooks And Statusline

Hooks and the custom statusline run shell commands that you configure. On Windows they run through PowerShell; elsewhere they run through `sh -c`. Each command receives JSON on stdin and runs with the project root as its working directory.

## Statusline

```toml
[statusline]
command = "jq -r '\"\\(.provider.display_name) / \\(.model.id) / \\(.context_window.used_percentage)% / $\\(.cost.total_cost_usd)\"'"
enabled = true
timeout_sec = 2
```

`enabled` is optional and defaults to true when `command` is set. If the command fails, times out, or prints nothing, packetcode falls back to the built-in status bar.

**A statusline is on by default.** With no `[statusline].command` configured, packetcode renders its own Claude Code-style line natively (no `jq`, no subprocess) — `[provider·model] 🟢 12% (12K/272K) | 📂 project | 🌿 branch | 💲cost | ◷ op` — updated live every second. Configure `command` only to override it with your own script.

Statusline stdin:

```json
{
  "session_id": "...",
  "working_dir": "/path/to/project",
  "project": "packetcode",
  "git_branch": "main",
  "provider": { "slug": "openai", "display_name": "OpenAI" },
  "model": { "id": "gpt-5.5" },
  "context_window": { "used": 12000, "max": 400000, "used_percentage": 3 },
  "cost": { "total_cost_usd": 0.42 },
  "jobs": { "active": 1 },
  "operation": { "active": true, "label": "thinking", "elapsed_seconds": 12, "queued_inputs": 1 },
  "duration_seconds": 360,
  "version": "v0.0.0"
}
```

Use `/statusline` to show the active output and `/statusline refresh` to force a rerender.

### Claude Code compatibility

The stdin snapshot is a superset of Claude Code's statusline JSON, so a statusline script written for Claude Code works against packetcode unchanged. Alongside the packetcode-native fields above, packetcode also emits the Claude Code aliases:

- `cwd` — mirror of `working_dir`
- `model.display_name` — falls back to `model.id`
- `context_window.context_window_size` — mirror of `context_window.max`
- `context_window.current_usage.{input_tokens,cache_creation_input_tokens,cache_read_input_tokens}` — packetcode reports its whole used total as `input_tokens` (caches zero), so a script summing the three arrives at packetcode's used total

Point `command` at your existing Claude Code statusline to reuse it verbatim:

```toml
[statusline]
command = "$HOME/.claude/statusline/statusline.sh"
```

packetcode ships a molded variant at `docs/statusline/statusline.sh` that keeps the Claude Code look and adds packetcode-native segments (provider, session cost, live operation). packetcode has no `rate_limits` field, so any Claude Code rate-limit segment is simply omitted.

## Hooks

Unix/macOS shell examples:

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

PowerShell examples:

```toml
[[hooks.user_prompt_submit]]
command = "if (Test-Path .packetcode-context) { Get-Content .packetcode-context -Raw }"
timeout_sec = 2

[[hooks.post_tool_use]]
matcher = "patch_file"
command = "$files = git diff --name-only -- '*.go'; if ($files) { gofmt -w $files }"
timeout_sec = 10
```

Hook fields:

| Field | Meaning |
| --- | --- |
| `command` | Shell command to run. Required. |
| `matcher` | Tool name for tool hooks. Empty or `*` matches all tools. |
| `enabled` | Optional; defaults to true. |
| `timeout_sec` | Optional; defaults to 10 seconds. |

Hook behavior:

- `user_prompt_submit` runs before the prompt is sent. Successful stdout is injected as extra context.
- `pre_tool_use` runs before approval/tool execution. A non-zero exit blocks the tool call.
- `post_tool_use` runs after a tool returns. Successful stdout is appended to the tool result. Failures are reported in the appended hook output.

Prompt hook stdin:

```json
{
  "event": "UserPromptSubmit",
  "session_id": "...",
  "working_dir": "/path/to/project",
  "prompt": "user text"
}
```

Tool hook stdin:

```json
{
  "event": "PreToolUse",
  "session_id": "...",
  "working_dir": "/path/to/project",
  "tool_name": "execute_command",
  "tool_call_id": "...",
  "arguments": { "command": "go test ./..." }
}
```

`PostToolUse` payloads also include `result`:

```json
{
  "content": "tool output",
  "is_error": false,
  "metadata": {}
}
```

Stdout and stderr are capped at 64 KB per hook or statusline command.

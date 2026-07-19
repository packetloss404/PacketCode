# Security and Permissions

packetcode runs provider requests and local tools as your user. The permission policy is a decision layer, not an OS sandbox. Review projects, hooks, MCP servers, commands, and custom provider endpoints before trusting them.

## Modes and Profiles

| TUI mode | Config profile | Behavior |
| --- | --- | --- |
| Manual | `ask` | Read/search/list auto; edits, shell, MCP, and agents ask. |
| Accept Edits | `accept_edits` | File edits auto; shell, MCP, and agents ask. |
| Auto | `auto` | File edits and shell auto; MCP and other approval-gated tools ask. |
| Plan | `read_only` plus plan instruction | Read/search/list auto; mutations denied. |
| Bypass Permissions | `bypass` | Tools auto unless a deny rule matches. |

Shift+Tab cycles the first four, including during an active foreground turn. The new profile applies to subsequent tool actions and re-evaluates a visible approval. An already-running command is not interrupted. Plan's safety profile applies immediately; its model-facing planning instruction is naturally strongest on turns started in Plan mode.

Bypass is deliberately outside the forward cycle and shown distinctly. Enter with `--trust`, `trust_mode = true`, or `/trust on`; Shift+Tab exits it to Manual. Explicit deny rules remain a floor.

## Approval Menu

1. Yes
2. Yes, and do not ask again
3. No

Option 2 installs a session rule. For `execute_command`, packetcode remembers the exact command string rather than inferring a broad command family. Other tools are remembered by tool name. Inspect session policy with `/permissions`.

## Rules

```toml
[permissions]
profile = "ask"

[[permissions.rules]]
tool = "execute_command"
action = "deny"
command_prefix = ["rm", "-rf"]
reason = "refuse broad recursive deletes"

[[permissions.rules]]
tool = "filesystem__*"
action = "ask"
```

Actions are `allow`, `ask`, and `deny`. Rules can match exact tool names, suffix wildcards, `mcp:*`, all tools (`*`), exact shell commands, or tokenized command prefixes. Later matching rules win according to policy specificity/order.

`[permissions.tools]` remains accepted for backward compatibility; prefer named profiles and `[[permissions.rules]]`.

## Project Boundaries

- Native file tools resolve paths inside the project and reject symlink escapes.
- Reads are bounded and skip binary content where appropriate.
- Write-capable background agents require isolated git worktrees based on current `HEAD`; uncommitted foreground changes are not copied.
- packetcode does not merge or delete completed worktrees automatically.
- Bounded model-facing outputs do not replace complete persisted transcripts/worktrees.

## MCP, Hooks, and Custom Providers

- MCP processes start as local child processes. Calls are policy-gated, but server startup itself is not sandboxed.
- MCP inherits a small environment allowlist plus explicit `env`/`env_from`; configure only trusted binaries.
- Hooks and statusline commands execute configured shell text as your user.
- Custom providers receive the system prompt, conversation, tool schemas, and tool results. Use HTTPS for hosted endpoints and plain HTTP only on controlled local/private networks.

## Background and Workflow Trust

Running background jobs snapshot policy at startup. Foreground mode changes do not retroactively broaden an already-running background agent. Workflow children obey job caps, per-run caps, worktree requirements, cancellation, and optional token boundaries.

## Diagnostics

```bash
packetcode doctor --check permissions
packetcode doctor --json
```

Never paste secrets into bug reports or commit generated PTY captures without inspection; terminal welcome/status output may contain local paths or account information.

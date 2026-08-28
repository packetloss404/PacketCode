---
description: Edit packetcode's own config.toml - providers, models, permission profiles and rules, background-agent limits, feature gates, and PACKETCODE_HOME. Use before changing any packetcode setting.
---

# Editing packetcode's config.toml

## Where the file is

`~/.packetcode/config.toml`, written atomically with user-only permissions.

`PACKETCODE_HOME` relocates the whole data home (config, sessions, jobs,
worktrees, commands, workflows, skills, themes, logs, cost tallies). It must be
an **absolute** path, and it does not replace the process `HOME`. `packetcode
doctor --json` reports `effective_home` and `home_source`, which is how you
confirm an override actually took effect rather than assuming it did.

There is no project-level `config.toml`. Prompt commands, workflows, and skills
have project scopes; settings do not.

## Shape of the file

```toml
[default]
provider = "codex"
model = "gpt-5.6-sol"

[providers.anthropic]
api_key = "sk-ant-..."
default_model = "claude-opus-4-8"
```

`[default]` picks the startup provider/model. `[providers.<slug>]` holds that
provider's key and saved default model. `codex` reuses the Codex CLI OAuth store
and `ollama` is keyless; every other hosted provider needs its own key.
`PACKETCODE_OLLAMA_HOST` overrides `[providers.ollama].host` at runtime.

A custom OpenAI-compatible endpoint is a provider slug with
`type = "openai_compatible"` plus `base_url`, and optionally `api_key_env`,
`api_key_required = false` for keyless local servers, `headers`, and
`[[providers.<slug>.models]]` entries supplying fallback model metadata
(`id`, `context_window`, `supports_tools`).

## Permissions

Prefer a named profile plus ordered rules. `[permissions.tools]` still parses,
but it is the older inline form.

```toml
[permissions]
profile = "balanced"

[[permissions.rules]]
tool = "execute_command"
action = "deny"
command_prefix = ["rm", "-rf"]
reason = "refuse broad recursive deletes"
```

Built-in profiles:

- `balanced` (alias `ask`) - read/search/list allowed; writes, shell,
  `spawn_agent`, and MCP ask.
- `accept_edits` - `write_file` and `patch_file` auto-approve; shell,
  `spawn_agent`, and MCP still ask.
- `auto` - edits and shell auto-approve; MCP and other gated tools still ask.
- `read_only` - read/search/list allowed, everything else denied, with a hard
  non-mutation floor that allow rules cannot lift.
- `bypass` - approve unless an explicit deny matches. Entered deliberately via
  `/trust on` or `--trust`; it is not in the Shift+Tab cycle
  (Ask -> Accept Edits -> Auto -> Plan).

A custom profile is `[permissions.profiles.<name>]` with values `ask`, `allow`,
`deny`, a `default` fallback, exact native tool names, and `mcp` covering every
MCP alias at once.

Rule ordering: a matching **deny is a safety floor** and wins outright; among
other matches, later rules win. `command` matches an exact `execute_command`
string; `command_prefix` matches the shell command from the beginning.

## Behavior and limits

```toml
[behavior]
trust_mode = false
auto_compact_threshold = 80
background_max_concurrent = 4
background_max_depth = 2
background_max_total = 32
background_token_budget = 0
workflow_token_budget = 0
provider_max_retries = 3
provider_stall_timeout = 60
```

The background settings govern `/spawn` and the `spawn_agent` tool together:
concurrency, nesting depth, and total jobs per run.
`background_default_provider`/`background_default_model` override the foreground
choice for jobs; empty means inherit. Token budgets stop work at a completed
provider/tool boundary, and `0` disables them. `provider_stall_timeout` aborts a
stream that goes silent for that many seconds.

Write-capable background agents get a git worktree under
`~/.packetcode/worktrees/<repo-key>/<job-id>` on branch
`packetcode-job-<job-id>`. Read-only jobs do not. There is no config key for
that location.

## Feature gates

`[packet_computers]`, `[acp]`, `[sugar]`, and `[conduit]` are on/off gates with
matching `PACKETCODE_*_ENABLED` environment overrides that apply to the current
process only and are never copied back into the file by an unrelated save.
Disabling a feature never deletes its state - credentials, registries, and
sessions survive and come back on re-enable.

## Doing the edit

1. Read the current file first; never regenerate it from this document.
2. Change only the keys asked for. Preserve unrelated sections and comments.
3. Prefer adding a `[[permissions.rules]]` entry over widening a profile.
4. Verify with `packetcode doctor --json` rather than assuming the parse
   succeeded.

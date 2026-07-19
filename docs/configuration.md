# Configuration

packetcode reads `~/.packetcode/config.toml`. The file is written atomically with user-only permissions.

## Full Example

```toml
[default]
provider = "codex"
model = "gpt-5.6-sol"

[providers.codex]
default_model = "gpt-5.6-sol"

[providers.anthropic]
api_key = "sk-ant-..."
default_model = "claude-opus-4-8"

[providers.gemini]
api_key = "AI..."
default_model = "gemini-2.5-pro"

[providers.minimax]
api_key = "sk-..."
default_model = "MiniMax-M3"

[providers.deepseek]
api_key = "sk-..."
default_model = "deepseek-chat"

[providers.grok]
api_key = "xai-..."
default_model = "grok-4.5"

[providers.mistral]
api_key = "..."
default_model = "mistral-large-latest"

[providers.ollama]
host = "http://localhost:11434"
default_model = "qwen2.5-coder:14b"

[providers.localai]
type = "openai_compatible"
display_name = "LocalAI"
base_url = "http://localhost:8080/v1"
default_model = "coder-large"
api_key_required = false

[[providers.localai.models]]
id = "coder-large"
context_window = 32768
supports_tools = true

[behavior]
trust_mode = false
auto_compact_threshold = 80
max_input_rows = 10
background_max_concurrent = 4
background_max_depth = 2
background_max_total = 32
background_default_provider = ""
background_default_model = ""
background_token_budget = 0
workflow_token_budget = 0
provider_max_retries = 3
provider_stall_timeout = 60

[permissions]
profile = "balanced"

[permissions.profiles.balanced]
default = "ask"
read_file = "allow"
search_codebase = "allow"
list_directory = "allow"
list_symbols = "allow"
find_definition = "allow"
find_references = "allow"
get_diagnostics = "allow"
write_file = "ask"
patch_file = "ask"
execute_command = "ask"
spawn_agent = "ask"
mcp = "ask"

[[permissions.rules]]
tool = "execute_command"
action = "deny"
command_prefix = ["rm", "-rf"]
reason = "refuse broad recursive deletes"

[statusline]
command = ""
timeout_sec = 2

[mcp.filesystem]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/project"]
enabled = true
timeout_sec = 10
```

## Sections

`[default]` selects the provider/model used at startup.

`[providers.<slug>]` stores API keys, saved default models, the Ollama host, and custom OpenAI-compatible endpoint settings. `codex` reuses the Codex CLI OAuth store and `ollama` is keyless; hosted API providers require their own API key. `PACKETCODE_OLLAMA_HOST` overrides `[providers.ollama].host` at runtime. Custom providers use `type = "openai_compatible"`, `base_url`, optional `api_key_env`, optional `api_key_required = false` for keyless local endpoints, optional `headers`, and optional `[[providers.<slug>.models]]` fallback metadata.

`[behavior]` controls trust mode, input height, automatic compaction, provider resilience, and background/workflow limits. Context occupancy includes the system prompt, transcript, tool schemas, and pending input estimate; compaction runs automatically before an over-threshold turn when enough history exists.

Background-agent settings affect both `/spawn` and the `spawn_agent` tool:

- `background_max_concurrent` limits how many jobs can run at once; extra jobs stay queued.
- `background_max_depth` limits nested `spawn_agent` calls.
- `background_max_total` caps jobs created during one packetcode run.
- `background_default_provider` and `background_default_model` override the foreground provider/model for jobs when set; empty values inherit the active provider/model.
- `background_token_budget` stops each background job at a completed provider/tool boundary after its cumulative input+output usage reaches the limit; `0` disables it.
- `workflow_token_budget` stops a workflow from starting later steps after completed child usage reaches the aggregate boundary; an already-running parallel fan-out may finish above it. `0` disables it.

Provider resilience settings:

- `provider_max_retries` — how many times to retry a failed provider request (default 3).
- `provider_stall_timeout` — abort a provider stream that goes silent for this many seconds (default 60).

Write-capable background agents create git worktrees under `~/.packetcode/worktrees/<repo-key>/<job-id>` using branch `packetcode-job-<job-id>` and the current `HEAD` commit as the base. This state directory is internal; there is no config key for it yet. Read-only jobs do not create worktrees.

Background job snapshots under `~/.packetcode/jobs/` also persist compact artifact metadata. Artifact previews are bounded and intended for manifests, not raw log or diff storage.

`[permissions]` controls tool-call policy. `profile` can name a built-in profile (`balanced`/`ask`, `accept_edits`, `auto`, `read_only`, or `bypass`) or a custom `[permissions.profiles.<name>]` table.

- `balanced` / `ask` allows read/search/list and prompts for writes, shell commands, background-agent spawns, and MCP tools.
- `accept_edits` auto-approves `write_file` and `patch_file`, but asks for `execute_command`, `spawn_agent`, and MCP tools.
- `auto` auto-approves file edits and shell commands, but still asks for MCP and other explicitly approval-gated tools.
- `read_only` allows read/search/list and denies everything else.
- `bypass` auto-approves tools unless an explicit deny rule matches.

Shift+Tab cycles Ask → Accept Edits → Auto → Plan in the TUI, including during an active turn. Bypass is entered deliberately with `/trust on` or `--trust` and is not in the forward cycle.

Custom profile values are `ask`, `allow`, and `deny`. Use `default` as the fallback, exact tool names for native tools, and `mcp = "ask"` for all MCP aliases.

`[permissions.tools]` is still accepted as a backward-compatible inline rule table, but new config should prefer named profiles plus `[[permissions.rules]]`.

`[[permissions.rules]]` adds ordered policy rules. Later rules win when more than one matches. `command` matches an exact `execute_command` string, and `command_prefix` matches shell command fields from the beginning.

`[statusline]` configures an optional shell command that replaces the native statusline, which is enabled by default even when `command` is empty. See [Hooks and statusline](hooks-and-statusline.md).

`[mcp.<name>]` declares stdio MCP servers. See [MCP servers](mcp.md).

## Custom Prompt Commands

Markdown prompt commands live in:

- `~/.packetcode/commands/<name>.md`
- `.packetcode/commands/<name>.md`

Project commands override user commands with the same name. Built-in slash commands cannot be shadowed.

```markdown
---
description: Review the selected code
---
Review this code and call out correctness risks:

$ARGUMENTS
```

## Workflows

Workflow files are separate from `config.toml`:

- User: `~/.packetcode/workflows/<name>.toml`
- Project: `.packetcode/workflows/<name>.toml`

Project definitions override user definitions, which override built-ins. See [Background agents](feature-background-agents.md#workflows) for the current schema.

## Themes

packetcode reads `~/.packetcode/theme.toml` when present. Presets live in `docs/themes/`.

```bash
cp docs/themes/high-contrast.toml ~/.packetcode/theme.toml
```

A missing theme file is ignored. A malformed theme logs one warning and packetcode keeps the built-in theme.

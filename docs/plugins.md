# Plugins

packetcode does not load arbitrary Go, JavaScript, or Python plugins in-process.
The current plugin model is deliberately declarative: compose the extension
surfaces that already run through packetcode's approval, config, and diagnostics
paths.

## Supported Extension Surfaces

- **MCP servers** add model-callable tools through `[mcp.<name>]` blocks.
- **Custom OpenAI-compatible providers** add model backends through
  `[providers.<slug>] type = "openai_compatible"`.
- **Markdown prompt commands** add reusable slash prompts from
  `~/.packetcode/commands` or `.packetcode/commands`.
- **Workflow TOML** adds reusable sequential/parallel orchestration from
  `~/.packetcode/workflows` or `.packetcode/workflows`.
- **Hooks and statusline commands** run local automation around prompts, tools,
  and the bottom statusline.
- **Themes and permission profiles** customize presentation and trust policy.

This keeps extension code at process boundaries instead of inside packetcode's
address space. MCP tool calls still require approval, custom providers use the
same provider registry and model picker, and `packetcode doctor` can inspect the
configuration without executing plugin code.

## Pack Pattern

A practical plugin pack today is a checked-in directory with a README plus the
config snippets it contributes:

```text
my-pack/
  README.md
  commands/review.md
  workflows/review.toml
  config.example.toml
```

Install the pieces by copying command/workflow files into their `.packetcode/`
or `~/.packetcode/` directories, and copying the relevant `[mcp.*]`,
`[providers.*]`, `[hooks.*]`, `[statusline]`, theme, or permission blocks into
`~/.packetcode/config.toml`.

## Deferred

A first-class `packetcode plugin install/list/enable` command, manifest format,
marketplace workflow, hot reload, and dependency management are deferred. MCP
remains the executable tool boundary for now.

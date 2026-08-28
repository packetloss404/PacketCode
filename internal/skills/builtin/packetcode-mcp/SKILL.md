---
description: Configure a stdio MCP server for packetcode - the [mcp.<name>] block, tool-name prefixing, env and env_from secret passing, timeouts, and why HTTP transports are unavailable. Use when adding or debugging an MCP server.
---

# Adding an MCP server to packetcode

packetcode spawns each configured MCP server as a child process at startup,
discovers its tools over the handshake, and exposes them to the model like
built-in tools. Tool calls become `tools/call` RPCs.

**stdio transport only.** HTTP+SSE, WebSocket, and Streamable HTTP remotes are
not supported. There is no URL or transport field; do not invent one. The
security design for Streamable HTTP is approved
(`docs/mcp-http-trust-contract.md`) but no client has landed, so a request for a
remote MCP server needs a "not yet" answer, not a config guess.

## The block

```toml
[mcp.<name>]
command     = "binary-name-or-absolute-path"
args        = ["--flag", "value"]
env         = { KEY = "value" }
env_from    = ["SOME_SERVICE_TOKEN"]
enabled     = true
timeout_sec = 10
```

- `command` - the executable. A bare name must be on `$PATH`; absolute paths are
  fine.
- `args` - passed in order.
- `env` - extra variables for this server. The child inherits only a small
  launch allowlist from packetcode (path, home/cache/temp dirs, proxy, locale,
  certificate settings); values here win on conflict.
- `env_from` - names of variables copied from packetcode's own environment.
  **This is where secrets belong** - a token in `env` is a token written to
  `config.toml`. Provider API keys are never inherited unless explicitly named.
- `enabled = false` keeps the block on disk but skips spawning.
- `timeout_sec` - budget for `initialize` and `tools/list`, default 10. Raise it
  for cold `npx`/`uvx`/docker-pull starts.

`<name>` becomes the tool prefix: the model sees `<name>__<tool>`, so
`read_file` on a server named `filesystem` is `filesystem__read_file`. Pick a
short, stable name - renaming the block renames every tool.

## Failure behaviour

A server that fails to spawn - missing binary, handshake timeout, `tools/list`
error - is logged to stderr and to the `/mcp` table and **never prevents
packetcode from starting**. Native tools and other servers keep working. Check
`/mcp` first when a server's tools are missing.

## Trust

A configured MCP server is trusted local code. packetcode starts the command as
your user; approval prompts gate the *tool calls*, they do not sandbox the child
process. Only add servers and arguments you would run in your own terminal, and
say this plainly when proposing one.

MCP tools follow the normal permission policy: Ask, Accept Edits, and Auto all
prompt for them; only Bypass or an explicit allow rule approves them
automatically.

## Working examples

```toml
[mcp.filesystem]
command = "npx"
args    = ["-y", "@modelcontextprotocol/server-filesystem", "/home/alice/projects"]

[mcp.git]
command     = "uvx"
args        = ["mcp-server-git", "--repository", "."]
timeout_sec = 20

[mcp.fetch]
command = "uvx"
args    = ["mcp-server-fetch"]
```

The filesystem server is scoped to the roots given in `args` - that argument
list is the sandbox, so review it rather than passing `/`.

## Doing the edit

1. Confirm the binary exists and runs before adding the block.
2. Put secrets in `env_from`, never in `env`.
3. Restart packetcode, then check `/mcp` to confirm the handshake and see the
   discovered tool names.

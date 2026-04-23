# MCP servers

packetcode can extend its tool surface with external **MCP (Model
Context Protocol) servers**. Each server is an external binary that
speaks MCP over stdio JSON-RPC 2.0; packetcode spawns it as a child
process at startup, discovers its tools over the handshake, and
exposes each discovered tool to the LLM exactly like a built-in tool.
Tool calls are forwarded as `tools/call` RPCs; the server's reply is
surfaced to the conversation pane and counts against the existing
approval flow.

Round 7 ships **stdio transport only** (the MCP spec's baseline). HTTP
+ SSE, WebSocket, and StreamableHTTP remotes are out of scope for this
round.

---

## Configuring a server

Every server lives under a `[mcp.<name>]` block in
`~/.packetcode/config.toml`. The `<name>` becomes the prefix on every
tool the server exposes — i.e. the LLM sees `<name>.<tool>`, so a
`read_file` tool on the filesystem server shows up as
`filesystem.read_file`.

```toml
[mcp.<name>]
command     = "binary-name-or-absolute-path"
args        = ["--flag", "value"]          # optional
env         = { KEY = "value" }            # optional
enabled     = true                         # optional; defaults to true
timeout_sec = 10                           # optional; initialize budget
```

Fields:

- **command** — the executable packetcode spawns. If it's a bare name
  it must be on `$PATH`. Absolute paths are fine.
- **args** — command-line arguments passed in order.
- **env** — extra environment variables merged on top of packetcode's
  own environment (your values win on conflict).
- **enabled** — set to `false` to keep the block on disk but skip
  spawning at startup. Omit the field to keep it enabled.
- **timeout_sec** — how long packetcode waits for the server to reply
  to `initialize` and `tools/list`. Bump this for slow-starting servers
  (npm-cold-cached, docker-pull-then-run, etc.). Defaults to 10.

A failure to spawn a server — binary missing, handshake timeout,
`tools/list` error — is logged to stderr and the `/mcp` table, but
**never prevents packetcode from starting**. Native tools and other
MCP servers keep working.

---

## Example: filesystem server

[`@modelcontextprotocol/server-filesystem`](https://www.npmjs.com/package/@modelcontextprotocol/server-filesystem)
is the reference read/write filesystem server from the MCP
organisation. It exposes `read_file`, `write_file`, `list_directory`,
`search_files`, and a few others scoped to one or more directory
roots.

```toml
[mcp.filesystem]
command = "npx"
args    = ["-y", "@modelcontextprotocol/server-filesystem", "/home/alice/projects"]
```

After packetcode starts, the LLM sees `filesystem.read_file`,
`filesystem.write_file`, etc. Since all MCP tools are
approval-gated, every call routes through the same Y/N prompt as a
native `write_file`.

---

## Example: git server

[`mcp-server-git`](https://github.com/modelcontextprotocol/servers/tree/main/src/git)
wraps `git` with a tool surface: `git.log`, `git.diff`, `git.show`,
`git.blame`, etc.

```toml
[mcp.git]
command     = "uvx"
args        = ["mcp-server-git", "--repository", "."]
timeout_sec = 20
```

`uvx` is the one-shot launcher bundled with
[`uv`](https://github.com/astral-sh/uv); it downloads the package the
first time, which is why the timeout is bumped to 20.

---

## Example: fetch server

[`mcp-server-fetch`](https://github.com/modelcontextprotocol/servers/tree/main/src/fetch)
lets the LLM GET URLs and receive the body back as a string —
equivalent to a sandboxed `curl`.

```toml
[mcp.fetch]
command = "uvx"
args    = ["mcp-server-fetch"]
```

No extra args, no extra env — `uvx` handles the pip install-cum-run.
As with every MCP tool, every `fetch.fetch` call requires approval.

---

## Tool names and approval

- MCP tools are ALWAYS prefixed with provider-safe aliases:
  `<server>__<tool>`. Two servers that both expose `read_file` won't
  collide — you'll see e.g. `filesystem__read_file` and
  `git__read_file`.
- Native tools (`read_file`, `write_file`, `patch_file`,
  `search_codebase`, `list_directory`, `execute_command`,
  `spawn_agent`) are never prefixed and never collide with MCP tools.
- Every MCP tool returns `true` from `RequiresApproval()`, no matter
  what the server is. Trust mode (`--trust` or `trust_mode = true`)
  auto-approves them like any other destructive tool.

The approval modal shows the exact tool name (`filesystem__write_file`)
and the arguments the LLM proposed, so you can inspect them before
pressing `Y`.

---

## Debugging

**`/mcp`** — list configured servers, their state, tool count, pid,
and command. Example output:

```
MCP servers
NAME         STATE      TOOLS  PID     COMMAND
filesystem   running    8      41283
git          running    5      41291
fetch        failed     0      -       command not found (uvx)
legacy       disabled   0      -       (disabled)
```

**`/mcp logs <name>`** — tail the last 50 lines of the server's stderr
log. The log lives at `~/.packetcode/mcp-<name>.log` and is appended
across runs (no auto-rotation — delete it manually when it grows).
Use this when a server fails the handshake; a lot of servers print
diagnostics to stderr before exiting.

---

## Known limits (Round 7)

- **No hot-reload.** Add or change a `[mcp.<name>]` block and you need
  to restart packetcode for it to take effect.
- **No `/mcp restart <name>`.** Deferred to Round 8. If a server
  crashes mid-session, every call to its tools returns a friendly
  "restart packetcode to reconnect" error; native tools and other
  MCP servers keep working.
- **stdio transport only.** HTTP+SSE, WebSocket, StreamableHTTP
  remotes are deferred.
- **No MCP prompts, resources, sampling, elicitation, logging, or
  roots.** Round 7 implements tools-only. Server-initiated requests
  for those surfaces are refused with a JSON-RPC `-32601` (method not
  supported), so the server stays healthy; packetcode just ignores
  them.
- **No per-server trust.** Every MCP call is approval-gated by the
  same flag as native destructive tools.
- **Text content only.** Server responses carrying image/audio/
  resource content are flattened to `[<type> content omitted]` in
  the tool result. Tools can still be useful — they just can't
  surface non-text payloads to the LLM.

See `docs/feature-mcp.md` for the full design spec.

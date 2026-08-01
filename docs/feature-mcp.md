# MCP Implementation Notes

For user configuration and diagnostics, see [MCP servers](mcp.md). This file records the maintained implementation contract rather than the pre-implementation design history.

## Contract

- Transport: local stdio, one JSON-RPC 2.0 object per UTF-8 line.
- Startup: initialize, `notifications/initialized`, then `tools/list`.
- Tool calls: `tools/call` through the normal permission policy.
- Provider name: `<sanitized-server>__<sanitized-tool>` with deterministic collision handling.
- Logs: bounded/redacted display from `~/.packetcode/mcp-<name>.log`.
- Failure isolation: one missing/crashed server does not prevent packetcode or other servers from running.
- Recovery: `/mcp restart <name>` replaces one process and its tool adapters
  without disturbing other configured servers.

Server processes inherit a small launch-environment allowlist plus explicit `env` and named `env_from` variables. This limits accidental secret inheritance; it is not a sandbox.

The client handles responses, notifications, out-of-order request IDs, cancellation, timeouts, EOF, process exit, and every signed `int64` request ID. Unsupported server-initiated requests receive JSON-RPC `-32601` where applicable.

## Supported Surface

Only tools are exposed. Prompts, resources, sampling, elicitation, roots, and non-text content are not model-facing today.

## Current Limits

- stdio only; no Streamable HTTP. The
  [network trust contract](mcp-http-trust-contract.md) is approved and backed
  by a transport-independent validator/redaction suite, but no transport flag
  or HTTP client exists.
- no live configuration reload; restart uses the configuration loaded at
  PacketCode startup.
- no automatic reconnect after process death.
- non-text result blocks are represented as omitted content.
- MCP calls remain approval-gated unless the active policy/rule allows them.

## Future Streamable HTTP Gate

`internal/mcp/http_trust.go` now validates the approved v1 decisions: exact
origins and explicit ports, separately allowed address classes, mixed-DNS
rejection, denied or bounded bodyless same-origin GET/HEAD redirects, disabled
ambient proxies, system-root TLS, identity compression, atomically bound
target-only environment credentials, per-call approval, bounded response/event/
header/output sizes, bounded timeout, and manual reconnect. It also defines a
credential-bound, labelled/redacted
untrusted-output envelope. This code is not wired to configuration or network
I/O; the implementation loop must use it rather than creating a second policy.

# Sugar Provider

Packetcode's built-in `sugar` provider connects the existing local Go agent to the Sugar service.

## Contract

- Authentication: a private `sgr_...` bearer key.
- Base URL: the Sugar deployment's `/api/v1` path.
- Model discovery: authenticated `GET /models` on every provider initialization.
- Default model: `sugar/conduit`.
- Direct models: every additional `sugar/*` ID returned by the live catalog.
- Completion transport: OpenAI-compatible `POST /chat/completions` with SSE.
- Local tools, permissions, sessions, worktrees, background agents, and MCP remain owned by Packetcode.
- Compute routing and upstream usage metering remain owned by Sugar and Runpod.

## Login

```text
packetcode sugar login --server https://your-sugar-service.up.railway.app --name your-name
```

The command requests a short-lived device code, opens Sugar's approval page, and shows the same code in the terminal. A signed-in member reviews the named device and approves it; Packetcode polls at Sugar's required interval until it receives a one-time, member-owned API key. It then validates the live model list, persists the provider with user-only config permissions, and activates `sugar/conduit`. Use `--no-browser` to open the printed URL manually.

The short user code and opaque device code expire after ten minutes. Sugar stores only purpose-separated HMACs, creates the `sgr_live` key in the same transaction that consumes an approved request, and never sends the key to the browser.

Remote Sugar URLs must use HTTPS. Plain HTTP is accepted only for localhost development.

## Live model selection

Use `/model` in Packetcode. The picker is populated from Sugar at runtime, so the service can add, remove, or advance models without a new CLI build. Choosing `sugar/conduit` restores task-aware routing; choosing another returned ID pins that Sugar lane.

## Phase 2

The cross-platform desktop application for Windows, macOS, and Linux should reuse this exact provider contract. It needs no separate compute integration: authenticate with Sugar, fetch `/models`, send chat requests, and render Sugar usage/routing metadata while keeping project tools local.

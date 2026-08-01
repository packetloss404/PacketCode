# PacketCode Feature Truth — BridgeCode-Plus Audit

Date: 2026-07-27
Updated: 2026-08-01

This is an implementation audit, not a parity claim. BridgeCode is no longer an
active BridgeSpace integration; it is useful only as a historical terminal-agent
workflow benchmark. PacketCode's current bar is evidence-backed terminal-native
work with clear bounds and an explicit handoff to PacketADE or PacketAgent where
another product owns the lifecycle.

Status values:

- **Present** — exercised by current implementation tests.
- **Partial** — useful implementation exists, with an important named limit.
- **Missing** — no current implementation.
- **External gate** — code is ready only up to a release, credential, host, or
  sibling-product boundary that this repository cannot prove alone.

## Audit matrix

| Capability | Status | Evidence | Remaining limit / decision |
| --- | --- | --- | --- |
| Subscription provider | Present | `internal/provider/codex/`, `codexauth/`, provider registry tests, and live opt-in tests support the existing Codex/ChatGPT login. | Only sanctioned subscription paths belong here; do not imitate unsupported consumer-login integrations. |
| API-key providers | Present | Native OpenAI, Anthropic, Gemini, MiniMax, DeepSeek, Grok, Mistral, OpenRouter, and custom OpenAI-compatible implementations all have focused tests. | Live provider contracts remain opt-in because ordinary CI must be credential-free. |
| Local models | Present | Native Ollama discovery, tool support, management, hardware sizing, context, and streaming paths have focused tests. | MLX or other runtimes are optional only if they preserve the same tool/stream contract. |
| Provider/model/reasoning changes during a session | Present | `internal/app/provider_switch.go`, `/provider`, `/model`, and `/effort` tests rebuild the active runtime and persist supported choices. | Reasoning controls are provider/model-specific rather than pretending to be universal. |
| Foreground session durability | Present | `internal/session/` persists transcripts, usage, metadata, resume state, and bounded transcript hydration. | The undo backup stack remains process-local. |
| Background work recovery | Partial | `internal/jobs/` persists snapshots, transcripts, results, artifacts, and worktree metadata; abandoned active jobs recover explicitly as cancelled. | PacketCode does not claim its TUI process survives closure. Restart/resubmit assistance is still useful; durable continuation belongs to PacketAgent. |
| Permissions and denial floors | Present | `internal/permissions/`, live Shift+Tab policy tests, approval tests, read-only Plan mode, explicit Bypass mode, and deny-rule precedence. | Allowed shell/MCP programs are not an OS or container sandbox. |
| Read-only and write sub-agents | Present | `spawn_agent`, nested limits, read-only default, write opt-in, isolated worktrees, artifact manifests, cancellation, and collection have focused tests. | No automatic merge/conflict resolution and no arbitrary clarification prompt from a child. |
| Reusable commands and workflows | Present | User/project Markdown commands plus versioned workflow TOML, offline validation, sequential/parallel phases, bindings, joins, fail-closed step verifiers, bounded retries, cancellation, views, and token/agent bounds. | Explicit pipeline stages beyond ordered phases/steps and a broader example library remain open. |
| Bounded repeat loops | Present | `/loop` supports interval and self-paced modes, a hard 25-iteration cap, non-overlap, kill controls, and a versioned structured stop decision with legacy compatibility. | Loops are intentionally process-local, not a daemon or PacketAgent substitute. |
| MCP lifecycle and policy | Present | Stdio startup/handshake/discovery/calls, aliases, approval policy, bounded redacted logs, crash isolation, doctor checks, and `/mcp restart <name>` are covered by tests. | Live config reload, Streamable HTTP, prompts, and resources remain open behind explicit trust/context design. |
| Context and compaction | Present | Full request occupancy accounting, automatic/manual compaction, complete tool-pair preservation, bounded model-facing tool results, and persisted full transcripts have focused tests. | Provider-native exact token counting is not universal. |
| Cost and usage | Present | Session usage and persistent per-provider/model tallies are independent from context occupancy and exposed through `/cost` and statusline data. | Subscription invoices remain authoritative; cache-input telemetry is incomplete. |
| Doctor and data-home contract | Present | Schema-1 `doctor --json`, additive `effective_home`, `home_source`, provider summary, absolute `PACKETCODE_HOME`, path isolation, and secret-redaction tests. | Schema compatibility policy still needs a pre-1.0 release document. |
| Cross-platform artifacts/install | Partial | GoReleaser builds Windows/macOS/Linux amd64/arm64 archives with checksums; `install.sh` and `install.ps1` verify archives before installation. | Signing, notarization, clean-machine upgrade/rollback, and published release assets are external release gates. |
| PacketADE integration | Partial | PacketADE owns executable override, documented-location detection, stable/preview install action, data-home/development-path settings, doctor probe, and local/SSH launch environment. | A release-like two-product smoke must still run from packaged PacketADE and a published PacketCode artifact. |
| PacketAgent durable handoff | External gate | Product boundary and desired handoff are documented in PacketADE's PacketAgent handoff loop. | PacketAgent owns the versioned Worker package and durable runtime; do not invent a competing contract here. |

## Commands run for this audit

```text
go test ./...
go run ./cmd/packetcode --version
PACKETCODE_HOME=<isolated absolute temp path> go run ./cmd/packetcode doctor --json
PowerShell parser check: install.ps1
PacketADE: cargo check
PacketADE: pnpm exec tsc --noEmit
PacketADE focused PacketCode/PTY/SSH/store/catalog tests
```

The full Go suite passed on Windows after the home/doctor, MCP restart, and
structured-loop hardening changes.

## High-value hardening decisions

1. **Closed now — safer repeat termination.** Structured loop decisions replace
   a bare text sentinel as the primary contract while the 25-iteration cap and
   manual stop remain authoritative.
2. **Closed now — MCP process recovery.** One crashed server can be restarted
   without restarting PacketCode or disturbing the rest of the MCP fleet.
3. **Closed 2026-08-01 — workflow verifier/retry policy.** Versioned workflow
   TOML, offline validation, fail-closed structured verdicts, bounded retries,
   and complete agent/token accounting now ship as PCH3.
4. **Closed 2026-07-31 — abandoned-job restart assistance.** Explicit
   resubmit starts a new job and links it to the preserved abandoned record;
   PacketCode never labels that as resumed execution.
5. **Next — Streamable HTTP MCP trust contract.** Define network targets,
   credentials, redirects, origins, output provenance, and approval scopes
   before enabling the transport.
6. **Release gate — signed clean-machine upgrades.** Exercise published stable
   and preview assets on Windows/macOS/Linux, including checksum failure and
   rollback. This cannot be proven from a source checkout alone.
7. **Sibling-product gate — durable handoff.** Consume PacketAgent's eventual
   versioned contract; do not author a divergent PacketCode-only Worker schema.

## Non-goals

- Recreating discontinued BridgeCode.
- Turning the PacketCode TUI into an always-on daemon.
- Automatic worktree merge or conflict resolution.
- Unsupported subscription credential reuse.
- Claiming signing, notarization, live-provider, SSH, or cross-product gates
  passed when the required external environment was not present.

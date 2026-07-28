# PacketCode BridgeCode-Plus Hardening Loop

Created: 2026-07-27

Source audit:
[`bridgecode-feature-truth-2026-07-27.md`](bridgecode-feature-truth-2026-07-27.md)

Status values: `queued` → `in-progress` → `gated` → `closed`; `external-gate`
means the implementation cannot honestly pass without a release host,
credential, SSH host, or PacketAgent contract.

| ID | Item | Acceptance condition | Status |
| --- | --- | --- | --- |
| **PCH1** | Structured loop decision | Self-paced loops accept a versioned stop/continue JSON decision, retain legacy compatibility, ignore malformed decisions, and always enforce the hard iteration cap. | closed |
| **PCH2** | Per-server MCP restart | Restart replaces one client and its tool adapters, preserves other clients, rejects unknown/disabled names, and exposes a recovery command in help/docs. | closed |
| **PCH3** | Versioned workflow verifier/retry | Workflow schema explicitly declares verifier prompt/provider/model, pass contract, and retry cap; invalid or missing verdict never passes; token/agent budgets include retries. | queued |
| **PCH4** | Abandoned-job reconcile/resubmit | Restarted PacketCode shows recovered cancelled jobs and can explicitly resubmit from bounded saved input while preserving old evidence and never claiming the old process resumed. | queued |
| **PCH5** | Streamable HTTP MCP trust contract | Network targets, credentials, redirects, origins, output provenance, and approval scopes are explicit before enabling remote MCP. | queued |
| **PCH6** | Signed clean-machine release matrix | Stable and preview assets install, update, fail closed on a bad checksum, and roll back on Windows/macOS/Linux. | external-gate |
| **PCH7** | PacketADE packaged smoke | Packaged PacketADE detects, installs, probes, configures, launches, restarts, and SSH-launches a published PacketCode build. | external-gate |
| **PCH8** | PacketAgent durable handoff | PacketCode/PacketADE consume PacketAgent's versioned Worker contract and pass close/reconnect evidence gates without duplicating its runtime. | external-gate |

PCH3 and PCH4 are the next PacketCode-owned implementation loops. PCH5 is
security-design work, not a transport toggle. PCH6–PCH8 stay visibly gated
until their named external substrate exists.

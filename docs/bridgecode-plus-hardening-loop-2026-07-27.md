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
| **PCH3** | Versioned workflow verifier/retry | Workflow schema explicitly declares verifier prompt/provider/model, pass contract, and retry cap; invalid or missing verdict never passes; token/agent budgets include retries. | **closed 2026-08-01** |
| **PCH4** | Abandoned-job reconcile/resubmit | Restarted PacketCode shows recovered cancelled jobs and can explicitly resubmit from bounded saved input while preserving old evidence and never claiming the old process resumed. | **closed 2026-07-31** |
| **PCH5** | Streamable HTTP MCP trust contract | Network targets, credentials, redirects, origins, output provenance, and approval scopes are explicit before enabling remote MCP. | **closed 2026-08-01** |
| **PCH6** | Signed clean-machine release matrix | Stable and preview assets install, update, fail closed on a bad checksum, and roll back on Windows/macOS/Linux. | external-gate |
| **PCH7** | PacketADE packaged smoke | Packaged PacketADE detects, installs, probes, configures, launches, restarts, and SSH-launches a published PacketCode build. | external-gate |
| **PCH8** | PacketAgent durable handoff | PacketCode/PacketADE consume PacketAgent's versioned Worker contract and pass close/reconnect evidence gates without duplicating its runtime. | external-gate |

PCH4 shipped 2026-07-31; PCH3 and PCH5 shipped 2026-08-01. PCH5 closes the
security-design gate without enabling a transport. Streamable HTTP is now an
independent implementation loop bound to the approved contract. PCH6–PCH8 stay
visibly gated until their named external substrate exists.

---

## PCH4 — implemented 2026-07-31

Reconciliation already rewrote abandoned Queued/Running jobs as Cancelled with
reason `previous app exit`. What was missing was any way to act on them, so
the abandoned work was visible and inert.

Shipped:

- `Job.Recovered` is set during reconciliation and persisted, so callers
  identify abandoned work from a typed field rather than by string-matching
  the free-form reason (`internal/jobs/persistence.go`).
- `Manager.Resubmit(id)` spawns a **new** job from the abandoned job's saved
  prompt/provider/model/allow-write, and links the pair in both directions via
  `ResubmitOf` and `ResubmittedAs` (`internal/jobs/resubmit.go`).
- `Manager.RecoveredResubmittable()` lists abandoned jobs still awaiting a
  decision, excluding ones already resubmitted or with no saved prompt.
- `/jobs resubmit [id]` runs it; bare `/jobs resubmit` lists candidates. The
  `/jobs` table now prints the lineage under affected rows.

Honesty rules encoded as tests, not just intent:

- The original job **keeps** its Cancelled state, reason, artifacts, worktree
  references, and token/cost totals. `TestResubmit_SpawnsNewJobAndLinksBothWays`
  asserts the abandoned record is not overwritten by its successor.
- All user-facing strings say "new run, not a resumption". Nothing claims the
  previous process continued, because it cannot.
- Resubmit is allowed **once**; a second call returns `already_resubmitted`
  naming the existing successor.
- An oversize saved prompt (`MaxResubmitPromptBytes`, 32 KiB) is **refused,
  not truncated** — a shortened prompt would start a materially different run
  than the one the user asked to re-run.
- Jobs that completed normally are rejected with `not_recovered`.

Gate: `go test ./internal/jobs/ ./internal/app/` green, `go vet ./...` clean.

## PCH3 — implemented 2026-08-01

Goal: a workflow's verifier is declared data, not an implicit convention, and
a retry can never launder a failure into a pass.

Schema (workflow TOML), all fields explicit and versioned:

- `schema_version` — integer; an unknown-but-newer version is refused rather
  than best-effort parsed.
- `[verify]` block per stage: `prompt`, `provider`, `model`, and
  `pass_contract` naming exactly how a verdict is expressed.
- `retry.max` — hard cap, defaulting to 0 (no retries). Absent means none.

Acceptance conditions:

1. A verifier that returns no verdict, a malformed verdict, or a verdict that
   does not match `pass_contract` counts as **fail**. There is no
   "assume pass on parse error" path — this is the same rule PCH1 applied to
   loop stop/continue decisions.
2. `retry.max` is enforced as a hard cap and is never extended by a nested
   stage or a verifier's own request.
3. Token and agent budgets account for **every** attempt including retries, so
   a retrying workflow cannot exceed the budget a user approved.
4. A missing `[verify]` block leaves a stage unverified and is reported as
   such — an unverified stage must never render as "passed".
5. Schema validation is available without executing anything, so a workflow
   can be checked before it spends tokens.

Tests: golden schema fixtures (valid, malformed verdict, missing verdict,
unknown newer version), retry-cap enforcement, and budget accounting across
retries.

Implementation:

- Workflow TOML now requires `schema_version = 1`; missing/future versions and
  unknown keys fail before execution.
- `/workflows validate <name>` validates schema and templates without starting
  an agent.
- A step-level `[phases.steps.verify]` declares an independent read-only
  verifier and the `packetcode-workflow-verdict-v1` contract.
- Missing, malformed, future-versioned, or unknown verdicts fail closed. Steps
  without a verifier remain explicitly `unverified` in Workflow View.
- `[phases.steps.retry] max = N` is a hard additional-attempt cap. Verifier
  feedback is appended to retry work, and work/verifier jobs all consume the
  same agent and token budgets.

Gate: focused workflow/App/UI tests green, workflow race suite green, strict
schema fixtures and malformed/missing verdict regressions included. Canonical
schema: [workflows.md](workflows.md).

## PCH5 — implemented 2026-08-01

Goal: decide the trust model **before** Streamable HTTP MCP is enabled. This
is a security-design item; shipping the transport without it is the failure
mode, not slow progress.

The contract must state, explicitly and per server:

1. **Target allowlist.** Exact scheme/host/port. Loopback, private,
   link-local, and reserved ranges are separately classified — a local
   Ollama-style endpoint is a different trust decision from a hosted one.
2. **Redirects.** Whether they are followed at all, and if so that the
   destination is re-checked against the allowlist. A redirect must never be
   able to move a session to an unlisted origin.
3. **Credentials.** Where a token comes from, which requests it is attached
   to, and the guarantee it is never sent cross-origin after a redirect.
4. **Output provenance.** Remote MCP output is untrusted content. It must be
   labelled as such in transcripts and must never occupy an instruction
   position — the same boundary Packet Control applies to captured evidence.
5. **Approval scope.** Whether approving a remote MCP tool once applies to the
   session, the server, or the single call, and how a remembered approval is
   revoked.
6. **Failure semantics.** Timeout, crash, and reconnect behaviour surfaced
   consistently in transcripts and Agent View, with no silent retry against a
   different origin.

Acceptance: the contract is written and reviewed, redaction tests cover remote
MCP output, and only then does an implementation loop open. No transport flag
lands before this document does.

Shipped design gate:

- [Streamable HTTP MCP trust contract](mcp-http-trust-contract.md), versioned
  `packetcode-mcp-http-trust-v1`, records the exact target/address,
  redirect, credential, provenance, approval/revocation, timeout, and reconnect
  decisions. It explicitly states that the executable remains stdio-only.
- `internal/mcp/http_trust.go` is a transport-independent, fail-closed
  validator. It canonicalizes exact origins with mandatory ports, classifies
  loopback/private/link-local/reserved/public addresses separately, rejects
  mixed DNS answers, permits only denied or bounded bodyless same-origin
  GET/HEAD redirects, disables ambient proxies, pins system-root TLS, and
  refuses public plain HTTP.
- Credentials are environment-sourced bearer values attached only to the
  original target origin after atomic runtime/output binding; cross-origin
  hops, POST redirects, and request-body replay are refused.
- Remote output has a labelled untrusted envelope plus shared credential
  redaction used by visible MCP logs. The credential-bound sanitizer also
  removes the exact bearer value, partial percent/JSON escapes, and common
  base64 variants, preserves provenance,
  and enforces the configured model-facing output cap. Tests cover prompt-
  injection-shaped text, bearer/JSON/URL secrets, redirect escape, address
  classes, and credential non-forwarding.
- Remote approval scope is one call. An explicit exact-tool session grant can
  still be chosen in the approval UI, and `/permissions reset` now provides a
  tested revocation path back to startup policy. Reconnect is manual with zero
  automatic attempts. Response, event, header, and output sizes are bounded,
  with identity compression required.

Gate: `go test ./internal/mcp ./internal/app` green. No config URL, transport
flag, HTTP client, or network request was added. The follow-up transport loop
must consume this validator and pass the adversarial integration matrix named
in the canonical contract.

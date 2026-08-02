# Packet Computers — Phases 1–2 Loop (PCMP1–PCMP9)

Created: 2026-07-31

Product source: [`../PACKETCOMPUTERS.md`](../PACKETCOMPUTERS.md) — research,
product definition, and the full six-phase arc. This file is the bounded
implementation ledger for **Phases 1–2 only** (BYO local/SSH computers, then
persistent jobs). Phases 3–6 stay in the source document until these close.

Owner: packetcode. The Packet Control half of `PACKETCOMPUTERS.md` was split
out on 2026-07-31 and is implemented in PacketADE
(`D:\projects\PacketADE\dev\packet-control-loop.md`, CTL1–CTL9).

Status values: `queued` → `in-progress` → `gated` → `closed`; `external-gate`
means the item cannot honestly pass without substrate that does not exist yet.

## Scope discipline

Packet Computers is a six-phase program whose last phase is cloud
provisioning, billing, quotas, and fleet updates — an operations product, not
a feature. The purpose of this ledger is to keep the early, genuinely useful
part (a machine registry and a local daemon) from being blocked behind it, and
to make each step's blast radius explicit before it lands.

Two hard rules for the whole loop:

- **No public PacketCode listener, ever, in Phases 1–2.** Daemon transport is
  loopback or SSH-forwarded loopback only. The foreground bridge may use the
  machine's existing SSH service; it never exposes a PacketCode port.
- **A remote computer does not inherit local secrets.** The default policy
  denies secrets and requires explicit approval; widening that is a per-record
  user decision, never a default.

## Loop ledger

| ID | Item | Acceptance condition | Gate | Depends on | Status |
|---|---|---|---|---|---|
| **PCMP1** | Computer registry data model | Versioned `Computer` record with kind, status, capabilities, policy, project roots, and SSH target. Conservative policy defaults applied on read and write. Malformed or future-versioned registries fail loudly rather than resetting. | Unit + round-trip + malformed/future-version tests | — | **closed 2026-07-31** |
| **PCMP2** | Read-only `/computers` surface | `/computers` lists records and `/computers status <name>` shows detail. Output states that status is stored, not probed, while no heartbeat exists. | Renderer tests; help/autocomplete coverage | PCMP1 | **closed 2026-07-31** |
| **PCMP3** | Registry write commands | `/computers register`, `/computers ssh`, and `/computers remove` with validation, duplicate-name refusal, and confirmation before removal. | Command tests incl. invalid input and duplicate names | PCMP1 | **closed 2026-08-01** |
| **PCMP4** | Loopback daemon | `packetcode daemon --listen 127.0.0.1:<port>` exposing `ping`, `capabilities`, `project.list`. Refuses any non-loopback bind address. Writes no credentials to disk. | Bind-refusal test; lifecycle tests | PCMP1 | queued |
| **PCMP5** | Heartbeat and real status | Registry status is driven by daemon heartbeat with an explicit staleness window. A record with no recent heartbeat reports `unknown`, never a stale `online`. | Staleness/clock-skew tests | PCMP4 | queued |
| **PCMP6** | SSH-forwarded transport | Reach a remote daemon over SSH-forwarded loopback with host-key verification enforced. No public port is ever opened on either end. | SSH integration tests; unpinned-host refusal | PCMP4 | queued |
| **PCMP7** | Runtime backend abstraction | `RuntimeBackend` (resolve/read/write/execute) with `LocalBackend` preserving today's behaviour exactly, plus `ComputerBackend` forwarding to a daemon. Tools gain no remote conditionals. | Backend parity suite run against both implementations | PCMP6 | **in-progress 2026-08-01** — core tools use local/direct-SSH backends; daemon parity remains |
| **PCMP8** | `ComputerID` on jobs | Jobs carry immutable computer/root identity; `/spawn --computer <name>` and whole-workflow routing use independent direct-SSH backends; remote write jobs require isolated worktrees; Agent View labels the computer. Global and computer permissions compose restrictively. | Job routing + permission-preservation + remote worktree tests | PCMP7 | **closed 2026-08-02 (direct-SSH process-lifetime slice)** |
| **PCMP9** | Persistent job reconcile | The daemon retains job state; a restarted packetcode reconnects and reconciles rather than abandoning. Must extend PCH4's honesty rules: anything not genuinely resumed is still reported as abandoned. | Reconnect, cancel-across-restart, and mislabelling regression tests | PCMP8; PCH4 | queued |

## Sequencing

```text
PCMP1 -> PCMP2 -> PCMP3
   \-> PCMP4 -> PCMP5
        \-> PCMP6 -> PCMP7 -> PCMP8 -> PCMP9
```

PCMP3 is independent of the daemon and shipped with the direct foreground SSH
slice. PCMP9 is the first
point at which the "jobs survive restart" promise in `PACKETCOMPUTERS.md`
Phase 2 becomes true, and it must not be claimed before then.

## PCMP1 / PCMP2 — implemented 2026-07-31

Milestone A from `PACKETCOMPUTERS.md` began data-only. The direct foreground
SSH slice described below now adds the first runtime backend.

- `internal/computers/computer.go` — `Computer`, `Kind`
  (`local`/`ssh`/`managed`), `Status`, `Capabilities`, and `Policy` with
  conservative defaults (`network`/`write`/`shell` = ask, `secrets` = deny,
  `approval` = explicit).
- `internal/computers/registry.go` — versioned `registry.json` with atomic
  temp-then-rename writes, case-insensitive unique names, `CreatedAt`
  preservation across updates, and loud failure on malformed or
  future-versioned files.
- `internal/config/paths.go` — `ComputersDir()` → `~/.packetcode/computers/`,
  honouring `PACKETCODE_HOME` like every other state dir.
- `internal/app/slashcmd_computers.go` began with read-only `/computers` and
  `/computers status <name>`; PCMP3 later added register/SSH/remove writes.

Two design decisions worth keeping:

**Status never claims freshness it cannot back.** There is no heartbeat until
PCMP5, so `normalize` forces `StatusUnknown` on any record with no
`DaemonVersion`, and both the table and detail views say status is stored
rather than probed. A registry that displayed a user-typed `online` would be
lying by construction.

**The approval axis is a string enum, not a bool.** `ApprovalMode`
(`explicit` / `trust-workspace` / `trust-computer`) matches the source doc's
policy vocabulary, but the deciding reason is encoding safety: the safe
default is "approve everything explicitly", and a bool whose safe value is
`true` decodes an absent JSON field to `false` — silently widening trust on
exactly the records that were never configured. The enum makes the absent case
fall back to `explicit`, and there are tests for the absent and
unknown-value paths.

Gate: `go test ./internal/computers/ ./internal/config/ ./internal/app/`
green, `go vet ./...` clean, `gofmt` clean.

## Foreground direct SSH slice — implemented 2026-08-01

This is a useful bridge ahead of the daemon path, not a claim that PCMP6–PCMP7
are complete. `packetcode --computer <name>` holds one host-key-pinned SSH
connection and SFTP client for a foreground session. Core file, patch, search,
directory, and shell tools run through `RuntimeBackend`; remote paths and cwd
values remain confined to the registered root. Remote transcripts bind their
computer ID/root and refuse cross-machine resume.

There is still no daemon, heartbeat, `ComputerID` on jobs,
durable process, remote `/undo`, or reconnect after the PacketCode process
exits. PCMP8's direct-SSH job routing was added on 2026-08-02. PCMP6 remains the planned SSH-forwarded daemon
transport; PCMP7 remains in progress until daemon parity and the full backend
parity gate land.

## Not yet true

Stated plainly so no reader infers more than shipped:

- packetcode can route foreground tools, background jobs, and whole workflows
  to a registered SSH computer for the lifetime of the local process.
- No daemon exists, so no status is ever live.
- Direct SSH is process-lifetime only; it is not persistent job execution.

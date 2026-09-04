# Packet Computers — Phase 1 Loop (PCMP1–PCMP10, PCMP9 cut)

Created: 2026-07-31

> **Ledger supersession note (2026-08-31).** Preserve milestone rows and their
> original evidence, but do not read the PCMP10 process-evidence note below as
> current. Local teardown now reports mechanism, confirmation, and surviving
> PIDs, with POSIX process groups and Windows Job Objects covered. The remaining
> gap is SSH: signalling the channel leader cannot confirm that detached remote
> descendants stopped. See [BACKLOG.md](../BACKLOG.md).

Product source: [`../PACKETCOMPUTERS.md`](../PACKETCOMPUTERS.md) — research,
product definition, and the full six-phase arc. This file is the bounded
implementation ledger for **Phase 1 only** (BYO local/SSH computers reached by a
session-scoped daemon). Phase 2's "persistent jobs" goal was cut on 2026-08-14
and is not work this repo will do — see the ruling below and the PCMP9 row.
Phases 3–6 stay in the source document until this one closes.

Owner: packetcode. The Packet Control half of `PACKETCOMPUTERS.md` was split
out on 2026-07-31 and is implemented in PacketADE
(`D:\projects\PacketADE\dev\packet-control-loop.md`, CTL1–CTL9).

Status values: `queued` → `in-progress` → `gated` → `closed`; `external-gate`
means the item cannot honestly pass without substrate that does not exist yet;
`cut` means the item was ruled out of scope for this repo and will not be built
here. A cut row is kept, never deleted — the ledger records what was planned and
what happened to it, and removing the row would only lose the decision.

## The 2026-08-14 ruling

**packetcode does not resume jobs across a restart.** Durable execution after
the originating app closes belongs to PacketAgent. The governing line is in
BACKLOG's PacketADE section — "Preserve PacketCode as an independently
installable product; durable execution after its originating app closes belongs
to PacketAgent" — which was in tension with PCMP4/PCMP5/PCMP9. The tension is
resolved in favour of that line.

What follows from it:

- **PCMP9 is cut, not deferred.** "Jobs survive restart" is not a promise this
  repo makes at all, in any timeframe.
- **The daemon survives but is session-scoped.** It exists to reach Packet
  Computers and dies with the app. It holds no durable job state.
- **PCH4's rule is now the whole story.** Anything not genuinely resumed is
  reported as abandoned, and `/jobs resubmit` starts a new job that never claims
  the old process resumed.
- **The abandoned terminal state is promoted**, from a PCMP9 precondition to the
  primary honest terminal state, and is no longer blocked on the daemon. Job
  records became versioned on 2026-08-14, which was the blocker for adding a new
  state: a state written by a newer build is now reported rather than silently
  swallowed. Tracked below as PCMP10, and shipped the same day. It was scoped
  down from `abandoned`/`indeterminate` to a single `abandoned` state with an
  `AbandonCause`, because nothing in the runtime can yet evidence
  "started, outcome unknown" as distinct — see the PCMP10 note.

**PCMP4 transport edit.** The acceptance condition moves off
`packetcode daemon --listen 127.0.0.1:<port>` to an AF_UNIX socket on POSIX and
a named pipe (or stdlib AF_UNIX) on Windows. PCMP4 carries two acceptance
conditions — refuse non-loopback binds *and* write no credentials to disk — and
loopback TCP is reachable by every local UID, so satisfying the first that way
forces an auth token, which breaks the second. A socket at `0600` inside a
`0700` directory gets both from filesystem permissions, and makes the loopback
rule structural rather than a validation check that a config path or
`--network host` can regress. There is no network listener anywhere in
production code today, so nothing is being migrated. Two caveats to carry into
PCMP6: SSH forwarding needs `AllowStreamLocalForwarding` on the remote sshd and
must fail with a clear diagnostic rather than a hang; and try stdlib AF_UNIX on
Windows before reaching for `go-winio`.

## Scope discipline

Packet Computers is a six-phase program whose last phase is cloud
provisioning, billing, quotas, and fleet updates — an operations product, not
a feature. The purpose of this ledger is to keep the early, genuinely useful
part (a machine registry and a session-scoped daemon) from being blocked behind
it, and to make each step's blast radius explicit before it lands.

Two hard rules for the whole loop:

- **No PacketCode network listener, ever.** Daemon transport is a filesystem
  socket — AF_UNIX on POSIX, named pipe or stdlib AF_UNIX on Windows — reached
  locally or through SSH stream-local forwarding. No port is bound on either
  end. The foreground bridge may use the machine's existing SSH service; it
  never exposes a PacketCode port.
- **A remote computer does not inherit local secrets.** The default policy
  denies secrets and requires explicit approval; widening that is a per-record
  user decision, never a default.

## Loop ledger

| ID | Item | Acceptance condition | Gate | Depends on | Status |
|---|---|---|---|---|---|
| **PCMP1** | Computer registry data model | Versioned `Computer` record with kind, status, capabilities, policy, project roots, and SSH target. Conservative policy defaults applied on read and write. Malformed or future-versioned registries fail loudly rather than resetting. | Unit + round-trip + malformed/future-version tests | — | **closed 2026-07-31** |
| **PCMP2** | Read-only `/computers` surface | `/computers` lists records and `/computers status <name>` shows detail. Output states that status is stored, not probed, while no heartbeat exists. | Renderer tests; help/autocomplete coverage | PCMP1 | **closed 2026-07-31** |
| **PCMP3** | Registry write commands | `/computers register`, `/computers ssh`, and `/computers remove` with validation, duplicate-name refusal, and confirmation before removal. | Command tests incl. invalid input and duplicate names | PCMP1 | **closed 2026-08-01** |
| **PCMP4** | Session-scoped socket daemon | `packetcode daemon` exposing `ping`, `capabilities`, `project.list` over an AF_UNIX socket at `0600` inside a `0700` directory on POSIX, and a named pipe (or stdlib AF_UNIX) with equivalent owner-only ACLs on Windows. Binds no network port at all. Writes no credentials to disk — filesystem permissions carry authorization, so there is no auth token. Session-scoped: it dies with the app and holds no durable job state. | No-network-listener test; socket permission/ownership test; lifecycle and stale-socket tests | PCMP1 | queued |
| **PCMP5** | Heartbeat and real status | Registry status is driven by daemon heartbeat with an explicit staleness window. A record with no recent heartbeat reports `unknown`, never a stale `online`. | Staleness/clock-skew tests | PCMP4 | queued |
| **PCMP6** | SSH-forwarded transport | Reach a remote daemon over an SSH-forwarded socket with host-key verification enforced. No port is ever opened on either end. A remote sshd without `AllowStreamLocalForwarding` fails with a clear diagnostic rather than hanging. | SSH integration tests; unpinned-host refusal; forwarding-disabled diagnostic test | PCMP4 | queued |
| **PCMP7** | Runtime backend abstraction | `RuntimeBackend` (resolve/read/write/execute) with `LocalBackend` preserving today's behaviour exactly, plus `ComputerBackend` forwarding to a daemon. Tools gain no remote conditionals. | Backend parity suite run against both implementations | PCMP6 | **in-progress 2026-08-01** — core tools use local/direct-SSH backends; daemon parity remains |
| **PCMP8** | `ComputerID` on jobs | Jobs carry immutable computer/root identity; `/spawn --computer <name>` and whole-workflow routing use independent direct-SSH backends; remote write jobs require isolated worktrees; Agent View labels the computer. Global and computer permissions compose restrictively. | Job routing + permission-preservation + remote worktree tests | PCMP7 | **closed 2026-08-02 (direct-SSH process-lifetime slice)** |
| **PCMP9** | Persistent job reconcile | *Planned, never built:* the daemon retains job state; a restarted packetcode reconnects and reconciles rather than abandoning. | *Was:* reconnect, cancel-across-restart, and mislabelling regression tests | PCMP8; PCH4 | **cut 2026-08-14** — durable execution after the originating app closes belongs to PacketAgent, so packetcode does not resume jobs across a restart at all. PCH4's rule stands alone: anything not genuinely resumed is reported as abandoned. Cut, not deferred. |
| **PCMP10** | `abandoned` terminal state | An explicit terminal state so a loss after work began is never flattened into a confirmed cancellation, plus an `AbandonCause` (`app-exit` / `transport-lost` / `unknown`). Cancel/CancelAll/Shutdown record a durable `CancelRequest` *before* cancelling the context, which is what makes a deliberate stop distinguishable from a loss at all. A running job left by an unclean exit reconciles as `abandoned`; a queued one still reconciles as `cancelled`, because it provably never started. `State.IsSuccess()` replaces the failure allowlists that reported any new state as a pass. | State + cause round-trip through versioned records; cancel-vs-lost discrimination table; older-build read-back; workflow/spawn/collect treat abandoned as an error | PCH4; versioned job records (2026-08-14) | **closed 2026-08-14** |

PCMP10 was independent of the daemon and shipped without it. It had been
blocked on versioned job records — a new state written by a newer build had to
be reported rather than silently swallowed — and that shipped on 2026-08-14.
The PCMP9 ruling promoted it from precondition to the primary honest terminal
state.

Two things about PCMP10 are worth keeping, because both were discovered while
building it rather than while planning it:

- **The defect was wider than "a state is missing".** Five sites treated any
  unrecognised terminal state as a *success*: two workflow gates, `spawn_agent`,
  `collect_agent_results`, and Agent View's `groupForState` default. Adding the
  state without fixing them would have created a new dishonesty — an abandoned
  sub-agent reported to the parent model as a win — while removing the old one.
  They now test for success rather than enumerate failures, so the next state
  added cannot repeat it.
- **`abandoned`, not `abandoned`/`indeterminate`.** The row originally named two
  states. One shipped, because nothing in the runtime can currently evidence
  "started, outcome unknown" as distinct from "abandoned": there is no
  acknowledged-start record, no remote pid, and no last-successful-operation
  timestamp. A second state would have been unreachable vocabulary. The
  `AbandonCause` field carries the distinction the runtime *can* back, and can
  grow a value when the evidence exists.

**Not shipped with PCMP10:** process-group-aware cancellation evidence. `procrun`
can kill a tree but reports nothing about whether it succeeded — `KillTree`
returns a bare `error`, `computers.ExecResult` carries only an exit code, and
`jobs.Manager.Cancel` is a fire-and-forget `context.CancelFunc` with no return
path. On SSH there is no mechanism to evidence at all. Until that exists,
`transport-lost` is claimed only where a transport error was actually observed,
and everything else honestly reads `unknown`.

## Sequencing

```text
PCMP1 -> PCMP2 -> PCMP3
   \-> PCMP4 -> PCMP5
        \-> PCMP6 -> PCMP7 -> PCMP8

PCMP10 (independent of the daemon; closed 2026-08-14)
```

PCMP3 is independent of the daemon and shipped with the direct foreground SSH
slice. Nothing in this ledger makes jobs survive a restart, and nothing is meant
to: that promise is out of scope for this repo (see the ruling above), so it
must not be claimed anywhere in packetcode's documentation or output.

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

There is still no daemon, heartbeat, `ComputerID` on jobs, durable process, or
remote `/undo`. PCMP8's direct-SSH job routing was added on 2026-08-02. PCMP6
remains the planned SSH-forwarded daemon transport; PCMP7 remains in progress
until daemon parity and the full backend parity gate land.

Reconnect after the PacketCode process exits is *not* on that pending list. It
is out of scope (ruled 2026-08-14), and no later ledger item restores it.

## Not yet true

Stated plainly so no reader infers more than shipped:

- packetcode can route foreground tools, background jobs, and whole workflows
  to a registered SSH computer for the lifetime of the local process.
- No daemon exists, so no status is ever live.
- Direct SSH is process-lifetime only; it is not persistent job execution.
- An interrupted job reports `abandoned` with a cause (PCMP10), but packetcode
  still cannot confirm that a detached remote descendant stopped. The state is
  honest about the uncertainty; it does not remove it.

## Out of scope, not pending

Kept separate from the list above, because "not yet" and "never here" are
different promises and readers act on the difference:

- **Jobs do not survive a packetcode restart**, and no ledger item will change
  that. Durable execution after the originating app closes belongs to
  PacketAgent (ruled 2026-08-14).
- **No reconnect-and-continue after process exit.** An interrupted job is
  reported as abandoned and can be explicitly re-run as a new job via
  `/jobs resubmit`, which never claims the old process resumed.
- **The daemon will not hold durable job state.** It is session-scoped by
  design and dies with the app.

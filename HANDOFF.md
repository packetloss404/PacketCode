# Packetcode Maintainer Handoff

Updated: 2026-07-31

Handoff baseline: `main` at `2195400` (`Document maintainer handoff`), plus the
2026-07-31 pass below.

This file is the quickest way to resume Packetcode work without reconstructing
the repository's recent history. Read it together with [README.md](README.md),
[CHANGELOG.md](CHANGELOG.md), and [BACKLOG.md](BACKLOG.md). The current commit,
branch, and working-tree state remain authoritative:

```powershell
git status -sb
git log -5 --oneline --decorate
```

## Current State

Packetcode is a pre-1.0, keyboard-first Go coding agent with a Claude
Code-inspired Bubble Tea TUI. It supports multiple hosted and local providers,
OpenAI Codex subscription authentication, permission-gated native tools,
foreground sessions, background agents, isolated write worktrees, workflows,
loops, MCP servers, hooks, themes, and native/custom statuslines.

The current `main` branch includes the recent interaction-polish and
hardening pass:

- Caret-accurate `@file` completion, bounded multiline input, portable newline
  bindings, visual-row history navigation, draft-first Ctrl+C, and native
  scrollback clearing.
- One-row responsive statuslines, asynchronous Git branch refresh, event-driven
  approvals, stable Agent View focus, live background transcripts, and visible
  agent reasoning activity.
- Terminal-output sanitization that prevents tool/provider text from enabling
  mouse modes, changing the clipboard, moving the cursor, or corrupting the
  renderer with split control sequences.
- Codex `/effort` support using catalog-advertised reasoning levels, persistent
  configuration, and a compact status indicator.
- Background-agent fan-out guidance, structured self-paced loop stops,
  workflow hardening, per-server MCP restart, and PacketADE/BridgeCode evidence
  documentation.
- A reviewed user manual, advanced guide, printable cheat sheet, and
  self-contained offline HTML5 manual.

The 2026-07-31 pass added:

- **PCH4 — abandoned-job resubmit.** `/jobs resubmit [id]` re-runs work a
  previous app exit abandoned. It spawns a *new* job and never claims the old
  process resumed; the abandoned record keeps its state and evidence, and the
  pair links via `ResubmitOf`/`ResubmittedAs`. See
  [docs/bridgecode-plus-hardening-loop-2026-07-27.md](docs/bridgecode-plus-hardening-loop-2026-07-27.md).
- **PCMP1/PCMP2 — Packet Computers registry (Milestone A).** `internal/computers`
  plus read-only `/computers`. Registry-only: no daemon, no transport, and a
  stored status is never shown as a live probe. See
  [docs/packet-computers-loop.md](docs/packet-computers-loop.md) and
  [docs/feature-packet-computers.md](docs/feature-packet-computers.md).
- **PCH3 and PCH5 specified, not implemented.** Acceptance conditions are
  written into the hardening loop; PCH3 is the next implementation item.
- **Packet Control split to PacketADE.** Phases 1–2 are implemented there
  (`D:\projects\PacketADE\dev\packet-control-loop.md`). No packetcode work is
  scheduled; if it ever lands here it must consume that manifest schema.

## Start Here

For normal operation:

```powershell
go run ./cmd/packetcode --provider codex --model gpt-5.6-sol
```

For an optimized Windows executable:

```powershell
$packetVersion = git describe --tags --always --dirty
$packetCommit = git rev-parse --short HEAD
$env:CGO_ENABLED = "0"
go build -trimpath `
  -ldflags "-s -w -X main.version=$packetVersion -X main.commit=$packetCommit" `
  -o bin\packetcode.exe ./cmd/packetcode
.\bin\packetcode.exe --version
.\bin\packetcode.exe doctor --check version
```

`bin/` and `*.exe` are intentionally ignored. Rebuild local binaries rather
than committing them.

The common Codex launch is:

```powershell
codex login
.\bin\packetcode.exe --provider codex --model gpt-5.6-sol
```

Inside Packetcode:

```text
/provider codex
/model gpt-5.6-sol
/effort high
/agents
/workflows
/help
```

## Documentation Map

- [README.md](README.md): project overview, installation, daily commands, and
  the canonical documentation index.
- [docs/manual.md](docs/manual.md): progressive everyday user manual.
- [docs/advanced-guide.md](docs/advanced-guide.md): architecture, permissions,
  agent orchestration, context, MCP, statusline, security, and advanced
  operating patterns.
- [docs/cheat-sheet.md](docs/cheat-sheet.md): compact terminal reference.
- [docs/packetcode-manual.html](docs/packetcode-manual.html): offline,
  responsive HTML5 synthesis with printable quick-reference mode.
- [docs/configuration.md](docs/configuration.md): configuration fields and
  examples.
- [docs/providers.md](docs/providers.md): provider authentication, model
  discovery, Codex, custom endpoints, and Ollama.
- [docs/security.md](docs/security.md): permission profiles and trust
  boundaries.
- [docs/feature-background-agents.md](docs/feature-background-agents.md) and
  [docs/feature-agent-view.md](docs/feature-agent-view.md): job orchestration
  and result lifecycle.
- [docs/feature-packet-computers.md](docs/feature-packet-computers.md): the
  computer registry as it actually ships, and an explicit list of what does
  not work yet.
- [docs/mcp.md](docs/mcp.md): MCP configuration and runtime management.
- [docs/troubleshooting.md](docs/troubleshooting.md): common setup and runtime
  failures.
- [BACKLOG.md](BACKLOG.md): unshipped work only.

When behavior changes, update the focused reference, the relevant manual
section, README if the user-facing surface changed, and `CHANGELOG.md`.
Regenerate or manually reconcile the offline HTML manual when its source
material changes.

## Architecture Map

The primary runtime wiring is in `cmd/packetcode/main.go`. Important packages:

| Area | Location | Responsibility |
| --- | --- | --- |
| TUI orchestration | `internal/app` | Bubble Tea state, input routing, slash commands, provider/model switching, sessions, approvals, jobs, and workflows. |
| Foreground agent | `internal/agent` | Provider/tool loop, approvals, cancellation, context estimation, and compaction. |
| Providers | `internal/provider` | Provider contract, registry, model metadata, streaming events, Codex auth/Responses behavior, and hosted/local adapters. |
| Native tools | `internal/tools` | Root-scoped filesystem/search/shell tools and provider definitions. |
| Permissions | `internal/permissions` | Profiles, matching rules, allow/ask/deny decisions, and remembered approvals. |
| Background jobs | `internal/jobs` | Job lifecycle, isolated sessions, write worktrees, transcripts, artifacts, persistence, and nested agents. |
| Workflows | `internal/workflow` | Ordered phases, parallel fan-out, cancellation, bindings, and token boundaries. |
| MCP | `internal/mcp` | Stdio server startup, discovery, namespaced adapters, calls, logs, and restart. |
| Persistence | `internal/session`, `internal/config`, `internal/cost` | Sessions, backups, user paths/configuration, and usage/cost tallies. |
| TUI components | `internal/ui/components` | Conversation, input, topbar, approvals, pickers, Agent View, workflow view, and transcripts. |
| Display safety | `internal/ui/terminaltext` | Stateful sanitization of untrusted terminal text. |

The foreground App owns visible state. Provider streams, background jobs, and
workflow updates enter it through Bubble Tea messages. Finalized messages are
committed to terminal-native scrollback; only active content remains in the
live region.

## State and Security Boundaries

User state is under `~/.packetcode/`:

| Path | Contents |
| --- | --- |
| `config.toml` | Providers, behavior, permissions, MCP, hooks, and statusline. |
| `sessions/` | Foreground and background transcripts. |
| `jobs/` | Persisted job snapshots and artifact metadata. |
| `worktrees/` | Isolated checkouts for write-capable jobs. |
| `commands/` | User prompt commands. |
| `workflows/` | User workflow definitions. |
| `theme.toml` | Optional semantic colors. |
| `cost-tally.json` | Persisted usage and cost data. |

Codex subscription credentials remain in the official Codex store:
`$CODEX_HOME/auth.json` or `~/.codex/auth.json`.

Permission policy is a decision layer, not an operating-system sandbox.
Allowed commands, hooks, statusline commands, and MCP servers run as the user.
Direct `/spawn` is already an explicit user action; model-initiated
`spawn_agent` remains policy-gated. Write jobs operate in separate Git
worktrees and are never merged or removed automatically.

Do not commit secrets, local PTY captures, generated binaries, `.env` files, or
machine-specific Codex/Packetcode state.

## Interaction Caveats

- `/model` is the reliable model-picker path. Ordinary terminals encode
  Ctrl+M as carriage return; the shortcut works only when reported distinctly.
- `Alt+Enter` and backslash-Enter are the most portable multiline bindings.
  Ctrl+J inserts a newline while completion is closed and moves the completion
  selection while its popup is open.
- Esc dismisses ordinary overlays, but in an approval prompt it means **No**
  and rejects the action.
- Finalized chat output is in terminal-native scrollback. Use the terminal or
  tmux scroll controls; `/transcript` opens saved session content.
- Completion notifications mark background results seen. Injecting or
  collecting is still explicit; results are never silently copied into
  foreground model context.
- Output sanitization protects terminal state, not the safety of the underlying
  command.

## Verification Baseline

Run these before merging behavior changes:

```powershell
go mod verify
go test ./...
go vet ./...
go test -race -count=1 ./...
```

For TUI changes:

```powershell
go build -o bin\packetcode.exe ./cmd/packetcode
.\bin\packetcode.exe --tui-fixture=normal
.\bin\packetcode.exe --tui-fixture=agents
```

The PTY harness is documented in
[docs/tui-parity-harness.md](docs/tui-parity-harness.md). Captures under
`testdata/tui/captures/` are ignored because they may contain local account or
machine data.

For documentation changes, check:

- Markdown relative targets and code-fence balance.
- HTML5 doctype, unique IDs, internal anchors, and absence of remote assets in
  `docs/packetcode-manual.html`.
- Command syntax against `internal/app/keymap.go`, the slash registry and
  handlers, and `cmd/packetcode` flags.

## Recommended Next Work

The authoritative queue is [BACKLOG.md](BACKLOG.md). The highest-leverage next
steps are:

1. Evaluate or execute the Bubble Tea v2 migration for enhanced keyboard
   reporting and synchronized output.
2. Promote reviewed PTY fixtures into CI golden tests, including widths below
   80 columns and tall approval/tool blocks.
3. Add end-to-end smoke coverage for first-run setup, provider switching,
   session resume, approvals, agents, workflows, and MCP.
4. Improve cancellation-drain visibility and add transcript search/jump-to-
   latest.
5. Add workflow pipeline stages, adversarial verification/retry policies, and
   a versioned workflow schema.
6. Add safe worktree apply/merge assistance and explicit cleanup commands.
7. Continue context/cost work: provider-native counting where stable, bounded
   model-facing output caps, cached-input telemetry, and opaque Codex reasoning
   continuity when required by the backend.
8. Continue the PacketADE/BridgeCode ledger from
   [docs/bridgecode-feature-truth-2026-07-27.md](docs/bridgecode-feature-truth-2026-07-27.md)
   and
   [docs/bridgecode-plus-hardening-loop-2026-07-27.md](docs/bridgecode-plus-hardening-loop-2026-07-27.md).

Avoid mixing these into one large change. Prefer bounded branches with focused
tests and a short changelog entry.

## Resume Checklist

```text
1. Read HANDOFF.md, README.md, CHANGELOG.md, and BACKLOG.md.
2. Run git status -sb and inspect the latest commits.
3. Run packetcode doctor and the relevant focused tests.
4. Choose one bounded backlog item.
5. For independent research/review, fan out read-only agents.
6. Serialize overlapping writes; use isolated write jobs where appropriate.
7. Update focused docs, README/CHANGELOG when user-facing, and this handoff if
   the architecture or recommended next work changes.
8. Run the verification baseline before publishing.
```

At the time this handoff was created, the working tree started clean from
`origin/main`; the handoff/documentation update itself is intended to be one
isolated commit.

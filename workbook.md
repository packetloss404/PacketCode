# packetcode QA workbook

Manual verification pass for the 2026-09-05 security audit.

This is the readable twin of the printable workbook. `build_qa_workbook.py`
renders the same content as a 23-page US Letter PDF with tick boxes, pass/fail
columns, and screenshot slots:

```bash
python build_qa_workbook.py            # writes packetcode-qa-workbook.pdf
```

reportlab is the only dependency. The PDF is a build artifact and is not
committed; regenerate it whenever the workbook changes.

Why this exists. `go test ./...` covers the packages and `smoke.sh` covers the
seams. Neither can cover the part a person has to look at, which for a terminal
UI is most of the product: whether an approval prompt reads clearly, whether the
mode footer says what is actually in force, whether a denial is legible, whether
the screenshots in the docs still match the program. This is that pass.

Sources: [docs/audit/security-audit-2026-09-05.md](docs/audit/security-audit-2026-09-05.md)
for the findings behind every patch row, [docs/runbooks.md](docs/runbooks.md)
for the procedures, [docs/handoff.md](docs/handoff.md) for what is still open.

---

## How to use it

Record what you observed, not the word "ok". Exit codes, counts, and paths are
the evidence; a tick is not. On any failure, give it the next `BUG-nn` in the
bug log and note the test id beside it.

**Set up an isolated environment first.** Nothing here should run against your
working data home.

```bash
export PACKETCODE_HOME=/tmp/pc-qa            # must be absolute
export PACKETCODE_LOG_FILE=/tmp/pc-qa/qa.jsonl
mkdir -p /tmp/pc-qa /tmp/pc-qa-project
cd /tmp/pc-qa-project && git init -q
```

On Windows use `$env:PACKETCODE_HOME = "C:\Temp\pc-qa"`. A relative path is
refused by design.

**Run the automated pass first.**

```bash
go build ./... && go vet ./...
go test -count=1 ./...
bash smoke.sh
make vulncheck
```

If `internal/jobs` hangs or fails, rerun it alone before believing it (runbook
R16). Its tests wait on real timers and are starved by concurrent compilation.

Budget: about five minutes automated, about ninety minutes manual if nothing
fails, most of it in TS-C and TS-E.

---

## Surface inventory

Confirm each surface still exists and behaves as described. Anything that has
moved since the audit is itself a finding.

**Command line.** `packetcode` (TUI, walks setup with no provider),
`packetcode run` (one headless turn, approvals fail closed with exit 3),
`packetcode doctor` (runs no hooks, MCP servers, or providers),
`packetcode skills` (`install` runs `git clone`), `packetcode acp` (JSON-RPC over
stdio), `packetcode sugar login` (writes a token to config.toml).

**ACP methods.** `initialize` (required first; advertises `permissionModes`),
`session/new` (client picks cwd and may supply MCP commands), `session/load`,
`session/prompt` (expands markdown slash commands), `session/cancel`,
`session/close`, and the `_packetcode/*` extensions for sessions, models, mcp,
commands, and project files.

**Model tools and their gate under the `ask` profile.**

| Tool | Gate |
| --- | --- |
| `read_file`, `search_codebase`, `list_directory` | read-only, no approval; refuse dotenv files |
| `list_symbols`, `find_definition`, `find_references`, `get_diagnostics` | read-only, no approval |
| `write_file`, `patch_file` | approval; backed up for undo |
| `execute_command` | approval; full shell as your user |
| `fetch` | approval; blocks private and loopback addresses |
| `spawn_agent`, `collect_agent_results` | background agents; worktree when writing |
| `skill`, `todo_write`, `read_tool_output` | read-only, no approval |
| `<server>__<tool>` | MCP; approval under every profile except bypass |

**File-shaped inputs.** Note the author column: four of these are written by
whatever repository happens to be open, which is what finding F-08 is about.

| Path | Author | Consumer |
| --- | --- | --- |
| `~/.packetcode/config.toml` | operator | everything |
| `~/.packetcode/.env`, `<project>/.env` | operator or repository | provider keys only |
| `.packetcode/commands/*.md` | repository | slash verbs, ACP catalogue |
| `.packetcode/workflows/*.toml` | repository | workflow steps, `system_prompt` |
| `.packetcode/skills`, `.claude/skills`, `.agents/skills` | repository | skill tool |
| `~/.codex/auth.json` | Codex CLI | codex provider, refreshed in place |

---

## Screenshot shot list

Capture at a known terminal size. Save as `qa-<shot-id>-<WIDTHxHEIGHT>.png`.
Check every capture for account names, absolute paths, hostnames, and tokens
before sharing it; the repository ignores `testdata/tui/captures` for this
reason.

| ID | Capture | How to get there |
| --- | --- | --- |
| SHOT-01 | Welcome screen, 80x24 | `packetcode`, fresh home |
| SHOT-02 | Approval prompt, `write_file` | ask mode, request a file |
| SHOT-03 | Approval prompt, `execute_command` | ask mode, request a command |
| SHOT-04 | Permission mode footer, all four | Shift+Tab through the cycle |
| SHOT-05 | Bypass indicator | `/trust on` |
| SHOT-06 | Denial message | read-only mode, request a write |
| SHOT-07 | Dotenv refusal | ask the model to read `.env` |
| SHOT-08 | Agent View, mixed states | `/spawn` twice, then `/agents` |
| SHOT-09 | Job transcript | `/jobs <id>` |
| SHOT-10 | Workflow view | `/workflows run review` |
| SHOT-11 | Provider picker | `Ctrl+P` |
| SHOT-12 | Model picker | `/model` |
| SHOT-13 | MCP table | `/mcp` with a server configured |
| SHOT-14 | doctor with a warning | config with an unset `env_from` |
| SHOT-15 | Narrow layout, 72x24 | resize, then `/help` |

The PDF carries paste-in slots for SHOT-01, 02, 03, 04, 06, 07, 08, and 13.

---

## Test sheets

### TS-A Startup, configuration, diagnostics

1. `--version` prints a version and commit, exits 0. Record both.
2. `--help` lists run, doctor, skills, acp, sugar.
3. First run with an empty home walks provider setup and writes `config.toml`.
4. `config.toml` is mode 0600 on POSIX. Record the mode.
5. `doctor` exits 0 when healthy and 1 when a check fails. Record both.
6. `doctor --json` is valid JSON and carries `schema_version`.
7. Add a key no setting matches: startup names it, `config.compatibility` warns.
8. Add `[mcp.x]` with `env_from` naming an unset variable: startup names the
   variable and `config.validation` warns. Record the exact line.
9. Set `schema_version = 99`: startup says later settings are ignored and does
   not refuse to start.
10. Set `PACKETCODE_HOME` to a relative path: refused, message names the variable.

### TS-B Credentials

1. Key only in the environment: doctor reports `env:<VAR>`, never the key.
2. Key only in `<project>/.env`: doctor reports `dotenv:<path>`.
3. Key in both: the environment wins.
4. No key for the default provider: startup names the exact variable to set.
5. Wrong key: the run fails, the error names the status, the key is not printed.
6. Grep the output and any log for the key value: it appears nowhere.
7. Gemini only, with the log on: `provider.http` URLs carry no query string.
8. `codex` with no `auth.json`: doctor explains that `codex login` is needed.

### TS-C Permissions and approvals

1. Shift+Tab cycles Manual, Accept Edits, Auto, Plan; the footer matches.
2. Bypass is outside the cycle; `/trust on` enters it with a distinct footer.
3. `/trust off` restores the previous profile and keeps session rules.
4. Ask mode: a write prompts; No feeds a refusal back and writes nothing.
5. "Yes, and do not ask again" stops the prompt for that exact command only.
   Confirm a different command still prompts.
6. `/permissions reset` revokes remembered rules and restores startup policy.
7. Plan mode denies a write even with an allow rule present.
8. A deny rule on `command_prefix` survives a compound command: `echo X; :` is
   still denied.
9. A command routed through an interpreter (`sh -c ...`) escalates to a prompt
   rather than silently running.
10. Changing mode while an approval is on screen resolves it; a running command
    is not killed.
11. `packetcode run` in ask mode exits 3 and writes nothing.

### TS-D Tool security refusals

1. `read_file .env` is refused and the message explains why.
2. `read_file .env.example` succeeds: the exception list is intact.
3. `search_codebase` for a string only in `.env` returns no hit from that file.
4. `@.env` attaches nothing; the secret is not in the transcript.
5. A symlink inside the project pointing outside it is not attached.
6. A symlink pointing inside the project is still attached.
7. `read_file ../../etc/passwd`, or an absolute path outside the root, is refused.
8. `fetch` to `http://127.0.0.1:<port>` is refused as loopback.
9. `fetch` output is wrapped in the untrusted boundary and markers cannot be forged.
10. A project skill body is labelled as repository content when loaded.

### TS-E Background jobs and worktrees

1. `/spawn` starts a read-only job; it appears in `/agents` and cannot write.
2. `/spawn --write` creates a worktree and branch. Record both paths.
3. The worktree is based on committed `HEAD`: uncommitted edits are absent.
4. A write job's approval prompt is labelled with its job id.
5. A pending approval shows the approval state, not the question state.
6. `/cancel <id>` reports cancelled, not failed.
7. Kill packetcode mid-job: on restart it reports abandoned, with a cause.
8. `/jobs resubmit` starts a new job; the original keeps its state and evidence.
9. Resubmitting the same job twice is refused.
10. Concurrency cap holds: five jobs with a cap of four leaves one queued.

### TS-F MCP

1. A configured server starts and its tools appear as `<server>__<tool>`.
2. An MCP tool call prompts under ask, accept-edits, and auto.
3. `/mcp status` names the server version and the last error.
4. `/mcp logs` redacts token-shaped values.
5. `/mcp restart` replaces the process without disturbing other servers.
6. A server that fails to start does not prevent packetcode from opening.
7. A server named with a path separator is refused at config validation.

### TS-G ACP

1. `initialize` advertises `permissionModes`. Record the list.
2. With a custom or empty profile, `bypass` is not offered and is refused if
   requested.
3. `session/new` with a relative cwd is refused with invalid params.
4. `session/prompt` expands a markdown slash command.
5. `session/cancel` reports `stopReason: cancelled`.
6. `session/close` releases the runtime and kills that session's MCP children.
7. Two clients cannot prompt one session at once; the second is told it is busy.

### TS-H Terminal behaviour

1. Finalized output lands in scrollback and survives Ctrl+L.
2. Mouse tracking and the alternate screen are never enabled; selection works.
3. Ctrl+C cancels a turn; a second press during teardown does not exit.
4. Ctrl+C with a draft clears the draft; from an empty prompt it exits.
5. Resize 100x30 to 72x24 mid-session repaints without stale chrome.
6. Tool output containing escape sequences cannot move the cursor or set the
   clipboard.
7. Backslash then Enter inserts a newline in every input state.

---

## Patch verification

Confirm the behaviour, not just that the commit is present: a reverted or
half-applied patch looks identical in `git log`.

| Patch | Commit | How to verify |
| --- | --- | --- |
| P01 | `dd71133` | log on, one Gemini turn, grep the log for query strings |
| P02 | `6fece2e` | TS-D-01 to D-04; `smoke.sh` section 6 |
| P03 | `d4f820c` | TS-D-05 and D-06 |
| P05 | `571fa98` | TS-E-05 |
| P06 | `f82868e` | run a turn, `cost-tally.json` parses, `/cost` non-zero |
| P12 | `f04688d` | TS-G-01 and G-02 with a custom profile |
| P08 | `195060b` | TS-A-07 and A-08 |
| P07 | `cea8ef7` | set `PACKETCODE_LOG_FILE`, confirm one JSON object per line |
| P10a | `d61a919` | `make vulncheck`: GO-2025-4116 is gone |
| P11 | `7aa0951` | `go build ./...` succeeds |
| 9 | `e0ce868` | `bash smoke.sh` reports 27 passed, 0 failed |

Automated gates to record: `go build`, `go vet`, `go test -count=1 ./...`
(package count and skips), `bash smoke.sh` (27/27), `make vulncheck` (compare
the reachable count with 16), `go mod verify`.

---

## Unresolved experiments

Each is a question the audit could not settle from the code, with the command
that settles it. Several decide whether an open finding closes or becomes a patch.

| ID | Question | How to settle it |
| --- | --- | --- |
| U-01 | Can anything but a same-user editor write to `packetcode acp` stdin? | inspect how PacketADE launches it and whether it owns both pipe ends |
| U-02 | Does any real config use plain `http` to a non-loopback host? | `grep -n 'base_url = "http://' ~/.packetcode/config.toml` |
| U-03 | Move the Go floor to 1.26? | a decision; CI runs 1.26.3, README says 1.24.2 |
| U-04 | Does anyone rely on `read_file .env` or `@.env`? | `grep -rl '"path":".env"' ~/.packetcode/sessions` |
| U-05 | Does an ACP client request a mode above ask over a custom profile? | check whether it reads `_packetcode.permissionModes` |
| U-06 | Is the jobs hooks test flaky only on this machine? | `go test ./internal/jobs -run TestRunJob_PassesHooksToBackgroundAgent -count=20` |
| U-07 | Label project commands and workflows untrusted like skills? | a product decision |
| U-08 | Is `internal/doctor` an empty directory in the primary checkout? | `ls -d internal/doctor` |
| U-09 | Should `collect_agent_results` prompt in the foreground? | a decision; one line either way |

---

## Operations reference

Full procedures in [docs/runbooks.md](docs/runbooks.md).

| ID | Situation | First move |
| --- | --- | --- |
| R1 | A setting seems to do nothing | `doctor --check config` |
| R2 | Set, rotate, or remove a key | `doctor --check providers` |
| R3 | A key may have been exposed | revoke first, then clean sessions |
| R4 | Provider requests failing | diagnostic log, `provider.http` |
| R5 | Codex auth broken | `codex login` |
| R6 | MCP will not start | `/mcp status`, `/mcp logs`, `/mcp restart` |
| R7 | Job stuck or abandoned | `/jobs`, `/cancel`, `/jobs resubmit` |
| R8 | Worktree left behind | `git worktree remove` |
| R9 | Disk usage | `du -sh $PC/*` |
| R10 | Undo did not restore | git is the real safety net |
| R11 | Turn on the log | `PACKETCODE_LOG_FILE`, absolute path |
| R12 | Tool denied or allowed oddly | `/permissions explain` |
| R13 | SSH computer will not connect | `ssh.connect` stage in the log |
| R14 | Vulnerability report | `make vulncheck` |
| R15 | Verify a release | `cosign verify-blob` |
| R16 | Verify a build | `smoke.sh`, then jobs alone if it hangs |
| R17 | Reset to known good | `/permissions reset`, move `config.toml` |

---

## Bug log

The PDF carries 26 blank rows. Columns: `BUG`, test id, what happened and what
was expected, runbook. A reproduction is worth more than a judgement.

---

## Day 31 backlog

1. **K-01** Move CI to Go 1.26.6: one line in `ci.yml` and `release.yml`. Clears
   nine reachable stdlib advisories with no code change.
2. **K-02** Decide U-03. If yes, apply `docs/audit/patches/P10b-*.patch` and
   update the Go version in `README.md` and `HANDOFF.md`.
3. **K-03** Add `smoke-e2e` to the `ci` target and to `ci.yml`. Watch the first
   macOS and Linux runs; it has only been exercised on Windows.
4. **K-04** Answer U-01 and U-02. Each closes an open medium finding or produces
   a small patch.
5. **K-05** Prune backups on startup. `$PC/backups` grows without bound and
   `BackupManager.Cleanup` has no production caller.
6. **K-06** Decide F-11: `collect_agent_results` either prompts or is read-only.
7. **K-07** Route code intelligence through `LocalBackend.Resolve` and retire
   `internal/tools/safefs.go`. A security boundary: needs its own review.
8. **K-08** Do not start Streamable HTTP MCP or the Packet Computers daemon in a
   low-capability window. Both have written contracts to build against later.

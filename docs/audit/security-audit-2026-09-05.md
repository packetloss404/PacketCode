# packetcode security, correctness, and half-built-feature audit

Date: 2026-09-05. Baseline audited: commit `c1bca77` on branch
`claude/security-audit-hardening-c9b2bf`. Toolchain used: go1.26.2 windows/amd64
(the module declares `go 1.24.2`; CI pins Go 1.26.3).

This document is deliverables 0 through 8 of the one-shot audit. Deliverables 9
through 12 were produced after the patches were applied and verified, and live
alongside it:

| Deliverable | Where |
| --- | --- |
| 9. Smoke test | `smoke.sh` and `tools/smokestub` at the repository root; `make smoke-e2e` |
| 10. Ops runbooks | [docs/runbooks.md](../runbooks.md) |
| 11. Handoff brief | [docs/handoff.md](../handoff.md) |
| 12. QA workbook | `build_qa_workbook.py` and [workbook.md](../../workbook.md) at the repository root |

Every behavioural claim cites `file:line`. Line numbers are against the baseline
`c1bca77` unless marked "(patched)". Where a patch moved lines, the patch commit
is named so the current position can be found with `git show`.

Verification actually run on the patched tree (`7aa0951`):

```text
go build ./...            OK
go vet ./...              OK
go test -count=1 ./...    every package ok (54 packages, 0 FAIL)
govulncheck ./...         16 reachable advisories remain (was 17); see section 8.5
```

One flaky test was observed and bisected to environment, not to any patch; see
section 7, item U6.

---

## Closeout (2026-09-05, after the follow-up work)

Everything below this section is the audit as written against baseline
`c1bca77`, and is left as written: the findings, line numbers and
reasoning were true then. This section says what has happened since, so
nobody acts on a question that already has an answer.

`main` is at `733d286`. **All sixteen CI jobs pass**, which had not
happened before: `lint` could not load its config, the TUI golden check had
never passed in its recorded history, and `vulncheck` reported fifteen
reachable advisories.

Merged since the audit: PRs #1 (day-31 backlog), #4 (lint), #5 (TUI
goldens), #7 (platform test failures), #8 (Go floor), #10 (intermittents).

### Findings that changed status

| ID | Was | Now |
| --- | --- | --- |
| F-07 | `x/crypto` v0.43.0, toolchain open | **closed.** `x/crypto` v0.56.0 and `go 1.26.0`; govulncheck reports no reachable module advisories at all. Section 8.5 is superseded. |
| F-11 | open: documented behaviour | **closed.** `RequiresApproval` returns false, so the tool and `permissions.readOnlyTool` finally agree. No profile changes its decision. |

Still open, unchanged by the follow-up work: **F-08** (repository content
presented as the user's own words), **F-09** (client-supplied MCP commands,
gated on U1), **F-10** (plain-http custom providers), **F-13** (Sigstore
verification default), **F-14** (MCP children started with
`context.Background()`).

### Four bugs the CI work surfaced that this audit did not

Worth recording, because each was found by making a check run rather than
by reading the code:

| Where | What |
| --- | --- |
| `internal/mcp/client.go` | `cmd.Wait` closes the stdout pipe under the reader, and because `markDead` is first-writer-wins the resulting `os.ErrClosed` stuck as the cause of death. `Shutdown` reported failure for a healthy server. |
| `internal/acp/server.go` | The session `active` flag was cleared in a `defer`, after the prompt response was already on the wire, so a client sending `session/load` immediately on receiving it could be rejected as busy. |
| `cmd/packetcode/main.go` | A cleanup called `jobsMgr.Shutdown` and returned `nil` unconditionally, so a job manager that failed to stop reported success. |
| `internal/procrun/process_posix.go`, `process_windows.go` | Unchecked type assertions on the tracked-process maps. |

### Unresolved questions: current state

| ID | State |
| --- | --- |
| U1 | **Open.** Still a question about how PacketADE launches `packetcode acp`. F-09 depends on it. |
| U2 | **Answered: no.** No provider `base_url` is set anywhere in the tree, so no plain-http endpoint exists to find. F-10 remains a decision about whether to refuse one pre-emptively. |
| U3 | **Resolved: yes.** The floor moved to `go 1.26.0` with `x/crypto` v0.56.0. The intermediate v0.52.0 / `go 1.25.0` option was rejected: 1.25 is itself end-of-life, and it would have left GO-2026-6354 and 6355 reachable. |
| U4 | **Answered: no.** No session under `~/.packetcode/sessions` had read a `.env`. |
| U5 | **Open.** Still a question for PacketADE's permission control code. |
| U6 | **Resolved.** The hypothesis in the question was right: it was PowerShell's cold start, not any patch. Fixed separately on `fix/windows-hook-cold-start`. |
| U7 | **Open.** Outside this tree. |
| U8 | **Open.** Product decision; F-08 waits on it. |
| U9 | **Answered: yes.** `internal/doctor/` exists in the primary checkout, is empty, and is untracked by git, so it is a local artefact rather than a repository one. The `BACKLOG.md` line describing it is accurate. |
| U10 | **Resolved: no.** Collection does not prompt; see F-11. |

---

## 0. Orientation

### What this is

`packetcode` is a single Go binary: a keyboard-first terminal coding agent
(Bubble Tea v1 TUI) that streams to hosted or local LLM providers and runs tools
against the project directory it is started in. There is **no HTTP server, no
database, no web UI, no cookies, no CSRF surface, and no webhooks** anywhere in
the tree (`grep -rn "net.Listen\|http.ListenAndServe" --include=*.go` finds only
`httptest` in tests). "Routes" in this audit therefore means: CLI subcommands and
flags, the ACP JSON-RPC methods served over stdio, the model-callable tools, the
slash commands, and the file-shaped inputs the program reads.

### Stack

| Layer | Evidence |
| --- | --- |
| Module `github.com/packetcode/packetcode`, `go 1.24.2` | `go.mod:1-3` |
| TUI: bubbletea v1.3.10, lipgloss v1.1.0, bubbles v1.0.0 | `go.mod:7-9` |
| Config: BurntSushi/toml v1.6.0 | `go.mod:6` |
| SSH/SFTP: golang.org/x/crypto v0.41.0 (now v0.43.0), pkg/sftp v1.13.10 | `go.mod:14,17` |
| No SDKs for any LLM provider; hand-rolled SSE parsers | `internal/provider/*` |
| No vendor directory; no generated code except embedded skill markdown | `git ls-files` |

### Entry points

| Entry | File | Notes |
| --- | --- | --- |
| `packetcode` (TUI) | `cmd/packetcode/main.go:56-100`, `run()` at `:117` | default when no subcommand |
| `packetcode run` | `cmd/packetcode/run_command.go:67` | one headless turn; approvals fail closed, exit 3 |
| `packetcode doctor` | `cmd/packetcode/doctor.go:80` | diagnostics, `--json`, `--check`; exit 1 on any fail |
| `packetcode skills list/install/remove/path` | `cmd/packetcode/skills.go:34` | `install` runs `git clone` |
| `packetcode acp` | `cmd/packetcode/acp.go:107` | ACP v1 JSON-RPC over stdin/stdout |
| `packetcode sugar login` | `cmd/packetcode/sugar_login.go:65` | OAuth device-code flow, saves token to config.toml |
| `--tui-fixture=<state>` | `cmd/packetcode/main.go:85-91` | dev renderer; loads no config |
| Subcommand table (help and dispatch share it) | `cmd/packetcode/usage.go:35-68` | |

### How it is deployed and who can reach it

- Distribution is GitHub Releases for `packetloss404/packetcode` via goreleaser
  (`.goreleaser.yml`, `.github/workflows/release.yml`), installed by
  `install.sh` / `install.ps1` which verify `checksums.txt` and, when `cosign`
  is present, its Sigstore signature (`install.sh:98-99`, `install.ps1:53-54`).
- **Live vs local: everything is local.** The binary runs as the invoking user
  on their machine. Nothing in this repository is hosted. **Guess, flagged:**
  the Sugar service (`https://usesugar.dev`, `internal/provider/sugar/sugar.go:32`)
  and the PacketADE desktop app referenced in docs are separate projects; their
  server-side behaviour is outside this audit.
- Reachability, in order of trust:
  1. The local user at the keyboard (TUI, `run`, `doctor`, `skills`, `sugar`).
  2. A local ACP client driving `packetcode acp` over stdio (an editor or the
     PacketADE desktop app). The ACP client chooses the working directory and
     may supply MCP server commands (`internal/acp/server.go:936-969, 1134-1164`).
     **Guess, flagged:** assumed to be the same OS user; if anything else can
     write to that stdin, section 3 item F-09 becomes high.
  3. MCP server child processes configured in `config.toml` (`[mcp.<name>]`);
     trusted local code, not sandboxed (`docs/security.md:63-64`).
  4. LLM providers: remote, and the model's output is the untrusted input that
     every tool call is built from.
  5. The repository being edited: `.packetcode/commands/*.md`,
     `.packetcode/workflows/*.toml`, `.packetcode/skills`, `.claude/skills`,
     `.agents/skills`, `<project>/.env`, symlinks. A cloned hostile repository
     reaches the model through several of these (section 3, F-08).
  6. Remote SSH hosts registered as Packet Computers (host key pinned,
     `internal/computers/ssh_backend.go:189-201`).
  7. Web pages reached by the `fetch` tool (`internal/tools/fetch.go`).

### What is unfinished or half-built (summary; full list in section 5)

Streamable HTTP MCP (validator without transport), Packet Computers `managed`
kind and daemon heartbeat, the background-agent "question" feature
(`AwaitingAnswer`), ACP skill catalogue, persisted undo stack, SSH teardown
confirmation, and a handful of test-only wrapper functions.

### Vendored or third-party code

None vendored. Go modules only (`go list -m all` in section 8.5). Embedded
assets: five builtin skills under `internal/skills/builtin/*/SKILL.md`. The
rule "do not patch vendored, generated, or third-party code" therefore applied
only to dependencies, handled by version pinning in section 8.5.

---

## 1. Architecture map

```text
                 user keyboard            ACP client (stdio JSON-RPC)        scripts / CI
                      |                           |                               |
               internal/app (TUI)          internal/acp.Server            cmd/packetcode run
                      \                           |                               /
                       \----------- cmd/packetcode/runtime.go: buildPacketRuntime ----------/
                                   provider registry | session manager | tool registry
                                   permission policy | hooks | MCP manager | cost tracker
                                                     |
                                          internal/agent.Agent.Run
                            (policy Decide -> PreToolUse hook -> Approver -> tool.Execute -> PostToolUse)
                                   /                |                    \
                    internal/provider/*      internal/tools/*        internal/jobs.Manager
                    (HTTP to LLM APIs)   (root-confined via          (background sub-agents,
                                          computers.RuntimeBackend)   worktrees, SSH backends)
                                                     |
                                    ~/.packetcode/{config.toml,sessions,jobs,worktrees,
                                     backups,tool-output,cost-tally.json,mcp-*.log}
```

Trust boundaries, each with its enforcement point:

| Boundary | Enforced at | Notes |
| --- | --- | --- |
| Model output -> tool execution | `internal/agent/agent.go:474-535` (Decide, hook, Approve, re-Decide on edited params) | policy is a decision layer, not a sandbox |
| Tool path -> filesystem | `internal/computers/local_backend.go:50-136` (lexical + EvalSymlinks both sides), SSH: `ssh_backend.go:296-376` (RealPath) | `internal/tools/safefs.go` is a second, mostly superseded implementation (section 5) |
| Shell command -> OS | `local_backend.go:239-253` `cmd /C` or `sh -c`, tree-kill via `internal/procrun` | full user privilege by design |
| fetch -> network | `internal/tools/fetch.go:343-362` post-DNS dial guard; `:105-125` proxy disabled, caps | approval-gated (`:147`) |
| ACP client -> session policy | `cmd/packetcode/acp.go:282-305` ceiling check | fixed to fail closed in P12 |
| Background job -> foreground approvals | `internal/jobs/approver.go:37-70`, `internal/app/approver.go:89-98` | read-only jobs never prompt; write jobs prompt with `[job:<id>]` |
| Repository content -> model | skills labelled (`internal/skills/block.go:19-34`); commands and workflows not labelled | F-08 |
| Secrets -> subprocesses | `.env` never exported (`internal/config/dotenv.go:19-26`); MCP env allowlist (`internal/mcp/process.go:96-127`) | |
| Secrets -> model | none before P02; now `internal/tools/secretfiles.go` | F-02 |
| Persistence -> disk | `internal/atomicfile` (fsync+rename) for sessions, jobs, config, registry; tally added in P06 | modes 0600 |

Data stores (all under `~/.packetcode`, or `PACKETCODE_HOME`, `internal/config/paths.go`):
`config.toml` (0600, may hold API keys and the Sugar token), `sessions/<id>.json`
(0600, full transcripts including tool output), `jobs/<id>.json`,
`worktrees/<sha12>/<id>`, `backups/<session>/`, `tool-output/s-<id>/`
(spilled tool output, pruned after 24h), `cost-tally.json`, `mcp-<name>.log`
(0600, server stderr), `computers/registry.json`, `skill-approvals.json`.
Outside the home: `~/.codex/auth.json` (Codex OAuth, read and refreshed in
place, `internal/provider/codexauth/codexauth.go:198-242`).

Outbound calls (complete list from `grep http.NewRequest` and constants):

| Destination | Auth | File |
| --- | --- | --- |
| `https://api.openai.com/v1` | Bearer | `internal/provider/openai` via `openaicompat/client.go:392`, responses via `responses/client.go` |
| `https://chatgpt.com/backend-api/codex` | Bearer (OAuth), `chatgpt-account-id` | `internal/provider/codex/codex.go:26`, `responses/client.go:369-385` |
| `https://auth.openai.com/oauth/token` | refresh_token grant | `codexauth/codexauth.go:38,265` |
| `https://api.anthropic.com/v1` | `x-api-key` | `anthropic/anthropic.go:216-219` |
| `https://generativelanguage.googleapis.com/v1beta` | was `?key=` query, now `x-goog-api-key` (P01) | `gemini/gemini.go` |
| `https://api.minimax.io/v1`, `https://api.deepseek.com/v1`, `https://api.x.ai/v1`, `https://api.mistral.ai/v1`, `https://openrouter.ai/api/v1` | Bearer | each provider package |
| `http://localhost:11434` (or `OLLAMA_HOST`) | none | `ollama/ollama.go`, `cmd/packetcode/main.go:566-581` |
| Sugar: `http://localhost:3211/api/v1` default, hosted `https://usesugar.dev` offered at login | Bearer | `sugar/sugar.go:23,32`, `sugar/runtime.go:264-284` |
| Custom `[providers.<slug>] base_url` | Bearer, `http` allowed | `custom/custom.go:255-270` |
| Any `http(s)` URL the model names | none, approval-gated | `tools/fetch.go` |
| `git clone <url>` for `skills install` | git's own | `internal/skills/install.go:217-251` |
| SSH to registered computers | agent or identity file; pinned SHA256 host key | `computers/ssh_backend.go` |
| GitHub releases (installers only) | none | `install.sh:65-81` |

Background jobs: `internal/jobs.Manager` (defaults 4 concurrent, depth 2, 32
total; `internal/config/defaults.go:33-35`), each with its own session file,
tool registry (read-only clone unless `AllowWrite`), and, for write jobs, a git
worktree (`internal/jobs/worktree.go`). Workflows (`internal/workflow`) fan out
over the same manager. Loops (`internal/app/slashcmd_loop.go`) re-run prompts,
capped at 25 iterations.

Config and secret loading: `config.LoadFrom` (`internal/config/config.go:464-512`)
decodes TOML into defaults, attaches `.env` from `~/.packetcode/.env` and
`<cwd>/.env` (`dotenv.go:66-98`), applies `PACKETCODE_*` feature-gate
variables, and validates Sugar/Conduit ranges. Key precedence: process
environment, then `.env`, then `config.toml` (`config.go:720-736`). Since P08,
`ValidationProblems` (`internal/config/validate.go`) reports inert settings at
boot.

---

## 2. Surface inventory

Auth column: who can invoke. Gate column: the permission decision under the
default `ask` profile (`internal/permissions/policy.go:461-471`) or the explicit
control. Mutates: yes/no. Model-callable tools are reachable by anything that
controls model output, which includes a hostile repository or web page that the
model reads.

### 2.1 CLI

| ID | Method | Path | Auth | Gate | Mutates | file:line |
| --- | --- | --- | --- | --- | --- | --- |
| S-CLI-01 | exec | `packetcode [--provider --model --resume --trust --permission-mode --computer]` | local user | first-run setup if no provider | sessions, backups, config (setup) | `cmd/packetcode/main.go:117-502`, flags `usage.go:98-109` |
| S-CLI-02 | exec | `packetcode run [flags] <prompt>` | local user / scripts | approvals rejected, exit 3 | sessions, files if profile allows | `run_command.go:67-139,170-252` |
| S-CLI-03 | exec | `packetcode doctor [--json] [--check]` | local user | none | creates and removes a temp file per state dir | `doctor.go:80-114,421-435` |
| S-CLI-04 | exec | `packetcode skills install <repo> [--project --force --ref --skill]` | local user | none | writes `~/.packetcode/skills` or `.packetcode/skills`; runs `git clone` | `skills.go:242-305`, `internal/skills/install.go:78-151` |
| S-CLI-05 | exec | `packetcode skills remove <name>` | local user | none | `os.RemoveAll` of one skill dir | `install.go:154-177` |
| S-CLI-06 | exec | `packetcode acp [--provider --model --permission-mode]` | local user; then the ACP client | see 2.2 | sessions, files per session policy | `acp.go:107-199` |
| S-CLI-07 | exec | `packetcode sugar login [--server --name --no-browser]` | local user | https required unless localhost | writes token + base_url into `config.toml` | `sugar_login.go:69-285,394-407` |
| S-CLI-08 | exec | `--tui-fixture=<state>` | local user | none | none | `main.go:85-91` |

### 2.2 ACP JSON-RPC (stdio; requires `initialize` first, `server.go:500-509`)

| ID | Method | Auth | Gate | Mutates | file:line |
| --- | --- | --- | --- | --- | --- |
| S-ACP-01 | `initialize` | client | once only | no | `internal/acp/server.go:543-613` |
| S-ACP-02 | `session/new` (cwd, mcpServers, `_packetcode.{provider,model,permissionMode}`) | client | cwd must be an existing absolute dir; permissionMode <= ceiling (P12); MCP commands absolute paths | spawns MCP children, creates session file | `server.go:915-1002`, `acp.go:282-305,1134-1164` |
| S-ACP-03 | `session/load` (sessionId, cwd, mcpServers) | client | sessionId validated against path chars (`session.go:615-620`) | replaces runtime; replays transcript | `server.go:1009-1122` |
| S-ACP-04 | `session/prompt` | client | one active prompt per session; slash expansion of markdown commands | drives the agent loop | `server.go:1166-1213,848-877` |
| S-ACP-05 | `session/cancel` | client | none | cancels turn | `server.go:1415-1441` |
| S-ACP-06 | `session/close` | client | idempotent | releases runtime, kills MCP children | `server.go:1472-1536` |
| S-ACP-07 | `_packetcode/sessions/list` | client | none | no | `server.go:622-640` |
| S-ACP-08 | `_packetcode/sessions/rename` | client | name sanitised | rewrites session file | `server.go:642-672`, `acp.go:316-326` |
| S-ACP-09 | `_packetcode/sessions/usage` | client | none | no | `server.go:674-696` |
| S-ACP-10 | `_packetcode/models/list` | client | none | no | `server.go:698-716` |
| S-ACP-11 | `_packetcode/mcp/list` | client | none | no | `server.go:729-769` |
| S-ACP-12 | `_packetcode/commands/list` (cwd) | client | none | no; reads `<cwd>/.packetcode/commands` | `server.go:771-798`, `acp.go:399-429` |
| S-ACP-13 | `_packetcode/project/files` (cwd, query, limit<=200) | client | none | no; runs `git ls-files` in cwd | `server.go:800-840`, `acp.go:467-480` |
| S-ACP-14 | `session/request_permission` (server -> client) | server | allow_once / reject_once only | no | `server.go:1538-1585` |

### 2.3 Model-callable tools (native)

| ID | Tool | RequiresApproval | Gate under `ask` | Mutates | file:line |
| --- | --- | --- | --- | --- | --- |
| S-TOOL-01 | `read_file` | no | allow (read-only); `.env` refused since P02 | no | `internal/tools/read_file.go:61-143` |
| S-TOOL-02 | `search_codebase` | no | allow; `.env` skipped since P02 | no (runs `rg`) | `search_codebase.go:77-107` |
| S-TOOL-03 | `list_directory` | no | allow | no | `list_directory.go:63-114` |
| S-TOOL-04 | `list_symbols`, `find_definition`, `find_references`, `get_diagnostics` | no | allow | no | `code_intelligence.go` |
| S-TOOL-05 | `write_file` | yes | ask (allow under accept_edits/auto/bypass) | yes, with backup | `write_file.go:73-129` |
| S-TOOL-06 | `patch_file` | yes | ask | yes, with backup | `patch_file.go:326-403` |
| S-TOOL-07 | `execute_command` (command, cwd, timeout<=600s) | yes | ask (allow under auto/bypass); deny floors and prefix rules | yes: full shell as user | `execute_command.go:78-179`, `local_backend.go:229-282` |
| S-TOOL-08 | `fetch` (url, timeout<=120s) | yes | ask (allow only under bypass) | outbound GET | `fetch.go:161-246` |
| S-TOOL-09 | `spawn_agent` (prompt, provider, model, computer, wait, allow_write) | yes | ask; read-only background jobs auto-approve it | spawns sub-agent, may create worktree | `spawn_agent.go:94-190`, `jobs/approver.go:38-43` |
| S-TOOL-10 | `collect_agent_results` | foreground yes, background no | allow: listed as read-only tool | injects results into context | `collect_agent_results.go:31-36`, `permissions/policy.go:481` |
| S-TOOL-11 | `skill` (name, file) | no | allow | no; project skills labelled untrusted | `skill.go:63-124` |
| S-TOOL-12 | `todo_write` | no | allow | in-memory only | `todo.go` |
| S-TOOL-13 | `read_tool_output` (handle) | no | allow; opaque random handles | no | `read_tool_output.go:80-105` |
| S-TOOL-14 | `<server>__<tool>` (MCP) | yes | ask under every profile except bypass | whatever the server does | `internal/mcp/tool.go:101-161`, `policy.go:488-490` |

### 2.4 Slash commands (TUI only; `internal/app/keymap.go:58-106`)

Mutating ones: `/spawn`, `/cancel`, `/jobs resubmit`, `/computers ssh|register|remove --yes`,
`/provider add` (saves key), `/model` and `/provider` (persist default),
`/effort` (persists), `/sessions rename|delete --yes`, `/undo`, `/compact`,
`/cost reset --yes`, `/trust on|off`, `/permissions profile|rule|reset`,
`/plan`, `/loop`, `/workflows run|stop`, `/mcp restart`, `/skills allow|revoke`.
All require the user to type them. Markdown commands and user-invocable skills
register as additional verbs (`internal/app/slash_registry.go:114-129`).

### 2.5 File-shaped inputs

| ID | Path | Author | Consumer | file:line |
| --- | --- | --- | --- | --- |
| S-IN-01 | `~/.packetcode/config.toml` | operator | everything | `internal/config/config.go:464` |
| S-IN-02 | `~/.packetcode/.env`, `<project>/.env` | operator / **repository** | key resolution only | `dotenv.go:66-98` |
| S-IN-03 | `.packetcode/commands/*.md` | **repository** | slash verbs, ACP command catalogue | `slash_registry.go:320-369`, `acp.go:399-429` |
| S-IN-04 | `.packetcode/workflows/*.toml` | **repository** | workflow engine (provider, model, system_prompt, allow_write per step) | `internal/workflow/loader.go` |
| S-IN-05 | `.packetcode/skills`, `.claude/skills`, `.agents/skills` | **repository** | skill tool, `/name` verbs; foreign layouts need `/skills allow` | `internal/skills/foreign.go:119-134`, `skills.go:587` |
| S-IN-06 | `~/.packetcode/theme.toml` | operator | theme | `main.go:147-154` |
| S-IN-07 | `~/.codex/auth.json` | Codex CLI | codex provider | `codexauth.go:84-93` |
| S-IN-08 | `~/.packetcode/jobs/*.json`, `sessions/*.json`, `computers/registry.json`, `skill-approvals.json` | packetcode itself | versioned readers, refuse newer | `internal/compat` |

### 2.6 Process-spawning surfaces (config-owned)

| ID | Spawned | Shell | file:line |
| --- | --- | --- | --- |
| S-PROC-01 | hooks (`user_prompt_submit`, `pre_tool_use`, `post_tool_use`) | `powershell -ExecutionPolicy Bypass -Command` or `sh -c`, cwd = project | `internal/hooks/hooks.go:194-203` |
| S-PROC-02 | statusline command | same | `internal/statusline/statusline.go:218-227` |
| S-PROC-03 | MCP servers | direct exec, env allowlist + `env`/`env_from` | `internal/mcp/process.go:26-94` |
| S-PROC-04 | `git` for worktrees, repo root, file index, skills install | direct exec, `GIT_TERMINAL_PROMPT=0` | `internal/jobs/worktree.go:252`, `internal/git/git.go`, `internal/app/fileindex.go:60`, `install.go:242-245` |
| S-PROC-05 | `rg` | direct exec, `--` before pattern | `search_codebase.go:195-209` |
| S-PROC-06 | browser opener for Sugar login | `rundll32`/`open`/`xdg-open <url>` | `sugar_login.go:381-392` |

---

## 3. Findings

Severity: crit / high / med / low. Status: the patch commit that fixes it, or
"open" with a decision owner. Section 3.2 lists every checklist item found clean.

### 3.1 Findings table

| ID | Sev | file:line (baseline) | What is wrong | Trigger | One-line fix | Status |
| --- | --- | --- | --- | --- | --- | --- |
| F-01 | med | `internal/provider/gemini/gemini.go:68,132,406-407` | Gemini API key sent as `?key=` URL query; URLs land in `*url.Error` text, proxy/gateway logs, and any printed request | Any Gemini request through a logging proxy, or any transport error path not covered by `redactAPIKey` | Send the key in the `x-goog-api-key` header | **P01** `dd71133` |
| F-02 | med | `internal/tools/read_file.go:69`; `search_codebase.go:150,199-207,313`; `internal/app/mentions.go:80-126` | `.env` files, which the program itself treats as credential stores (`dotenv.go:19-26`), are readable by the model with no approval and inlineable by `@.env` | Model calls `read_file .env` (read-only tool, allowed under every profile) or a repository command body says `@.env`; contents go to the provider and into `sessions/*.json` | One name rule (`tools.IsSecretFilePath`) applied in read_file (supplied and resolved path), all three search engines, and mention expansion | **P02** `6fece2e` |
| F-03 | low | `internal/app/mentions.go:103-108` | `@`-mention confinement is lexical only; `os.Stat`/`os.ReadFile` follow symlinks, so an in-repo link to an outside file is inlined | Repository contains `notes.txt -> /home/user/.ssh/id_rsa`; user types `@notes.txt` (or a repository command does) | `EvalSymlinks` both root and target and re-check containment, as `local_backend.go:57-70` does | **P03** `d4f820c` |
| F-04 | low (auth, fail-open) | `cmd/packetcode/acp.go:55-66` | ACP permission ceiling is `full` when the profile is `""` or a custom profile, while the policy those produce is `ask` (`permissions/policy.go:63-65,356-385`) | Operator writes `profile = ""` or `[permissions.profiles.team]`; ACP client requests `permissionMode: "bypass"` and gets it | Resolve the ceiling through `ParseProfile` like the policy does; default `ask`; only `trust_mode` raises to full | **P12** `f04688d` |
| F-05 | low (correctness) | `internal/jobs/worker.go:263-268` | `NeedsInput` set equal to `NeedsApproval` on every tool proposal, so `Snapshot.AwaitingAnswer` (`job.go:305`) is never true and Agent View draws the question icon for approvals (`agentview.go:622`) | Any write job proposes a tool | Pass `false` for `needsInput` | **P05** `571fa98` |
| F-06 | low (durability) | `internal/cost/tally.go:84-111` | tally written temp-then-rename without fsync; a crash can publish an empty file that `Load` refuses (`:74`), disabling `/cost` and the statusline cost | Power loss during any usage update | Use `internal/atomicfile.Write` | **P06** `f82868e` |
| F-07 | med | `go.mod:17` (`golang.org/x/crypto v0.41.0`); toolchain go1.26.2 | 8 reachable `x/crypto/ssh` advisories and 9 reachable stdlib advisories (`govulncheck`), all via `computers.NewSSHBackend` and provider HTTP | Connecting to a hostile or compromised SSH computer; hostile HTTP/2 server | `x/crypto` v0.43.0 (keeps `go 1.24` floor) now; v0.56.0 + `go 1.26.0` as opt-in patch; build with Go >= 1.26.6 | **P10a** `d61a919`; **closed** by v0.56.0 + `go 1.26.0` (PR #8) — see Closeout |
| F-08 | med | `internal/app/app.go:2320-2357`; `internal/acp/server.go:848-877`; `internal/workflow/loader.go` | Repository content is treated as the user's own words: `.packetcode/commands/*.md` bodies are mention-expanded and shown as the user's message; `.packetcode/workflows/*.toml` may set `system_prompt`, `provider`, `model`, `allow_write` per step. Skills, by contrast, are labelled untrusted (`skills/block.go:19-34`) | Clone a hostile repo, type `/review` (a name the repo chose) or `/workflows run <name>`; the prompt and system prompt are the attacker's, presented as yours. Every tool call is still gated, and P02 removes the `.env` exfil path | Label project command bodies the way skill bodies are labelled (`skills.Block` framing) and stop mention-expanding them; require `/workflows validate` + confirm for project workflows that set `system_prompt` or `allow_write` | open: product decision (documented as accepted in `docs/security.md:3`) |
| F-09 | med (conditional) | `internal/acp/server.go:936-969,1134-1164`; `cmd/packetcode/acp.go:441-453` | An ACP client may choose any existing absolute `cwd` and supply arbitrary MCP `command` + `env`; the server execs them. Fine for a same-user editor; equivalent to arbitrary code execution for anything else that can reach the stdio pipe | A non-trusted local process gets hold of the `packetcode acp` stdin | Restrict client-supplied MCP commands to those already in `config.toml` unless `[acp] allow_client_mcp = true` | open: depends on U1 |
| F-10 | med | `internal/provider/custom/custom.go:255-270`; `cmd/packetcode/doctor.go:513-523` | Custom OpenAI-compatible providers accept plain `http://` to non-loopback hosts and send the Bearer key plus the whole conversation in cleartext; only `doctor` warns | Operator types `base_url = "http://models.corp/v1"` | Refuse non-loopback `http` unless `allow_insecure_http = true` on the provider table; safe partial is the existing doctor warning plus a startup warning (add to `ValidationProblems`) | open: decision (see U5) |
| F-11 | low | `internal/tools/collect_agent_results.go:31-36`; `internal/jobs/spawner_adapter.go:172-175`; `permissions/policy.go:481` | Foreground `collect_agent_results` is classified read-only, so it never prompts under any profile despite `RequiresApproval` returning true, and it may collect any job id | Model collects a job the user did not intend to inject | Either drop it from `readOnlyTool` or drop `RequiresApproval`; today the two disagree | **closed** by dropping `RequiresApproval` (PR #1) — see Closeout |
| F-12 | low | `internal/hooks/hooks.go:197`; `internal/statusline/statusline.go:221` | Hooks and statusline run through `powershell -ExecutionPolicy Bypass` | Operator config only | None needed; documented (`docs/security.md:70`) | checked, accepted |
| F-13 | low | `install.sh:98-99`; `install.ps1:53-54` | Sigstore verification is skipped when `cosign` is absent unless `REQUIRE_SIGNATURE=1` / `-RequireSignature` | Machine without cosign installs an unsigned or substituted archive whose checksums also match | Default the requirement on once a release with signatures exists; until then document | open: release decision |
| F-14 | low | `internal/mcp/process.go:27` | MCP children are started with `context.Background()`, so `cmd.Cancel` never fires; shutdown relies on stdin close + `KillTree` | None (works) | Pass the manager's context; cosmetic | open: BACKLOG |
| F-15 | low | `internal/session/session.go:682-711` | Session files persist full tool output (0600). Anything the model read is on disk for the life of the session | `execute_command cat secret` | Covered by 0600 and by P02 for `.env`; document | checked, accepted |
| F-16 | low | `internal/jobs/approver.go:38-43` | Read-only background jobs auto-approve `spawn_agent` | Runaway fan-out | Bounded by `MaxDepth`/`MaxTotal` (`manager.go` `checkSpawnAllowedLocked`); children cannot gain `allow_write` (`spawn_agent.go:114-122`) | checked, clean |
| F-17 | low | `internal/agent/agent.go:474-501` | `PreToolUse` hook runs after the policy decision, so a denied call never reaches hooks and a hook cannot pre-empt policy | None today (hooks cannot rewrite params) | Reorder only if hook verdicts are added; see `docs/research/upstream-adoption-plan.md` 6.3 | checked, accepted |
| F-18 | low | `internal/tools/atomic.go` | Dead `atomicWrite` duplicating a live security boundary | none | Delete | **P11** `7aa0951` |

### 3.2 Security checklist, item by item

- **Session issuance/expiry/rotation/revocation/fixation**: not applicable
  (no login sessions). Conversation sessions are UUIDs (`session.go:146`),
  files 0600, ids validated against path characters (`:615-620`), and a
  session written by a newer build is refused rather than overwritten
  (`:259-262,691-694`). ACP session ids are the same UUIDs. Codex OAuth
  refresh rewrites `auth.json` atomically with 0600 (`codexauth.go:198-242`).
  Checked, clean.
- **Authorization on every route/handler, IDOR**: every ACP method requires
  `initialize` (`server.go:500-509`); per-session state keyed by id the client
  was given; `sessions/rename` and `usage` accept any persisted id, which is
  the same OS user's data. Tool authorization is centralised in
  `agent.handleToolCall` with re-check after edited params (`agent.go:521-534`).
  Background `collect_agent_results` is limited to descendants
  (`spawner_adapter.go:172-183`). F-04 fixed; F-11 noted. Otherwise clean.
- **Webhook signature/replay**: none exist. Checked, clean.
- **CSRF and cookie flags**: no browser surface. Checked, clean.
- **SSRF**: `fetch` guards on the resolved address at dial time, disables
  proxies, refuses userinfo, caps redirects (`fetch.go:104-125,287-362`);
  Sugar verification URL must share the login origin (`sugar_login.go:409-419`);
  custom provider URLs are operator config. Checked, clean (F-10 is a
  cleartext issue, not SSRF).
- **Injection**: no SQL. Command injection: shell commands are the model's
  own text by design and approval-gated; `rg`, `git`, worktree branch names
  use argv with `--` separators and `validateBranch`
  (`worktree.go`, `search_codebase.go:207`, `install.go:237`); remote SSH
  commands are `cd -- '<quoted>' && <command>` (`ssh_backend.go:530,586-588`);
  `quotePOSIX` for remote worktrees. Template injection: workflow prompts use
  `text/template` with `missingkey=zero` over repo-authored templates (F-08).
  Header injection: none (headers are constants). Log injection: MCP logs are
  raw server stderr but displayed through `RedactSensitiveText` and the
  terminal sanitizer (`slashcmd_mcp.go:381-383`, `internal/ui/terminaltext`).
  Prompt-marker forgery: fetch and skill blocks defang their own markers
  (`fetch.go:567-571`, `block.go:113-128`). Checked, clean apart from F-08.
- **Rate limiting and cost controls**: retry with backoff honouring
  `Retry-After` (`retry.go:112-159`); stall guard; per-job and per-workflow
  token budgets; 25-iteration turn cap and loop detector (`agent.go:36,250`);
  job caps 4/2/32; tool-output spill budget 64 MiB (`toolout/store.go:40`).
  Checked, clean.
- **Secrets in code, bundles, logs, error bodies**: no hardcoded secrets
  (`oauthClientID` at `codexauth.go:34` is a public client id). Gemini key in
  URLs fixed (F-01). `doctor` redacts URL userinfo and secret-looking args
  (`doctor.go:837-908`); provider error bodies are capped at 64 KiB
  (`provider/errors.go`). New log (P07) never writes bodies, arguments, or
  query strings. Checked, clean after P01.
- **TLS enforcement/redirects**: hosted providers are `https`; Sugar login
  requires `https` unless loopback (`sugar_login.go:394-407`); custom
  providers may use `http` (F-10); fetch re-checks scheme per hop
  (`fetch.go:321-333`). No `InsecureSkipVerify` anywhere (grep).
- **Error handling leaks**: error strings include local paths and provider
  messages; all land in the local terminal or the local transcript. No
  network-facing error responses exist. Checked, clean.
- **File upload / path traversal**: all native file tools resolve through
  `RuntimeBackend.Resolve` with `EvalSymlinks` on both sides
  (`local_backend.go:57-113`; SSH `RealPath` `ssh_backend.go:305-352`);
  `read_tool_output` handles are random and never paths (`toolout/store.go:5-12`);
  skill resources are confined lexically and after symlink resolution
  (`skills/resources.go:104-158`); session ids and MCP names are
  character-allowlisted. `@`-mentions were the one lexical-only path (F-03).
  Checked, clean after P03.
- **CORS/origin**: not applicable. Sugar device-flow verification URL is
  origin-pinned to the login server (`sugar_login.go:409-419`). Checked, clean.

---

## 4. Patches

All patches are commits on branch `claude/security-audit-hardening-c9b2bf`, in
dependency order, each independently revertable with `git revert <sha>`. Patch
files exported with `git format-patch` are under `docs/audit/patches/`. Nothing
has been pushed. Apply to `main` with `git cherry-pick <sha>` or
`git am docs/audit/patches/NNNN-*.patch`.

Dependency order and file overlap:

| # | Commit | Files | Depends on | Notes |
| --- | --- | --- | --- | --- |
| P01 | `dd71133` gemini: send the API key in x-goog-api-key | `internal/provider/gemini/gemini.go`, `gemini_test.go` | none | |
| P02 | `6fece2e` tools: refuse dotenv secret files | `internal/tools/{secretfiles.go,secretfiles_test.go,read_file.go,read_file_test.go,search_codebase.go}`, `internal/app/{mentions.go,mentions_test.go}` | none | behaviour change: `.env` unreadable by tools |
| P03 | `d4f820c` app: resolve symlinks before an @-mention | `internal/app/{mentions.go,mentions_test.go}` | none (separate hunks from P02; revert either alone) | |
| P05 | `571fa98` jobs: pending approval no longer reports NeedsInput | `internal/jobs/{worker.go,job.go,manager_test.go}` | none | |
| P06 | `f82868e` cost: fsync the tally | `internal/cost/tally.go` | none | |
| P12 | `f04688d` acp: ceiling matches policy | `cmd/packetcode/{acp.go,acp_test.go}` | none | behaviour change for ACP clients on `""`/custom profiles |
| P08 | `195060b` config: validate at boot | `internal/config/{validate.go,validate_test.go}`, `cmd/packetcode/{main.go,run_command.go,acp.go,doctor.go}` | none (touches `acp.go` in a different function from P12) | |
| P07 | `cea8ef7` diaglog: opt-in structured log | `internal/diaglog/*`, `cmd/packetcode/{main.go,sugar_login.go}`, `internal/provider/{retry.go,codexauth/codexauth.go}`, `internal/agent/agent.go`, `internal/acp/server.go`, `internal/mcp/client.go`, `internal/tools/fetch.go`, `internal/computers/ssh_backend.go`, `internal/hooks/hooks.go` | none (touches `main.go` in `main()`, P08 in `run()`) | |
| P10a | `d61a919` deps: x/crypto v0.43.0 | `go.mod`, `go.sum` | none | also bumps `x/text` indirect to v0.30.0 |
| P11 | `7aa0951` tools: remove atomicWrite | `internal/tools/atomic.go` (deleted) | none | |
| P10b | file only: `docs/audit/patches/P10b-x-crypto-v0.56.0-go1.26.patch` | `go.mod`, `go.sum` | apply after P10a | raises `go` directive to 1.26.0; built, vetted, and `internal/computers` + `internal/jobs` SSH tests passed with it applied |

The full diffs are in the patch files; the summary of each change follows.

**P01** (`gemini.go`): `ValidateKey`, `ListModels`, and `ChatCompletion` build
URLs without `key=` and call `setAPIKeyHeader(req, key)` which sets
`x-goog-api-key`. `redactAPIKey` is kept. Tests assert the header is present
and `RawQuery` never contains `key=`.

**P02** (`secretfiles.go`): `IsSecretFilePath(name)` is true for basename
`.env` or `.env.*` except `.example`, `.sample`, `.template`, `.dist`; handles
both separators. `read_file` refuses on the supplied name and on
`Backend.Resolve`'s real path; `search_codebase` skips the name in the Go and
backend walkers and appends `--glob '!.env' --glob '!.env.*'` after the caller's
glob for ripgrep; `readMention` refuses the raw token. Tests added in three
packages.

**P03** (`mentions.go`): after the lexical check, `EvalSymlinks(absRoot)` and
`EvalSymlinks(p)`, re-check containment, `Lstat` the real path, read the real
path. Two tests (escape refused, in-root link allowed; both skip if the
platform cannot create symlinks).

**P05** (`worker.go:268`): `updateActivity(j, activity, name, false, needsApproval)`.
Test expectation at `manager_test.go:991` updated; comment in `job.go` updated.

**P06** (`tally.go`): `Save` becomes `atomicfile.Write(path, data, 0o600, ".tally.*.json.tmp")`.

**P12** (`acp.go`): `serverPermissionCeiling` starts at `ProfileAsk`, resolves
`ParseProfile(cfg.Permissions.Profile)`, and `trust_mode` raises to full. Test
`TestServerPermissionCeilingDefaults` updated to the new contract.

**P08** (`validate.go`): `(*Config).ValidationProblems()` checks MCP names,
enabled-without-command, negative timeouts, missing `env_from` variables;
custom provider `base_url`; `api_key_env` naming an unset variable; default
provider unknown or keyless-but-required (names the `PACKETCODE_<SLUG>_API_KEY`
variable); behavior ranges; hook/statusline shape. Printed as
`packetcode: config <problem>` by the TUI (after setup), `run`, and `acp`;
`doctor` gains `config.validation`. Seven tests.

**P07** (`internal/diaglog`): `InitFromEnv` reads `PACKETCODE_LOG_FILE`
(absolute path required), opens append 0600, JSON handler, `pid` attribute.
`RedactURL` strips query, fragment, userinfo; `ErrText` redacts URLs inside
`*url.Error`. Call sites: `main()` startup; `provider.DoWithRetry` per attempt;
`codexauth.Refresh`; `agent.handleToolCall` policy decision and approval
outcome (tool name only); ACP session new/load/close and permission outcome;
`mcp.NewClient` spawn/handshake; `fetch` request and result; `NewSSHBackend`
dial/handshake/connect; `hooks.runOne`; Sugar login success.

**P10a**: `go get golang.org/x/crypto@v0.43.0`.

**P11**: `git rm internal/tools/atomic.go`.

Public interfaces changed: none removed or re-signatured. Added:
`tools.IsSecretFilePath`, `(*config.Config).ValidationProblems`, package
`internal/diaglog`. Callers of every touched exported function were listed in
each commit message.

---

## 5. Half-built inventory

Grep for `TODO|FIXME|XXX|HACK|not yet implemented|unimplemented|stub|placeholder`
in `*.go` returns no genuine markers (hits are the word "placeholder" in
`$ARGUMENTS` comments and test stubs). The inventory below comes from BACKLOG.md
and from reference counting (`grep -rn <symbol> --include=*.go` excluding tests).

| # | Item | Evidence | State | Recommendation |
| --- | --- | --- | --- | --- |
| H-01 | Streamable HTTP MCP | `internal/mcp/http_trust.go` (1113 lines): validator, redaction, `BindRuntime`; no transport, no config field; production refs to `ValidateRemoteHTTPTrust` = 1 (definition), test refs = 30 | designed, not built | **Document** (already in `docs/mcp-http-trust-contract.md`, `docs/feature-mcp.md:36-46`). Keep; `RedactSensitiveText` is live via `/mcp logs`. |
| H-02 | Packet Computers `managed` kind | `internal/computers/computer.go:20` accepted by `ValidKind`; `Reachable()` false; nothing provisions | stub kind | **Document** in `docs/feature-packet-computers.md`; consider rejecting `managed` in `normalize` until a provisioner exists (fail closed: 3 lines). Not done: would break any registry file that already carries it. |
| H-03 | `DaemonVersion` / heartbeat | `computer.go:180,235`; PCMP4/PCMP5 queued (`docs/packet-computers-loop.md:96-97`) | field never written | **Document** (already: status "stored, not probed"). |
| H-04 | Local computers registered but unusable | `/computers register <name> <root>` creates `KindLocal`; `--computer` rejects non-SSH (`main.go:215-217`); `resolveWorkspace` rejects non-SSH (`main.go:348-350`) | half | **Document**: say `register` is a record only; or remove the verb. |
| H-05 | Background "question" feature | `Snapshot.AwaitingAnswer` (`job.go:305`) now correct after P05 but nothing sets `NeedsInput` alone; Agent View already renders the `»` case (`agentview.go:619-626`) | UI ready, no producer | **Document** as planned (BACKLOG). |
| H-06 | ACP command catalogue excludes skills | `cmd/packetcode/acp.go:399-429` passes `nil` skills | deliberate | **Document** (done in `docs/advanced-guide.md:540-542`). |
| H-07 | SSH teardown unconfirmed | `ssh_backend.go:545-572` returns `KillMethodNone` | honest limit | **Document** (done). |
| H-08 | Undo stack in memory; `BackupManager.Cleanup()` has no production caller | `session/backup.go:19-21,221-232`; grep `Cleanup()` non-test refs = 0; backups never pruned | half | **Finish later**: age-based prune at startup (BACKLOG); `Cleanup()` could be removed now (`CleanupSession` is the live path, `slashcmd_sessions.go`). Not done in this pass: prune policy is a product choice. |
| H-09 | Test-only wrappers | `app.ParseSlashCommand` (`slashcmd.go:15`), `app.ParseSpawnFlags` (`:64`), `jobs.Manager.DrainResults` (`manager.go:752`), `jobs.Manager.buildJobToolRegistry` (`registry.go:69`), `jobs.loadOrphaned` (`persistence.go:402`), `jobs.appendWorktreeArtifacts` (`worktree.go:318`), `app.renderHelp()` (`slashcmd_help.go:79`): each has only its definition in production and 1-47 test references | dead in production | **Rip out later** by moving into `_test.go` files or rewriting the tests against the live entry points. Not done: 80+ test references; a refactor, not a fix. |
| H-10 | Two path-confinement implementations | `internal/tools/safefs.go`: `resolveInRoot` and `resolveWritePath` have only test callers; `resolveExistingInRoot` is used by `code_intelligence.go:403,635,727,891` | partial duplicate | **Finish**: route code intelligence through `LocalBackend.Resolve` and delete `safefs.go` (BACKLOG item). Not done: changes a security boundary; needs its own review. |
| H-11 | `tools.atomicWrite` | 0 callers | dead | **Ripped out** (P11). |
| H-12 | `provider.Registry.InitializeAll` | `registry.go:170-203`; 0 production callers, 1 test | dead | Remove or keep as API; harmless. |
| H-13 | `internal/doctor/` empty directory | BACKLOG names it; absent in this checkout (git does not track empty directories) | stale note | **Document**: delete the BACKLOG line, or `rmdir` it in the primary checkout. |
| H-14 | Conduit shadow runtime | `internal/agent/conduit_shadow.go`, `sugar/runtime.go`; off by default; endpoint 404 disables itself | complete, opt-in | Nothing to do. |
| H-15 | Remote project workflow discovery | deferred (`docs/manual.md:513`) | documented | Nothing to do. |
| H-16 | `ProfileFull` bypass of `fetch` | by design (`policy.go:459-460`) | complete | Documented. |

---

## 6. Rollback notes

Each patch is one commit; `git revert <sha>` restores the previous behaviour.
Specifics:

- **P01** `dd71133`: revert returns the key to the URL. No config or data change.
  If Google ever stops accepting `x-goog-api-key` (it is the documented
  alternative), reverting restores the query form.
- **P02** `6fece2e`: revert makes `.env` readable again by tools and mentions.
  No data change. If a user needs the agent to read a `.env`-named file, the
  interim workaround without reverting is `execute_command cat .env`, which is
  approval-gated and visible.
- **P03** `d4f820c`: revert restores symlink-following mentions. Independent of
  P02 (different hunks in the same file; `git revert` of either applies cleanly,
  verified by construction: P02 edits `readMention` head and imports, P03 edits
  its tail).
- **P05** `571fa98`: revert restores `NeedsInput == NeedsApproval`. Persisted job
  records carry both fields; either build reads either shape. Agent View
  reverts to drawing `»` for approvals.
- **P06** `f82868e`: revert restores the unsynced write. Tally format unchanged.
- **P12** `f04688d`: revert restores the `full` ceiling for `""` and custom
  profiles. An ACP client that was relying on requesting `auto`/`bypass` over a
  custom profile regains it. `permissionModes` advertised at `initialize`
  change accordingly, so a client caches nothing wrong.
- **P08** `195060b`: revert removes the startup warnings and the
  `config.validation` doctor check. Doctor JSON consumers filtering by id are
  unaffected either way (additive check).
- **P07** `cea8ef7`: revert removes the log. `PACKETCODE_LOG_FILE` is then
  ignored. Any log file already written stays on disk (0600) and should be
  deleted by the operator if no longer wanted.
- **P10a** `d61a919`: `git revert` restores `x/crypto v0.41.0`; run `go mod tidy`
  afterwards only if `go build` complains (it should not).
- **P10b** (file): apply with `git apply docs/audit/patches/P10b-*.patch`; revert
  with `git checkout -- go.mod go.sum`. Raises the module floor to Go 1.26.0;
  contributors on Go 1.24/1.25 cannot build until they upgrade. README and
  HANDOFF state "Go 1.24.2 or newer" and would need editing.
- **P11** `7aa0951`: revert restores the dead helper.

Batching suggestion: batch 1 = P01, P06, P11 (no behaviour change visible to
users); batch 2 = P02, P03, P05, P12 (behaviour changes, each with tests);
batch 3 = P08, P07 (new output on stderr and an optional log file); batch 4 =
P10a, then decide P10b.

---

## 7. Unresolved

| ID | Question | Evidence that would settle it |
| --- | --- | --- |
| U1 | Is `packetcode acp` ever launched such that a different OS user or an untrusted process can write its stdin? (Decides whether F-09 is medium or high.) | PacketADE's launch code for packetcode (`D:\projects\PacketADE`): does it spawn the process itself and own both pipe ends? Any systemd/service unit or socket wrapper around `packetcode acp` anywhere. |
| U2 | Does any real deployment run a custom OpenAI-compatible provider over plain `http` to a non-loopback host? (Decides whether F-10 can be made a hard refusal.) | `grep -n 'base_url = "http://' ~/.packetcode/config.toml` on every machine that uses packetcode; `packetcode doctor --check providers` output on each. |
| U3 | Will the Go toolchain floor move to 1.26? (Decides P10b and the remaining 9 stdlib advisories.) | A decision. CI already runs Go 1.26.3 (`.github/workflows/ci.yml:13`); README says 1.24.2. If yes: apply P10b, set `GO_VERSION: '1.26.6'` in both workflows, edit README and HANDOFF. |
| U4 | Does anyone rely on `read_file .env` or `@.env` working? (Behaviour change in P02.) | Search session transcripts for `"path":".env"` under `~/.packetcode/sessions/`; ask users. |
| U5 | Does any ACP client request `permissionMode` above `ask` while the operator uses a custom profile? (Behaviour change in P12.) | PacketADE's permission control code: which modes it offers and whether it reads `_packetcode.permissionModes` from `initialize` (if it does, it adapts automatically). |
| U6 | Is `TestRunJob_PassesHooksToBackgroundAgent` flaky on this machine only? It failed twice (20 s wait; PowerShell hook has a 2 s budget) and then passed in 0.46 s; every audit commit passed in a clean throwaway worktree. | `go test ./internal/jobs/ -run TestRunJob_PassesHooksToBackgroundAgent -count=20`; time `powershell -NoProfile -Command "Write-Output x"` cold. If startup exceeds 2 s intermittently, raise the hook `TimeoutSec` in that test to 10. |
| U7 | Does the Sugar service enforce anything about the `client_name` or token lifetime that packetcode should surface? | Sugar's server-side repository or API docs; outside this tree. |
| U8 | Should project markdown commands and workflows be labelled untrusted like skills (F-08)? | A product decision. Cost: transcript shows a labelled block instead of "your" message; workflow `system_prompt` from a repo would need a confirm. |
| U9 | Is `internal/doctor/` present in the primary checkout? | `Test-Path D:\projects\packetcode\internal\doctor`. If yes, `rmdir` it and drop the BACKLOG line. |
| U10 | Should `collect_agent_results` prompt in the foreground (F-11)? `RequiresApproval` says yes; `readOnlyTool` says no. | A decision; the fix is one line either way (`policy.go:481` or `collect_agent_results.go:35`). |

---

## 8. Hardening for the dark period

### 8.1 Error messages

Checked every path that renders an error to the user or transcript: provider
error bodies are capped at 64 KiB (`internal/provider/errors.go:8-21`) and
passed through `extractAPIErrorMessage`; transport errors were the only place a
secret could appear (Gemini URL) and P01 removes it; `doctor` redacts URL
userinfo and secret-shaped arguments (`doctor.go:837-908`). No change needed
beyond P01. Every new message added in P02 and P08 names the setting or file
and the action to take.

### 8.2 Structured logging on the auth path and external calls

P07. Enable with an absolute path:

```bash
PACKETCODE_LOG_FILE=/home/you/.packetcode/logs/packetcode.jsonl packetcode
```

```powershell
$env:PACKETCODE_LOG_FILE = "$env:USERPROFILE\.packetcode\logs\packetcode.jsonl"; packetcode
```

One JSON object per line. Event names: `startup`, `provider.http`,
`codex.token_refresh`, `policy.decision`, `approval`, `acp.session_new`,
`acp.session_load`, `acp.session_close`, `acp.permission`, `mcp.spawn`,
`fetch`, `ssh.connect`, `hook.run`, `sugar.login`. Level `WARN` marks transport
errors, 4xx/5xx statuses, failed refreshes, failed handshakes, failed hooks.
The file is 0600 and append-only; rotate it yourself (nothing in packetcode
does). Never contains bodies, headers, arguments, prompts, or query strings.

### 8.3 Config validation at boot

P08. The TUI prints `packetcode: config <problem>` lines after first-run setup;
`packetcode run` and `packetcode acp` print them before doing anything;
`packetcode doctor --check config` shows the same list as `config.validation`.
Each missing variable is named, for example:

```text
packetcode: config [mcp.github] env_from names GITHUB_TOKEN, which is not set in this environment; the server starts without it
packetcode: config [default] provider openai has no API key: set PACKETCODE_OPENAI_API_KEY (environment or .env) or api_key under [providers.openai]
```

### 8.4 Health endpoint

There is no HTTP server to put one on, and the brief's own rule ("do not add a
health endpoint if the architecture has none") applies. The health surface is:

```bash
packetcode doctor --check version,config,permissions --json
```

Exit code 0 = ok or warnings, 1 = at least one `fail` (`doctor.go:110-113`).
`doctor` executes no hooks, MCP servers, or providers (`doctor.go:984-987`), so
it is safe to run from a cron job. `packetcode --version` exits 0 and prints
`packetcode <version> (<commit>)`.

### 8.5 Dependency snapshot

> **Superseded for the `x/*` rows and the advisory table below.** `x/crypto`
> is now v0.56.0, `x/sys` v0.47.0, `x/text` v0.41.0, and the module floor is
> `go 1.26.0`; govulncheck reports no reachable module advisories. The
> upgrade notes on the other rows still hold. See the Closeout section.

`go list -m all` (direct requirements from `go.mod`, versions after P10a):

| Module | Version | Note |
| --- | --- | --- |
| github.com/BurntSushi/toml | v1.6.0 | **do not upgrade blindly**: `internal/config/tomlpatch.go` and `MetaData.Undecoded` depend on decoder behaviour; run `internal/config` tests after any bump |
| github.com/charmbracelet/bubbletea | v1.3.10 | **do not upgrade to v2**: breaking; the migration is its own BACKLOG item with a golden-file harness |
| github.com/charmbracelet/lipgloss | v1.1.0 | **do not upgrade to v2** (same reason) |
| github.com/charmbracelet/bubbles | v1.0.0 | **do not upgrade to v2** (same reason) |
| github.com/charmbracelet/x/ansi, x/term | v0.11.6, v0.2.2 | pinned by bubbletea v1 compatibility |
| github.com/mattn/go-runewidth | v0.0.19 | **do not upgrade without re-running `make tui-golden-check`**: width tables change cell goldens |
| github.com/google/uuid | v1.6.0 | safe |
| github.com/pkg/sftp | v1.13.10 | patch bumps safe; check `internal/computers` tests |
| github.com/pmezard/go-difflib | v1.0.0 | safe |
| github.com/stretchr/testify | v1.11.1 | test only |
| golang.org/x/crypto | v0.43.0 (P10a) | v0.56.0 needs Go 1.26 (P10b) |
| golang.org/x/sys | v0.38.0 | safe; P10b moves it to v0.47.0 |
| golang.org/x/text (indirect) | v0.30.0 | safe |

Reachable advisories after P10a (`govulncheck ./...`, 16, was 17):

| Advisory | Component | Fixed in | Reached via |
| --- | --- | --- | --- |
| GO-2026-6354, GO-2026-6355 | x/crypto/ssh (deadlock DoS) | v0.56.0 (go 1.26) | `computers.NewSSHBackend` |
| GO-2026-5013, 5017, 5018, 5019, 5020 | x/crypto/ssh | v0.52.0 (go 1.25) | same |
| GO-2026-4971 (net), GO-2026-4918 (net/http) | stdlib | go1.26.3 | ssh dial; every provider `http.Client.Do` |
| GO-2026-5037 (crypto/x509), GO-2026-5039 (net/textproto) | stdlib | go1.26.4 | TLS to providers |
| GO-2026-5856 (crypto/tls) | stdlib | go1.26.5 | same |
| GO-2026-5026 (net/http), GO-2026-5972 (encoding/asn1), GO-2026-6090 (crypto/tls), GO-2026-6218 (net/url) | stdlib | go1.26.6 | same |

Fixed by P10a: GO-2025-4116 (x/crypto/ssh/agent DoS).

All stdlib items are fixed by building with Go 1.26.6. CI currently uses 1.26.3
(`.github/workflows/ci.yml:13`, `release.yml:19`); the local machine has 1.26.2.
Recommended, not applied (cannot be verified from here): change both workflow
`GO_VERSION` values to `'1.26.6'` and install Go 1.26.6 locally. Every provider
is exposed to a hostile server through these stdlib issues; the SSH items need
a hostile or compromised Packet Computer.

### 8.6 What to watch for after applying

- Startup now prints `config` lines for settings that were silently inert. A
  sudden new line after a deploy is a config regression, not a code one.
- If `.env` reads were relied on (U4), the model will report
  `read_file: refusing to read .env ...`; that is the intended behaviour.
- ACP clients on a custom profile may see fewer `permissionModes` (U5).
- Gemini users: if any request fails with an auth error after P01, the
  provider has stopped accepting the header; revert P01 (see section 6).


# Upstream adoption plan: what packetcode should take from opencode and crush

Compiled 2026-08-14 from the two research documents, then cross-verified against
the packetcode tree at `25a13ac` (plus working-tree changes) by four parallel
analyses covering providers/cost, tools/execution, agent-loop/daemon, and
extensibility/UX.

- [`upstream-opencode.md`](upstream-opencode.md) — MIT, TypeScript/Effect, server-first
- [`upstream-crush.md`](upstream-crush.md) — FSL-1.1-MIT, Go, same TUI stack as packetcode

**Every recommendation was checked against the actual code before it was
written.** That pass changed the plan substantially: it found three live defects
that outrank every adoption item, killed several recommendations outright
(including both library imports the research docs were most enthusiastic about),
and shrank others. Items that turned out to be already built are recorded in
[§3](#3-already-built--corrections-to-the-research-docs) rather than silently
dropped.

---

## 1. The rule this plan is built on

**Implement from these documents, with the upstream source closed.**

The research docs are written as specifications, not pointers — mechanisms,
constants, failure modes, and rationale in their own words. That is what makes
them safe to build from. If a detail is missing when you go to implement, add it
to the research doc in your own words first, then implement from the doc. Full
licensing position in [§8](#8-licensing-position).

---

## 2. Wave 0 — live defects found during verification

**Status: fixed 2026-08-14, with regression tests.** None of these were adoption
items — they are bugs, and they outranked everything below. All three were found
by reading packetcode's code against the upstream designs, which is the
highest-value thing this research produced. The descriptions below are kept as
the record of what was wrong and why the fix takes the shape it does.

### 2.1 The deny floor fails open on any shell metacharacter

`commandPrefixMatches` (`internal/permissions/policy.go:448-473`) returns `false`
when a command contains any of `; & | < >`, a backtick, `$()`, or a newline. The
inline comment explains why: a prefix rule must "never authorize a larger shell
program." Correct reasoning **for allow rules**. But `matchingRule`
(`policy.go:262-276`) runs **deny-floor rules through the same predicate**, so a
non-match means *not denied*, and execution falls through to the profile default.

Probed against the real package (profile `auto`, deny rule
`{tool=execute_command, command_prefix=["git","push"]}`):

```
"git push origin main"          -> deny
"git push origin main; :"       -> allow
"true && git push origin main"  -> allow
"sh -c 'git push origin main'"  -> allow
"./deploy.sh"                   -> allow
```

One metacharacter defeats the deny floor. This undercuts a stated BACKLOG
guarantee:

> Keep Bypass Permissions explicit, visible, outside the normal Shift+Tab cycle,
> and subordinate to deny rules.

**Fixed** in `internal/permissions/policy.go` by splitting the predicate by
direction. Allow-direction matching keeps the metacharacter refusal unchanged
(and non-deny rules keep their existing behaviour exactly). Deny-direction
matching now:

- **splits a compound command into its simple commands** (`splitSimpleCommands`)
  and checks each one, so `true && git push` and `echo $(git push)` match a
  `git push` deny rule, while a redirection *target* (`> /tmp/out`) is correctly
  not treated as a command;
- strips leading `NAME=value` words, so `GIT_SSH_COMMAND=… git push` still matches;
- returns **indeterminate** rather than "no match" when a stage hands its
  arguments to something the policy cannot see through — an interpreter
  (`sh -c …`, `xargs`, `env`, `sudo`, …) or a script path (`./deploy.sh`). An
  indeterminate deny floor escalates an `allow` to `ask`; it never loosens an
  existing `ask` or `deny`.

Deliberately unchanged: a *provably unrelated* compound command
(`ls -la | wc -l`) still runs without a prompt, so failing closed does not turn
into prompt noise. Regression tests pin all five original probe cases plus
escalation and non-escalation. **Shipped.**

**Adjacent, not fixed:** the same allow-direction refusal applies to `ask`
prefix rules, where `git status; :` falls through to allow under a permissive
profile. That behaviour is currently pinned by
`TestPolicy_CommandPrefixMatchesFields`, so changing it is a deliberate decision
rather than a bug fix — but it is the same shape of weakening, one severity
level down.

Two consequences for the rest of this plan: packetcode's deny surface is weaker
than either research doc assumes (there is no substring or regex command deny at
all, only exact `command` and `command_prefix`); and crush's "deny rules recurse
into sourced scripts" improvement ([§6.5](#65-in-process-posix-shell--last-not-first))
is worth nothing until this lands, because recursion into a predicate that
already fails open buys nothing.

### 2.2 Unscoped state directories let one project clobber another's running jobs

Every state directory is global with no project component —
`~/.packetcode/{sessions,backups,jobs,worktrees}` (`config/paths.go:66-117`) —
and there is no lock file anywhere in the tree (`grep flock|LOCK_EX|\.lock` →
zero hits). `jobs.NewManager` (`manager.go:264`) unconditionally calls
`loadPersistedJobs`, which rewrites **every** on-disk job in `queued`/`running`
to `Cancelled` + `Recovered`, reason `"previous app exit"`
(`persistence.go:290-310`).

So: start packetcode in project B while an instance in project A has a running
job, and B marks A's live job abandoned and offers it under
`RecoveredResubmittable()`. `Resubmit` only checks `Recovered`,
`ResubmittedAs == ""`, and `State.IsTerminal()` (`resubmit.go:36-57`) — all
satisfied — so **`/jobs resubmit` can launch a duplicate of a job that is still
running**, including a write-enabled one with its own worktree. Meanwhile the
on-disk record flaps: B rewrites at the same `Seq`, and A's next snapshot at
`Seq+1` wins the guard at `persistence.go:209`.

**Fixed (the cheap v1).** `Job` and `persistedJob` gained an `OwnerRoot`, set
from the manager's project root at spawn. `loadPersistedJobs` now takes the
caller's root and **leaves another root's queued/running records strictly
alone** — not rewritten, not loaded, not offered for resubmit. Records written
before ownership tracking carry no root and are still recovered, so upgrading
strands nothing. An empty root disables scoping, which keeps the existing test
helpers meaningful. **Shipped.**

**Still open:** the advisory lock itself — a file under `~/.packetcode/` holding
pid, start time, and workspace root, so two instances rooted at the *same*
project can also be told apart. The owner-root fix closes the duplicate-launch
hole across projects, which is the reachable bug; the lock closes the
same-project case and is the piece PCMP4 would need if a daemon ever shares the
directory.

**This is a hard precondition for PCMP4.** A daemon introduces a second process
legitimately touching the same job directory, and PCMP9's honesty contract
collapses if "no live owner" cannot be distinguished from "another owner."

### 2.3 Job records are unversioned and vanish silently when unreadable

`persistedJob` (`persistence.go:17-56`) has **no version field** — the only
persisted structure in the codebase without one. `computers/registry.go:18,58-61`
has `registryVersion` and refuses future versions loudly; `session.Session` has
`FormatVersion: 2` with a migrator; workflow TOML and verdict payloads are both
versioned and reject unknown versions.

Worse, `loadPersistedJobs` silently `continue`s on both `ReadFile` and
`Unmarshal` failure (`persistence.go:283-289`). A corrupt or future-versioned job
file **disappears from `/jobs` with no message** — precisely what the PCH4
honesty rule forbids: abandoned work reported as nothing at all.

**Fixed.** `persistedJob` gained `format_version` (records predating it decode
as 0 and stay readable). Unreadable records are now returned as typed
`UnreadableRecord{Path, Reason}` values and exposed via
`Manager.UnreadableRecords()` instead of being skipped, covering four cases:
read failure, malformed JSON, a version newer than this build supports, and an
**unrecognised state** — `parseKnownState` no longer flattens an unknown state
to `failed`. The write path also refuses to overwrite a record whose
`format_version` is newer than this build, so an older binary cannot silently
downgrade a newer one's state. **Shipped.**

This closes the jobs half of the BACKLOG compatibility item, and it was the
prerequisite for the `abandoned` state in
[§7.2](#72-abandoned--indeterminate-terminal-state): a new state written by a
newer binary is now reported rather than silently swallowed.

**Remaining wiring:** `UnreadableRecords()` is populated but nothing surfaces it
yet. The App should report the count at startup alongside the recovered count,
and `doctor` should list them.

---

## 3. Already built — corrections to the research docs

Verification found packetcode at or ahead of the upstreams in eleven places.
These narrow the plan; several reverse a recommendation outright.

| Area | Reality in packetcode | Effect on the plan |
|---|---|---|
| **`multiedit`** | **Already shipped.** `patch_file` takes an ordered `patches: []{search,replace}` array applied in sequence with per-op ambiguity errors (`patch_file.go:76-79, 107-151`), plus an exact-first / normalized-whitespace fallback that preserves original EOL bytes. Better than crush's edit tool. | Dropped. Only *multi-file* in one call is missing, and that is low value |
| **Hooks** | Three events (`UserPromptSubmit`, `PreToolUse`, `PostToolUse`); `PreToolUse` already blocks (non-zero exit → tool-role rejection, `agent.go:415-428`); stdout already injected as context. Sequential, config order, per-hook timeouts | crush has *fewer* events, not more. Delta shrinks to rewrite-input + auto-approve — plus a cheaper gap: the matcher is exact-name-or-`*` (`hooks.go:189`), not regex |
| **Prompt cache** | Anthropic cache breakpoints already ship (`anthropic.go:287,345,363`), with cache tokens parsed at `:419-473,568-571`. `internal/provider/cache.go` is a *Sugar* prefix fingerprinter over canonicalized tool definitions — related but not the same thing | More rigorous than either upstream. opencode's `cache-policy.ts` pointer is mis-mapped at `cache.go`; do not re-recommend either |
| **Skill guidance deltas** | Fingerprint inputs are `(a.systemPrompt, req.Tools)` with `StablePrefixMessages` counting one system message (`agent.go:250-263`). A startup-loaded skill set never changes mid-session | The delta protocol would be **dead code**. Build the static block |
| **Pricing tables** | **Seven** hand-maintained `pricing.go`, not twelve. `openrouter` already fetches live pricing/context/tool-support from `/models`; `codex` reads the CLI's `models_cache.json`; `ollama`/`sugar` are $0; `custom` is config-driven | Real, but ~40% smaller than stated |
| **Ollama enrichment** | `/api/show`, `context_length` from `model_info`, `detectToolSupport`, all memoized (`ollama.go:242-335`) | crush's enrichment idea is already here. The gap is *other runtimes* |
| **Approver seam** | `jobApprover` → `uiApprover.PromptApproval` already lets a background job block on a foreground prompt, with snapshot-bound policy so a later foreground widening cannot broaden a running job (`app/approver.go:221-229`) | Both research docs undersell this. The question tool is a smaller change than described — see [§4.7](#47-question-tool-on-its-own-lane) |
| **Ripgrep** | `search_codebase` uses system `rg` when on PATH, Go `WalkDir` otherwise (`search_codebase.go:89-96`) | Conclusion (don't bundle) stands; the premise "already has it in Go" was imprecise |
| **FS confinement** | Two implementations: `internal/tools/safefs.go` (largely superseded) and `internal/computers/local_backend.go:49-135` (**the live path** — every core tool goes through `RuntimeBackend.Resolve`) | Target `LocalBackend`, not `safefs.go`. Two parallel confinement implementations is its own cleanup ticket |
| **Sessions/jobs** | `FormatVersion: 2` + migration, bounded immutable tool-result projection, `Seq`-guarded debounced persistence, eager per-job ctx, panic→`StateFailed`, recover-guarded fan-out | Only *jobs* lack versioning ([§2.3](#23-job-records-are-unversioned-and-vanish-silently-when-unreadable)) |
| **Live-provider tests** | An env-gated pattern already exists (`internal/provider/codex/live_test.go`, `CODEX_LIVE=1`) | Cassettes extend a pattern rather than introducing one |

Also confirmed and not revisited: `StallGuard`, retry with jitter + `Retry-After`,
bounded provider-error bodies, a security-tested OAuth device-grant client
(`cmd/packetcode/sugar_login.go`), git worktree isolation, `doctor` with
`--json` + redaction, five permission modes with bypass outside the cycle.

---

## 4. Wave 1 — small, high value, no new dependencies

### 4.1 Loop detection

**From:** crush. **Lands in:** new `internal/agent/loopdetect.go`, called from
`Agent.oneTurn` after `handleToolCall` completes for all calls
(`agent.go:357-362`), with state owned by the `run` invocation — **not** by
`callAssembler`.

Sliding window of the last 10 turns; per turn, SHA-256 over the sorted
`(call.Name, executed arguments, res.Content)` triples; more than 5 occurrences
of one signature in the window aborts the run. Turns with no tool calls are
skipped.

Two details that matter: sign the **executed** arguments (`agent.go:478-479`,
post-approval-edit) and the authoritative `res.Content` handed to the model, not
the live chunk stream. And include the *output* — identical calls with differing
output are progress; identical calls with identical output are not.

Today the only guard is `maxToolIterations = 25` (`agent.go:31`), whose own
comment names the exact failure it is guarding against: *"retrying read_file on a
path that keeps not existing."* Loop detection aborts around iteration 6–11
instead of 25, and says *why* — which `exceeded N tool iterations` does not. For
background jobs the reason lands directly in `Job.Error`.

**Effort: S.** **Risk: very low.** Mitigate legitimately-repeated idempotent
calls (polling a file during a build) via the identical-output requirement plus a
config-visible threshold. **Do this first among adoption items.**

### 4.2 Cached-input token plumbing

**Closes:** *"Add explicit cache-hit/cached-input telemetry to `/cost` and
statusline snapshots."*

`provider.Usage` already carries `CacheCreationInputTokens` and
`CacheReadInputTokens` — but **nothing else in the chain does.** The wire is
populated at one end, cut at four points, and rendered as a hard-coded zero at
the other:

- `openaicompat/client.go:433` — `chatUsage` is `{prompt_tokens, completion_tokens}`
  only; no `prompt_tokens_details.cached_tokens`. This one struct blinds **eight**
  providers.
- `gemini/gemini.go:330-334` — no `cachedContentTokenCount`.
- `session/session.go:59-71` — `TokenUsage` is `{TotalInput, TotalOutput,
  ContextTokens}`; `UpdateUsage` (`:266`) discards cache fields *even for
  Anthropic*, which supplies them.
- `cost/tally.go:39-44,131` — `SessionCost` and `RecordUsage` take input/output only.
- `statusline/statusline.go:104-118` — the `cache_creation_input_tokens` /
  `cache_read_input_tokens` JSON slots exist and are hard-coded to zero, with a
  comment admitting it.

The shape is "widen a struct through three layers," not "add a column" — three
of the five files are struct or signature changes, and `RecordUsage`'s signature
`(sessionID, slug, modelID, input, output int)` has to widen. But the *parsing*
gap is concentrated rather than per-provider: one struct (`chatUsage`) covers
eight providers, and Gemini is the only separate fix.

**Effort: S.** **Risk: low** — additive JSON fields decode as zero on old files.
**The one thing to get right:** `provider.Usage` documents `InputTokens` as
*including* cached input (`types.go:189-195`), and OpenAI-compatible
`prompt_tokens` does too. Cached figures are a reported **subset, never an
addend** — assert that per provider in tests.

### 4.3 Tool-output truncation store

**Closes:** *"Add configurable model-facing caps for search, command, MCP, and
artifact output."*

`execute_command` is the **only** tool with a byte cap (100 KB,
`execute_command.go:22`). `search_codebase` caps at 500 *matches* with no byte
bound; `list_directory` caps at 2000 *entries*; **MCP tool results are entirely
uncapped** (`internal/mcp/tool.go` `Execute` joins every text item), so a server
returning 5 MB lands whole in the transcript.

There is exactly one chokepoint: `agent.go:508-513`, where `res.Content` becomes
a `provider.RoleTool` message. Cap there. New `internal/toolstore` writes full
output to `<home>/tool-output/<session>/<callID>`, hands the model a head+tail
excerpt plus an explicit notice and a handle, and adds a read-only
`read_tool_output{handle, start_line, end_line}`. Reuse the prune shape already
in `internal/jobs/artifacts.go`.

**Effort: S.** **Risk: low.** Failure mode: a handle referenced after prune must
degrade to "output no longer retained", never an error that stalls a turn.

### 4.4 Backup pruning — a bug fix, not a feature

`internal/session/backup.go` copies the whole file on every write into
`~/.packetcode/backups/<session>/`. The undo stack is **in-memory only**
(comment at `:19-21`), so `.bak` files orphan on restart. `Cleanup()` has **no
production caller**; `CleanupSession` is reachable only from `/sessions delete`.
Background jobs get their own manager (`jobs/worker.go:118`) whose backups are
never cleaned and never reachable from `/undo`.

Age-based prune on startup, a per-session byte cap, and an actual `Cleanup()`
call. ~100 lines. **Effort: S.** Do not let this wait on the snapshot design in
[§6.2](#62-git-shadow-repo-snapshots).

### 4.5 Stale-write protection

No equivalent exists — no read-time recording, no mtime/hash check before write.
`write_file.Execute` (`write_file.go:76-88`) resolves, backs up, and overwrites
without comparing against what the model last saw. `patch_file` is protected by
accident (its search text won't match), but `write_file` silently clobbers a
concurrent formatter, a `git rebase`, or a second agent in the same worktree.

Record `(path, mtime, size, sha256)` on every read, keyed by session; refuse
`write_file` with "file changed on disk since you last read it; re-read first."
Key it off the existing session store — **do not add SQLite.** Must be
per-`RuntimeBackend`; the correct v1 enables it for `KindLocal` only and records
why it is skipped on `KindSSH`. Bonus: a factual "files touched this session"
list for Agent View. **Effort: S–M.** **Risk: low.**

### 4.6 Todo tool

Confirmed absent. Cheap, improves steering on long turns, gives Agent View
structured content for background jobs. Two local constraints: the system prompt
says *"Don't narrate a long plan before acting on a simple task"*
(`main.go:56-65`), so it must render as a compact TUI block and never be echoed
as prose; and under the inline-scrollback/no-mouse contract it must **re-emit
rather than mutate a fixed region** — the pattern `internal/ui/components/jobs`
already uses. **Effort: S.**

### 4.7 Question tool, on its own lane

**Closes:** *"Let background agents request user clarification through Agent
View."* Both upstreams converged on this independently.

Structured question: prompt, labelled options, `multiple` for multi-select, and
an auto-appended "type your own answer" so the model never invents an "Other".
The answer returns as a normal tool result.

Routing is **half-done already** — `jobApprover.Approve` forwards to
`uiApprover.PromptApproval` (`jobs/approver.go:57-60`), so a background job can
block on a foreground prompt today. Four real obstacles remain:

1. `jobApprover.Approve` (`approver.go:35-60`) rejects *everything* for read-only
   jobs before reaching the parent, so a question from a read-only job returns
   "background job is read-only". Needs the explicit pass-through `spawn_agent`
   already has at lines 36-41.
2. `uiApprover` renders exactly one `active` envelope at a time
   (`app/approver.go:30,152-182`) with `pendingCh` capped at 16. A long-lived
   question would head-of-line-block every approval behind it, from *all* jobs.
   **Give questions a second lane** — separate queue, separately renderable.
3. `Job.NeedsInput` exists in the model and `Snapshot` (`job.go:108,153`) but is
   **always set equal to `NeedsApproval`** (`worker.go:220-225`). Make it
   independently meaningful so Agent View can render "waiting on a question".
4. No timeout. A blocked question holds a semaphore slot out of
   `MaxConcurrent` (default 4), so four unanswered questions wedge all background
   work. Needs a deadline resolving to a tool-role "user did not answer" result —
   and a question that outlives the process must become `abandoned`
   ([§7.2](#72-abandoned--indeterminate-terminal-state)), never a silent cancel.

**Effort: M.** **Risk: low–medium.**

### 4.8 Skills

**Mechanism from opencode, content strategy from crush.**

**Not redundant with what exists.** Slash commands are human-typed and the model
cannot invoke them (`slash_registry.go:211`). Workflows are human-started
orchestration that spawns background jobs. Neither can be selected by the model
mid-turn. Skills add exactly three things: model-initiated retrieval, an index
costing ~20 tokens per *unused* skill instead of the full body, and composition
inside a turn rather than starting a new one.

**Where it lands:** not a new subsystem. `~/.packetcode/skills/<name>/SKILL.md`
plus `.packetcode/skills/`, parsed by the same frontmatter parser slash commands
use (`parseMarkdownCommandFile`, `validSlashCommandName` — reusable as-is), an
`<available_skills>` block appended to the system prompt
(`cmd/packetcode/main.go:51`), and one `skill` tool. Add `"skill"` to
`readOnlyTool()` (`policy.go:414`) so *loading* a body is not approval-gated —
the actions a body suggests stay individually gated, which is the correct
boundary.

**Ship embedded builtin skills for packetcode's own configuration**, from the
existing `docs/{configuration,hooks-and-statusline,workflows,mcp,feature-theming}.md`,
so "add a hook that blocks force-pushes" becomes something the agent does
correctly. There is no `go:embed` in the tree today; this introduces the first
(stdlib, fine). Write our own SKILL.md text — crush's bodies are FSL prompt text.

**Skip the delta protocol** ([§3](#3-already-built--corrections-to-the-research-docs)).
**Skip remote/URL discovery in v1.** **Trust:** a project-directory skill body is
attacker-controllable in a hostile repo — the same trust class as project slash
commands and workflows, which packetcode already accepts. State it; don't treat
it as new. **Effort: M.** **Risk: low.**

### 4.9 Self-diagnostic tool

`cmd/packetcode/doctor.go` is 863 lines and already emits a versioned structured
report (`doctorReport{SchemaVersion: 1}`, `--json`, per-section `--check`, secret
redaction at `:695-757`). Exposing `buildDoctorReport()` as a read-only,
no-approval tool is nearly free. Pair with a bounded read of the per-server MCP
log files `internal/mcp` already writes — the highest-frequency thing users ask
the agent to debug. Reuse the existing redaction path; the BACKLOG already asks
for redaction tests over exactly this surface. Note `internal/doctor/` is an
**empty directory** — the checks live in `cmd/`; this is the moment to move them.
**Effort: S.**

---

## 5. Wave 2 — the dependency decisions

This is where the research docs were most enthusiastic and verification was most
corrective. All three candidate libraries were checked against packetcode's
`go 1.24.2` — **and they give three different answers.** Do not generalise any
one of them into a blanket rule.

| Module | Import path | `go` directive | Transitive set | What you get | Verdict |
|---|---|---|---|---|---|
| catwalk | `charm.land/catwalk` | **1.26.6** ✗ | prometheus, protobuf, procfs, xxhash, perks, goautoneg | **70 lines** of stdlib HTTP + a struct, with worse data than models.dev | **Reject** |
| vcr | `charm.land/x/vcr` | 1.24 ✓ | go-cmp, yaml/v4, `dnaeon/go-vcr.v4` | A thin wrapper over an engine you could import directly | **Hand-roll** (preference, not teardown) |
| powernap | `github.com/charmbracelet/x/powernap` | 1.24 ✓ | `sourcegraph/jsonrpc2`, `mitchellh/mapstructure` | **~460 KB** of real asset | **Import** |

Note the module paths are asymmetric — vcr is `charm.land/x/vcr`, powernap is
`github.com/charmbracelet/x/powernap`. The research doc gets vcr's wrong.

**The scoping rule this establishes**, worth writing into `CLAUDE.md`:
*stdlib-only for LLM provider and MCP wire code, where hand-rolling is the
deliberate differentiator. That rule does not reach adjacent protocols with a
published spec and standard generated bindings (LSP), catalogs, or tests —
those are judged on ordinary dependency merits.*

### 5.1 Model catalog — models.dev via stdlib, not catwalk

**Closes:** *"Keep provider catalogs, pricing, context windows, and
tool-capability metadata current; prefer live discovery when authoritative."*

Both catalogs were fetched live and diffed against what packetcode needs. **Four
independent reasons catwalk is wrong here, none of them "we dislike
dependencies":**

1. **Toolchain.** `charm.land/catwalk`'s go.mod declares `go 1.26.6`; packetcode
   is `go 1.24.2`. Importing it forces a Go bump on every contributor and CI runner.
2. **Weight for near-zero code.** It requires `prometheus/client_golang` +
   `protobuf` + two `charmbracelet/x` packages. What `pkg/catwalk` actually gives
   you is `client.go`: **70 lines** of `http.NewRequestWithContext` +
   `json.NewDecoder`. That is stdlib packetcode already writes in every provider.
3. **No `mistral` provider.** packetcode ships one, so the hand table survives anyway.
4. **Its data is wrong for packetcode's default MiniMax model.**
   `catwalk.charm.sh/v2/providers` returns `MiniMax-M3` at `cost_per_1m_in: 0.6,
   cost_per_1m_out: 2.4` — the **long-context tier**, so every ordinary sub-512K
   request would bill at 2×. It also reports `can_reason: false` for M3. And
   `catwalk.Model` has no tiered-pricing field at all, so it **structurally
   cannot express** the thing the BACKLOG item asks for.

**models.dev carries exactly the missing data.** `https://models.dev/api.json`
(3.7 MB, 185 providers). For `MiniMax-M3`:

```json
"cost": { "input": 0.3, "output": 1.2, "cache_read": 0.06,
  "tiers": [ { "input": 0.6, "output": 2.4, "cache_read": 0.12,
               "tier": { "type": "context", "size": 512000 } } ] }
```

That is the BACKLOG's *"a request over 512K tokens bills entirely at the 2x
long-context tier"* and *"cached input reads are cheaper"*, as data, verbatim.
`gemini-2.5-pro` has the same shape at 200K. models.dev also covers `mistral` and
`github-copilot`, and correctly has no `ollama` entry.

**The tradeoff, stated honestly:** the real case *for* importing a catalog library
is that someone else maintains the schema and absorbs upstream churn. If catwalk's
data were correct and complete for packetcode's provider set, that would be worth
taking despite the stdlib preference. But catwalk's schema is the *less* capable
of the two, and the more capable source has no Go client to import. The choice is
"import a library with worse data and a toolchain bump" versus "write a 200-line
stdlib fetcher against better data."

**Build:** new `internal/catalog/` — `fetch.go` (stdlib, reusing
`provider.DoWithRetry`), `snapshot.go` with `//go:embed catalog.json` (nine
relevant providers trimmed to used fields measures **~150 KB**, ~50 KB excluding
openrouter, so the single-binary property is preserved and no runtime download is
required for correct behaviour), and `refresh.go` (background refresh at startup,
hard timeout, never blocks the TUI, with a `doctor` line reporting catalog age).

**Precedence:** user config `[providers.x.models]` > live catalog > embedded
snapshot > existing hand table — opencode's "local tables are overrides"
inverted, so the user still wins.

**Effort: M.** **Risk: medium.** Three failure modes: demoting the hand tables
silently changes displayed cost, so ship behind a flag for one release with a
test that diffs both tables; models.dev IDs won't always match provider-returned
IDs (Gemini's `models/` prefix is already normalized, but Ollama tags, `-latest`
pointers, and dated snapshots need a normalizer with tests); and models.dev is a
third-party project with its own terms — confirm before shipping, and never block
startup or degrade on fetch failure.

### 5.2 Tiered and cached-rate pricing

Depends on 4.2 and 5.1. `Tracker.priced` (`cost/tally.go:236`) is a flat two-rate
multiply and `PricingFunc` returns exactly `(inputPer1M, outputPer1M)`. Billing a
context tier needs per-request occupancy, and the tally deliberately stores only
high-water-mark totals — a session that crossed 512K once **cannot** be re-priced
from `{Input, Output}` alone. That is a design constraint, not an oversight.

The honest fix prices at *record* time: `RecordUsage` takes the per-request
`provider.Usage` and the tracker accumulates a USD delta per rate band while
keeping token totals for display. That reverses a deliberate design
(`tally.go:158-166`, whose comment explains why re-price-at-read-time exists), so
it should be an explicit decision, not something that slides in behind 5.1.

**Recommended interim:** keep read-time pricing, add cached-rate discounting only
(4.2 supplies the token split), defer context tiers, and **say so in `/cost`
output** rather than silently under-reporting. **Effort: M.**

### 5.3 Cassette-based provider contract tests

**Closes:** *"Add opt-in live-provider contract tests that never run in ordinary
CI"* and the MiniMax wire-shape item — the `<think>` path was *"implemented from
the published tool-use guide, not from an observed response."* Twelve hand-rolled
SSE parsers with no vendor SDK is exactly the codebase where a provider's wire
change breaks silently.

The single highest-risk piece of never-verified code in the tree is
`internal/provider/openaicompat/think.go` — 108 lines of hand-written parser for
a wire shape nobody has observed. That is exactly what a cassette pins. All 21
provider test files currently use `httptest` with hand-written fixture strings,
and `codex/live_test.go` already establishes the env-gate convention
(`CODEX_LIVE=1`) to extend rather than replace.

The design to copy is one rule: record on `PACKETCODE_RECORD=1`, replay always,
and **fail on a missing cassette when `CI=true`.** Without that rule cassettes
quietly re-record drift and assert nothing.

**Hand-roll it — but as a preference, not a teardown.** Unlike catwalk, vcr is
version-compatible (`go 1.24`) with a clean three-module transitive set, so the
toolchain objection does not transfer and this decision rests on value alone.
Three honest reasons to write it: it is a *wrapper*, not an engine (the engine is
`dnaeon/go-vcr.v4`, and importing the wrapper means inheriting a YAML-v4 pin for
someone else's matcher opinions); **the actual requirement is SSE frame
chronology**, which a generic HTTP recorder does not provide — every provider
here is a hand-rolled SSE or NDJSON parser, and the tests need frame-by-frame
replay with controllable inter-frame timing so `StallGuard`
(`provider/stream.go:58`) is exercised rather than bypassed; and it is ~250–300
LOC against a codebase that already writes this shape 21 times.

Counterweight, stated fairly: it is test-only, so it never links into the binary
and the single-binary rule does not defend the decision — the cost is go.mod
surface and supply-chain review area. **If the team would rather not own a
recorder, importing `gopkg.in/dnaeon/go-vcr.v4` directly is defensible**; take
the engine, not the wrapper.

**Effort: M.** **Risk: low overall, one sharp edge** — a key committed inside a
cassette is unrecoverable once pushed. **Allow-list** headers on write rather
than deny-listing, and add a test asserting the scrubber ran.

### 5.4 LSP client and post-edit diagnostics

Take **both halves, and prioritise the second.**

- **Explicit tools** (`lsp_definition`, `lsp_symbols`, `lsp_references`,
  `lsp_call_hierarchy`, `lsp_restart`) generalise `code_intelligence.go` beyond
  Go. Real, but the marginal gain is smaller than either research doc implies —
  that file is already AST-grade for Go (1,135 LOC: `list_symbols`,
  `find_definition`, `find_references`, `get_diagnostics`, all capped) and
  lexically adequate for ~a dozen other languages.
- **Automatic post-edit diagnostics appended to `write_file`/`patch_file`
  results** is the higher-value half by a wide margin and is cheap once a client
  exists. Today the model can break the build and only find out if it runs a
  command. Append at `agent.go:489-505` — the same seam `PostToolUse` already
  uses via `appendHookOutput`.

`internal/mcp/client.go` already proves the stdio JSON-RPC + framing + reaper +
timeout competence this needs.

**Skip `lsp_rename` / `lsp_replace_symbol` in v1.** The research docs called
these "strictly better primitives than `patch_file`"; verification disagrees.
LSP-driven mutation bypasses `patch_file`'s diff preview, its backup call, and
the approval renderer's `PreviewPatchDiff` seam (`jobs/registry.go:225-232`).
Routing a workspace-wide semantic rename through approval correctly is separate,
larger work.

**Constraints:** `auto_lsp` must be opt-in or fail-silent — "no server installed"
and "server crashed" degrade to exactly today's behaviour, never an error.
Lifecycle tied to workspace root; disabled for `KindSSH` (remote code
intelligence is explicitly still open in the BACKLOG). Keep `code_intelligence.go`
as the zero-dependency floor with LSP layered above.

**Dependency: import `github.com/charmbracelet/x/powernap`.** It passed the same
scrutiny catwalk failed, and comfortably: `go 1.24`, two pure-Go direct deps
(`sourcegraph/jsonrpc2`, `mitchellh/mapstructure`), no cgo, nothing else
transitive. What it actually contains, measured:

| Package | Size | Contents |
|---|---|---|
| `pkg/lsp/protocol` | **388 KB** | Generated LSP bindings (`tsprotocol.go` 279 KB, `tsjson.go` 89 KB) |
| `pkg/config` | 72 KB | `lsps.json` — a **372-server** table of configs, root markers, install commands |
| `pkg/lsp` | 45 KB | Client, sync, language detection |
| `pkg/transport` / `pkg/registry` | 19 KB | Connection, router, multi-server lifecycle |

That 372-entry table is a strictly better version of the "reusable asset"
opencode flags as `lsp/language.ts`. Reproducing 388 KB of generated protocol
types by hand is not stdlib discipline, it is declining the task — and the
stdlib rule is scoped to LLM provider wire code, where hand-rolling is the
differentiator. It does not reach LSP.

Two caveats to record now rather than discover later: `mitchellh/mapstructure` is
**archived upstream** (maintained fork is `go-viper/mapstructure`) — stable and
feature-complete, not a blocker, but a maintenance signal. And
`pkg/lsp/protocol` ships its **own nested LICENSE**, distinct from the repo-level
MIT — the bindings are gopls-derived, so almost certainly the Go Authors' BSD-3.
Read it and give it its own `THIRD-PARTY-NOTICES` entry; the repo-level MIT
notice will not cover it.

**Effort: M.** **Risk: medium.**

### 5.5 Local runtime discovery

Ollama enrichment is already done ([§3](#3-already-built--corrections-to-the-research-docs)),
and all three target runtimes are *already usable today* through
`internal/provider/custom/custom.go` with a configured base URL. **The missing
half is zero-config**: probing `localhost:1234` (LM Studio), `:8080`
(llama.cpp), `:4000` (LiteLLM) and registering what answers. New
`internal/discover/`, one prober per runtime returning a `custom.Config`, called
from `providerFactoriesFromConfig` (`cmd/packetcode/providers.go:48`), reusing
the shape of `ollama.fetchMeta`. Carry over crush's `/v1`-stripping detail —
enrichment endpoints live at the server root.

**The BACKLOG's own constraint is the trap:** a discovered llama.cpp server that
advertises tool support but mangles parallel tool calls is *worse* than no
discovery, because it silently degrades a working setup. Gate registration on a
real capability probe; default `SupportsTools` to **false** for anything
unverified (`ollama.go:335` already uses a conservative allow-list for exactly
this reason); put discovery behind an opt-in config key so a stray listener on
`:8080` cannot hijack a provider list; and enforce a short client timeout so a
dead endpoint cannot delay startup.

**Effort: M**, decomposing into one independently shippable file per runtime —
ship LM Studio first and stop if it doesn't earn its keep. **Risk: low.**

### 5.6 Catalog-driven reasoning and output controls

**Closes:** *"Add provider-specific output/reasoning controls to the model picker
when the upstream catalog exposes them."* Falls out of 5.1 nearly free —
models.dev carries `reasoning`, `reasoning_options`, and `limit.output` per
model, and the consuming interface already exists:
`provider.ReasoningEffortController` (`provider/provider.go:52`), implemented
today only by `codex` from the Codex CLI's own catalog. **Effort: S on top of 5.1.**

**Pull forward independently:** *"Map `/effort` onto MiniMax
`thinking.type=disabled` so thinking can be turned off for cheap turns"* needs no
catalog at all — a `ReasoningEffortController` implementation on
`minimax.Provider` plus one wire field. **S, independent of everything above**,
and it pairs naturally with 5.3: the cassette that verifies the `<think>` shape
is the same test run that verifies `thinking.type=disabled`.

---

## 6. Wave 3 — bigger bets

### 6.1 Background shell jobs

The most concrete user-facing gap in the tool surface. `execute_command` is
synchronous with a hard `maxExecTimeout = 10 * time.Minute`
(`execute_command.go:21`) — no way to start a dev server, watcher, or long build
and keep working. `spawn_agent`/`collect_agent_results` solve the *agent* case,
not the *process* case.

Compatible with the no-PTY contract: a detached process with piped stdout/stderr
is not a PTY. Needs `job_kill` scoped to jobs this session started, never
arbitrary PIDs. **Gated on the procrun group-handle work in
[§6.5](#65-in-process-posix-shell--last-not-first)** — a process that outlives
the turn is exactly where containment has to be correct. **Effort: M.**

### 6.2 Git shadow-repo snapshots

The case is stronger than the research doc made it: **deletions and renames
performed by `execute_command` are currently unrecoverable.** Only `write_file`
and `patch_file` call `Backup`, so `rm -rf` and `mv` leave no trace at all.

Separate `--git-dir` under `~/.packetcode/snapshot/<project-key>/`, `--work-tree`
at the project root, one commit per user message, with `core.autocrlf=false`,
`core.longpaths=true`, `core.symlinks=true`, `core.quotepath=false`. New
`internal/snapshot`, reusing the exec pattern proven in `jobs/worktree.go:233`
(`gitOutput`, 30s timeout, `GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=never`).
`internal/git/git.go` is 46 lines and read-only; it needs no change. No
`--git-dir`/`--work-tree` usage exists anywhere in the tree today.

**Effort: M.** **Risk: low–medium**, additive — keep per-file backup until proven.
Failure modes: not a git repo (degrade, don't error); huge working tree (needs
per-file size exclusion); and **snapshot the job worktree, not the parent**, or a
job commit captures unrelated foreground edits. Sequence after
[§4.4](#44-backup-pruning--a-bug-fix-not-a-feature).

### 6.3 Hooks: matcher globs, then verdicts

**(a) Widen the matcher — do this regardless.** `matchesTool` (`hooks.go:189`) is
exact-name-or-`*`. Widening to globs (`mcp:*`, `*_file`) by reusing
`permissions.toolPatternMatches` gives hooks and permission rules one pattern
language for ~10 lines.

**(b) A structured verdict channel.** Keep exit-code/stdout working exactly as
documented (it is Claude Code-shaped and users depend on it). Add: if stdout
parses as a JSON object carrying a `packetcode_hook` version key, treat it as
`{"decision":"block|allow|continue","reason":…,"arguments":{…},"context":"…"}`.
Anything else stays plain injected text.

**(c) The laundering problem — why this is Wave 3.** Today's order in
`handleToolCall` is `Decide` → reject on deny → `RunPreToolUse` → approval prompt
→ `EditedParams` → re-`Decide` (`agent.go:392-473`). The hook fires **after** the
policy decision, so crush's framing that auto-approve "integrates with the policy
engine rather than bypassing it" is **not true as currently sequenced**. With the
documented `deny execute_command command_prefix=["rm","-rf"]` rule in place, a
hook rewriting `{"command":"ls"}` into `{"command":"rm -rf /"}` executes
unchecked. Three rules close it:

1. **Move `RunPreToolUse` above the first `Decide`**, so hook-rewritten arguments
   enter the policy engine on the same footing as model-produced ones. Smaller
   diff than re-deciding afterward.
2. **Deny is an absolute floor over hook verdicts.** `matchingRule` already sweeps
   `DenyFloor` first (`policy.go:262-276`) and `safe` short-circuits at `:149`. A
   hook `allow` may only promote `DecisionAsk`; over `DecisionDeny` or
   `ProfileSafe` it is discarded and the discard reported. Precedent: the
   post-approval-edit path already re-checks deny at `agent.go:456-471`.
3. **Both mutating verdicts opt-in per hook** — `allow_rewrite` /
   `allow_auto_approve` on `HookConfig`, default false.

Assert with a test that a rewrite mutates `params` **before** the approver call
(`ApprovalRequest.Params` comes from the local `params` at `agent.go:434`), so
the user never approves text the model didn't produce. Document the limit
honestly: `Decide` is not path-aware, so for filesystem tools the real floor
against a rewritten path stays `LocalBackend.Resolve`. **Do not adopt crush's
parallel execution** — sequential is already deterministic at realistic hook
counts. **Effort: M.** **Risk: medium — entirely in the sequencing.**

### 6.4 Bounded `fetch`

`internal/tools/` has zero `net/http` usage. A single bounded `fetch`
(HTML→markdown, size cap, timeout, redirect limit, refuse non-http(s) schemes,
refuse loopback/link-local/RFC1918 by default) is the useful 80%. It must land
under the truncation store (4.3) so a 10 MB page cannot enter the transcript, and
its result must be framed as **untrusted evidence**, per BACKLOG:

> Treat remote/browser/desktop content as untrusted evidence rather than
> instructions.

That framing is a requirement, not a nicety — fetched content is the classic
prompt-injection vector and packetcode has no network policy axis today. Defer
`agentic_fetch`, `download`, and `web_search` until that axis exists. **Effort: M.**

### 6.5 In-process POSIX shell — last, not first

The largest idea in this plan, and verification moved it to the back of the queue.

**Where the in-flight procrun work stands.** The new
`ConfigureTrackedTreeCancel`/`TrackTree`/`ReleaseTree` API (`procrun/run.go:16-38`)
uses Windows `CREATE_SUSPENDED` + Job Object assignment + `NtResumeProcess` — the
correct atomic pattern, no child code runs before containment. But it is wired
into **MCP only** (`mcp/process.go:28,76`, `mcp/client.go:484`).
`LocalBackend.Execute` still uses the older `ConfigureTreeCancel`
(`local_backend.go:246`), as do `hooks.go:201` and `statusline.go:205` — so
**`execute_command` gets no Job Object containment today.** Three notes on that
work, since it gates this item and 6.1:

1. **POSIX is a no-op.** `process_posix.go` defines `trackTree`/`releaseTree` as
   `return nil` and the new test is `//go:build windows`. "Descendants cannot
   survive a normally exiting parent" is a **Windows-only** guarantee; a
   double-forked child still escapes `Setpgid` + `kill(-pgid)`. Either add a POSIX
   equivalent (`PR_SET_CHILD_SUBREAPER`/cgroup) or narrow the doc comment.
2. **`cmd.Cancel` is inert in the MCP path** — `spawnServerProcess` builds with
   `exec.CommandContext(context.Background(), …)`, so the `Cancel` func can never
   fire. Not a bug (`WaitDelay` still bounds post-exit I/O), but it reads as if
   cancellation is wired when it is not.
3. **`trackedJobs` leaks without `ReleaseTree`** — a `sync.Map` keyed by
   `*exec.Cmd` holding a raw handle. Fine for MCP, which always reaches
   `reaperLoop`; not fine for a shell running hundreds of commands per session.

**Complements the procrun work:** dropping the `cmd /C` / `sh -c` wrapper means
the interpreter spawns the real target binary directly, so containment applies to
the process the user actually approved rather than to a shell wrapper. And it
retires the per-host capability string baked into the tool description
(`execute_command.go:283-332`).

**Conflicts with it:** the interpreter spawns **one `exec.Cmd` per external
command in a pipeline**, while procrun's API is one Job Object per `*exec.Cmd`.
`a | b | c` would create three containment boundaries. Procrun needs a **group
handle** first — one Job Object (POSIX: one process group) that many `exec.Cmd`s
join, with a single Release. Small, self-contained, and needed for 6.1 regardless.

**What changes under `internal/permissions/policy.go`.** Nothing *breaks* —
policy decides on `req.ToolName` plus the `command` string and never inspects the
shell. But three meanings shift:

1. `commandPrefixMatches`'s metachar refusal becomes a semantic mismatch. With a
   real parser the correct test is "does this program's AST consist of a single
   simple command with this argv prefix" — stricter *and* more accurate. That is
   the natural place to also land the [§2.1](#21-the-deny-floor-fails-open-on-any-shell-metacharacter)
   fix. Do not ship the shell without revisiting this function.
2. **Persistent shell state invalidates the approval string** — the real
   regression. If `export PATH=/tmp/evil:$PATH` in one approved call changes what
   the next approved `git push` resolves to, the string the user approved no
   longer describes what runs. Same for `cd` drift versus the displayed `cwd`.
   **Recommendation: ship the interpreter without persistent state**, or persist
   only an explicit allowlist (cwd plus configured env keys) and render the
   effective delta in the approval prompt. `policy.go` has no state concept and
   would need one to reason about this.
3. **Rules about processes become rules about goroutines.** A builtin in a tight
   loop is cancelled cooperatively, not by `KillTree` — `while true; do :; done`
   becomes an unkillable goroutine *inside* packetcode. Needs a hard iteration
   budget in the interpreter and ctx checks in every builtin.

**Security: better and worse, and the research docs overstate the improvement.**
The genuine win is that `mvdan.cc/sh/v3` exposes `ExecHandler`, `OpenHandler`,
`StatHandler`, `ReadDirHandler` — routing those through
`computers.RuntimeBackend.Resolve` would give `execute_command` **actual root
confinement for builtins for the first time**. Today `LocalBackend.Execute`
confines only `cwd` (`local_backend.go:229-243`); the command itself can
`cat /etc/shadow` freely. That is the largest real security win available in this
domain. Against it: persistent state and uncancellable in-process builtins.

**Re-validation required:** every `policy_test.go` case against the new path;
`execute_command_test.go` exit-code/timeout/cancellation semantics; the
`ShellRuntimeInfo`/`SafetyText` description (factually wrong the moment commands
stop going to `cmd /C`); `PreToolUse` payloads carrying the raw command string;
and the approval renderer. `search_codebase`'s `rg` and `worktree.go`'s `git`
calls must keep using real `exec.Cmd`.

**Verdict: worth doing, last.** L effort, changes the semantics of the most
security-sensitive tool, and 2.1 / 4.3 / 4.5 deliver more per unit of risk.
**Start with the handler stack, not the coreutils** — reimplementing `ls`/`cat`/
`grep` is a long tail with a large test surface; let PATH find real binaries.

---

## 7. Wave 4 — architecture, aligned with PACKETCOMPUTERS

### 7.1 Precondition

[§2.2](#22-unscoped-state-directories-let-one-project-clobber-anothers-running-jobs)
(data-dir lock / project scoping) and
[§2.3](#23-job-records-are-unversioned-and-vanish-silently-when-unreadable)
(versioned, fail-loud job records) are **hard preconditions for PCMP4**. Neither
is optional and neither is large.

### 7.2 `abandoned` / `indeterminate` terminal state

Already mandated by the BACKLOG. Verified absent: `State` has exactly five values
(`job.go:24-30`). The semantic gap today is that a remote SSH job which started,
was acknowledged, then lost its transport is marked `Cancelled` via
`jobCtx.Err()` (`worker.go:272-276`) — indistinguishable from the user pressing
cancel, which is exactly the "flattened into a confirmed cancellation" the ledger
prohibits. **Sequence after §2.3**, since `parseState` currently defaults unknown
strings to `StateFailed`. **Effort: M.**

### 7.3 Per-job append-only event log — the narrow slice of event sourcing worth taking

Take the idea from opencode; reject the rewrite. Alongside
`<jobsDir>/<id>.json`, append `<jobsDir>/<id>.jsonl` — one versioned line per
lifecycle-significant event (queued, running, remote-start-acknowledged, tool
executed, terminal). O(1) appends, crash-safe under a "discard trailing partial
line" rule, no migration burden beyond a per-line `v`.

**Why this and not projectors:** what PCMP9 actually needs to answer is "where
did the record stop, and was the last thing we know a *local intent* or a *remote
acknowledgement*?" A log answers that directly. Full event-sourced sessions with
projectors and context-epoch answer it too, at the cost of rewriting
`session.Manager`, jobs persistence, and `Transcript()`. The snapshot JSON stays
the system of record for state; the log is the evidence trail. **Do not make the
log authoritative in v1.** **Effort: M.**

### 7.4 Versioned event manifest and compatibility policy

> Define compatibility and migration policy for config, sessions, persisted jobs,
> workflow TOML, and MCP definitions.

opencode treats its event and schema surface as a published contract with its own
changelog. This should land **before** the daemon: once a daemon has clients, the
daemon becomes the compatibility problem. Cheap as a document; medium as enforced
versioning. §2.3 closes the jobs half already.

### 7.5 Transport for the daemon

**Recommendation: drop the TCP listener from PCMP4's v1. Use an AF_UNIX socket
on POSIX and a named pipe (or stdlib AF_UNIX) on Windows.** PCMP4's acceptance
condition currently specifies `packetcode daemon --listen 127.0.0.1:<port>`, so
this is a **ledger edit**, not just an implementation choice — make it
explicitly rather than letting the implementation quietly diverge.

Reasons, in order of weight:

1. **PCMP4 also requires that the daemon "writes no credentials to disk," and
   TCP forces it to break that.** A loopback port is reachable by every local
   process and every local UID, so it needs an auth token — which needs a token
   store, a daemon→client handoff, and a rotation story. packetcode has no auth
   infrastructure and **no network listener anywhere in production code today**
   (`grep net.Listen` → zero hits outside provider test helpers). A socket at
   `0600` inside a `0700` directory gets the same property from the filesystem,
   free, writing no credential anywhere. This is the decisive argument: the TCP
   path makes PCMP4 violate one of its own two acceptance conditions to satisfy
   the other.
2. **"Refuses non-loopback binds" becomes structural rather than validated.** A
   bind-address check is a runtime guard that a future config path, env override,
   or a container run with host networking can regress — with the test still
   green while the property is gone. An AF_UNIX listener has no address family
   that can accept a remote connection at all.
3. **SSH forwarding (PCMP6) still works** — `ssh -L /local.sock:/remote.sock`
   (StreamLocalForward), or `-L 127.0.0.1:port:/remote.sock`. Write the caveat
   into PCMP6 now: this needs `AllowStreamLocalForwarding` on the remote sshd
   (default yes, disabled in some hardened configs), and that failure must
   produce a clear diagnostic rather than a hang.
4. **Windows:** try stdlib `net.Listen("unix", …)` first — works on Windows 10
   1803+, pure Go, zero new dependencies, one code path. Add `Microsoft/go-winio`
   named pipes only if AF_UNIX proves flaky, and check its transitive deps first
   (it has historically pulled `sirupsen/logrus`).

**Also lift from crush:** stale-socket classification. "Daemon running" vs "died
and left the socket" vs "another user owns it" is the *same three-way ambiguity*
as §2.2's instance lock and §7.2's `abandoned` state. Build the vocabulary once
and share it across all three.

**Do not lift "multi-client from day one."** For PCMP4/5 the product need is one
TUI ↔ one daemon per registered machine; multi-client UX is scope inflation. The
half that matters — an event stream with no single-consumer assumption —
packetcode already has in `jobs.Manager.Subscribe` (`manager.go:293`). Keep the
two-subscribers-one-job test; skip the feature.

From opencode, take the **typed-contract discipline**, not the transport: a
versioned RPC + event manifest written **before** PCMP4 code. Content: the method
list from `PACKETCOMPUTERS.md` (`ping`, `capabilities`, `project.list`, `job.*`,
`tool.execute`, `fs.*`), the snapshot/event payloads, a version field on every
message, and a changelog. Skip swagger (wrong artifact for JSON-RPC over a
socket) and skip generated SDKs (there is one client).

### 7.6 Persistence: stay on JSON, sequence SQLite after PCMP9

**Recommendation: stay on JSON through PCMP9. Do not make PCMP9 depend on
SQLite. Introduce SQLite later, if at all, only as a rebuildable derived index —
never as the system of record.**

crush's framing — that atomic JSON is what makes PCMP9 awkward — **does not
survive reading `jobs/manager.go`.** Job records are small per-file documents
that already have debounced write coalescing and `Seq`-based stale-write guards
(`manager.go:1283-1370`). Storage is not what makes PCMP9 hard; what makes it
hard is that **no process on the other end kept running**, and SQLite changes
nothing about that.

The one place JSON genuinely does not scale is *sessions*, not jobs:
`session.Manager.Save` rewrites the entire session document — every message — on
every `AddMessage`. That is O(n) per turn, and it is the real cost behind
"transcript search/filter" and "persist request-level occupancy samples." It is
also **independent of PCMP9**.

**Sequence:** §2.3 versioned job records → §7.4 compatibility policy → PCMP9 on
JSON plus the §7.3 event log → *only then* evaluate SQLite for transcript search
and stats. The reverse order means writing a migration policy for a format you
are about to replace, and gating PCMP9 behind a persistence rewrite. If SQLite
does land: pure-Go driver only (`modernc.org/sqlite` or `ncruces/go-sqlite3` —
cgo is non-negotiable), kept rebuildable from the JSON/JSONL files of record, so
a corrupt or deleted database is a rebuild rather than data loss and needs no
migration policy of its own. Do not add SQLite for stale-write tracking (4.5) in
the meantime.

### 7.7 CodeMode — spike only, if at all

The model writes a small program calling only host-supplied tools inside a
confined interpreter (Starlark over goja in Go: deterministic, sandboxed by
default, no JS-ecosystem surface). Verification pushed back on the security
framing: **packetcode's tool set is already the capability boundary**, so a
confined interpreter buys token savings, not safety. Time-boxed spike at most,
and not this cycle.

---

## 8. Licensing position

**Ideas: yes. Code: from opencode only, and even then rarely worth it.**

| | opencode | crush |
|---|---|---|
| Licence | MIT | FSL-1.1-MIT |
| Copy code? | Legally yes, with notice retained | **No** |
| Copy prompt text? | Yes, with a `THIRD-PARTY-NOTICES` entry | **No** |
| Copy ideas/architecture? | Yes | Yes |

**Why crush is different.** FSL grants use only for a "Permitted Purpose" and
excludes **Competing Use** — making the software available in a commercial
product or service that substitutes for crush. packetcode is a terminal AI coding
agent; it is the substitute. Each version converts to MIT two years after its
release date. Even while packetcode stays non-commercial (arguably outside the
literal definition), this is the wrong thing to be arguing about later.

**What is and isn't protected.** Copyright covers *expression*, not function,
method, or system. Independently implementing "hash the tool-call signature over
a sliding window and stop on repeats" in packetcode's own Go is outside
copyright's reach. Near-verbatim transliteration — same structure, same
identifiers, same comments, mechanically converted — is a derivative work and is
not. Prompt text is expression too: no copying `.md`, `.md.tpl`, `.txt`, or
`SKILL.md` bodies from crush at all, and only from opencode with a notice.

**Unaffected in principle:** the separately published MIT/BSD modules — catwalk,
powernap, vcr, `mvdan.cc/sh`, `gojq`. These are not crush and may be imported as
ordinary dependencies with their notices retained. Note that this plan
nonetheless **declines** catwalk and vcr on engineering grounds
([§5](#5-wave-2--the-dependency-decisions)), and that `mvdan.cc/sh` and possibly
powernap are the imports that actually survive. Add a `THIRD-PARTY-NOTICES` file
when the first one lands.

**Practice:** build from these three documents with upstream source closed;
extend the documents rather than the source when detail is missing.

---

## 9. Where both upstreams independently agree

Convergence is the strongest signal this research produced — two teams, different
languages, different architectures, same conclusions:

| Both built it | packetcode status |
|---|---|
| External model/pricing catalog | Seven hand-maintained tables; openrouter/codex already live |
| Record/replay provider tests | Env-gated live tests exist; no cassettes |
| A question tool routed off the foreground UI | Routing half-exists; no tool, one approval lane |
| Skills with progressive disclosure | None (slash commands and workflows are different things) |
| A todo tool | None |
| LSP-backed code intelligence | Go AST + lexical heuristics only |
| Tool-output truncation with full retention | 100 KB cap on commands only; MCP uncapped |
| A daemon with the TUI as one client | PCMP4/5, not yet built; no listener exists today |
| Web fetch/search tools | None |

Where they **disagree**, follow crush on transport (local socket, because of the
loopback-only rule) and opencode on contract discipline (typed schemas, generated
clients, a versioned event manifest with a changelog).

---

## 10. Explicitly skipped, with reasons

| Candidate | Why not |
|---|---|
| **`charm.land/catwalk`** | go 1.26.6 toolchain bump, prometheus/protobuf deps, no `mistral`, prices MiniMax-M3 at the long-context tier, and structurally cannot express tiered pricing. [§5.1](#51-model-catalog--modelsdev-via-stdlib-not-catwalk) |
| **`multiedit`** | `patch_file` already is it for a single file; multi-file adds partial-failure semantics for negligible gain |
| **`lsp_rename` / `lsp_replace_symbol`** | Bypasses backup, diff preview, and the approval renderer's `PreviewPatchDiff` seam. Revisit after read-only LSP tools are stable |
| **Full Go coreutils suite** | The handler stack and dispatch middleware are the valuable parts of the shell work; reimplementing `ls`/`cat`/`grep` is a long tail with a large test surface |
| **Config as generated JSON Schema** | packetcode's config is **TOML** — a JSON Schema only reaches editors via Taplo and needs a `#:schema` directive in the atomically-written config, plus a new dependency. `doctor --check config` already validates |
| **MCP OAuth with a local callback server** | Needs HTTP transport (contract-gated), adds a third credential mode beyond `none`/`bearer-env`, and a callback server is an **inbound listener** in an executable whose trust contract says it "remains stdio-only… does not add a network listener." Needs its own contract amendment; do not scope it inside the Streamable HTTP work |
| **MCP prompts** (resources are fine) | An MCP prompt is server-supplied text intended to *become* a conversation message; auto-injecting it is what the trust contract's §4 forbids. Only safe shape is `/mcp prompt <server> <name>` into the **user's input buffer** |
| **Skill guidance deltas; remote skill discovery** | Delta path is dead code against packetcode's cache inputs; remote discovery is v2 at best |
| **Parallel hook execution** | Sequential is already deterministic at realistic hook counts |
| **SQLite for read-file tracking** | Key off the existing session store; a persistence rewrite is the compatibility-policy item's problem |
| **Event-sourced sessions with projectors + context-epoch** | A persistence rewrite for a property the per-job event log (§7.3) delivers at a fraction of the risk. The research doc itself says "sequence with PCMP9, not before" and then ranks it alongside things to do now — those statements are in tension |
| **HTTP + typed route groups + generated SDK** | Keep the *discipline* (typed contract, version manifest), skip the transport — one client, and HTTP reintroduces exactly the bind problem §7.5 avoids |
| **Multi-client-from-day-one; swagger** | Scope inflation for one TUI ↔ one daemon. Keep the two-subscribers-one-job test; swagger is the wrong artifact for JSON-RPC over a socket |
| **`WithGlobalMirror`-style global-state discipline** | Verified not applicable — `grep '^var '` across the nine relevant packages returns two immutable lookup maps and a sentinel error block; the only `sync.Once` is ripgrep discovery. Managers are all instance-scoped and `os.Chdir` is never called. The real analogue is global *state directories* (§2.2) |
| **CodeMode / Starlark** | High effort, speculative payoff, and the confinement argument is weak — the tool set is already the capability boundary |
| **`csync`-style versioned map; desktop notifications; chroma highlighting** | No demonstrated problem; new dependencies against a 14-direct-dep discipline |
| **`charm.land/fantasy` as the provider layer** | The hand-rolled zero-SDK parsers are a deliberate differentiator, better tested than a swap would preserve. Wire-shape reference only |
| **Bundled ripgrep** | Downloads a binary at runtime; `search_codebase` already has an rg-when-present / Go-fallback split |
| **Session sharing, web/desktop clients, mDNS, containers, enterprise policy, Slack, Sourcegraph, telemetry** | Product surface packetcode has not chosen; mDNS actively conflicts with loopback-only daemon trust |

---

## 11. Decisions — ruled 2026-08-14

1. **PCMP9 is out of scope for this repo.** Durable execution belongs to
   PacketAgent; the PacketADE line governs. packetcode never claims a job
   survives a restart. Consequences, which the sections above now reflect:
   - **PCMP9 is cut.** Anything interrupted is reported as abandoned. That makes
     [§7.2](#72-abandoned--indeterminate-terminal-state)'s
     `abandoned`/`indeterminate` state *more* important, not less: it stops
     being a PCMP9 precondition and becomes the primary honest terminal state.
   - **The daemon survives but shrinks.** PCMP4/5 remain for reaching Packet
     Computers (PCMP8's direct-SSH routing already shipped), but session-scoped:
     it dies with the app. The socket recommendation in
     [§7.5](#75-transport-for-the-daemon) is unchanged and, if anything,
     stronger — a session-scoped daemon has even less business holding a port.
   - **Persisted job records stay.** They are what lets packetcode say "this job
     was abandoned" honestly after a restart, which is precisely what
     [§2.3](#23-job-records-are-unversioned-and-vanish-silently-when-unreadable)
     fixed. `/jobs resubmit` (PCH4) also stays; re-running an abandoned job was
     never a resume claim.
   - **The per-job JSONL event log ([§7.3](#73-per-job-append-only-event-log--the-narrow-slice-of-event-sourcing-worth-taking))
     loses most of its justification**, since its value was PCMP9 replay.
     Demoted to optional post-mortem tooling rather than planned work.
   - **Wave 0 is unaffected** — the cross-instance clobbering was a live bug
     whatever the daemon does.
2. **GitHub Copilot: parked until after v1.** Remains in BACKLOG as gated and
   unranked. Nothing changes in `docs/providers.md`.
3. **Own the cassette recorder** — the ~300-LOC `RoundTripper` in
   [§5.3](#53-cassette-based-provider-contract-tests), not `dnaeon/go-vcr`. SSE
   frame chronology stays local and go.mod gains no YAML parser.

---

## 12. Suggested order

**Wave 0 — defects. Done 2026-08-14.**

1. ~~Deny-floor fail-closed fix~~ — shipped with regression tests
2. ~~Versioned, fail-loud job records~~ — shipped; `UnreadableRecords()` still
   needs surfacing in the App and `doctor`
3. ~~Project-scoped job recovery~~ — shipped; the same-project advisory lock
   remains open

**Wave 1 — small, high value**

4. Loop detection (S) · 5. Cached-token plumbing (S) · 6. Tool-output store (S)
7. Backup pruning (S) · 8. Stale-write protection (S–M) · 9. Todo tool (S)
10. Self-diagnostic tool (S) · 11. Question tool on its own lane (M)
12. Skills + embedded self-config skills (M)

**Wave 2 — catalogs, tests, LSP** *(cassettes run in parallel with everything —
they depend on nothing and de-risk the catalog and pricing work by pinning the
wire shapes both touch)*

13. Cassette provider tests (M) · 14. MiniMax `/effort` mapping (S, standalone)
15. models.dev catalog (M) · 16. Cached-rate pricing (M)
17. LSP via powernap + post-edit diagnostics (M) · 18. Catalog-driven reasoning
    controls (S) · 19. Local runtime discovery, LM Studio first (M)

**Wave 3 — bigger**

20. Hook matcher globs (S) · 21. Git shadow snapshots (M)
22. Bounded `fetch` (M) · 23. procrun group handle + POSIX parity (M)
24. Background shell jobs (M) · 25. Hook verdicts with reordered policy (M)
26. In-process shell behind a flag (L) — last

**Wave 4 — architecture** *(reshaped by the PCMP9 ruling in
[§11](#11-decisions--ruled-2026-08-14))*

27. `abandoned`/`indeterminate` state — now the primary honest terminal state,
    not a PCMP9 precondition. Independent of the daemon; do it early
28. Surface `UnreadableRecords()` in the App and `doctor`
29. RPC + event manifest, written before daemon code
30. Session-scoped daemon over a socket (PCMP4/5) → PCMP6 → PCMP7
31. Optional, if a need appears: per-job JSONL event log; SQLite as a
    rebuildable index for transcript search

Wave 0 is done. Items 4–12 are roughly two weeks of focused work and close five
BACKLOG items.

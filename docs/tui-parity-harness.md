# TUI parity capture harness

The parity harness captures a real POSIX terminal through a PTY, feeds its ANSI
stream into a terminal-cell emulator, and compares reviewed, credential-free
goldens. It does not use a browser or Playwright.

The PTY layer requires Linux, macOS, or WSL. Native Windows/ConPTY remains a
manual release check; cross-platform Go layout/key tests still run on Windows
CI. See [Supported terminals](supported-terminals.md).

## Reproducible setup

- Python 3
- `pyte==0.8.2`, pinned in `scripts/requirements-tui.txt`
- packetcode built at `bin/packetcode`
- optional: the `claude` CLI on `PATH` for local comparison only

```sh
python3 -m pip install -r scripts/requirements-tui.txt
make build
```

No credentials are written to reviewed goldens. Run Claude comparisons only in
an already authenticated local environment and inspect generated `.ansi` files
before sharing them.

## CI goldens

```sh
make tui-golden-check   # regenerate in a temporary directory and diff
make tui-golden-update  # deliberately promote new normalized evidence
```

The tracked matrix captures `72x24` and `100x30` renders for the shared chat
chrome, streaming, tool result, approval, queue, Plan mode, Agent View, and
Workflow View. A separate `100x30` → `72x24` live-resize fixture exercises the
SIGWINCH path after startup. Its normalized evidence replays only the
post-SIGWINCH live frame (not old inline rows committed to scrollback) and must
contain exactly one status row; its stable input/status tail must match a clean
`72x24` layout.

Only normalized `.txt` text plus deterministic styled-cell spans and stable
`.json` geometry metadata live under `testdata/tui/golden/`. The capture
environment explicitly enables truecolor and removes `NO_COLOR`, so foreground,
background, and emphasis changes participate in diffs. Raw `.ansi` streams are
checked and discarded. The protocol gate parses both individual and grouped
private-mode sequences and rejects:

- alternate-screen activation (`?1049`, `?1047`, or `?47`);
- mouse tracking (`?1000`, `?1002`, `?1003`, or `?1006`);
- OSC 52 clipboard writes;
- unbalanced synchronized-output (`?2026`) markers;
- bracketed-paste mode that is enabled but not restored on graceful exit.

`pyte` is not grapheme-complete for every emoji ZWJ sequence. Unicode fixtures
therefore also require the exact UTF-8 grapheme in the raw render stream and in
the production-component Go view; the resize fixture requires it again after
SIGWINCH. Cell goldens alone are not treated as proof that a complex cluster
survived.

CI runs the golden gate on Ubuntu. The same gate blocks tagged releases.
Component/layout tests also render every lifecycle fixture at `72x24`, `80x24`,
`100x30`, and `120x40` on Ubuntu, macOS, and Windows.

## Local captures

```sh
make tui-snapshots
make tui-snapshots-claude  # skips cleanly when claude is unavailable
```

Local output lives under `testdata/tui/captures/<target>/<scenario>/` and is
git-ignored because installed tools can render account, organization, path, or
machine data. Each capture contains raw `.ansi`, normalized `.txt`, and stable
`.json` metadata.

Commands can be overridden without changing the harness:

```sh
PACKETCODE_CMD='./bin/packetcode' scripts/tui_snapshot_suite.sh packetcode
CLAUDE_CMD='claude' scripts/tui_snapshot_suite.sh claude
```

The development-only `--tui-fixture=<state>` renderer never loads config,
credentials, providers, sessions, hooks, MCP servers, or project files. It
covers user/assistant spacing, thinking and streaming, tool states, approval,
errors, cancellation, queuing, compaction, permission modes, agents, and
workflows through production components.

Literal UTF-8 key input is preserved. `--keys` also understands documented
ASCII escapes such as `\n`, `\r`, `\t`, `\0`, and `\x1b`. Geometry can be
selected with repeated `--size WIDTHxHEIGHT` arguments; `--resize WIDTHxHEIGHT`
changes a live PTY after its initial render.

## Promotion rules

Before running `make tui-golden-update`:

1. Inspect the normalized diff at both widths.
2. Confirm no account, hostname, absolute path, token, or provider response is
   present.
3. Confirm raw protocol checks pass; never promote `.ansi` files.
4. Explain intentional layout changes in `CHANGELOG.md`.
5. Run `make tui-golden-check` from a clean checkout.

Future Bubble Tea v2 work must extend this gate with true `Shift+Enter`,
distinct `Ctrl+M`, synchronized frame updates around streaming/cancellation,
exact-width Unicode, and manual Windows Terminal/tmux evidence.

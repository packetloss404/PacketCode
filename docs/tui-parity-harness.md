# TUI parity capture harness

The parity harness captures a real terminal through a PTY, then feeds ANSI output into a terminal cell emulator. It does not use a browser or Playwright.

## Dependencies

- Python 3
- [`pyte`](https://pypi.org/project/pyte/): `python3 -m pip install pyte`
- packetcode built at `bin/packetcode`
- optional: the `claude` CLI on `PATH`

No credentials are written to captures. Run Claude comparisons only in an already authenticated local environment and inspect generated `.ansi` files before sharing them.

## Usage

```sh
make build
make tui-snapshots
make tui-snapshots-claude  # skips cleanly when claude is unavailable
```

Both targets capture fixed `100x30` and `120x40` terminals under `testdata/tui/captures/<target>/<scenario>/`. Each capture contains raw `.ansi`, normalized cell text `.txt`, and fixed metadata `.json`. The normalized text is suitable for golden diffs in CI; raw ANSI is retained for local color/style inspection.

The capture directory is git-ignored because installed tools may render an account name, organization, or machine-specific path in their welcome screen. Promote only reviewed, normalized fixtures into a separate golden directory.

Commands can be overridden without changing the harness:

```sh
PACKETCODE_CMD='./bin/packetcode' scripts/tui_snapshot_suite.sh packetcode
CLAUDE_CMD='claude' scripts/tui_snapshot_suite.sh claude
```

The credential-free base suite covers welcome, focused/idle input, autocomplete, and narrow layout. Packetcode also ships a development-only lifecycle renderer that never loads config, credentials, providers, sessions, hooks, MCP servers, or project files:

```sh
TUI_FIXTURE_CMD='./bin/packetcode' make tui-snapshots
```

It captures user/assistant spacing, thinking and streaming, tool running/results, approval, errors, cancellation, queuing, compaction, all permission-mode footers, agents, and workflows. Claude lifecycle captures remain optional and use a separately reviewed command because Claude has no equivalent credential-free fixture mode.

Volatile cursor position, timing, process IDs, and environment-specific provider data should be normalized by the fixture wrapper before golden promotion. Terminal dimensions and cell text are not normalized away because geometry is the evidence this harness is intended to preserve.

# Supported terminals

PacketCode is a keyboard-first inline TUI. Its core contract is UTF-8 plus a
modern VT/ANSI terminal with normal scrollback. It deliberately does not enable
mouse tracking or the alternate screen, so native selection, wheel scrolling,
and tmux copy mode remain terminal-owned.

## Geometry

- `80x24` or larger is recommended.
- `72x24` is the smallest geometry in the committed golden matrix.
- `72x24`, `80x24`, `100x30`, and `120x40` are covered by cross-platform Go
  layout tests.
- Extremely short or narrower windows may reduce detail. Resize is supported;
  a live `100x30` → `72x24` PTY transition is checked in CI.

## Portable keys

| Intent | Reliable input |
| --- | --- |
| Send | `Enter` |
| Newline | `Ctrl+J` or `\` then `Enter`; `Alt+Enter` when Alt is reported distinctly |
| Provider picker | `Ctrl+P` or `/provider` |
| Model picker | `/model`; `Alt+M` when Alt is reported distinctly |
| Permission mode | `Shift+Tab` |
| Cancel / clear draft / quit when empty | `Ctrl+C` |
| Quit from empty prompt | `Ctrl+D` |

Under Bubble Tea v1, `Ctrl+M` aliases carriage return/Enter and is intentionally
unbound so it cannot accidentally submit a draft. `Shift+Enter` inserts a
newline only where the terminal maps it to `Ctrl+J`; the other newline bindings
are the portable contract. True modifier-aware `Ctrl+M` and `Shift+Enter` are
gated on the Bubble Tea v2 migration and its PTY/manual evidence matrix.

## Evidence matrix

| Environment | Automated evidence | Required manual evidence |
| --- | --- | --- |
| Ubuntu terminal/PTY | Go tests, normalized cell goldens, protocol safety, live resize | tmux copy mode and an interactive stream before release changes |
| macOS | Go tests and native build/smoke | Terminal.app or iTerm2 input, resize, scrollback, selection |
| Windows | Go tests and native build/smoke | Windows Terminal/ConPTY input, paste, resize, Unicode, cancellation |
| Git Bash / MSYS / WSL | Build-compatible paths where applicable | Key chords and Unicode because the input path differs from native ConPTY |
| tmux | No separate CI runner | Native scrollback/copy mode, resize, and no mouse capture |

The Python PTY capture harness is POSIX-only (Linux, macOS, or WSL). It does not
claim native ConPTY coverage. Windows behavior is covered by deterministic Go
tests plus the manual release check until a credential-free ConPTY harness is
added.

## Terminal safety contract

CI rejects raw PacketCode output that enables alternate-screen buffers, mouse
tracking, or OSC 52 clipboard writes. Bracketed-paste mode must be restored on a
graceful quit. Synchronized-output markers, when emitted by a future renderer,
must be balanced.

Provider, tool, hook, statusline, and MCP text is sanitized before display.
That protects terminal state; it does not make an underlying command or remote
process safe.

## When a key behaves differently

1. Use the slash-command equivalent (`/provider`, `/model`, `/clear`).
2. Use backslash-Enter for multiline input; try `Alt+Enter` only when the
   terminal reports Alt distinctly.
3. Run `packetcode doctor` and note the terminal, shell, multiplexer, and whether
   the session is native, SSH, WSL, or MSYS.
4. Reproduce at `80x24` outside a multiplexer, then repeat inside it.
5. Include only sanitized cell output; never attach raw terminal captures until
   they have been inspected for account or path data.

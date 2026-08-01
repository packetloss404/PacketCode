#!/usr/bin/env python3
"""Deterministic PTY/cell capture for packetcode and optional Claude Code."""
import argparse
import json
import os
import pathlib
import re
import select
import shlex
import signal
import struct
import subprocess
import time

if os.name != "nt":
    import fcntl
    import pty
    import termios
else:
    fcntl = None
    pty = None
    termios = None

try:
    import pyte
except ImportError:
    pyte = None

DEFAULT_SIZES = ((72, 24), (100, 30))


def parse_size(value):
    try:
        width, height = (int(part) for part in value.lower().split("x", 1))
    except (TypeError, ValueError) as exc:
        raise argparse.ArgumentTypeError("size must be WIDTHxHEIGHT") from exc
    if width < 20 or height < 8:
        raise argparse.ArgumentTypeError("size must be at least 20x8")
    return width, height


def decode_key_spec(value):
    """Decode documented ASCII escapes without corrupting literal Unicode."""
    escapes = {"n": "\n", "r": "\r", "t": "\t", "0": "\0", "\\": "\\"}
    out = []
    index = 0
    while index < len(value):
        if value[index] != "\\" or index + 1 >= len(value):
            out.append(value[index])
            index += 1
            continue
        code = value[index + 1]
        if code in escapes:
            out.append(escapes[code])
            index += 2
            continue
        if code == "x" and index + 3 < len(value):
            try:
                out.append(chr(int(value[index + 2:index + 4], 16)))
                index += 4
                continue
            except ValueError:
                pass
        out.extend(("\\", code))
        index += 2
    return "".join(out).encode("utf-8")


def answer_terminal_queries(master, raw, answered):
    """Answer standard terminal queries that can block TUI startup.

    OSC color queries are intentionally not answered: their response can be
    mistaken for input by some programs. Device attributes and cursor-position
    reports are safe and sufficient for the Claude/packetcode startup paths.
    """
    probe = bytes(raw[-64:])
    replies = (
        (b"\x1b[c", b"\x1b[?1;2c", "primary-da"),
        (b"\x1b[>c", b"\x1b[>0;95;0c", "secondary-da"),
        (b"\x1b[6n", b"\x1b[1;1R", "cursor-position"),
    )
    for query, reply, name in replies:
        if name not in answered and query in probe:
            os.write(master, reply)
            answered.add(name)


def drain_until_quiet(master, raw, quiet, timeout, answered):
    deadline = time.monotonic() + timeout
    last_data = None
    while time.monotonic() < deadline:
        ready, _, _ = select.select([master], [], [], 0.05)
        if ready:
            try:
                chunk = os.read(master, 65536)
            except OSError:
                return
            if not chunk:
                return
            raw.extend(chunk)
            answer_terminal_queries(master, raw, answered)
            last_data = time.monotonic()
            continue
        if last_data is not None and time.monotonic() - last_data >= quiet:
            return


def resize_pty(master, proc, width, height):
    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
    os.killpg(proc.pid, signal.SIGWINCH)


def private_mode_changes(raw):
    """Yield every mode change from DECSET/DECRST, including grouped modes."""
    for marker in re.finditer(rb"\x1b\[\?([0-9;]+)([hl])", raw):
        enabling = marker.group(2) == b"h"
        for value in marker.group(1).split(b";"):
            if value:
                yield int(value), enabling


def assert_balanced_mode(changes, code, label):
    active = False
    for mode, enabling in changes:
        if mode != code:
            continue
        if enabling and active:
            raise RuntimeError(f"terminal protocol check failed: nested {label} begin")
        if not enabling and not active:
            raise RuntimeError(f"terminal protocol check failed: {label} end without begin")
        active = enabling
    if active:
        raise RuntimeError(f"terminal protocol check failed: {label} mode was not restored")


def assert_protocol_safety(raw):
    changes = list(private_mode_changes(raw))
    forbidden_modes = {
        "alternate screen": {47, 1047, 1049},
        "mouse tracking": {1000, 1002, 1003, 1006},
    }
    for label, modes in forbidden_modes.items():
        if any(enabling and mode in modes for mode, enabling in changes):
            raise RuntimeError(f"terminal protocol check failed: {label} was enabled")
    if b"\x1b]52;" in raw:
        raise RuntimeError("terminal protocol check failed: OSC 52 clipboard was used")

    assert_balanced_mode(changes, 2026, "synchronized-output")
    assert_balanced_mode(changes, 2004, "bracketed-paste")


def cell_style(cell):
    """Return a stable description of non-default pyte cell attributes."""
    attributes = []
    for name in ("fg", "bg"):
        value = getattr(cell, name, "default")
        if value != "default":
            attributes.append(f"{name}={value}")
    for name in ("bold", "italics", "underscore", "strikethrough", "reverse", "blink"):
        if getattr(cell, name, False):
            attributes.append(name)
    return ",".join(attributes)


def render_snapshot(screen):
    """Serialize visible text and styled cell spans for regression review."""
    lines = [line.rstrip() for line in screen.display]
    while lines and not lines[-1]:
        lines.pop()

    spans = []
    for row in range(screen.lines):
        start = None
        previous = None
        for column in range(screen.columns + 1):
            style = cell_style(screen.buffer[row][column]) if column < screen.columns else ""
            if style == previous:
                continue
            if previous:
                end = column
                location = f"{row + 1}:{start + 1}" if end == start + 1 else f"{row + 1}:{start + 1}-{end}"
                spans.append(f"{location} {previous}")
            start = column
            previous = style

    text = "\n".join(lines) + ("\n" if lines else "")
    if spans:
        text += "\n-- cell styles --\n" + "\n".join(spans) + "\n"
    return text


def assert_semantic_text(raw, expected_values, post_resize_raw=None):
    """Require semantic UTF-8 in the final render and any post-resize repaint."""
    for expected in expected_values:
        encoded = expected.encode("utf-8")
        if encoded not in raw:
            raise RuntimeError(
                f"semantic render check failed: raw stream omitted {expected!r}"
            )
        if post_resize_raw is not None and encoded not in post_resize_raw:
            raise RuntimeError(
                f"semantic resize check failed: post-resize stream omitted {expected!r}"
            )


def capture(command, width, height, keys, settle, timeout, resize):
    if os.name == "nt":
        raise RuntimeError(
            "the PTY capture harness requires POSIX (Linux, macOS, or WSL); "
            "Windows/ConPTY is covered by Go layout tests and manual release checks"
        )
    initial_width, initial_height = width, height
    master, slave = pty.openpty()
    # TIOCSWINSZ takes rows, columns.
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
    env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", CLICOLOR="1")
    # NO_COLOR is enabled by presence, even when its value is "0".
    env.pop("NO_COLOR", None)
    proc = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave,
                            env=env, start_new_session=True, close_fds=True)
    os.close(slave)
    raw = bytearray()
    display_raw = b""
    resize_offset = None
    answered = set()
    try:
        # Wait for startup rendering to settle before typing. Sending keys
        # immediately races slower TUIs (notably Claude Code), which may clear
        # the screen after the keys land and produce a false comparison.
        drain_until_quiet(master, raw, settle, timeout, answered)
        for key in keys:
            os.write(master, decode_key_spec(key))
            drain_until_quiet(master, raw, settle, timeout, answered)
        if resize is not None:
            resize_offset = len(raw)
            width, height = resize
            resize_pty(master, proc, width, height)
            drain_until_quiet(master, raw, settle, timeout, answered)
        display_raw = bytes(raw)
    finally:
        if proc.poll() is None:
            # Exercise Bubble Tea's normal quit path so terminal-mode cleanup
            # becomes part of the captured protocol evidence.
            try:
                os.write(master, b"\x03")
                drain_until_quiet(master, raw, 0.1, 1.0, answered)
                proc.wait(timeout=1)
            except (OSError, subprocess.TimeoutExpired):
                os.killpg(proc.pid, signal.SIGTERM)
                try:
                    proc.wait(timeout=1)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
        os.close(master)
    if pyte is None:
        raise RuntimeError("pyte is required for cell normalization: python3 -m pip install pyte")
    if resize_offset is None:
        screen = pyte.Screen(initial_width, initial_height)
        stream = pyte.Stream(screen)
        stream.feed(display_raw.decode("utf-8", "replace"))
    else:
        # A shrinking real terminal may commit rows from the old inline frame
        # to scrollback. Normalize the live repaint independently so reviewed
        # goldens cannot bless stale pre-resize chrome as part of the new frame.
        screen = pyte.Screen(width, height)
        stream = pyte.Stream(screen)
        stream.feed(display_raw[resize_offset:].decode("utf-8", "replace"))
    post_resize_raw = None if resize_offset is None else display_raw[resize_offset:]
    return bytes(raw), render_snapshot(screen), width, height, post_resize_raw


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", choices=("packetcode", "claude"), required=True)
    ap.add_argument("--command", help="command override (otherwise PACKETCODE_CMD/CLAUDE_CMD)")
    ap.add_argument("--scenario", default="welcome")
    ap.add_argument("--keys", action="append", default=[], help=r"keys to send; supports escapes such as \n")
    ap.add_argument("--output", default="testdata/tui/captures")
    ap.add_argument("--settle", type=float, default=0.35)
    ap.add_argument("--timeout", type=float, default=5.0)
    ap.add_argument("--size", action="append", type=parse_size, dest="sizes",
                    help="capture geometry WIDTHxHEIGHT; repeatable (default: 72x24 and 100x30)")
    ap.add_argument("--resize", type=parse_size,
                    help="resize the live PTY after initial rendering")
    ap.add_argument("--protocol-check", action="store_true",
                    help="reject alternate-screen, mouse, clipboard, or unbalanced terminal modes")
    ap.add_argument("--expect-text", action="append", default=[],
                    help="require exact UTF-8 text in the raw rendered stream; repeatable")
    args = ap.parse_args()
    if pyte is None:
        print("pyte unavailable; PTY cell capture skipped (install with: python3 -m pip install pyte)")
        return 0
    default = "./bin/packetcode" if args.target == "packetcode" else "claude"
    command = args.command or os.getenv(args.target.upper() + "_CMD", default)
    argv = shlex.split(command)
    if not shutil_which(argv[0]):
        if args.target == "claude":
            print("Claude CLI unavailable; optional comparison skipped")
            return 0
        raise SystemExit(f"command unavailable: {argv[0]}")
    root = pathlib.Path(args.output) / args.target / args.scenario
    root.mkdir(parents=True, exist_ok=True)
    sizes = args.sizes or DEFAULT_SIZES
    for initial_width, initial_height in sizes:
        raw, cells, width, height, post_resize_raw = capture(
            argv, initial_width, initial_height, args.keys, args.settle, args.timeout, args.resize
        )
        if args.protocol_check:
            assert_protocol_safety(raw)
        assert_semantic_text(raw, args.expect_text, post_resize_raw)
        geometry = f"{initial_width}x{initial_height}"
        if args.resize is not None:
            geometry += f"-to-{width}x{height}"
        stem = root / geometry
        stem.with_suffix(".ansi").write_bytes(raw)
        stem.with_suffix(".txt").write_text(cells)
        stem.with_suffix(".json").write_text(json.dumps({
            "target": args.target,
            "scenario": args.scenario,
            "initial_width": initial_width,
            "initial_height": initial_height,
            "width": width,
            "height": height,
            "keys_hex": [decode_key_spec(key).hex() for key in args.keys],
            "expected_text": args.expect_text,
        }, indent=2) + "\n")
    return 0


def shutil_which(command):
    if "/" in command:
        return os.path.isfile(command) and os.access(command, os.X_OK)
    for directory in os.getenv("PATH", "").split(os.pathsep):
        path = os.path.join(directory, command)
        if os.path.isfile(path) and os.access(path, os.X_OK):
            return path
    return None


if __name__ == "__main__":
    raise SystemExit(main())

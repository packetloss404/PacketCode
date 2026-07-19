#!/usr/bin/env python3
"""Deterministic PTY/cell capture for packetcode and optional Claude Code."""
import argparse
import json
import os
import pathlib
import pty
import select
import shlex
import signal
import struct
import subprocess
import termios
import time

try:
    import pyte
except ImportError:
    pyte = None

SIZES = ((100, 30), (120, 40))


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


def capture(command, width, height, keys, settle, timeout):
    master, slave = pty.openpty()
    # TIOCSWINSZ takes rows, columns.
    import fcntl
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
    env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", NO_COLOR="0")
    proc = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave,
                            env=env, start_new_session=True, close_fds=True)
    os.close(slave)
    raw = bytearray()
    answered = set()
    try:
        # Wait for startup rendering to settle before typing. Sending keys
        # immediately races slower TUIs (notably Claude Code), which may clear
        # the screen after the keys land and produce a false comparison.
        drain_until_quiet(master, raw, settle, timeout, answered)
        for key in keys:
            os.write(master, key.encode().decode("unicode_escape").encode())
            drain_until_quiet(master, raw, settle, timeout, answered)
    finally:
        if proc.poll() is None:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=1)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
        os.close(master)
    if pyte is None:
        raise RuntimeError("pyte is required for cell normalization: python3 -m pip install pyte")
    screen = pyte.Screen(width, height)
    stream = pyte.Stream(screen)
    stream.feed(raw.decode("utf-8", "replace"))
    lines = [line.rstrip() for line in screen.display]
    while lines and not lines[-1]:
        lines.pop()
    return bytes(raw), "\n".join(lines) + ("\n" if lines else "")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", choices=("packetcode", "claude"), required=True)
    ap.add_argument("--command", help="command override (otherwise PACKETCODE_CMD/CLAUDE_CMD)")
    ap.add_argument("--scenario", default="welcome")
    ap.add_argument("--keys", action="append", default=[], help=r"keys to send; supports escapes such as \n")
    ap.add_argument("--output", default="testdata/tui/captures")
    ap.add_argument("--settle", type=float, default=0.35)
    ap.add_argument("--timeout", type=float, default=5.0)
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
    for width, height in SIZES:
        raw, cells = capture(argv, width, height, args.keys, args.settle, args.timeout)
        stem = root / f"{width}x{height}"
        stem.with_suffix(".ansi").write_bytes(raw)
        stem.with_suffix(".txt").write_text(cells)
        stem.with_suffix(".json").write_text(json.dumps({"target": args.target, "scenario": args.scenario, "width": width, "height": height}, indent=2) + "\n")
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

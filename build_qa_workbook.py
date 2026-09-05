#!/usr/bin/env python3
"""Build the packetcode QA workbook as a printable PDF.

    python build_qa_workbook.py [-o packetcode-qa-workbook.pdf]

Why this exists. The automated suite covers what can be asserted from code:
`go test ./...` for the packages, `smoke.sh` for the seams. What neither can
cover is the part a person has to look at, which for a terminal UI is most of
the product: whether the approval prompt reads clearly, whether the mode footer
says what is actually in force, whether a denial is legible, whether the
screenshots still match the docs. This workbook is that pass, on paper, so it
can be done without a model in the loop and the evidence survives the session.

Content is the audit of 2026-09-05: see docs/audit/security-audit-2026-09-05.md
for the reasoning behind every patch row and unresolved experiment here, and
docs/runbooks.md for the operational procedures the ops pages index.

The only dependency is reportlab. Everything is ASCII: no box-drawing or check
glyphs, because a workbook that is going to be printed on an unknown printer
should not depend on font coverage. A checkbox here is an empty bordered table
cell, drawn rather than typed.
"""

from __future__ import annotations

import argparse
import datetime as _dt

from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import inch
from reportlab.pdfgen import canvas as pdfcanvas
from reportlab.platypus import (
    KeepTogether,
    PageBreak,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)

# ---------------------------------------------------------------- page setup

PAGE = letter
MARGIN = 0.85 * inch
CONTENT_WIDTH = PAGE[0] - 2 * MARGIN

TITLE = "packetcode QA Workbook"
SUBTITLE = "Manual verification pass for the 2026-09-05 security audit"

INK = colors.HexColor("#111111")
MUTED = colors.HexColor("#5A5A5A")
RULE = colors.HexColor("#9A9A9A")
BAND = colors.HexColor("#ECECEC")

# Helvetica throughout, per the workbook spec.
BODY = ParagraphStyle(
    "body", fontName="Helvetica", fontSize=9.5, leading=12.5,
    textColor=INK, alignment=TA_LEFT, spaceAfter=6,
)
SMALL = ParagraphStyle("small", parent=BODY, fontSize=8.5, leading=11, spaceAfter=4)
CELL = ParagraphStyle("cell", parent=BODY, fontSize=8.5, leading=10.8, spaceAfter=0)
CELL_HEAD = ParagraphStyle("cellhead", parent=CELL, fontName="Helvetica-Bold")
MONO = ParagraphStyle(
    "mono", parent=BODY, fontName="Courier", fontSize=8, leading=10.5,
    textColor=INK, spaceAfter=6, leftIndent=8,
)
H1 = ParagraphStyle(
    "h1", parent=BODY, fontName="Helvetica-Bold", fontSize=15, leading=19,
    spaceBefore=0, spaceAfter=9,
)
H2 = ParagraphStyle(
    "h2", parent=BODY, fontName="Helvetica-Bold", fontSize=11, leading=14,
    spaceBefore=11, spaceAfter=5,
)
NOTE = ParagraphStyle("note", parent=BODY, fontSize=8.5, leading=11, textColor=MUTED)


class NumberedCanvas(pdfcanvas.Canvas):
    """Two-pass canvas so the footer can say 'Page N of M'.

    The page count is not known until the document is finished, so every page
    is buffered, then replayed with the decorations drawn on top.
    """

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._pages = []

    def showPage(self):
        self._pages.append(dict(self.__dict__))
        self._startPage()

    def save(self):
        total = len(self._pages)
        for index, state in enumerate(self._pages, start=1):
            self.__dict__.update(state)
            if index > 1:  # the cover carries no running header
                self._decorate(index, total)
            super().showPage()
        super().save()

    def _decorate(self, page: int, total: int) -> None:
        y = PAGE[1] - MARGIN + 0.22 * inch
        self.setFont("Helvetica", 8)
        self.setFillColor(MUTED)
        self.drawString(MARGIN, y, TITLE)
        self.drawRightString(PAGE[0] - MARGIN, y, BUILD_STAMP)
        self.setStrokeColor(RULE)
        self.setLineWidth(0.5)
        self.line(MARGIN, y - 0.06 * inch, PAGE[0] - MARGIN, y - 0.06 * inch)

        foot = MARGIN - 0.34 * inch
        self.line(MARGIN, foot + 0.16 * inch, PAGE[0] - MARGIN, foot + 0.16 * inch)
        self.drawString(MARGIN, foot, "Tester: ____________________   Date: ____________")
        self.drawRightString(PAGE[0] - MARGIN, foot, "Page %d of %d" % (page, total))


BUILD_STAMP = "audit 2026-09-05"


# ---------------------------------------------------------------- helpers

def para(text: str, style: ParagraphStyle = BODY) -> Paragraph:
    return Paragraph(text, style)


def bullets(items: list[str], style: ParagraphStyle = BODY) -> list:
    return [Paragraph("- " + item, style) for item in items]


def checklist(rows: list[tuple[str, str]], id_head: str = "ID",
              what_head: str = "Check") -> Table:
    """A test sheet: id, what to check, an empty done box, and P / F cells.

    The three trailing cells are deliberately empty. Their borders are the
    checkbox; nothing is typed into them, so no glyph coverage is required.
    """
    data = [[
        para(id_head, CELL_HEAD), para(what_head, CELL_HEAD),
        para("Done", CELL_HEAD), para("P", CELL_HEAD), para("F", CELL_HEAD),
    ]]
    for ident, what in rows:
        data.append([para(ident, CELL), para(what, CELL), "", "", ""])
    table = Table(
        data,
        colWidths=[0.62 * inch, 4.41 * inch, 0.55 * inch, 0.61 * inch, 0.61 * inch],
        repeatRows=1,
    )
    table.setStyle(TableStyle([
        ("GRID", (0, 0), (-1, -1), 0.5, RULE),
        ("BACKGROUND", (0, 0), (-1, 0), BAND),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("ALIGN", (2, 0), (-1, -1), "CENTER"),
        ("TOPPADDING", (0, 0), (-1, -1), 4),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
        ("LEFTPADDING", (0, 0), (-1, -1), 5),
        ("RIGHTPADDING", (0, 0), (-1, -1), 5),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#F7F7F7")]),
    ]))
    return table


def grid(headers: list[str], rows: list[list[str]], widths: list[float],
         box_last: bool = True) -> Table:
    """A reference table. When box_last is set the final column is left empty
    as a tick box."""
    data = [[para(h, CELL_HEAD) for h in headers]]
    for row in rows:
        cells = [para(c, CELL) for c in row]
        if box_last:
            cells.append("")
        data.append(cells)
    table = Table(data, colWidths=widths, repeatRows=1)
    table.setStyle(TableStyle([
        ("GRID", (0, 0), (-1, -1), 0.5, RULE),
        ("BACKGROUND", (0, 0), (-1, 0), BAND),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("TOPPADDING", (0, 0), (-1, -1), 4),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
        ("LEFTPADDING", (0, 0), (-1, -1), 5),
        ("RIGHTPADDING", (0, 0), (-1, -1), 5),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#F7F7F7")]),
    ]))
    return table


def screenshot_slot(shot_id: str, caption: str, height: float = 2.75) -> Table:
    """An empty framed box to tape or paste a capture into, with a caption
    line naming the file it should be saved as."""
    data = [
        [para("<b>%s</b>  %s" % (shot_id, caption), CELL)],
        [""],
        [para("file: ______________________________    "
              "size: __________    matches docs: Y / N", CELL)],
    ]
    table = Table(data, colWidths=[CONTENT_WIDTH], rowHeights=[0.28 * inch,
                                                              height * inch,
                                                              0.26 * inch])
    table.setStyle(TableStyle([
        ("BOX", (0, 0), (-1, -1), 0.7, RULE),
        ("LINEBELOW", (0, 0), (0, 0), 0.5, RULE),
        ("LINEABOVE", (0, 2), (0, 2), 0.5, RULE),
        ("BACKGROUND", (0, 0), (0, 0), BAND),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("LEFTPADDING", (0, 0), (-1, -1), 6),
        ("RIGHTPADDING", (0, 0), (-1, -1), 6),
    ]))
    return table


def blank_rows(headers: list[str], widths: list[float], count: int,
               height: float = 0.44) -> Table:
    data = [[para(h, CELL_HEAD) for h in headers]]
    data.extend([[""] * len(headers) for _ in range(count)])
    heights = [0.26 * inch] + [height * inch] * count
    table = Table(data, colWidths=widths, rowHeights=heights, repeatRows=1)
    table.setStyle(TableStyle([
        ("GRID", (0, 0), (-1, -1), 0.5, RULE),
        ("BACKGROUND", (0, 0), (-1, 0), BAND),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 5),
    ]))
    return table


# ---------------------------------------------------------------- content

def cover() -> list:
    field = "________________________________"
    rows = [
        ["Build version (packetcode --version)", field],
        ["Commit", field],
        ["Platform (OS, terminal, shell)", field],
        ["Go toolchain (go version)", field],
        ["Tester", field],
        ["Date started / finished", field],
        ["Result: all pass / failures logged", field],
    ]
    table = grid(["Field", "Value"], rows, [3.0 * inch, 3.8 * inch], box_last=False)
    return [
        Spacer(1, 1.5 * inch),
        para(TITLE, ParagraphStyle("cover", parent=H1, fontSize=25, leading=30)),
        para(SUBTITLE, ParagraphStyle("covsub", parent=BODY, fontSize=12,
                                      leading=16, textColor=MUTED)),
        Spacer(1, 0.35 * inch),
        para(
            "This workbook covers what automation cannot: the surfaces a person "
            "has to look at. Run the automated checks first and record their "
            "result on the patch verification pages, then work through the test "
            "sheets in order. Write down what you observed, not the word ok: a "
            "workbook full of ticks and no values is not evidence.",
            BODY,
        ),
        Spacer(1, 0.3 * inch),
        table,
        Spacer(1, 0.4 * inch),
        para(
            "Sources. Findings and patch rows: docs/audit/security-audit-2026-09-05.md. "
            "Procedures referenced by the ops pages: docs/runbooks.md. "
            "What is deliberately still open: docs/handoff.md.",
            NOTE,
        ),
        PageBreak(),
    ]


def how_to_use() -> list:
    return [
        para("How to use this workbook", H1),
        para("Conventions", H2),
        *bullets([
            "Done is the left box: tick it when the step has been performed.",
            "P and F are the outcome: mark one. A step performed but not judged "
            "is not a result.",
            "Where a step asks for a value, write the value. Exit codes, counts, "
            "and file paths are the evidence; a tick is not.",
            "On any F, open the bug log at the back, give it the next BUG-nn, and "
            "note the test id beside it.",
        ]),
        para("Set up an isolated environment first", H2),
        para(
            "Every test sheet below assumes a throwaway data home, so a failed "
            "run cannot damage real sessions, jobs, or credentials. Nothing in "
            "the workbook should be run against your working home.",
            BODY,
        ),
        para(
            "export PACKETCODE_HOME=/tmp/pc-qa            # must be absolute<br/>"
            "export PACKETCODE_LOG_FILE=/tmp/pc-qa/qa.jsonl<br/>"
            "mkdir -p /tmp/pc-qa &amp;&amp; cd /tmp/pc-qa-project &amp;&amp; git init -q",
            MONO,
        ),
        para(
            "On Windows PowerShell use $env:PACKETCODE_HOME with a full path such "
            "as C:\\Temp\\pc-qa. A relative path is refused by design.",
            NOTE,
        ),
        para("Run the automated pass before the manual one", H2),
        para(
            "go build ./... &amp;&amp; go vet ./...<br/>"
            "go test -count=1 ./...<br/>"
            "bash smoke.sh<br/>"
            "make vulncheck",
            MONO,
        ),
        para(
            "If the internal/jobs package hangs or fails, rerun it alone before "
            "believing it (runbook R16); its tests wait on real timers and are "
            "starved by concurrent compilation.",
            NOTE,
        ),
        para("Time budget", H2),
        para(
            "The automated pass is about five minutes. The manual sheets are "
            "roughly ninety minutes if nothing fails, most of it in the "
            "permissions and background job sections.",
            BODY,
        ),
        PageBreak(),
    ]


def surface_inventory() -> list:
    cli = [
        ["packetcode", "TUI. First run walks setup when no provider is configured.", "local user"],
        ["packetcode run", "One headless turn. Approvals fail closed with exit 3.", "local user, scripts"],
        ["packetcode doctor", "Diagnostics. Runs no hooks, MCP servers, or providers.", "local user"],
        ["packetcode skills", "list / install / remove / path. install runs git clone.", "local user"],
        ["packetcode acp", "ACP v1 JSON-RPC over stdio.", "local user, then the client"],
        ["packetcode sugar login", "OAuth device flow; writes the token to config.toml.", "local user"],
    ]
    acp = [
        ["initialize", "Required first. Advertises permissionModes and the ceiling."],
        ["session/new", "Client picks cwd and may supply MCP server commands."],
        ["session/load", "Resumes a persisted session and replays it."],
        ["session/prompt", "Drives the agent loop. Expands markdown slash commands."],
        ["session/cancel, session/close", "Cancel a turn; release a runtime."],
        ["_packetcode/*", "sessions list/rename/usage, models, mcp, commands, files."],
    ]
    tools = [
        ["read_file, search_codebase, list_directory", "read-only, no approval; refuse dotenv files"],
        ["list_symbols, find_definition, find_references, get_diagnostics", "read-only, no approval"],
        ["write_file, patch_file", "approval; backed up for undo"],
        ["execute_command", "approval; full shell as your user"],
        ["fetch", "approval; blocks private and loopback addresses"],
        ["spawn_agent, collect_agent_results", "background agents; worktree when writing"],
        ["skill, todo_write, read_tool_output", "read-only, no approval"],
        ["<server>__<tool>", "MCP; approval under every profile except bypass"],
    ]
    inputs = [
        ["~/.packetcode/config.toml", "operator", "everything"],
        ["~/.packetcode/.env, <project>/.env", "operator or repository", "provider keys only"],
        [".packetcode/commands/*.md", "repository", "slash verbs, ACP catalogue"],
        [".packetcode/workflows/*.toml", "repository", "workflow steps, system_prompt"],
        [".packetcode/skills, .claude/skills, .agents/skills", "repository", "skill tool; foreign layouts need approval"],
        ["~/.codex/auth.json", "Codex CLI", "codex provider, refreshed in place"],
    ]
    return [
        para("Surface inventory", H1),
        para(
            "Confirm each surface still exists and behaves as described. Tick the "
            "right-hand box when confirmed. Anything here that has moved since "
            "the audit is itself a finding.",
            BODY,
        ),
        para("Command line", H2),
        grid(["Command", "What it is", "Who can reach it", "OK"],
             cli, [1.55 * inch, 3.05 * inch, 1.6 * inch, 0.6 * inch]),
        para("ACP methods (stdio JSON-RPC)", H2),
        grid(["Method", "What it does", "OK"],
             acp, [1.9 * inch, 4.3 * inch, 0.6 * inch]),
        PageBreak(),
        para("Surface inventory, continued", H1),
        para("Model-callable tools", H2),
        grid(["Tool", "Gate under the ask profile", "OK"],
             tools, [3.05 * inch, 3.15 * inch, 0.6 * inch]),
        para("File-shaped inputs", H2),
        grid(["Path", "Author", "Consumer", "OK"],
             inputs, [2.5 * inch, 1.35 * inch, 2.35 * inch, 0.6 * inch]),
        para(
            "Note the author column. Four of these are written by whatever "
            "repository happens to be open, which is the trust boundary audit "
            "finding F-08 is about.",
            NOTE,
        ),
        PageBreak(),
    ]


def shot_list() -> list:
    shots = [
        ["SHOT-01", "Welcome screen, 80x24", "packetcode with no arguments, fresh home"],
        ["SHOT-02", "Approval prompt for write_file", "ask mode, ask the model to create a file"],
        ["SHOT-03", "Approval prompt for execute_command", "ask mode, ask it to run a command"],
        ["SHOT-04", "Permission mode footer, all four", "press Shift+Tab through the cycle"],
        ["SHOT-05", "Bypass mode indicator", "/trust on"],
        ["SHOT-06", "Denial message", "read-only mode, ask for a file write"],
        ["SHOT-07", "Dotenv refusal", "ask the model to read .env"],
        ["SHOT-08", "Agent View, mixed states", "/spawn twice, then /agents"],
        ["SHOT-09", "Job transcript", "/jobs <id>"],
        ["SHOT-10", "Workflow view", "/workflows run review"],
        ["SHOT-11", "Provider picker", "Ctrl+P"],
        ["SHOT-12", "Model picker", "/model"],
        ["SHOT-13", "MCP table", "/mcp with at least one server configured"],
        ["SHOT-14", "doctor output with a warning", "config with an unset env_from"],
        ["SHOT-15", "Narrow layout, 72x24", "resize, then /help"],
    ]
    return [
        para("Screenshot shot list", H1),
        para(
            "Capture each at a known terminal size and record it. These are the "
            "surfaces documentation and release notes point at; a stale "
            "screenshot is a documentation defect. Save as "
            "qa-&lt;shot-id&gt;-&lt;WIDTHxHEIGHT&gt;.png.",
            BODY,
        ),
        grid(["ID", "What to capture", "How to get there", "Taken"],
             shots, [0.75 * inch, 2.35 * inch, 3.1 * inch, 0.6 * inch]),
        para(
            "Before sharing any capture, check it for account names, absolute "
            "paths, hostnames, and tokens. The repository ignores "
            "testdata/tui/captures for exactly this reason.",
            NOTE,
        ),
        PageBreak(),
    ]


def screenshot_pages() -> list:
    slots = [
        ("SHOT-01", "Welcome screen, 80x24"),
        ("SHOT-02", "Approval prompt, write_file"),
        ("SHOT-03", "Approval prompt, execute_command"),
        ("SHOT-04", "Permission mode footer"),
        ("SHOT-06", "Denial message, read-only"),
        ("SHOT-07", "Dotenv refusal"),
        ("SHOT-08", "Agent View, mixed states"),
        ("SHOT-13", "MCP table"),
    ]
    flow: list = []
    for index in range(0, len(slots), 2):
        flow.append(para("Screenshot evidence", H1 if index == 0 else H1))
        for shot_id, caption in slots[index:index + 2]:
            flow.append(screenshot_slot(shot_id, caption))
            flow.append(Spacer(1, 0.16 * inch))
        flow.append(PageBreak())
    return flow


def test_sheets() -> list:
    a = [
        ("A-01", "packetcode --version prints a version and a commit, exits 0. Record both."),
        ("A-02", "packetcode --help lists run, doctor, skills, acp, and sugar."),
        ("A-03", "First run with an empty home walks provider setup and writes config.toml."),
        ("A-04", "config.toml is mode 0600 on POSIX. Record the mode."),
        ("A-05", "doctor exits 0 on a healthy config and 1 when a check fails. Record both exit codes."),
        ("A-06", "doctor --json is valid JSON and carries schema_version."),
        ("A-07", "Add a key no setting matches. Startup names it and doctor reports config.compatibility."),
        ("A-08", "Add an [mcp.x] block with env_from naming an unset variable. Startup names the variable "
                 "and doctor reports config.validation. Record the exact line."),
        ("A-09", "Set schema_version = 99. Startup says the settings after 1 are ignored, and does not refuse to start."),
        ("A-10", "Set PACKETCODE_HOME to a relative path. It is refused with a message naming the variable."),
    ]
    b = [
        ("B-01", "With the key only in the environment, doctor reports env:<VAR> and never the key."),
        ("B-02", "With the key only in <project>/.env, doctor reports dotenv:<path>."),
        ("B-03", "With the key in both, the environment wins. Confirm doctor says env."),
        ("B-04", "With no key for the default provider, startup names the exact variable to set."),
        ("B-05", "Wrong key: the run fails and the error names the status. The key is not printed."),
        ("B-06", "grep the terminal output and any log for the key value. It appears nowhere."),
        ("B-07", "Gemini only: with the log on, provider.http URLs carry no query string."),
        ("B-08", "codex: with no ~/.codex/auth.json, doctor explains that codex login is needed."),
    ]
    c = [
        ("C-01", "Shift+Tab cycles Manual, Accept Edits, Auto, Plan. The footer matches each."),
        ("C-02", "Bypass is not in the cycle. /trust on enters it and the footer is distinct."),
        ("C-03", "/trust off restores the previous profile and keeps session rules."),
        ("C-04", "In ask mode a write prompts. Choosing No feeds a refusal back and writes nothing."),
        ("C-05", "Choosing yes-and-do-not-ask-again stops the prompt for that exact command only. "
                 "Confirm a different command still prompts."),
        ("C-06", "/permissions reset revokes remembered rules and restores the startup policy."),
        ("C-07", "Plan mode denies a write even with an allow rule present."),
        ("C-08", "A deny rule on command_prefix survives a compound command: `echo X; :` is still denied."),
        ("C-09", "A command routed through an interpreter (`sh -c ...`) escalates to a prompt rather than "
                 "silently running."),
        ("C-10", "Changing mode while an approval is on screen resolves it. The running command is not killed."),
        ("C-11", "packetcode run in ask mode exits 3 when approval is needed and writes nothing."),
    ]
    d = [
        ("D-01", "read_file .env is refused and the message explains why."),
        ("D-02", "read_file .env.example succeeds. The exception list is intact."),
        ("D-03", "search_codebase for a string that only exists in .env returns no hit from that file."),
        ("D-04", "@.env in a prompt attaches nothing. The secret is not in the transcript."),
        ("D-05", "A symlink inside the project pointing outside it is not attached by @mention."),
        ("D-06", "A symlink pointing inside the project is still attached."),
        ("D-07", "read_file with ../../etc/passwd or an absolute path outside the root is refused."),
        ("D-08", "fetch to http://127.0.0.1:<port> is refused as a loopback address."),
        ("D-09", "fetch output is wrapped in the untrusted-content boundary and the markers cannot be forged."),
        ("D-10", "A project skill body is labelled as repository content when loaded."),
    ]
    e = [
        ("E-01", "/spawn starts a read-only job. It appears in /agents and cannot write."),
        ("E-02", "/spawn --write creates a worktree and branch. Record both paths."),
        ("E-03", "The worktree is based on committed HEAD: uncommitted foreground edits are absent."),
        ("E-04", "A write job's approval prompt is labelled with its job id."),
        ("E-05", "A pending approval shows the approval state, not the question state, in Agent View."),
        ("E-06", "/cancel <id> stops a job and it reports cancelled, not failed."),
        ("E-07", "Kill packetcode mid-job. On restart the job reports abandoned, with a cause."),
        ("E-08", "/jobs resubmit lists it and starts a new job. The original keeps its state and evidence."),
        ("E-09", "Resubmitting the same job twice is refused."),
        ("E-10", "Concurrency cap holds: spawn five jobs with a cap of four and confirm one queues."),
    ]
    f = [
        ("F-01", "A configured server starts and its tools appear as <server>__<tool>."),
        ("F-02", "An MCP tool call prompts under ask, accept-edits, and auto."),
        ("F-03", "/mcp status names the server version and the last error."),
        ("F-04", "/mcp logs redacts token-shaped values. Confirm with a server that logs one."),
        ("F-05", "/mcp restart replaces the process without disturbing other servers."),
        ("F-06", "A server that fails to start does not prevent packetcode from opening."),
        ("F-07", "A server named with a path separator is refused at config validation."),
    ]
    g = [
        ("G-01", "initialize advertises permissionModes. Record the list."),
        ("G-02", "With a custom or empty profile, bypass is NOT offered and is refused if requested."),
        ("G-03", "session/new with a relative cwd is refused with invalid params."),
        ("G-04", "session/prompt expands a markdown slash command."),
        ("G-05", "session/cancel ends the turn and reports stopReason cancelled."),
        ("G-06", "session/close releases the runtime and kills that session's MCP children."),
        ("G-07", "Two clients cannot run a prompt on one session at once: the second is told it is busy."),
    ]
    h = [
        ("H-01", "Finalized output lands in terminal scrollback and survives Ctrl+L."),
        ("H-02", "Mouse tracking and the alternate screen are never enabled. Selection still works."),
        ("H-03", "Ctrl+C cancels a turn; a second press during teardown does not exit."),
        ("H-04", "Ctrl+C with a draft clears the draft; from an empty prompt it exits."),
        ("H-05", "Resize from 100x30 to 72x24 mid-session repaints without stale chrome."),
        ("H-06", "Tool output containing escape sequences cannot move the cursor or set the clipboard."),
        ("H-07", "A backslash then Enter inserts a newline in every input state."),
    ]
    sheets = [
        ("TS-A  Startup, configuration, diagnostics", a),
        ("TS-B  Credentials", b),
        ("TS-C  Permissions and approvals", c),
        ("TS-D  Tool security refusals", d),
        ("TS-E  Background jobs and worktrees", e),
        ("TS-F  MCP", f),
        ("TS-G  ACP", g),
        ("TS-H  Terminal behaviour", h),
    ]
    flow: list = []
    for title, rows in sheets:
        flow.append(para(title, H1))
        flow.append(checklist(rows))
        flow.append(PageBreak())
    return flow


def patch_verification() -> list:
    rows = [
        ("P01", "dd71133 gemini key in a header",
         "Log on, run one Gemini turn, grep the log for query strings"),
        ("P02", "6fece2e dotenv refusal",
         "TS-D-01 through D-04; smoke.sh section 6"),
        ("P03", "d4f820c mention symlink escape",
         "TS-D-05 and D-06"),
        ("P05", "571fa98 approval vs question state",
         "TS-E-05; Agent View shows the approval icon, not the question icon"),
        ("P06", "f82868e cost tally fsync",
         "Run a turn, confirm cost-tally.json parses and /cost is non-zero"),
        ("P12", "f04688d ACP permission ceiling",
         "TS-G-01 and G-02 with a custom profile configured"),
        ("P08", "195060b boot config validation",
         "TS-A-07 and A-08"),
        ("P07", "cea8ef7 diagnostic log",
         "Set PACKETCODE_LOG_FILE, run a turn, confirm one JSON object per line"),
        ("P10a", "d61a919 x/crypto v0.43.0",
         "make vulncheck: GO-2025-4116 is gone"),
        ("P11", "7aa0951 dead helper removed",
         "go build ./... succeeds"),
        ("9", "e0ce868 smoke test",
         "bash smoke.sh reports 27 passed, 0 failed"),
    ]
    auto = [
        ("V-01", "go build ./... succeeds. Record the Go version used."),
        ("V-02", "go vet ./... is clean."),
        ("V-03", "go test -count=1 ./... passes. Record the package count and any skips."),
        ("V-04", "bash smoke.sh reports 27 passed, 0 failed."),
        ("V-05", "make vulncheck. Record the number of reachable advisories and compare with 16."),
        ("V-06", "go mod verify reports all modules verified."),
    ]
    return [
        para("Patch verification", H1),
        para(
            "One row per audit change. Confirm the behaviour, not just that the "
            "commit is present: a reverted or half-applied patch looks identical "
            "in git log.",
            BODY,
        ),
        grid(["Patch", "Commit and subject", "How to verify", "OK"],
             [[a, b, c] for a, b, c in rows],
             [0.6 * inch, 2.35 * inch, 3.25 * inch, 0.6 * inch]),
        para("Automated gates", H2),
        checklist(auto, id_head="ID", what_head="Gate"),
        PageBreak(),
    ]


def unresolved() -> list:
    rows = [
        ("U-01", "Can anything other than a same-user editor write to packetcode acp stdin? "
                 "Inspect how PacketADE launches it and whether it owns both pipe ends. "
                 "Answer: ______________________________________________"),
        ("U-02", "Does any real config use a plain http base_url to a non-loopback host? "
                 "Run: grep -n 'base_url = \"http://' ~/.packetcode/config.toml on every machine. "
                 "Answer: ______________________________________________"),
        ("U-03", "Move the Go floor to 1.26? CI already runs 1.26.3; README says 1.24.2. "
                 "Decision: ____________________________________________"),
        ("U-04", "Does anyone rely on read_file .env or @.env? "
                 "Run: grep -rl '\"path\":\".env\"' ~/.packetcode/sessions. "
                 "Answer: ______________________________________________"),
        ("U-05", "Does any ACP client request a mode above ask over a custom profile? "
                 "Check whether it reads _packetcode.permissionModes. "
                 "Answer: ______________________________________________"),
        ("U-06", "Is the internal/jobs hooks test flaky only on this machine? "
                 "Run: go test ./internal/jobs -run TestRunJob_PassesHooksToBackgroundAgent -count=20. "
                 "Failures: ____________________________________________"),
        ("U-07", "Should project commands and workflows be labelled untrusted like skills? "
                 "Decision: ____________________________________________"),
        ("U-08", "Is internal/doctor an empty directory in the primary checkout? "
                 "Run: ls -d internal/doctor. "
                 "Answer: ______________________________________________"),
        ("U-09", "Should collect_agent_results prompt in the foreground? "
                 "Decision: ____________________________________________"),
    ]
    return [
        para("Unresolved experiments", H1),
        para(
            "Each of these is a question the audit could not settle from the code "
            "alone, with the exact command or inspection that settles it. Record "
            "the answer here; several of them decide whether an open finding is "
            "closed or becomes a patch.",
            BODY,
        ),
        checklist(rows, id_head="ID", what_head="Question, command, and answer"),
        PageBreak(),
    ]


def ops_pages() -> list:
    runbooks = [
        ["R1", "A setting seems to do nothing", "doctor --check config"],
        ["R2", "Set, rotate, or remove a key", "doctor --check providers"],
        ["R3", "A key may have been exposed", "revoke first, then clean sessions"],
        ["R4", "Provider requests failing", "diagnostic log, provider.http"],
        ["R5", "Codex auth broken", "codex login"],
        ["R6", "MCP will not start", "/mcp status, /mcp logs, /mcp restart"],
        ["R7", "Job stuck or abandoned", "/jobs, /cancel, /jobs resubmit"],
        ["R8", "Worktree left behind", "git worktree remove"],
        ["R9", "Disk usage", "du -sh $PC/*"],
        ["R10", "Undo did not restore", "git is the real safety net"],
        ["R11", "Turn on the log", "PACKETCODE_LOG_FILE, absolute path"],
        ["R12", "Tool denied or allowed oddly", "/permissions explain"],
        ["R13", "SSH computer will not connect", "ssh.connect stage in the log"],
        ["R14", "Vulnerability report", "make vulncheck"],
        ["R15", "Verify a release", "cosign verify-blob"],
        ["R16", "Verify a build", "smoke.sh, then jobs alone if it hangs"],
        ["R17", "Reset to known good", "/permissions reset, move config.toml"],
    ]
    return [
        para("Operations reference", H1),
        para(
            "Full procedures are in docs/runbooks.md. This index is here so the "
            "workbook is usable away from a machine.",
            BODY,
        ),
        grid(["ID", "Situation", "First move", "Used"],
             runbooks, [0.55 * inch, 2.75 * inch, 2.9 * inch, 0.6 * inch]),
        para("Commands worth knowing by heart", H2),
        para(
            "packetcode doctor --json<br/>"
            "packetcode doctor --check config,providers,mcp<br/>"
            "PACKETCODE_LOG_FILE=/abs/path packetcode<br/>"
            "bash smoke.sh<br/>"
            "make vulncheck<br/>"
            "go test ./internal/jobs/ -count=1 -timeout 420s",
            MONO,
        ),
        para(
            "Data home layout: config.toml, sessions/, jobs/, worktrees/, backups/, "
            "commands/, skills/, workflows/, computers/registry.json, theme.toml, "
            "cost-tally.json, tool-output/, mcp-&lt;name&gt;.log, skill-approvals.json.",
            NOTE,
        ),
        PageBreak(),
    ]


def bug_log() -> list:
    headers = ["BUG", "Test id", "What happened, and what was expected", "Runbook"]
    widths = [0.6 * inch, 0.7 * inch, 4.9 * inch, 0.6 * inch]
    return [
        para("Bug log", H1),
        para(
            "One row per F. Write what you saw and what you expected; a "
            "reproduction is worth more than a judgement. Note the runbook if one "
            "applies.",
            BODY,
        ),
        blank_rows(headers, widths, 11),
        PageBreak(),
        para("Bug log, continued", H1),
        blank_rows(headers, widths, 15),
        PageBreak(),
    ]


def day31() -> list:
    rows = [
        ("K-01", "Move CI to Go 1.26.6: one line in ci.yml and release.yml. Clears nine "
                 "reachable stdlib advisories with no code change."),
        ("K-02", "Decide U-03. If yes, apply docs/audit/patches/P10b-*.patch and update the "
                 "Go version in README.md and HANDOFF.md."),
        ("K-03", "Add smoke-e2e to the ci target and to ci.yml. Watch the first macOS and "
                 "Linux runs; it has only been exercised on Windows."),
        ("K-04", "Answer U-01 and U-02. Each closes an open medium finding or produces a "
                 "small patch."),
        ("K-05", "Prune backups on startup. $PC/backups grows without bound and "
                 "BackupManager.Cleanup has no production caller."),
        ("K-06", "Decide F-11: collect_agent_results either prompts or is read-only. "
                 "One line either way."),
        ("K-07", "Route code intelligence through LocalBackend.Resolve and retire "
                 "internal/tools/safefs.go. Security boundary: needs its own review."),
        ("K-08", "Do not start Streamable HTTP MCP or the Packet Computers daemon in a "
                 "low-capability window. Both have written contracts to build against later."),
    ]
    return [
        para("Day 31 backlog", H1),
        para(
            "What to pick up when capacity returns, in order. The first three are "
            "small and high value; the last is a warning, not a task.",
            BODY,
        ),
        checklist(rows, id_head="ID", what_head="Work item"),
        Spacer(1, 0.2 * inch),
        para("Notes", H2),
        blank_rows(["", ""], [3.4 * inch, 3.4 * inch], 6),
    ]


def build(path: str) -> None:
    doc = SimpleDocTemplate(
        path,
        pagesize=PAGE,
        leftMargin=MARGIN, rightMargin=MARGIN,
        topMargin=MARGIN, bottomMargin=MARGIN,
        title=TITLE, author="packetcode audit", subject=SUBTITLE,
    )
    story: list = []
    story += cover()
    story += how_to_use()
    story += surface_inventory()
    story += shot_list()
    story += screenshot_pages()
    story += test_sheets()
    story += patch_verification()
    story += unresolved()
    story += ops_pages()
    story += bug_log()
    story += day31()
    doc.build(story, canvasmaker=NumberedCanvas)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-o", "--output", default="packetcode-qa-workbook.pdf",
                        help="output PDF path")
    args = parser.parse_args()
    build(args.output)
    print("wrote %s on %s" % (args.output, _dt.date.today().isoformat()))


if __name__ == "__main__":
    main()

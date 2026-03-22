# Improvement 1: Status Bars Are Information Overload at Tiny Font Sizes

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

`ClaudeStatusBar` and `CodexStatusBar` cram model name, context %, git branch, cost, duration, rate limits, and CLI version into a single bar at `text-[11px]`. On a 1080p monitor it's a wall of nearly unreadable text. Prioritize the 2–3 most important metrics and hide the rest behind a hover or expandable section.

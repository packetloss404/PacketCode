# Improvement 2: No Visual Feedback When AI Commands Are Running

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

Clicking "Scan Codebase" in Memory or "Analyze" in Code Quality gives no feedback. The backend runs `run_claude()` which can take 30+ seconds with no progress indicator, no spinner, and no "thinking..." state. Users are left wondering if the click registered.

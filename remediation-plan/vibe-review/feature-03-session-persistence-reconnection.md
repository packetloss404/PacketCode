# Feature Request 3: Session Persistence / Reconnection

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

If the app closes or crashes, all PTY sessions are gone forever. The `PtyManager` stores sessions in an in-memory `HashMap` — nothing survives a restart. Sessions should either persist and reconnect, or at minimum the scrollback history should be saved so it can be reviewed after reopening.

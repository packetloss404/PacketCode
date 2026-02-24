# Improvement 8: Insights Chat Has No Code Context Awareness

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

When asking a question in Insights, you have to manually describe which file or function you're asking about. The chat can't see what's in your terminal, can't reference open files, and can't pull in relevant code. The backend sends the message + conversation history to Claude CLI with the working directory set, but there's no file content injection.

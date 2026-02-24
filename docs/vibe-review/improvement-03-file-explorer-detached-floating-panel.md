# Improvement 3: The File Explorer Is a Detached Floating Panel

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

`FileExplorer.tsx` is draggable and floating, which sounds cool but in practice it covers the terminal panes. It should be a collapsible sidebar docked to the left, like every other IDE. It also has no integration — you can't drag a file into a prompt or open it in an editor.

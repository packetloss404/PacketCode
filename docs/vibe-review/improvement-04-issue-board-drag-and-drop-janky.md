# Improvement 4: Issue Board Drag-and-Drop Feels Janky

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

The Kanban board uses raw `onDragStart`/`onDragOver`/`onDrop` HTML5 drag events. There's no drag preview, no smooth animation, no visual placeholder showing where the card will land. It needs a proper DnD library like `@dnd-kit` or `react-beautiful-dnd` to match the polish of Trello or Linear.

# Feature Request 1: Streaming Responses in Insights Chat

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

`ask_insights` blocks until Claude finishes the entire response, then dumps it all at once. The backend uses `run_claude()` which is a single blocking call — it needs a streaming mode with chunked output piped back via Tauri events. Every other AI chat UI streams tokens in real-time; this should too.

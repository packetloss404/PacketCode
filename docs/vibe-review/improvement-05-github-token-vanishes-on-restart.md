# Improvement 5: GitHub Token Vanishes on Restart

> A candid review from a vibe coder who saw QuadCode on YouTube, couldn't get it, found PacketCode, and compiled it. The bones are solid — the multi-pane PTY system, the issue tracker, the dual Claude/Codex support — that's exactly what I was jealous of in QuadCode. But here's what would make it great.

`GitHubAuthState` is stored in a `RwLock<Option<String>>` in memory. Every app reopen requires re-pasting the token. This is maddening. The token should be encrypted and stored in the OS keychain via `tauri-plugin-stronghold` or at minimum persisted to disk.

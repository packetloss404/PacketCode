# Completed Foundation Roadmap

> Historical record. All items below shipped; current work is tracked only in [BACKLOG.md](../BACKLOG.md).

The original seven-round roadmap established the current TUI and extension foundation:

1. Slash-command parsing and session/provider/model commands.
2. Provider and model picker modals.
3. Slash-command autocomplete.
4. Unified diff previews and richer tool rendering.
5. Real foreground HTTP/tool cancellation.
6. User-configurable semantic themes.
7. MCP stdio tools.

Later work built on that foundation:

- Codex subscription auth, refreshed providers, and native Ollama management.
- Claude Code-inspired flat rendering, statusline, permission-mode footer, and lifecycle states.
- `@` file completion and bounded context expansion.
- Background worktree isolation, artifact manifests, Agent View, result collection, and persistence.
- `/loop` and the current phase/parallel `/workflows` engine.
- Context-occupancy correction, automatic compaction, request estimation, prompt caching, and token boundaries.

The original specs were converted into current-state docs under `docs/feature-*.md`. Use the git history for pre-implementation details.

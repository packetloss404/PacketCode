# Diff and Tool Rendering

packetcode uses a presentation-only unified-diff component for approval previews and completed patch results.

## Behavior

- `write_file` previews a new-file or replacement diff before approval.
- `patch_file` applies proposed operations to an in-memory copy and previews the resulting unified diff.
- Added, removed, and context lines use semantic theme colors with old/new line-number gutters.
- Headers show file names and hunk metadata.
- Height and width are bounded; omitted lines are identified rather than silently lost.
- Preview failure falls back to readable parameters and an error instead of blocking the approval UI.

The same parser/renderer is used after a patch completes, keeping proposed and completed output visually consistent. Diff rendering never mutates files; mutation remains in the tool implementation after permission approval.

## Approval Flow

The flat numbered approval menu displays the specialized diff body, then:

1. Yes
2. Yes, and do not ask again
3. No

Arrow keys, Enter, number keys, and legacy `Y`/`A`/`N` shortcuts work. Large previews wrap inside the body width and do not shift selector indentation.

## Limits

- Unified layout only; no side-by-side or word-level diff.
- No per-line acceptance.
- No binary preview.
- Full file/worktree inspection remains the source of truth for large changes.

Implementation: `internal/ui/components/diff`, `internal/ui/components/approval`, and the write/patch tool preview helpers.

# PacketCode Code Audit and Remediation Status - 2026-02-24

## Scope
- Reviewed frontend (`src`) and backend (`src-tauri/src`) for redundant loops, dead code, stale architecture paths, and cleanup opportunities.
- Tracked each finding through remediation and re-validation.

## Remediation Status

### 1. PTY lifecycle and stale session cleanup
- Status: Remediated
- Implemented in:
  - `src-tauri/src/commands/pty.rs`
- Notes:
  - PTY child handle is retained per session.
  - `kill_pty` now removes sessions from manager state and kills child processes.
  - Sessions are removed on natural exit to prevent stale map growth.

### 2. Statusline polling and backend scan load
- Status: Remediated
- Implemented in:
  - `src-tauri/src/commands/statusline.rs`
  - `src/hooks/useStatusLine.ts`
  - `src/hooks/useCodexStatusLine.ts`
  - `src/hooks/useStatusLinePollerBase.ts`
  - `src/stores/statusLineStore.ts`
  - `src/stores/codexStatusLineStore.ts`
  - `src/stores/statusLineStoreUtils.ts`
- Notes:
  - Poll cadence moved from 2s to 5s.
  - Codex statusline processing now uses buffered first-line reads, in-memory cache, bounded full scans, and incremental tail parsing for changed files only.
  - Store merge includes stale-entry pruning.

### 3. Legacy JSONL session architecture
- Status: Remediated
- Implemented in:
  - `src-tauri/src/lib.rs`
  - `src-tauri/src/commands/mod.rs`
  - `src-tauri/src/claude/mod.rs`
  - `src/lib/tauri.ts`
- Notes:
  - Legacy JSONL session command path removed from backend command registration.
  - Legacy frontend JSONL session stack removed; runtime session behavior is PTY-only.

### 4. Non-reactive store access during render
- Status: Remediated
- Implemented in:
  - `src/components/views/ToolsView.tsx`
  - `src/components/layout/Toolbar.tsx`
- Notes:
  - Render-time `getState()` usage replaced with reactive selectors.

### 5. Test-file heuristic overmatching
- Status: Remediated
- Implemented in:
  - `src-tauri/src/commands/code_quality.rs`
  - `src/components/quality/CodeQualityModal.tsx`
- Notes:
  - Detection now uses strict patterns: `*.test.*`, `*.spec.*`, `/tests/`, `/__tests__/`.
  - UI explanation text aligned with implementation.

### 6. Hook dependency issues
- Status: Remediated
- Implemented in:
  - `src/components/views/GitHubView.tsx`
  - `src/hooks/useGitInfo.ts`
- Notes:
  - Dependency arrays updated to remove stale closure risk.

### 7. Duplicate statusline hook/store code
- Status: Remediated
- Implemented in:
  - `src/hooks/useStatusLinePollerBase.ts`
  - `src/stores/statusLineStoreUtils.ts`
- Notes:
  - Shared helper abstractions now back both Claude and Codex statusline flows.

### 8. Duplicate markdown rendering paths
- Status: Remediated
- Implemented in:
  - `src/components/common/MarkdownRenderer.tsx`
  - `src/components/views/InsightsView.tsx`
  - `src/components/views/GitHubView.tsx`
- Notes:
  - Active views now use a single shared markdown rendering path.

### 9. GitHub token persistence in localStorage
- Status: Remediated
- Implemented in:
  - `src-tauri/src/commands/github.rs`
  - `src-tauri/src/lib.rs`
  - `src/stores/githubStore.ts`
  - `src/lib/tauri.ts`
  - `src/types/github.ts`
- Notes:
  - Token moved to backend memory state only.
  - Frontend persistence now stores only non-secret repo selection metadata.
  - One-time migration path scrubs legacy stored token.

### 10. Mixed package manager artifacts
- Status: Remediated
- Implemented in:
  - `src-tauri/tauri.conf.json`
  - `README.md`
  - removed `package-lock.json`
- Notes:
  - pnpm is now the canonical workflow.

## Validation Snapshot
- `pnpm run lint`: pass (0 warnings, 0 errors)
- `pnpm run build`: pass
- `cargo check` / `cargo test`: not executed in this environment (`cargo` unavailable)

## Remaining Manual QA
1. PTY churn testing (create/kill/restart cycles) to verify no orphan/stale behavior under repeated use.
2. GitHub auth lifecycle validation (connect, operate, disconnect, restart requires re-entry) including legacy token migration behavior.
3. Statusline load test with many Codex session files to confirm bounded polling overhead and stale-status pruning in real usage.
4. Test-heuristic sanity check against representative file naming patterns in a sample project.

## Notes
- This file documents the current post-remediation state as of 2026-02-24.

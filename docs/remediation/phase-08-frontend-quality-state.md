# Phase 8: Frontend Quality & State Management

**Priority:** P2 — Next Sprint
**Timeline:** Weeks 3–4
**Effort:** Medium (1 day total)
**Risk Level:** Medium
**Owners:** Frontend Lead

---

## Overview

This phase addresses frontend code quality issues that affect reliability, maintainability, and developer experience. While none of these are direct security vulnerabilities, they create operational risks: silent data loss, confusing behavior on fresh installs, bypassed abstractions, and missing environment distinction.

---

## Finding F-13: Hardcoded Developer Path as Default

### Severity: Medium

### Description

The `layoutStore.ts` file ships with a hardcoded Windows-specific developer path as the default `projectPath`. On any machine other than the original developer's, this path is invalid, causing confusing errors or incorrect behavior on first launch.

### Evidence

```typescript
// src/stores/layoutStore.ts, line 26
projectPath: "D:\\projects\\PacketCode",
```

This path is used by every command that requires a `project_path` parameter — git operations, code quality analysis, Claude CLI commands, and the file explorer.

### Impact

- First launch on any other machine uses an invalid project path
- Git status polling, file explorer, and other features will error silently
- The path leaks a developer's local filesystem layout into the shipped codebase
- Inconsistent behavior until the user manually changes the project path

### Remediation

**Option A (Simplest):** Default to an empty string and require explicit path selection:

```typescript
export const useLayoutStore = create<LayoutStore>((set, get) => ({
  panes: [],
  activePaneId: "",
  projectPath: "",
  // ...
}));
```

Add a check in the UI that prompts the user to select a project directory if `projectPath` is empty.

**Option B (Better UX):** Resolve the current working directory on startup via a Tauri command:

```rust
// src-tauri/src/commands/fs.rs
#[tauri::command]
pub fn get_cwd() -> Result<String, String> {
    std::env::current_dir()
        .map(|p| p.to_string_lossy().to_string())
        .map_err(|e| format!("Failed to get CWD: {}", e))
}
```

```typescript
// Initialize on app startup
const cwd = await invoke<string>("get_cwd");
useLayoutStore.getState().setProjectPath(cwd);
```

**Option C (Persist across sessions):** Store the last-used project path in localStorage and restore it on launch:

```typescript
const savedPath = localStorage.getItem("packetcode:projectPath");
projectPath: savedPath || "",
```

### Risk of Change

Low. The default path is wrong for everyone except the original developer. Any change improves the situation.

---

## Finding F-16: localStorage Persistence Without Size Limits

### Severity: Medium

### Description

Multiple Zustand stores persist their state to `localStorage` with no size monitoring, no data pruning, and silent failure when the quota (~5–10MB) is exceeded. Empty `catch {}` blocks mean the application continues running with stale persisted state and no indication that writes are failing.

### Evidence

```typescript
// src/stores/issueStore.ts, lines 107-111
function saveState(state: IssueState) {
  try {
    localStorage.setItem("packetcode:issues", JSON.stringify(state));
  } catch {}  // <-- Silent failure
}
```

Stores that persist to localStorage:
- `packetcode:issues` — Issues, potentially growing indefinitely
- `packetcode:memory` — File maps, session summaries
- `packetcode:insights-sessions` — Full chat history with AI responses
- `packetcode:ideation` — Generated ideas
- `packetcode:profiles` — Agent profiles

Stores that read from localStorage on init:
- `issueStore.ts:97` — `JSON.parse(localStorage.getItem(...))` in `catch {}`
- `profileStore.ts:68` — Same pattern
- `memoryStore.ts:14` — Same pattern
- `ideationStore.ts:27,38` — Same pattern
- `insightsStore.ts:30,37` — Same pattern

### Impact

- When localStorage exceeds its quota, new writes fail silently
- The user continues working, believing their data is being saved
- On next launch, the stored data is stale (from the last successful write)
- Chat histories and issue data can grow indefinitely, making quota exhaustion inevitable for active users

### Remediation

**Step 1: Add storage monitoring utility:**

```typescript
// src/lib/storage.ts
export function getStorageUsage(): { used: number; available: number } {
  let totalSize = 0;
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    if (key?.startsWith("packetcode:")) {
      totalSize += (localStorage.getItem(key) || "").length * 2; // UTF-16
    }
  }
  return { used: totalSize, available: 5 * 1024 * 1024 }; // Approximate
}

export function safePersist(key: string, data: unknown): boolean {
  try {
    const json = JSON.stringify(data);
    localStorage.setItem(key, json);
    return true;
  } catch (e) {
    console.warn(`[PacketCode] Failed to persist ${key}:`, e);
    return false;
  }
}
```

**Step 2: Replace empty catch blocks with `safePersist`:**

```typescript
// BEFORE:
function saveState(state: IssueState) {
  try {
    localStorage.setItem("packetcode:issues", JSON.stringify(state));
  } catch {}
}

// AFTER:
function saveState(state: IssueState) {
  if (!safePersist("packetcode:issues", {
    issues: state.issues,
    nextId: state.nextId,
    lastUpdated: state.lastUpdated,
  })) {
    // Optional: notify user that data wasn't saved
  }
}
```

**Step 3: Implement data pruning for growing stores:**

For `insightsStore` (chat history):
```typescript
function pruneOldSessions(sessions: InsightSession[], maxSessions: number = 50): InsightSession[] {
  if (sessions.length <= maxSessions) return sessions;
  return sessions
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, maxSessions);
}
```

**Step 4 (Future): Migrate to Tauri filesystem storage:**

For production readiness, consider migrating from localStorage to Tauri's app data directory, which has no practical size limit:

```rust
// Store data in %APPDATA%/PacketCode/ or ~/Library/Application Support/PacketCode/
#[tauri::command]
pub fn save_app_data(key: String, data: String) -> Result<(), String> {
    let app_dir = dirs::data_dir()
        .ok_or("Cannot determine app data directory")?
        .join("PacketCode");
    std::fs::create_dir_all(&app_dir)
        .map_err(|e| format!("Failed to create app data dir: {}", e))?;
    std::fs::write(app_dir.join(format!("{}.json", key)), data)
        .map_err(|e| format!("Failed to write data: {}", e))
}
```

### Risk of Change

Low. Adding monitoring and replacing empty catch blocks only improves visibility. Data pruning should be opt-in with sensible defaults.

---

## Finding F-08b: Direct `invoke()` Calls Bypass Centralized IPC Wrapper

### Severity: Medium

### Description

The codebase has a centralized IPC wrapper module at `src/lib/tauri.ts` that provides typed function wrappers around Tauri `invoke()` calls. However, several components and stores call `invoke()` directly, bypassing this abstraction.

### Evidence

**Centralized wrapper (correct pattern):**

```typescript
// src/lib/tauri.ts
export async function getGitBranch(projectPath: string): Promise<string> {
  return invoke<string>("get_git_branch", { projectPath });
}
```

**Direct invoke calls (bypassing wrapper):**

```typescript
// src/stores/memoryStore.ts
const result = await invoke<string>("scan_codebase_memory", { projectPath });
const result = await invoke<string>("summarize_session", { projectPath, sessionLog });
const result = await invoke<string>("extract_patterns", { projectPath, summaries });
```

```typescript
// src/components/explorer/FileExplorer.tsx
const entries = await invoke<DirEntry[]>("list_directory", { dirPath: currentPath });
```

```typescript
// src/components/session/TerminalPane.tsx
invoke("create_pty_session", { projectPath, cols, rows, command, args });
invoke("write_pty", { sessionId: sid, data });
invoke("resize_pty", { sessionId: sid, cols, rows });
invoke("kill_pty", { sessionId: sid });
```

### Impact

- IPC calls are spread across multiple files instead of being centralized
- Type safety is inconsistent — direct calls may have incorrect type parameters
- Refactoring IPC commands (renaming, changing parameters) requires finding all call sites
- Audit logging or middleware for IPC calls cannot be applied consistently
- Input validation logic would need to be duplicated

### Remediation

Add wrapper functions to `src/lib/tauri.ts` for all missing commands:

```typescript
// src/lib/tauri.ts — add these wrappers

// Memory
export async function scanCodebaseMemory(projectPath: string): Promise<string> {
  return invoke<string>("scan_codebase_memory", { projectPath });
}
// (scanCodebaseMemory, summarizeSession, extractPatterns already exist in tauri.ts)

// Filesystem
export async function listDirectory(dirPath: string): Promise<DirEntry[]> {
  return invoke<DirEntry[]>("list_directory", { dirPath });
}

// PTY
export async function createPtySession(
  projectPath: string, cols: number, rows: number,
  command: string, args?: string[]
): Promise<string> {
  return invoke<string>("create_pty_session", { projectPath, cols, rows, command, args });
}

export async function writePty(sessionId: string, data: string): Promise<void> {
  return invoke("write_pty", { sessionId, data });
}

export async function resizePty(sessionId: string, cols: number, rows: number): Promise<void> {
  return invoke("resize_pty", { sessionId, cols, rows });
}

export async function killPty(sessionId: string): Promise<void> {
  return invoke("kill_pty", { sessionId });
}

// Code Quality
export async function analyzeCodeQuality(projectPath: string): Promise<CodeQualityReport> {
  return invoke<CodeQualityReport>("analyze_code_quality", { projectPath });
}
```

Then update the calling components to use the wrappers instead of direct `invoke()` calls.

### Risk of Change

Low. This is a refactoring that preserves behavior while improving maintainability.

---

## Finding F-08c: No Dev vs. Prod Environment Distinction

### Severity: Medium

### Description

The entire frontend codebase has zero references to `import.meta.env.DEV`, `import.meta.env.PROD`, `process.env.NODE_ENV`, or any environment detection. There is no conditional logic for development vs. production behavior.

### Evidence

A search across all `.ts` and `.tsx` files for `import.meta.env`, `process.env`, `NODE_ENV`, `DEV`, or `PROD` returns zero results.

### Impact

- No way to enable verbose console logging only in development
- No way to disable debug features in production
- The hardcoded `projectPath` (F-13) ships in all builds
- No feature flags for experimental features
- No way to point at different backend endpoints (if ever needed)
- DevTools-related code runs identically in dev and prod

### Remediation

**Step 1: Add environment-aware logging:**

```typescript
// src/lib/env.ts
export const isDev = import.meta.env.DEV;
export const isProd = import.meta.env.PROD;

export function devLog(context: string, ...args: unknown[]): void {
  if (isDev) {
    console.log(`[DEV:${context}]`, ...args);
  }
}
```

**Step 2: Use environment checks where appropriate:**

```typescript
// src/main.tsx
if (import.meta.env.DEV) {
  window.addEventListener("unhandledrejection", (event) => {
    console.error("[PacketCode:DEV] Unhandled rejection:", event.reason);
  });
}

// In production, errors should be reported to a service, not just logged
```

**Step 3: Gate development-only features:**

```typescript
// src/stores/layoutStore.ts
projectPath: import.meta.env.DEV ? "D:\\projects\\PacketCode" : "",
```

### Risk of Change

None. Adding environment checks is purely additive.

---

## Finding F-08d: `OtherViewContent` Uses `getState()` in Render

### Severity: Medium

### Description

The `OtherViewContent` component in `App.tsx` calls `useModuleStore.getState()` during render instead of using the hook's subscription mechanism. This means the component will not re-render when the module's enabled state changes.

### Evidence

```typescript
// src/App.tsx, line 187
if (!mod || !useModuleStore.getState().isEnabled(modId)) return null;
```

### Impact

- If a module is disabled while its view is active, the component won't re-render to reflect the change
- The UI can show stale state until something else triggers a re-render

### Remediation

Use the Zustand hook properly:

```typescript
// BEFORE:
function OtherViewContent({ view }: { view: AppView }) {
  const modId = getModuleId(view);
  if (!modId) return null;
  const mod = moduleRegistry.get(modId);
  if (!mod || !useModuleStore.getState().isEnabled(modId)) return null;
  // ...
}

// AFTER:
function OtherViewContent({ view }: { view: AppView }) {
  const modId = getModuleId(view);
  const isEnabled = useModuleStore((s) => modId ? s.isEnabled(modId) : false);
  if (!modId) return null;
  const mod = moduleRegistry.get(modId);
  if (!mod || !isEnabled) return null;
  // ...
}
```

### Risk of Change

None. This is a bug fix that makes the component reactive to state changes.

---

## Testing Checklist

- [ ] Replace hardcoded `projectPath` default with empty string or CWD resolution
- [ ] Add CWD resolution Tauri command if using Option B for F-13
- [ ] Add startup check: if `projectPath` is empty, prompt user to select a directory
- [ ] Create `src/lib/storage.ts` with `safePersist` and `getStorageUsage` utilities
- [ ] Replace empty catch blocks in all stores with `safePersist` calls
- [ ] Add data pruning for `insightsStore` sessions (max 50)
- [ ] Add IPC wrapper functions to `src/lib/tauri.ts` for all missing commands
- [ ] Update `TerminalPane.tsx` to use PTY wrappers
- [ ] Update `FileExplorer.tsx` to use `listDirectory` wrapper
- [ ] Update `memoryStore.ts` to use existing wrappers (they already exist in tauri.ts)
- [ ] Create `src/lib/env.ts` with environment utilities
- [ ] Fix `OtherViewContent` to use Zustand hook subscription
- [ ] Test fresh install behavior (empty `projectPath`)
- [ ] Test localStorage persistence with large data sets
- [ ] Verify all IPC calls still work after refactoring to use wrappers

---

## References

- `src/stores/layoutStore.ts` — line 26 (hardcoded path)
- `src/stores/issueStore.ts` — lines 97, 107–111 (localStorage persistence)
- `src/stores/memoryStore.ts` — line 14 (empty catch)
- `src/stores/ideationStore.ts` — lines 27, 38 (empty catch)
- `src/stores/insightsStore.ts` — lines 30, 37 (empty catch)
- `src/stores/profileStore.ts` — line 68 (empty catch)
- `src/lib/tauri.ts` — centralized IPC wrapper
- `src/components/session/TerminalPane.tsx` — direct invoke calls
- `src/components/explorer/FileExplorer.tsx` — direct invoke call
- `src/App.tsx` — line 187 (getState in render)

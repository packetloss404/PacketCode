# Phase 10: Testing, Data Migration & Long-Term Quality

**Priority:** P3 — Next Quarter
**Timeline:** Weeks 8–12
**Effort:** Large (2–3 weeks total)
**Risk Level:** Medium
**Owners:** Full Team

---

## Overview

This final phase addresses the foundational quality gaps that determine PacketCode's long-term viability: the near-total absence of tests, the lack of a data migration framework, and several quality-of-life improvements that compound over time. These items are lower urgency than security and stability fixes, but they are essential for sustainable development and production operation.

---

## Finding T-01: No Frontend Test Suite

### Severity: Medium (Long-term Risk)

### Description

The frontend has zero tests. There is no test runner (`vitest`, `jest`, `@testing-library/react`) in the project's dependencies. No `__tests__` directories exist. No `.test.ts`, `.test.tsx`, `.spec.ts`, or `.spec.tsx` files exist anywhere in the frontend codebase.

### Evidence

```json
// package.json — no test runner in devDependencies
"devDependencies": {
  "@eslint/js": "^10.0.1",
  "@tauri-apps/cli": "^2.5.0",
  "@types/react": "^19.0.0",
  "@types/react-dom": "^19.0.0",
  // ... no vitest, jest, or testing-library
}
```

No test script in `package.json`:

```json
"scripts": {
  "dev": "vite",
  "build": "tsc && vite build",
  "preview": "vite preview",
  "tauri": "tauri",
  "lint": "eslint .",
  "format": "prettier --write \"src/**/*.{ts,tsx,css}\""
  // No "test" script
}
```

### Impact

- No regression detection for UI changes
- No validation that store logic works correctly
- No confidence when refactoring components
- Code review cannot rely on automated verification
- New developers cannot verify their changes don't break existing functionality

### Remediation

**Step 1: Add Vitest and testing libraries:**

```bash
pnpm add -D vitest @testing-library/react @testing-library/jest-dom jsdom @testing-library/user-event
```

**Step 2: Configure Vitest:**

```typescript
// vitest.config.ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
```

**Step 3: Add test setup:**

```typescript
// src/test/setup.ts
import "@testing-library/jest-dom";

// Mock Tauri invoke for tests
vi.mock("@tauri-apps/api/core", () => ({
  invoke: vi.fn(),
}));

vi.mock("@tauri-apps/api/event", () => ({
  listen: vi.fn(() => Promise.resolve(() => {})),
  emit: vi.fn(),
}));
```

**Step 4: Add test script:**

```json
"scripts": {
  "test": "vitest run",
  "test:watch": "vitest"
}
```

**Step 5: Write priority tests:**

Start with the highest-value tests — Zustand store logic, which is pure functions that can be tested without DOM rendering:

```typescript
// src/stores/__tests__/issueStore.test.ts
import { useIssueStore } from "../issueStore";

describe("issueStore", () => {
  beforeEach(() => {
    useIssueStore.setState(useIssueStore.getInitialState());
  });

  it("creates an issue with correct defaults", () => {
    const { createIssue } = useIssueStore.getState();
    createIssue({ title: "Test issue", description: "Test desc" });
    const { issues } = useIssueStore.getState();
    expect(issues).toHaveLength(1);
    expect(issues[0].title).toBe("Test issue");
    expect(issues[0].status).toBe("backlog");
  });

  it("moves issue between columns", () => {
    const { createIssue, moveIssue } = useIssueStore.getState();
    createIssue({ title: "Test" });
    const { issues } = useIssueStore.getState();
    moveIssue(issues[0].id, "in-progress");
    expect(useIssueStore.getState().issues[0].status).toBe("in-progress");
  });
});
```

**Priority test targets:**
1. `issueStore.ts` — CRUD operations, column moves, filtering
2. `layoutStore.ts` — pane management, session assignment
3. `tabStore.ts` — tab lifecycle
4. `appStore.ts` — view routing
5. `statusLineStoreUtils.ts` — data parsing utilities
6. `ErrorBoundary.tsx` — error catching behavior

**Step 6: Add test step to CI:**

```yaml
  frontend:
    steps:
      # ... existing steps ...
      - name: Test
        run: pnpm test
```

### Risk of Change

None. Tests are additive and do not modify application code.

---

## Finding T-02: Limited Rust Unit Tests

### Severity: Medium

### Description

The Rust backend has unit tests only in `code_quality.rs` (5 tests). The remaining 15 source files — including the security-critical `pty.rs`, `fs.rs`, and `github.rs` — have zero tests.

### Evidence

```rust
// src-tauri/src/commands/code_quality.rs, lines 394-501
#[cfg(test)]
mod tests {
    // 5 test functions: is_test_file, line_complexity, analyze_file, walk_dir, analyze_code_quality
}
```

No `#[cfg(test)]` blocks in any other file.

### Impact

- PTY session lifecycle (create, write, resize, kill) is untested
- Filesystem path handling is untested
- GitHub API integration is untested
- Status line parsing is untested
- Git command execution is untested

### Remediation

**Priority test targets:**

1. **`statusline.rs` — `iso_to_epoch` function:**

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn iso_to_epoch_valid_timestamp() {
        let ts = iso_to_epoch("2024-06-15T10:30:00Z");
        assert!(ts > 0);
    }

    #[test]
    fn iso_to_epoch_invalid_month_returns_zero() {
        let ts = iso_to_epoch("2024-15-01T10:30:00Z");
        assert_eq!(ts, 0); // After Phase 4 fix
    }

    #[test]
    fn iso_to_epoch_empty_string_returns_zero() {
        assert_eq!(iso_to_epoch(""), 0);
    }
}
```

2. **`fs.rs` — directory listing:**

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn list_directory_returns_sorted_entries() {
        let dir = std::env::temp_dir();
        let result = list_directory(dir.to_string_lossy().to_string()).await;
        assert!(result.is_ok());
        let entries = result.unwrap();
        // Verify directories come before files
        let first_file = entries.iter().position(|e| !e.is_dir);
        let last_dir = entries.iter().rposition(|e| e.is_dir);
        if let (Some(ff), Some(ld)) = (first_file, last_dir) {
            assert!(ld < ff);
        }
    }

    #[tokio::test]
    async fn list_directory_rejects_invalid_path() {
        let result = list_directory("/nonexistent/path".to_string()).await;
        assert!(result.is_err());
    }
}
```

3. **`github.rs` — URL construction and token validation:**

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn github_set_token_rejects_empty() {
        let state = GitHubAuthState::new();
        let result = github_set_token_inner(&state, "   ".to_string()).await;
        assert!(result.is_err());
    }
}
```

### Risk of Change

None. Tests are additive.

---

## Finding T-03: No Integration or End-to-End Tests

### Severity: Medium (Long-term Risk)

### Description

There are no integration tests that verify the frontend and backend work together correctly. No Tauri test harness, no Playwright/Cypress tests, no WebDriver tests.

### Impact

- IPC contract changes between frontend and backend are not automatically verified
- UI workflows (create session, view issues, explore files) are only manually tested
- Regressions in the integration layer are invisible until user reports

### Remediation

**Phase 1: Tauri integration tests**

Tauri v2 supports integration testing via the `tauri-test` crate:

```rust
// src-tauri/tests/integration.rs
#[cfg(test)]
mod tests {
    use tauri::test::{mock_builder, MockRuntime};

    #[test]
    fn test_list_directory_command() {
        let app = mock_builder().build().unwrap();
        // Invoke commands against the mock runtime
    }
}
```

**Phase 2: E2E tests with WebDriver**

For critical user flows, add Playwright or WebDriver tests:
- Create a new Claude session
- Type in terminal
- Switch views
- Create and manage issues
- Open file explorer

This is a larger effort and should be planned as a separate initiative.

### Risk of Change

None. Tests are additive.

---

## Finding D-01: No Data Versioning or Migration Framework

### Severity: Medium

### Description

All persistent state is stored in `localStorage` as JSON blobs under `packetcode:*` keys. There is no version number associated with the stored data format. If the shape of a store's state changes between app versions (e.g., adding a new required field to an issue, renaming a property), the old data will either fail to load or be silently corrupted.

### Evidence

Loading from localStorage with no version check:

```typescript
// src/stores/issueStore.ts, lines 93-100
const stored = localStorage.getItem("packetcode:issues");
if (stored) {
  try {
    const data = JSON.parse(stored);
    // Direct assignment — no schema validation, no migration
    return { ...initialState, ...data };
  } catch {}
}
```

The same pattern is used in `profileStore.ts`, `memoryStore.ts`, `ideationStore.ts`, and `insightsStore.ts`.

### Impact

- Adding a new required field to an issue type would cause all stored issues to lack that field
- Renaming a store property would cause old data to be orphaned
- Removing a property would cause TypeScript errors on stored data with extra fields
- No way to run migrations on existing user data

### Remediation

**Step 1: Add a data version to each persisted store:**

```typescript
const CURRENT_VERSION = 2;

interface PersistedIssueState {
  version: number;
  issues: Issue[];
  nextId: number;
  lastUpdated: number;
}
```

**Step 2: Add migration functions:**

```typescript
function migrateIssueData(data: unknown): PersistedIssueState {
  const raw = data as Record<string, unknown>;
  const version = (raw.version as number) || 1;

  if (version === 1) {
    // Migration from v1 to v2: add 'labels' field to each issue
    const issues = (raw.issues as Issue[]).map(issue => ({
      ...issue,
      labels: issue.labels || [],
    }));
    return { version: 2, issues, nextId: raw.nextId as number, lastUpdated: Date.now() };
  }

  return raw as PersistedIssueState;
}
```

**Step 3: Apply migrations on load:**

```typescript
const stored = localStorage.getItem("packetcode:issues");
if (stored) {
  try {
    const raw = JSON.parse(stored);
    const migrated = migrateIssueData(raw);
    if (migrated.version !== raw.version) {
      localStorage.setItem("packetcode:issues", JSON.stringify(migrated));
    }
    return { ...initialState, ...migrated };
  } catch {
    // Corrupted data — use defaults
  }
}
```

### Risk of Change

Low for the framework itself. Each individual migration should be tested.

---

## Finding Q-01: GitHub Token Should Use OS Keychain

### Severity: Low

### Description

The GitHub personal access token is stored as a plain `String` in Rust process memory (`github.rs:6`). When cleared, the memory is not zeroized — the old string value remains in deallocated memory until overwritten. Any process running as the same OS user can read another process's memory.

### Evidence

```rust
// src-tauri/src/commands/github.rs, lines 5-7
pub struct GitHubAuthState {
    token: RwLock<Option<String>>,
}
```

```rust
// github.rs, line 64 — clearing doesn't zeroize
*guard = None;  // String is dropped but memory not cleared
```

### Remediation

**Option A (Quick): Add zeroize:**

```toml
# Cargo.toml
zeroize = { version = "1", features = ["derive"] }
```

```rust
use zeroize::Zeroize;

pub async fn github_clear_token(auth: State<'_, GitHubAuthState>) -> Result<(), String> {
    let mut guard = auth.token.write().await;
    if let Some(ref mut token) = *guard {
        token.zeroize();
    }
    *guard = None;
    Ok(())
}
```

**Option B (Better): Use OS keychain:**

```toml
# Cargo.toml
keyring = "2"
```

```rust
use keyring::Entry;

const SERVICE_NAME: &str = "PacketCode";
const GITHUB_TOKEN_KEY: &str = "github_pat";

pub async fn github_set_token(token: String) -> Result<(), String> {
    let entry = Entry::new(SERVICE_NAME, GITHUB_TOKEN_KEY)
        .map_err(|e| format!("Keychain error: {}", e))?;
    entry.set_password(&token)
        .map_err(|e| format!("Failed to store token: {}", e))?;
    Ok(())
}

pub async fn github_get_token() -> Result<Option<String>, String> {
    let entry = Entry::new(SERVICE_NAME, GITHUB_TOKEN_KEY)
        .map_err(|e| format!("Keychain error: {}", e))?;
    match entry.get_password() {
        Ok(token) => Ok(Some(token)),
        Err(keyring::Error::NoEntry) => Ok(None),
        Err(e) => Err(format!("Keychain error: {}", e)),
    }
}
```

This stores the token in:
- **Windows:** Windows Credential Manager
- **macOS:** Keychain
- **Linux:** Secret Service (GNOME Keyring / KDE Wallet)

### Risk of Change

Low for zeroize. Medium for keychain migration — requires testing on all platforms.

---

## Finding Q-02: Frontend Bundle Size Optimization

### Severity: Low

### Description

`react-syntax-highlighter@15.6.1` imports all language grammars by default, adding significant bundle weight. No code splitting or lazy loading is configured in Vite.

### Evidence

```json
// package.json
"react-syntax-highlighter": "^15.6.1",
```

All views are eagerly imported in `App.tsx`:

```typescript
// src/App.tsx — top-level imports
import GitHubView from "./components/views/GitHubView";
import MemoryView from "./components/views/MemoryView";
import ToolsView from "./components/views/ToolsView";
// ... all views imported eagerly
```

### Remediation

**Step 1: Use light build of react-syntax-highlighter:**

```typescript
// Use the light build
import { Light as SyntaxHighlighter } from "react-syntax-highlighter/dist/esm/light";
import typescript from "react-syntax-highlighter/dist/esm/languages/prism/typescript";
import rust from "react-syntax-highlighter/dist/esm/languages/prism/rust";
import json from "react-syntax-highlighter/dist/esm/languages/prism/json";

SyntaxHighlighter.registerLanguage("typescript", typescript);
SyntaxHighlighter.registerLanguage("rust", rust);
SyntaxHighlighter.registerLanguage("json", json);
```

**Step 2: Lazy load infrequently used views:**

```typescript
import { lazy, Suspense } from "react";

const GitHubView = lazy(() => import("./components/views/GitHubView"));
const MemoryView = lazy(() => import("./components/views/MemoryView"));
const IdeationView = lazy(() => import("./components/views/IdeationView"));

// In render:
<Suspense fallback={<LoadingSpinner />}>
  <GitHubView />
</Suspense>
```

**Step 3: Analyze bundle:**

```bash
pnpm add -D rollup-plugin-visualizer
```

Add to `vite.config.ts`:

```typescript
import { visualizer } from "rollup-plugin-visualizer";

export default defineConfig({
  plugins: [
    react(),
    visualizer({ open: true, gzipSize: true }),
  ],
});
```

### Risk of Change

Low for syntax highlighter optimization. Medium for lazy loading (requires testing loading states).

---

## Finding Q-03: Add Crash Reporter

### Severity: Low

### Description

There is no crash reporting mechanism. When the Tauri process panics (e.g., due to the `iso_to_epoch` bug), there is no record of what happened. The user sees the app disappear with no explanation.

### Remediation

**Step 1: Add a Rust panic hook:**

```rust
// src-tauri/src/lib.rs
use std::panic;

pub fn run() {
    panic::set_hook(Box::new(|info| {
        let log_dir = dirs::data_local_dir()
            .unwrap_or_else(|| std::path::PathBuf::from("."))
            .join("PacketCode")
            .join("crashes");
        let _ = std::fs::create_dir_all(&log_dir);

        let timestamp = chrono::Utc::now().format("%Y%m%d_%H%M%S");
        let crash_file = log_dir.join(format!("crash_{}.log", timestamp));

        let message = format!(
            "PacketCode crashed at {}\n\nPanic info: {}\n\nBacktrace:\n{:?}",
            timestamp, info, std::backtrace::Backtrace::capture()
        );

        let _ = std::fs::write(&crash_file, &message);
        eprintln!("PacketCode crashed. Report saved to {:?}", crash_file);
    }));

    // ... existing tauri::Builder setup
}
```

**Step 2: Show crash report to user on next launch:**

On startup, check for crash report files. If found, show a dialog offering to view or submit the report, then delete the file.

### Risk of Change

None. The panic hook is additive and does not change normal execution flow.

---

## Testing Checklist

- [ ] Add Vitest and testing libraries to devDependencies
- [ ] Create `vitest.config.ts` and `src/test/setup.ts`
- [ ] Write store unit tests (issueStore, layoutStore, tabStore)
- [ ] Add `pnpm test` script to package.json
- [ ] Add test step to CI pipeline
- [ ] Write Rust unit tests for `iso_to_epoch` edge cases
- [ ] Write Rust unit tests for `list_directory` validation
- [ ] Write Rust unit tests for GitHub token management
- [ ] Add data version number to all persisted stores
- [ ] Create migration framework for localStorage data
- [ ] Add zeroize for GitHub token memory clearing
- [ ] Evaluate OS keychain integration (keyring crate)
- [ ] Switch to react-syntax-highlighter light build
- [ ] Add lazy loading for infrequently used views
- [ ] Analyze bundle size with rollup-plugin-visualizer
- [ ] Add Rust panic hook for crash reporting
- [ ] Add crash report viewer on next launch
- [ ] Verify all existing functionality still works after changes

---

## References

- `package.json` — devDependencies (no test runner)
- `src-tauri/src/commands/code_quality.rs` — lines 394–501 (existing Rust tests)
- `src/stores/issueStore.ts` — lines 93–100 (no-migration loading)
- `src-tauri/src/commands/github.rs` — lines 5–7, 64 (token storage)
- `src/App.tsx` — view imports (no lazy loading)
- `src/components/common/MarkdownRenderer.tsx` — syntax highlighter usage
- Vitest documentation
- @testing-library/react documentation
- Tauri v2: Integration testing
- keyring crate documentation
- zeroize crate documentation

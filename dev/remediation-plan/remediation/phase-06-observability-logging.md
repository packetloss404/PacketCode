# Phase 6: Observability & Error Reporting

**Priority:** P2 — Next Sprint
**Timeline:** Weeks 2–3
**Effort:** Medium (1 day total)
**Risk Level:** Medium
**Owners:** Backend Lead, Frontend Lead

---

## Overview

The PacketCode backend has zero logging infrastructure. No errors, security events, command invocations, or operational metrics are recorded anywhere. On the frontend, async errors are silently lost because there is no global rejection handler. This phase establishes a foundation for observability across both layers.

---

## Finding F-10: No Logging Framework in Rust Backend

### Severity: Medium

### Description

The entire Rust backend — 16 source files, 22 IPC commands, PTY session management, GitHub API integration, and Claude CLI orchestration — has zero log statements. The `tracing`, `log`, and `env_logger` crates are not in `Cargo.toml`. Errors are either propagated to the frontend as `Result::Err(String)` or silently discarded with `let _ = ...`.

### Evidence

No logging crate in dependencies:

```toml
# src-tauri/Cargo.toml, lines 15-27
[dependencies]
tauri = { version = "2", features = [] }
# ... no tracing, log, or env_logger
```

Silent error suppression without logging:

```rust
// src-tauri/src/commands/pty.rs, line 167
let _ = app_handle.emit("pty:output", &event);

// src-tauri/src/commands/pty.rs, line 189
let _ = app_handle.emit("pty:exit", &sid);

// src-tauri/src/commands/pty.rs, line 256
let _ = session.child.kill();
```

Poisoned mutex recovery without logging:

```rust
// src-tauri/src/commands/statusline.rs, lines 561-563
let mut cache = match codex_status_cache().lock() {
    Ok(guard) => guard,
    Err(poisoned) => poisoned.into_inner(),  // No log of mutex poisoning
};
```

### Impact

- **No audit trail:** Security-relevant operations (PTY command spawning, GitHub token changes, file access) are invisible
- **No error diagnostics:** When users report issues, there is no log file to examine
- **No operational visibility:** Cannot determine if commands are succeeding, failing, or timing out
- **No abuse detection:** Cannot detect if the IPC surface is being probed or misused
- **No performance instrumentation:** Cannot measure command execution times

### Remediation

**Step 1: Add `tracing` crate to dependencies:**

```toml
# src-tauri/Cargo.toml
[dependencies]
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["fmt", "env-filter"] }
tracing-appender = "0.2"
```

**Step 2: Initialize tracing in `lib.rs`:**

```rust
use tracing_subscriber::{fmt, EnvFilter};
use tracing_appender::rolling;

pub fn run() {
    // File-based logging with daily rotation
    let log_dir = dirs::data_local_dir()
        .unwrap_or_else(|| std::path::PathBuf::from("."))
        .join("PacketCode")
        .join("logs");
    let file_appender = rolling::daily(&log_dir, "packetcode.log");
    let (non_blocking, _guard) = tracing_appender::non_blocking(file_appender);

    tracing_subscriber::fmt()
        .with_writer(non_blocking)
        .with_env_filter(
            EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| EnvFilter::new("info"))
        )
        .with_target(true)
        .with_thread_ids(true)
        .init();

    tracing::info!("PacketCode starting");

    tauri::Builder::default()
        // ... existing setup
}
```

**Step 3: Add logging to critical paths:**

Security-relevant commands (minimum):

```rust
// pty.rs — log all PTY session creation
tracing::info!(command = %command, project_path = %project_path, "Creating PTY session");

// github.rs — log token changes
tracing::info!("GitHub token set");
tracing::info!("GitHub token cleared");

// fs.rs — log directory listings
tracing::debug!(dir_path = %dir_path, "Listing directory");
```

Error paths (replace `let _ =` with logged errors):

```rust
// pty.rs — replace silent suppression
if let Err(e) = app_handle.emit("pty:output", &event) {
    tracing::warn!(session_id = %sid, error = %e, "Failed to emit PTY output");
}

if let Err(e) = session.child.kill() {
    tracing::warn!(session_id = %session_id, error = %e, "Failed to kill PTY child");
}
```

Mutex recovery:

```rust
// statusline.rs — log mutex poisoning
Err(poisoned) => {
    tracing::warn!("Codex status cache mutex was poisoned, recovering");
    poisoned.into_inner()
}
```

**Step 4: Add structured logging for IPC command tracing:**

Consider adding a middleware-style log for all command invocations:

```rust
tracing::info!(command = "analyze_code_quality", project_path = %project_path, "IPC command invoked");
```

### Log File Location

Store logs in the OS-standard application data directory:
- **Windows:** `%LOCALAPPDATA%\PacketCode\logs\`
- **macOS:** `~/Library/Application Support/PacketCode/logs/`
- **Linux:** `~/.local/share/PacketCode/logs/`

### Risk of Change

None. Logging is purely additive. Use `tracing::debug!` for verbose output and `tracing::info!` for security-relevant events to keep log volume manageable.

---

## Finding F-17: No Global Unhandled Rejection Handler

### Severity: Medium

### Description

The frontend has `ErrorBoundary` components at three levels (app root, view content, per-module), but these only catch synchronous rendering errors. Promise rejections in async event handlers, `useEffect` callbacks, and Zustand store actions are not caught by any error boundary. There is no `window.addEventListener("unhandledrejection", ...)` handler.

### Evidence

The `ErrorBoundary` component in `src/components/ui/ErrorBoundary.tsx` is a React class component that implements `componentDidCatch`. This is the standard React error boundary pattern, which is limited to synchronous render errors.

```tsx
// src/App.tsx, lines 138-162
<ErrorBoundary fallbackMessage="PacketCode encountered an error">
  <div ...>
    <ErrorBoundary fallbackMessage="View error">
      {/* view content */}
    </ErrorBoundary>
  </div>
</ErrorBoundary>
```

Async errors that escape error boundaries:

```typescript
// src/stores/ideationStore.ts, lines 91-94
} catch (err) {
  set({ isGenerating: false });
  throw err;  // Re-thrown — becomes unhandled rejection if caller doesn't catch
}
```

Multiple empty catch blocks silently swallow errors:

```typescript
// src/stores/issueStore.ts, line 97
} catch {}

// src/stores/memoryStore.ts, line 14
} catch {}
```

### Impact

- Async errors in store actions produce no user-visible feedback
- Promise rejections in `useEffect` cleanup or event handlers are silently lost
- Users experience broken functionality with no indication of what went wrong
- Developers cannot diagnose issues without adding console logging and reproducing

### Remediation

**Step 1: Add global error handlers in `main.tsx`:**

```typescript
// src/main.tsx
window.addEventListener("unhandledrejection", (event) => {
  console.error("[PacketCode] Unhandled promise rejection:", event.reason);
  // Optional: send to a lightweight error reporting service
  // Optional: show a toast notification to the user
});

window.addEventListener("error", (event) => {
  console.error("[PacketCode] Uncaught error:", event.error);
});
```

**Step 2: Add error reporting utility:**

Create a lightweight error reporting utility that can be used across the frontend:

```typescript
// src/lib/errors.ts
export function reportError(context: string, error: unknown): void {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`[PacketCode:${context}]`, message, error);
  // Future: integrate with Sentry, LogRocket, or custom backend endpoint
}
```

**Step 3: Replace empty catch blocks with logged errors:**

```typescript
// BEFORE:
try {
  localStorage.setItem("packetcode:issues", JSON.stringify(state));
} catch {}

// AFTER:
try {
  localStorage.setItem("packetcode:issues", JSON.stringify(state));
} catch (e) {
  reportError("issueStore:persist", e);
}
```

Priority locations for adding error reporting (at least 15 empty catch blocks):
- `issueStore.ts:97,110`
- `profileStore.ts:68`
- `memoryStore.ts:14`
- `ideationStore.ts:27,38`
- `insightsStore.ts:30,37`
- `TerminalPane.tsx` (multiple PTY calls)
- `FileExplorer.tsx:102,206`

### Risk of Change

None. Adding error handlers is purely additive. The `unhandledrejection` handler runs as a last resort and does not change application behavior.

---

## Testing Checklist

- [ ] Add `tracing`, `tracing-subscriber`, `tracing-appender` to `Cargo.toml`
- [ ] Initialize tracing in `lib.rs` with file-based logging
- [ ] Add `tracing::info!` to PTY session creation
- [ ] Add `tracing::info!` to GitHub token set/clear
- [ ] Replace `let _ =` patterns in `pty.rs` with logged errors
- [ ] Add `tracing::warn!` to mutex poisoning recovery in `statusline.rs`
- [ ] Add global `unhandledrejection` handler in `main.tsx`
- [ ] Add global `error` handler in `main.tsx`
- [ ] Create `src/lib/errors.ts` utility
- [ ] Replace at least the 5 most important empty catch blocks with `reportError`
- [ ] Verify log files are created in the expected directory
- [ ] Verify log rotation works (daily)
- [ ] Test that `RUST_LOG=debug` environment variable increases verbosity
- [ ] Verify no performance impact from logging on normal operation

---

## References

- `src-tauri/Cargo.toml` — dependency declarations
- `src-tauri/src/lib.rs` — app initialization
- `src-tauri/src/commands/pty.rs` — lines 167, 189, 256 (silent error suppression)
- `src-tauri/src/commands/statusline.rs` — lines 561–563 (mutex recovery)
- `src/main.tsx` — React entry point
- `src/components/ui/ErrorBoundary.tsx` — existing error boundary
- `src/stores/ideationStore.ts` — line 93 (re-thrown error)
- tracing crate documentation
- tracing-appender documentation for log rotation

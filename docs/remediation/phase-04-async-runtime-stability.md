# Phase 4: Async Runtime & Stability Fixes

**Priority:** P1 — This Week
**Timeline:** 5–8 days
**Effort:** Small (2–3 hours total)
**Risk Level:** High (stability) / Medium (panic)
**Owners:** Backend Lead

---

## Overview

This phase addresses two categories of stability issues in the Rust backend: blocking I/O operations that starve the Tokio async runtime, and a panic-risk code path that could crash the entire Tauri process. Both issues are straightforward to fix and have low risk of regressions.

---

## Finding F-08: Blocking I/O on Tokio Async Runtime

### Severity: High

### Description

Four Tauri commands are declared as `async fn` but internally call synchronous, blocking operations from the Rust standard library. In Tauri v2, `async` commands run on the Tokio multi-threaded runtime. When a blocking call executes on a Tokio worker thread, it prevents that thread from processing other async tasks. Under load — particularly with rapid polling — this can starve the runtime and freeze IPC processing.

### Evidence

**`git.rs` — Blocking `std::process::Command::output()`:**

```rust
// src-tauri/src/commands/git.rs, lines 3-7
use std::process::Command;  // <-- Synchronous Command

#[tauri::command]
pub async fn get_git_branch(project_path: String) -> Result<String, String> {
    let mut cmd = Command::new("git");
    cmd.args(["rev-parse", "--abbrev-ref", "HEAD"])
        .current_dir(&project_path);
    // ...
    let output = cmd.output()  // <-- BLOCKS the Tokio worker thread
        .map_err(|e| format!("Failed to run git: {}", e))?;
```

```rust
// src-tauri/src/commands/git.rs, lines 26-30
#[tauri::command]
pub async fn get_git_status(project_path: String) -> Result<String, String> {
    let mut cmd = Command::new("git");
    // Same blocking pattern
```

These two functions are polled every few seconds by the frontend's `useGitInfo` hook, creating repeated blocking on the async runtime.

**`fs.rs` — Blocking `std::fs::read_dir`:**

```rust
// src-tauri/src/commands/fs.rs, lines 21-22
#[tauri::command]
pub async fn list_directory(dir_path: String) -> Result<Vec<DirEntry>, String> {
    // Uses std::fs::read_dir and entry.metadata() — both blocking
```

**`code_quality.rs` — Blocking filesystem walk and file reads:**

```rust
// src-tauri/src/commands/code_quality.rs, lines 281-282
#[tauri::command]
pub async fn analyze_code_quality(project_path: String) -> Result<CodeQualityReport, String> {
    // Calls walk_dir -> std::fs::read_dir (recursive)
    // Calls analyze_file -> std::fs::read_to_string
    // All blocking operations
```

**Contrast with correctly implemented async commands:**

The Claude CLI commands in `memory.rs`, `insights.rs`, `ideation.rs`, and `github.rs` correctly use `tokio::process::Command` (async):

```rust
// src-tauri/src/claude/binary.rs
use tokio::process::Command;  // <-- Async Command — correct

pub fn claude_command() -> Result<Command, String> {
    // ...
    Ok(Command::new(path))
}
```

### Impact

- **Git polling:** With the default polling interval, `get_git_branch` and `get_git_status` block a Tokio worker thread every few seconds. Each call blocks for the duration of the git process (typically 50–200ms, but can be seconds on large repos or slow disks).
- **Directory listing:** Each expansion of a directory in the File Explorer blocks a worker thread. With rapid navigation, this compounds.
- **Code quality analysis:** A full filesystem walk blocks a worker thread for potentially seconds on large codebases. During this time, other async commands (GitHub API calls, status line polling) queue up.
- **Worst case:** If all Tokio worker threads are blocked simultaneously (default is number of CPU cores), the entire IPC system becomes unresponsive until a blocking call completes.

### Remediation

**Option A (Recommended — Simplest):** Remove the `async` keyword from the blocking commands. In Tauri v2, synchronous `#[tauri::command]` functions automatically run on a dedicated blocking thread pool, which is exactly the right behavior for these functions.

```rust
// BEFORE (broken):
#[tauri::command]
pub async fn get_git_branch(project_path: String) -> Result<String, String> {
    let mut cmd = Command::new("git");
    // ...
}

// AFTER (correct):
#[tauri::command]
pub fn get_git_branch(project_path: String) -> Result<String, String> {
    let mut cmd = Command::new("git");
    // ...
}
```

Apply this change to:
- `get_git_branch` in `git.rs`
- `get_git_status` in `git.rs`
- `list_directory` in `fs.rs`
- `analyze_code_quality` in `code_quality.rs`

**Option B (Alternative):** Keep `async` but wrap blocking work in `tokio::task::spawn_blocking`:

```rust
#[tauri::command]
pub async fn get_git_branch(project_path: String) -> Result<String, String> {
    tokio::task::spawn_blocking(move || {
        let mut cmd = Command::new("git");
        cmd.args(["rev-parse", "--abbrev-ref", "HEAD"])
            .current_dir(&project_path);
        let output = cmd.output()
            .map_err(|e| format!("Failed to run git: {}", e))?;
        // ...
    })
    .await
    .map_err(|e| format!("Task join error: {}", e))?
}
```

Option A is preferred for simplicity and correctness. Option B is useful if you need to compose these commands with other async operations in the future.

**Note:** The PTY commands (`create_pty_session`, `write_pty`, `resize_pty`, `kill_pty`, `list_pty_sessions`) are already correctly declared as synchronous `fn` — they run on Tauri's blocking thread pool. No changes needed for those.

### Risk of Change

Low. Removing `async` from these functions changes their execution model from "run on Tokio worker thread" to "run on Tauri's blocking thread pool," which is the correct behavior for blocking I/O.

---

## Finding F-15: Panic Risk in `iso_to_epoch`

### Severity: Medium

### Description

The `iso_to_epoch` function in `src-tauri/src/commands/statusline.rs` (line 303) parses ISO timestamps from JSONL files and converts them to epoch seconds. The function indexes into a fixed-size array using a parsed `month` value without validating it is within the valid range (1–12). A malformed JSONL file with a month value of 14 or higher would cause an array index out-of-bounds panic, crashing the entire Tauri process.

### Evidence

```rust
// src-tauri/src/commands/statusline.rs, lines 303-311
let month_days = [0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
let is_leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
for m in 1..month {
    days += month_days[m as usize];  // <-- PANIC if month > 13
    if m == 2 && is_leap {
        days += 1;
    }
}
days += day - 1;  // <-- Underflow if day == 0 (wraps to u64::MAX)
```

The `month_days` array has 13 elements (indices 0–12). If `month` is 14, the loop iterates `m = 1..14`, and `month_days[13]` is out of bounds.

Additionally, if `day == 0`, the subtraction `day - 1` underflows on `u64`, wrapping to `u64::MAX`. While this doesn't panic (it produces a wildly incorrect timestamp), it would cause the status line cache to behave unexpectedly.

The function validates that `date_parts.len() == 3` but does not validate the range of individual values.

### Impact

- A malformed JSONL file from Claude or Codex (or a corrupted file) could crash the entire PacketCode application
- The status line reader is called on a polling interval (every few seconds), so the crash would recur immediately on restart if the malformed file persists
- This effectively creates a denial-of-service via a corrupt data file

### Remediation

Add range validation before the array indexing:

```rust
fn iso_to_epoch(iso: &str) -> u64 {
    // ... existing parsing ...

    if month < 1 || month > 12 || day < 1 || day > 31 {
        return 0;
    }

    let month_days = [0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
    // ... rest of function
}
```

Alternatively, use Rust's `get()` method for safe array access:

```rust
for m in 1..month {
    days += month_days.get(m as usize).copied().unwrap_or(30);
    if m == 2 && is_leap {
        days += 1;
    }
}
if day > 0 {
    days += day - 1;
}
```

### Risk of Change

None. This is a pure safety guard that makes the function return `0` (a reasonable sentinel) instead of panicking on malformed input.

---

## Finding F-15b: Silent Error Suppression in PTY Operations

### Severity: Low

### Description

Three sites in `src-tauri/src/commands/pty.rs` silently discard errors using `let _ = ...`:

```rust
// pty.rs, line 167 — PTY output emit failure
let _ = app_handle.emit("pty:output", &event);

// pty.rs, line 189 — PTY exit emit failure
let _ = app_handle.emit("pty:exit", &sid);

// pty.rs, line 256 — PTY child kill failure
let _ = session.child.kill();
```

### Impact

- If `emit` fails, the frontend never receives PTY output or exit events, causing sessions to appear frozen
- If `kill` fails, the child process becomes orphaned
- No logging means these failures are invisible

### Remediation

At minimum, log warnings (once the `tracing` crate is added in Phase 6):

```rust
if let Err(e) = app_handle.emit("pty:output", &event) {
    tracing::warn!("Failed to emit PTY output for session {}: {}", sid, e);
}
```

For the kill failure, attempt alternative cleanup:

```rust
if let Err(e) = session.child.kill() {
    tracing::warn!("Failed to kill PTY child for session {}: {}", session_id, e);
    // Process may have already exited — acceptable
}
```

### Risk of Change

None. Replacing `let _ =` with logged error handling only adds observability.

---

## Testing Checklist

- [ ] Remove `async` from `get_git_branch` in `git.rs`
- [ ] Remove `async` from `get_git_status` in `git.rs`
- [ ] Remove `async` from `list_directory` in `fs.rs`
- [ ] Remove `async` from `analyze_code_quality` in `code_quality.rs`
- [ ] Add bounds checking to `iso_to_epoch` (month 1–12, day >= 1)
- [ ] Test git branch polling under normal conditions
- [ ] Test git status polling under normal conditions
- [ ] Test file explorer directory listing
- [ ] Test code quality analysis on current project
- [ ] Create a test JSONL file with an invalid month (e.g., month=15) and verify `iso_to_epoch` returns 0 instead of panicking
- [ ] Verify status line polling still works with valid data
- [ ] Run `cargo test` to confirm no regressions

---

## References

- `src-tauri/src/commands/git.rs` — lines 1–47
- `src-tauri/src/commands/fs.rs` — lines 21–81
- `src-tauri/src/commands/code_quality.rs` — lines 281–392
- `src-tauri/src/commands/statusline.rs` — lines 303–311
- `src-tauri/src/commands/pty.rs` — lines 167, 189, 256
- `src-tauri/src/claude/binary.rs` — correct async pattern reference
- Tokio documentation: blocking and CPU-bound tasks

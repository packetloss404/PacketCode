# Phase 1: Critical Security — Process Execution Lockdown

**Priority:** P0 — Immediate
**Timeline:** 0–3 days
**Effort:** Small (2–4 hours total)
**Risk Level:** Critical
**Owners:** Security Lead, Backend Lead

---

## Overview

This phase addresses the two most dangerous findings in the entire codebase. Together, they create a direct, unmitigated path from webview JavaScript to arbitrary command execution on the host operating system with the user's full privileges. If the webview is ever compromised — via a supply-chain attack on an npm dependency, a malicious iframe payload, or any future XSS vector — an attacker gains full remote code execution.

These two findings must be resolved before any other work. They are independent fixes that can be applied in parallel.

---

## Finding F-01: Unrestricted Process Spawning via `shell:allow-spawn`

### Severity: Critical

### Description

The Tauri capability file `src-tauri/capabilities/default.json` includes `shell:allow-spawn` on line 17 as a bare permission with no scoping object. This grants the webview's JavaScript unrestricted ability to spawn any process on the host machine via the Tauri shell plugin API.

This permission completely bypasses the scoped `shell:allow-execute` allowlist defined on lines 21–39, which restricts execution to `claude`, `git`, and `where`. The `allow-spawn` permission operates independently and has no restrictions whatsoever.

### Evidence

```json
// src-tauri/capabilities/default.json, lines 16-19
"shell:default",
"shell:allow-spawn",
"shell:allow-stdin-write",
"shell:allow-kill",
```

The permission is listed without any scoping object. Compare this to the scoped `shell:allow-execute` block on lines 21–39 which at least restricts command names (though it allows arbitrary arguments — addressed in Phase 7).

### Impact

- Any JavaScript running in the webview can call the Tauri shell spawn API to execute any binary on the system
- A compromised npm dependency (supply-chain attack) could silently spawn processes
- Malicious content loaded in the VibeArchitectView iframe (if it escapes sandbox) could spawn processes
- No audit trail — the Rust backend has no logging, so spawned processes would be invisible

### Why It Matters

Desktop applications have a fundamentally different threat model than web apps. When a webview has unrestricted process spawning, the application effectively grants every piece of JavaScript running inside it root-equivalent access to the user's system. This is the single highest-impact vulnerability in the codebase.

### Remediation

Remove `shell:allow-spawn` from `src-tauri/capabilities/default.json` entirely. All legitimate process spawning already goes through the `create_pty_session` Tauri command (which will be separately hardened in F-02 below).

**Before:**
```json
"shell:default",
"shell:allow-spawn",
"shell:allow-stdin-write",
"shell:allow-kill",
```

**After:**
```json
"shell:default",
"shell:allow-stdin-write",
"shell:allow-kill",
```

If `shell:allow-spawn` is genuinely needed for PTY operations via the shell plugin (verify this), scope it to specific commands:

```json
{
  "identifier": "shell:allow-spawn",
  "allow": [
    { "name": "claude", "cmd": "claude" },
    { "name": "codex", "cmd": "codex" }
  ]
}
```

### Verification

After removal, test that:
1. Claude PTY sessions still spawn correctly via `create_pty_session`
2. Codex PTY sessions still spawn correctly
3. The shell plugin's execute functionality (scoped allowlist) still works
4. No frontend code directly calls the shell spawn API (search for `@tauri-apps/plugin-shell` spawn usage)

### Risk of Change

Low. The PTY session creation uses `portable-pty` through a dedicated Tauri command, not the shell plugin's spawn API. The `shell:allow-stdin-write` and `shell:allow-kill` permissions remain for managing spawned processes.

---

## Finding F-02: Arbitrary Command Execution via `create_pty_session`

### Severity: Critical

### Description

The `create_pty_session` Tauri command in `src-tauri/src/commands/pty.rs` (lines 58–66) accepts an unrestricted `command: String` parameter that is used to spawn a child process via `portable-pty`. There is no allowlist, no validation, and no sanitization. The `args` parameter (line 65) and `project_path` parameter (line 61) are equally unvalidated.

### Evidence

```rust
// src-tauri/src/commands/pty.rs, lines 57-66
#[tauri::command]
pub fn create_pty_session(
    app: AppHandle,
    manager: State<'_, SharedPtyManager>,
    project_path: String,
    cols: u16,
    rows: u16,
    command: String,        // <-- ANY string accepted
    args: Option<Vec<String>>,  // <-- ANY arguments accepted
) -> Result<String, String> {
```

The `command` string flows directly to process spawning at line 88:

```rust
// src-tauri/src/commands/pty.rs, line 88
let mut cmd = CommandBuilder::new(&resolved_command);
cmd.cwd(&project_path);  // <-- ANY directory accepted
```

On Windows, the command gets `.cmd` appended (line 83–84), but this doesn't restrict what can be executed — it merely adjusts the extension for npm global installs.

### Impact

- Frontend JavaScript can invoke `invoke("create_pty_session", { command: "powershell", args: ["-c", "malicious_script"], projectPath: "C:\\" })` to execute arbitrary commands
- The `project_path` parameter sets the working directory with no validation, allowing execution in any directory
- The `args` parameter passes arbitrary arguments to the spawned process
- Combined with F-01, this creates two independent paths to arbitrary code execution

### Why It Matters

Even after fixing F-01 (removing `shell:allow-spawn`), this IPC command remains a direct code execution vector. Any code running in the webview — legitimate or malicious — can spawn any process through this command.

### Remediation

Add a command allowlist at the top of `create_pty_session`:

```rust
const ALLOWED_COMMANDS: &[&str] = &["claude", "codex"];

#[tauri::command]
pub fn create_pty_session(
    app: AppHandle,
    manager: State<'_, SharedPtyManager>,
    project_path: String,
    cols: u16,
    rows: u16,
    command: String,
    args: Option<Vec<String>>,
) -> Result<String, String> {
    // Validate command against allowlist
    if !ALLOWED_COMMANDS.contains(&command.as_str()) {
        return Err(format!(
            "Command '{}' is not allowed. Permitted commands: {:?}",
            command, ALLOWED_COMMANDS
        ));
    }

    // Validate project_path is a real directory
    let project_dir = std::path::Path::new(&project_path);
    if !project_dir.is_dir() {
        return Err(format!("Invalid project path: {}", project_path));
    }

    // ... rest of function
}
```

If additional commands are needed in the future (e.g., `bash`, `powershell` for general terminal use), they should be added to the allowlist explicitly with careful consideration of the security implications.

### Verification

After applying the allowlist:
1. Verify Claude sessions still launch correctly
2. Verify Codex sessions still launch correctly
3. Attempt to spawn an unauthorized command (e.g., `calc`, `notepad`) and confirm it is rejected with a clear error message
4. Verify the error message is surfaced to the user in the frontend

### Risk of Change

Low. The frontend currently only passes `"claude"` or `"codex"` as the command parameter (confirmed by reading `NewSessionModal.tsx` and the custom event handlers). Adding an allowlist will not break any existing functionality.

---

## Testing Checklist

- [ ] Remove `shell:allow-spawn` from `default.json`
- [ ] Add command allowlist to `create_pty_session`
- [ ] Add `project_path` directory validation to `create_pty_session`
- [ ] Test Claude session creation on Windows
- [ ] Test Codex session creation on Windows
- [ ] Test that unauthorized commands are rejected
- [ ] Search frontend for any direct shell plugin spawn API usage
- [ ] Verify no regressions in PTY write, resize, and kill operations

---

## References

- `src-tauri/capabilities/default.json` — lines 16–19
- `src-tauri/src/commands/pty.rs` — lines 57–96
- `src-tauri/src/lib.rs` — lines 18–22 (command registration)
- Tauri v2 Shell Plugin documentation: capabilities and scoping

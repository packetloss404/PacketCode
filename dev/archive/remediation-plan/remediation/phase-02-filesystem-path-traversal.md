# Phase 2: Critical Security — Filesystem Path Traversal

**Priority:** P0 — Immediate
**Timeline:** 3–5 days
**Effort:** Small (3–5 hours total)
**Risk Level:** High
**Owners:** Backend Lead

---

## Overview

Multiple Tauri IPC commands accept arbitrary filesystem paths from the frontend with no scope validation. This allows any JavaScript running in the webview to enumerate directories, read file metadata, and scan file contents anywhere on the host filesystem. Combined with the explicit `.env` file allowlist in the directory listing command, this creates a path to secret discovery and data exfiltration.

This phase hardens all path-accepting commands to restrict access to the user's active project workspace.

---

## Finding F-04: Arbitrary Path Traversal in `list_directory`

### Severity: High

### Description

The `list_directory` command in `src-tauri/src/commands/fs.rs` (line 22) accepts a `dir_path: String` parameter that can point to any directory on the filesystem. The only validation is an `is_dir()` check, which confirms the path exists but does not restrict where it points.

Additionally, the hidden-file filter on line 37 explicitly allows `.env` and `.env.local` files through, which typically contain secrets like API keys, database credentials, and tokens.

### Evidence

```rust
// src-tauri/src/commands/fs.rs, lines 21-26
#[tauri::command]
pub async fn list_directory(dir_path: String) -> Result<Vec<DirEntry>, String> {
    let path = Path::new(&dir_path);
    if !path.is_dir() {
        return Err(format!("Not a directory: {}", dir_path));
    }
```

The `.env` exception in the hidden-file filter:

```rust
// src-tauri/src/commands/fs.rs, line 37
if file_name.starts_with('.') && !matches!(file_name.as_str(),
    ".env" | ".env.local" | ".gitignore" | ".eslintrc" | ".prettierrc") {
    continue;
}
```

A compromised webview could call:
```javascript
invoke("list_directory", { dirPath: "C:\\Users" })
invoke("list_directory", { dirPath: "C:\\Users\\victim\\.ssh" })
invoke("list_directory", { dirPath: "/etc" })
```

### Impact

- Full filesystem directory enumeration from the webview
- Discovery of `.env` and `.env.local` files containing secrets at any path
- Reconnaissance for further attacks (finding SSH keys, config files, other projects)
- File metadata exposure (sizes, extensions) aids targeted attacks

### Remediation

Add workspace path validation. The `list_directory` command should only allow listing directories within the active project workspace:

```rust
use std::path::Path;

fn is_within_workspace(candidate: &Path, workspace: &Path) -> bool {
    match (candidate.canonicalize(), workspace.canonicalize()) {
        (Ok(c), Ok(w)) => c.starts_with(&w),
        _ => false,
    }
}

#[tauri::command]
pub async fn list_directory(dir_path: String, workspace_root: String) -> Result<Vec<DirEntry>, String> {
    let path = Path::new(&dir_path);
    let workspace = Path::new(&workspace_root);

    if !path.is_dir() {
        return Err(format!("Not a directory: {}", dir_path));
    }

    if !is_within_workspace(path, workspace) {
        return Err(format!("Path is outside the project workspace: {}", dir_path));
    }

    // ... rest of function
}
```

Alternatively, pass the workspace root as Tauri managed state rather than a parameter (to prevent the frontend from manipulating it).

### Additional Note on `.env` Exposure

Consider whether `.env` files should be shown in the file explorer at all. If they must be visible for developer convenience, ensure the explorer only operates within the project workspace (which this fix accomplishes). If `.env` exposure is not needed, remove the exception from line 37.

### Risk of Change

Low. The `FileExplorer.tsx` component always passes subdirectories of the current project path. Adding workspace validation will not break existing usage. The frontend caller will need to pass the workspace root parameter.

---

## Finding F-05: Arbitrary Path Traversal in `analyze_code_quality`

### Severity: High

### Description

The `analyze_code_quality` command in `src-tauri/src/commands/code_quality.rs` (line 282) accepts a `project_path: String` parameter and recursively walks the entire directory tree, reading the contents of every recognized source file. There is no path scope validation and no recursion depth limit.

### Evidence

```rust
// src-tauri/src/commands/code_quality.rs, lines 281-286
#[tauri::command]
pub async fn analyze_code_quality(project_path: String) -> Result<CodeQualityReport, String> {
    let base = Path::new(&project_path);
    if !base.is_dir() {
        return Err(format!("Path is not a directory: {}", project_path));
    }
```

The `walk_dir` function (line 251) recursively descends into all subdirectories with no depth limit:

```rust
// src-tauri/src/commands/code_quality.rs, lines 251-278
fn walk_dir(dir: &Path, base: &Path, files: &mut Vec<(String, String)>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        // ...
        if path.is_dir() {
            // No depth limit, no symlink cycle detection
            walk_dir(&path, base, files);
        }
    }
}
```

A compromised webview could call:
```javascript
invoke("analyze_code_quality", { projectPath: "C:\\" })
```

This would attempt to read every source file on the entire C: drive.

### Impact

- Full source code reading of any accessible directory on the filesystem
- Data exfiltration of proprietary code from other projects
- Denial of service: scanning a large directory tree (e.g., `C:\`) would block the async runtime and consume excessive memory
- Symlink cycles could cause unbounded recursion leading to stack overflow and process crash

### Remediation

1. **Add workspace path validation** (same pattern as F-04):

```rust
#[tauri::command]
pub async fn analyze_code_quality(
    project_path: String,
    workspace_root: String,
) -> Result<CodeQualityReport, String> {
    let base = Path::new(&project_path);
    let workspace = Path::new(&workspace_root);

    if !base.is_dir() {
        return Err(format!("Path is not a directory: {}", project_path));
    }

    if !is_within_workspace(base, workspace) {
        return Err(format!("Path is outside the project workspace: {}", project_path));
    }

    // ... rest of function
}
```

2. **Add recursion depth limit and file count cap to `walk_dir`**:

```rust
const MAX_DEPTH: usize = 20;
const MAX_FILES: usize = 10_000;

fn walk_dir(dir: &Path, base: &Path, files: &mut Vec<(String, String)>, depth: usize) {
    if depth > MAX_DEPTH || files.len() >= MAX_FILES {
        return;
    }

    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        let file_name = entry.file_name().to_string_lossy().to_string();

        if path.is_dir() {
            if SKIP_DIRS.contains(&file_name.as_str()) || file_name.starts_with('.') {
                continue;
            }
            walk_dir(&path, base, files, depth + 1);
        } else if path.is_file() {
            // ... existing file handling
        }
    }
}
```

3. **Add symlink detection**: Use `entry.file_type()` to check for symlinks before following directories, preventing cycle-induced stack overflow.

### Risk of Change

Low. The `CodeQualityModal.tsx` component only passes the current project path. Adding workspace validation and depth limits will not break existing usage.

---

## Finding F-04b: Unvalidated `project_path` in Multiple Commands

### Severity: Medium

### Description

Beyond `list_directory` and `analyze_code_quality`, nine additional commands accept a `project_path` parameter without validation. These commands use the path as a working directory for CLI processes or git operations:

| Command | File | Usage |
|---------|------|-------|
| `get_git_branch` | `git.rs:4` | `current_dir(&project_path)` for git process |
| `get_git_status` | `git.rs:27` | `current_dir(&project_path)` for git process |
| `scan_codebase_memory` | `memory.rs:9` | `current_dir` for Claude CLI |
| `summarize_session` | `memory.rs:36` | `current_dir` for Claude CLI |
| `extract_patterns` | `memory.rs:64` | `current_dir` for Claude CLI |
| `ask_insights` | `insights.rs:35` | `current_dir` for Claude CLI |
| `generate_ideas` | `ideation.rs:30` | `current_dir` for Claude CLI |
| `github_investigate_issue` | `github.rs:248` | `current_dir` for Claude CLI |
| `parse_spec_to_tickets` | `spec.rs:4` | No path parameter (text-only input) |

### Impact

These commands set the working directory for spawned CLI processes. While the impact is limited to what the invoked CLI does with that directory (git reads repo data, Claude CLI analyzes code), it still allows operating on any directory on the system.

### Remediation

Create a shared validation utility and apply it to all path-accepting commands:

```rust
// src-tauri/src/commands/mod.rs or a new validation module
use std::path::Path;

pub fn validate_project_path(path: &str) -> Result<(), String> {
    let p = Path::new(path);
    if !p.is_dir() {
        return Err(format!("Not a valid directory: {}", path));
    }
    // Additional workspace scoping can be added here
    Ok(())
}
```

Apply this validation as the first operation in every command that accepts `project_path`.

### Risk of Change

Low. All callers already pass valid project directories.

---

## Testing Checklist

- [ ] Add `is_within_workspace` validation utility
- [ ] Apply workspace validation to `list_directory`
- [ ] Apply workspace validation to `analyze_code_quality`
- [ ] Add recursion depth limit (20) and file count cap (10,000) to `walk_dir`
- [ ] Add symlink detection to `walk_dir`
- [ ] Apply basic path validation to all `project_path` parameters
- [ ] Update frontend callers to pass `workspace_root` parameter where needed
- [ ] Test `list_directory` with project subdirectories (should work)
- [ ] Test `list_directory` with paths outside project (should be rejected)
- [ ] Test `analyze_code_quality` on a normal project (should work)
- [ ] Test `analyze_code_quality` with an outside path (should be rejected)
- [ ] Test `walk_dir` with a directory containing symlinks
- [ ] Verify `FileExplorer.tsx` still functions correctly

---

## References

- `src-tauri/src/commands/fs.rs` — lines 21–81
- `src-tauri/src/commands/code_quality.rs` — lines 251–392
- `src-tauri/src/commands/git.rs` — lines 4–47
- `src-tauri/src/commands/memory.rs` — lines 4–64
- `src-tauri/src/commands/insights.rs` — lines 11–35
- `src-tauri/src/commands/ideation.rs` — lines 4–30
- `src-tauri/src/commands/github.rs` — lines 213–261
- `src/components/explorer/FileExplorer.tsx` — directory listing calls
- `src/components/quality/CodeQualityModal.tsx` — quality analysis calls

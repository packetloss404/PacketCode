pub mod analytics;
pub mod code_quality;
pub mod deploy;
pub mod fs;
pub mod git;
pub mod github;
pub mod history;
pub mod ideation;
pub mod insights;
pub mod mcp;
pub mod memory;
pub mod pty;
pub mod scaffold;
pub mod shared;
pub mod spec;
pub mod statusline;

use std::path::Path;

/// Validate that a project path is a real, existing directory.
pub fn validate_project_path(path: &str) -> Result<(), String> {
    let p = Path::new(path);
    if path.is_empty() {
        return Err("Project path cannot be empty".to_string());
    }
    if !p.is_absolute() {
        return Err(format!("Project path must be absolute: {}", path));
    }
    if !p.is_dir() {
        return Err(format!("Project path is not a directory: {}", path));
    }
    Ok(())
}

/// Check that a path does not escape above the given workspace root via `..` or symlinks.
pub fn is_within_workspace(path: &str, workspace: &str) -> Result<(), String> {
    let canonical_workspace = std::fs::canonicalize(workspace)
        .map_err(|e| format!("Cannot resolve workspace '{}': {}", workspace, e))?;
    let canonical_path = std::fs::canonicalize(path)
        .map_err(|e| format!("Cannot resolve path '{}': {}", path, e))?;

    if !canonical_path.starts_with(&canonical_workspace) {
        return Err(format!(
            "Path '{}' is outside the workspace '{}'",
            path, workspace
        ));
    }
    Ok(())
}

/// Maximum allowed input size for text payloads (1 MB).
pub const MAX_INPUT_SIZE: usize = 1_000_000;

/// Maximum allowed size for PTY write payloads (64 KB).
pub const MAX_PTY_WRITE_SIZE: usize = 65_536;

/// Validate that an input string does not exceed the given size limit.
pub fn validate_input_size(input: &str, max_size: usize, field_name: &str) -> Result<(), String> {
    if input.len() > max_size {
        return Err(format!(
            "{} exceeds maximum size ({} bytes, limit {})",
            field_name,
            input.len(),
            max_size
        ));
    }
    Ok(())
}

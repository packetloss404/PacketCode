use std::path::PathBuf;

/// Discover the Claude CLI binary path.
/// Checks PATH first (via `where` on Windows), then common global install locations.
pub fn find_claude_binary() -> Option<PathBuf> {
    // Try `where claude` on Windows
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        if let Ok(output) = std::process::Command::new("where")
            .arg("claude")
            .creation_flags(0x08000000) // CREATE_NO_WINDOW
            .output()
        {
            if output.status.success() {
                let stdout = String::from_utf8_lossy(&output.stdout);
                if let Some(first_line) = stdout.lines().next() {
                    let path = PathBuf::from(first_line.trim());
                    if path.exists() {
                        return Some(path);
                    }
                }
            }
        }
    }

    // Try `which claude` on Unix
    #[cfg(not(windows))]
    {
        if let Ok(output) = std::process::Command::new("which")
            .arg("claude")
            .output()
        {
            if output.status.success() {
                let stdout = String::from_utf8_lossy(&output.stdout);
                if let Some(first_line) = stdout.lines().next() {
                    let path = PathBuf::from(first_line.trim());
                    if path.exists() {
                        return Some(path);
                    }
                }
            }
        }
    }

    // Check common npm global locations
    let home = std::env::var("USERPROFILE")
        .or_else(|_| std::env::var("HOME"))
        .unwrap_or_default();

    let candidates = vec![
        PathBuf::from(&home).join("AppData/Roaming/npm/claude.cmd"),
        PathBuf::from(&home).join("AppData/Roaming/npm/claude"),
        PathBuf::from(&home).join(".local/bin/claude.exe"),
        PathBuf::from(&home).join(".local/bin/claude"),
        PathBuf::from(&home).join(".nvm/versions"),
    ];

    for candidate in candidates {
        if candidate.exists() {
            return Some(candidate);
        }
    }

    // Fallback: just try "claude" and let the OS resolve it
    Some(PathBuf::from("claude"))
}

/// Build a `tokio::process::Command` for the Claude CLI with the correct binary path
/// and Windows console-hiding flags pre-applied.
pub fn claude_command() -> Result<tokio::process::Command, String> {
    let binary = find_claude_binary()
        .ok_or_else(|| "Failed to run Claude CLI: program not found. Is claude installed and on PATH?".to_string())?;

    let mut cmd = tokio::process::Command::new(binary);

    // Prevent "nested session" detection when PacketCode itself runs inside Claude Code
    cmd.env_remove("CLAUDECODE");
    cmd.env_remove("CLAUDE_CODE_ENTRYPOINT");

    #[cfg(windows)]
    {
        #[allow(unused_imports)]
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000);
    }

    Ok(cmd)
}

/// Check if Claude CLI is available and return its version string.
#[allow(dead_code)]
pub fn get_claude_version() -> Option<String> {
    let binary = find_claude_binary()?;

    let mut cmd = std::process::Command::new(binary);
    cmd.arg("--version");

    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000);
    }

    let output = cmd.output().ok()?;

    if output.status.success() {
        Some(String::from_utf8_lossy(&output.stdout).trim().to_string())
    } else {
        None
    }
}

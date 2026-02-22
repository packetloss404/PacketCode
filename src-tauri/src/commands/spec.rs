#[tauri::command]
pub async fn parse_spec_to_tickets(spec_text: String) -> Result<String, String> {
    let prompt = format!(
        r#"You are a project manager. Parse the following project spec into a JSON array of tickets.
Each ticket must have exactly these fields:
- "title": string (concise task title)
- "description": string (detailed description of what to implement)
- "priority": one of "low", "medium", "high", "critical"
- "labels": array of strings (relevant tags like "frontend", "backend", "api", "bug", "feature", etc.)
- "acceptanceCriteria": array of strings (testable conditions for completion)

Output ONLY the JSON array, no markdown fences, no explanation.

PROJECT SPEC:
{}
"#,
        spec_text
    );

    // On Windows, npm global installs use .cmd wrappers
    let command = if cfg!(windows) { "claude.cmd" } else { "claude" };

    let mut cmd = tokio::process::Command::new(command);
    cmd.args(&["-p", &prompt, "--output-format", "text"]);

    // Windows: hide console window
    #[cfg(windows)]
    {
        #[allow(unused_imports)]
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000);
    }

    let output = cmd
        .output()
        .await
        .map_err(|e| format!("Failed to run Claude CLI: {}. Is claude installed and on PATH?", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("Claude CLI exited with error: {}", stderr));
    }

    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    Ok(stdout)
}

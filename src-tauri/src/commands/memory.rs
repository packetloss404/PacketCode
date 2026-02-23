use crate::claude::binary::claude_command;

#[tauri::command]
pub async fn scan_codebase_memory(project_path: String) -> Result<String, String> {
    let prompt = r#"List the key files in this project with 1-line summaries. Output ONLY a JSON array with no markdown formatting, like: [{"path": "src/main.ts", "summary": "App entry point"}]. Include the most important 30-50 files."#;

    let mut cmd = claude_command()?;
    cmd.args(&["-p", prompt, "--output-format", "text"]);
    cmd.current_dir(&project_path);

    let output = cmd
        .output()
        .await
        .map_err(|e| format!("Failed to run Claude CLI: {}. Is claude installed and on PATH?", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("Claude CLI exited with error: {}", stderr));
    }

    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

#[tauri::command]
pub async fn summarize_session(project_path: String, session_log: String) -> Result<String, String> {
    let prompt = format!(
        r#"Summarize this coding session. Output ONLY a JSON object with no markdown formatting, like: {{"summary": "...", "keyDecisions": ["..."], "filesModified": ["..."]}}.

Session log:
{}"#,
        session_log
    );

    let mut cmd = claude_command()?;
    cmd.args(&["-p", &prompt, "--output-format", "text"]);
    cmd.current_dir(&project_path);

    let output = cmd
        .output()
        .await
        .map_err(|e| format!("Failed to run Claude CLI: {}. Is claude installed and on PATH?", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("Claude CLI exited with error: {}", stderr));
    }

    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

#[tauri::command]
pub async fn extract_patterns(project_path: String, summaries: String) -> Result<String, String> {
    let prompt = format!(
        r#"Given these session summaries, extract recurring patterns about the codebase. Output ONLY a JSON array with no markdown formatting, like: [{{"pattern": "Uses Zustand for state management", "category": "architecture", "confidence": 0.9}}].
Categories: architecture, convention, preference, pitfall.

Session summaries:
{}"#,
        summaries
    );

    let mut cmd = claude_command()?;
    cmd.args(&["-p", &prompt, "--output-format", "text"]);
    cmd.current_dir(&project_path);

    let output = cmd
        .output()
        .await
        .map_err(|e| format!("Failed to run Claude CLI: {}. Is claude installed and on PATH?", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("Claude CLI exited with error: {}", stderr));
    }

    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

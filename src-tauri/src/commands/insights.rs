use serde::Deserialize;

#[derive(Deserialize)]
pub struct InsightsMessage {
    pub role: String,
    pub content: String,
}

#[tauri::command]
pub async fn ask_insights(
    project_path: String,
    messages: Vec<InsightsMessage>,
) -> Result<String, String> {
    // Build conversation context from prior messages
    let mut context = String::new();
    for msg in &messages {
        let prefix = if msg.role == "user" { "User" } else { "Assistant" };
        context.push_str(&format!("{}: {}\n\n", prefix, msg.content));
    }

    let prompt = format!(
        r#"You are a helpful codebase assistant. You have access to the project files in the current directory.
Answer questions about the codebase accurately and concisely. When referencing code, include file paths and relevant snippets.

Conversation so far:
{}

Provide a helpful response to the user's latest message."#,
        context
    );

    let command = if cfg!(windows) { "claude.cmd" } else { "claude" };

    let mut cmd = tokio::process::Command::new(command);
    cmd.args(&["-p", &prompt, "--output-format", "text"]);
    cmd.current_dir(&project_path);

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

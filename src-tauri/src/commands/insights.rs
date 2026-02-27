use crate::claude::binary::{claude_command, run_claude};
use serde::Deserialize;
use tokio::io::{AsyncBufReadExt, BufReader};

#[derive(Deserialize)]
pub struct InsightsMessage {
    pub role: String,
    pub content: String,
}

fn build_insights_prompt(messages: &[InsightsMessage], context: Option<&str>) -> String {
    let mut conversation = String::new();
    for msg in messages {
        let prefix = if msg.role == "user" { "User" } else { "Assistant" };
        conversation.push_str(&format!("{}: {}\n\n", prefix, msg.content));
    }

    let context_section = match context {
        Some(ctx) if !ctx.is_empty() => format!(
            "\nAdditional context from the user's current session:\n{}\n",
            ctx
        ),
        _ => String::new(),
    };

    format!(
        r#"You are a helpful codebase assistant. You have access to the project files in the current directory.
Answer questions about the codebase accurately and concisely. When referencing code, include file paths and relevant snippets.
{context_section}
Conversation so far:
{conversation}
Provide a helpful response to the user's latest message."#,
    )
}

#[tauri::command]
pub async fn ask_insights(
    project_path: String,
    messages: Vec<InsightsMessage>,
) -> Result<String, String> {
    let prompt = build_insights_prompt(&messages, None);
    run_claude(&prompt, Some(&project_path)).await
}

#[tauri::command]
pub async fn ask_insights_stream(
    app_handle: tauri::AppHandle,
    project_path: String,
    messages: Vec<InsightsMessage>,
    session_context: Option<String>,
) -> Result<(), String> {
    let prompt = build_insights_prompt(&messages, session_context.as_deref());

    let mut cmd = claude_command()?;
    cmd.args(&["-p", &prompt, "--output-format", "text"]);
    cmd.current_dir(&project_path);
    cmd.stdout(std::process::Stdio::piped());
    cmd.stderr(std::process::Stdio::piped());

    let mut child = cmd
        .spawn()
        .map_err(|e| format!("Failed to spawn Claude CLI: {}", e))?;

    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| "Failed to capture stdout".to_string())?;

    let handle = app_handle.clone();
    tokio::spawn(async move {
        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();

        while let Ok(Some(line)) = lines.next_line().await {
            let _ = handle.emit("insights:chunk", &line);
        }

        let status = child.wait().await;
        let success = status.map(|s| s.success()).unwrap_or(false);
        let _ = handle.emit("insights:done", success);
    });

    Ok(())
}

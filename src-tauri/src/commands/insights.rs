use crate::claude::binary::run_claude;
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

    run_claude(&prompt, Some(&project_path)).await
}

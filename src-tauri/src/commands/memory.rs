use crate::claude::binary::run_claude;
use tracing::info;

#[tauri::command]
pub async fn scan_codebase_memory(project_path: String) -> Result<String, String> {
    super::validate_project_path(&project_path)?;
    info!(project_path = %project_path, "Scanning codebase memory");
    let prompt = r#"List the key files in this project with 1-line summaries. Output ONLY a JSON array with no markdown formatting, like: [{"path": "src/main.ts", "summary": "App entry point"}]. Include the most important 30-50 files."#;
    run_claude(prompt, Some(&project_path)).await
}

#[tauri::command]
pub async fn summarize_session(project_path: String, session_log: String) -> Result<String, String> {
    super::validate_project_path(&project_path)?;
    super::validate_input_size(&session_log, super::MAX_INPUT_SIZE, "Session log")?;
    let prompt = format!(
        r#"Summarize this coding session. Output ONLY a JSON object with no markdown formatting, like: {{"summary": "...", "keyDecisions": ["..."], "filesModified": ["..."]}}.

Session log:
{}"#,
        session_log
    );
    run_claude(&prompt, Some(&project_path)).await
}

#[tauri::command]
pub async fn extract_patterns(project_path: String, summaries: String) -> Result<String, String> {
    super::validate_project_path(&project_path)?;
    super::validate_input_size(&summaries, super::MAX_INPUT_SIZE, "Summaries")?;
    let prompt = format!(
        r#"Given these session summaries, extract recurring patterns about the codebase. Output ONLY a JSON array with no markdown formatting, like: [{{"pattern": "Uses Zustand for state management", "category": "architecture", "confidence": 0.9}}].
Categories: architecture, convention, preference, pitfall.

Session summaries:
{}"#,
        summaries
    );
    run_claude(&prompt, Some(&project_path)).await
}

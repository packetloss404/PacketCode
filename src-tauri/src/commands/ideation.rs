use crate::claude::binary::run_claude;
use tracing::info;

#[tauri::command]
pub async fn generate_ideas(
    project_path: String,
    idea_types: Vec<String>,
) -> Result<String, String> {
    super::validate_project_path(&project_path)?;
    info!(project_path = %project_path, types = ?idea_types, "Generating ideas");
    let types_str = idea_types.join(", ");

    let prompt = format!(
        r#"You are a senior software engineer performing a codebase audit. Analyze the project files in the current directory.

Generate improvement ideas for these categories: {}.

Output ONLY a JSON array (no markdown fences, no explanation). Each element must have exactly these fields:
- "type": one of "code_improvements", "security", "performance", "code_quality", "documentation", "ui_ux"
- "title": string (concise title)
- "description": string (detailed explanation of the issue or opportunity)
- "severity": one of "low", "medium", "high", "critical"
- "affectedFiles": array of file paths (relative to project root)
- "suggestion": string (specific actionable recommendation)
- "effort": one of "trivial", "small", "medium", "large"

Generate 5-15 ideas across the requested categories. Be specific and actionable."#,
        types_str
    );

    run_claude(&prompt, Some(&project_path)).await
}

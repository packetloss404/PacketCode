use crate::claude::binary::run_claude;

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

    run_claude(&prompt, None).await
}

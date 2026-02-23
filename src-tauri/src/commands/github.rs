use reqwest::header::{ACCEPT, AUTHORIZATION, USER_AGENT};

fn github_client(token: &str) -> Result<reqwest::Client, String> {
    let mut headers = reqwest::header::HeaderMap::new();
    headers.insert(
        AUTHORIZATION,
        format!("Bearer {}", token)
            .parse()
            .map_err(|e| format!("Invalid token header: {}", e))?,
    );
    headers.insert(
        ACCEPT,
        "application/vnd.github+json"
            .parse()
            .map_err(|e| format!("Invalid accept header: {}", e))?,
    );
    headers.insert(
        USER_AGENT,
        "PacketCode/1.0"
            .parse()
            .map_err(|e| format!("Invalid user-agent header: {}", e))?,
    );

    reqwest::Client::builder()
        .default_headers(headers)
        .build()
        .map_err(|e| format!("Failed to build HTTP client: {}", e))
}

#[tauri::command]
pub async fn github_list_repos(token: String) -> Result<String, String> {
    let client = github_client(&token)?;
    let resp = client
        .get("https://api.github.com/user/repos?sort=updated&per_page=30")
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        return Err(format!("GitHub API error {}: {}", status, body));
    }

    resp.text()
        .await
        .map_err(|e| format!("Failed to read response: {}", e))
}

#[tauri::command]
pub async fn github_list_issues(
    token: String,
    owner: String,
    repo: String,
) -> Result<String, String> {
    let client = github_client(&token)?;
    let url = format!(
        "https://api.github.com/repos/{}/{}/issues?state=open&per_page=50",
        owner, repo
    );
    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        return Err(format!("GitHub API error {}: {}", status, body));
    }

    resp.text()
        .await
        .map_err(|e| format!("Failed to read response: {}", e))
}

#[tauri::command]
pub async fn github_get_issue(
    token: String,
    owner: String,
    repo: String,
    issue_number: u32,
) -> Result<String, String> {
    let client = github_client(&token)?;
    let url = format!(
        "https://api.github.com/repos/{}/{}/issues/{}",
        owner, repo, issue_number
    );
    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        return Err(format!("GitHub API error {}: {}", status, body));
    }

    resp.text()
        .await
        .map_err(|e| format!("Failed to read response: {}", e))
}

#[tauri::command]
pub async fn github_create_pr(
    token: String,
    owner: String,
    repo: String,
    title: String,
    body: String,
    head: String,
    base: String,
) -> Result<String, String> {
    let client = github_client(&token)?;
    let url = format!(
        "https://api.github.com/repos/{}/{}/pulls",
        owner, repo
    );

    let payload = serde_json::json!({
        "title": title,
        "body": body,
        "head": head,
        "base": base,
    });

    let resp = client
        .post(&url)
        .json(&payload)
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;

    if !resp.status().is_success() {
        let status = resp.status();
        let resp_body = resp.text().await.unwrap_or_default();
        return Err(format!("GitHub API error {}: {}", status, resp_body));
    }

    resp.text()
        .await
        .map_err(|e| format!("Failed to read response: {}", e))
}

#[tauri::command]
pub async fn github_investigate_issue(
    project_path: String,
    token: String,
    owner: String,
    repo: String,
    issue_number: u32,
) -> Result<String, String> {
    // First fetch the issue details
    let issue_json = github_get_issue(token, owner, repo, issue_number).await?;
    let issue: serde_json::Value =
        serde_json::from_str(&issue_json).map_err(|e| format!("Failed to parse issue: {}", e))?;

    let title = issue["title"].as_str().unwrap_or("Unknown");
    let body = issue["body"].as_str().unwrap_or("No description");

    let prompt = format!(
        r#"Investigate this GitHub issue in the context of the current codebase:

Title: {}
Description: {}

Analyze the codebase and provide:
1. Which files are likely affected
2. Root cause analysis (if it's a bug)
3. Suggested implementation approach
4. Potential risks or edge cases

Be specific — reference actual file paths and code."#,
        title, body
    );

    let mut cmd = crate::claude::binary::claude_command()?;
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

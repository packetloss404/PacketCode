use reqwest::header::{ACCEPT, AUTHORIZATION, USER_AGENT};
use tauri::State;
use tokio::sync::RwLock;

pub struct GitHubAuthState {
    token: RwLock<Option<String>>,
}

impl GitHubAuthState {
    pub fn new() -> Self {
        Self {
            token: RwLock::new(None),
        }
    }
}

pub fn create_github_auth_state() -> GitHubAuthState {
    GitHubAuthState::new()
}

fn github_client(token: &str) -> Result<reqwest::Client, String> {
    let mut headers = reqwest::header::HeaderMap::new();
    headers.insert(
        AUTHORIZATION,
        format!("Bearer {}", token)
            .parse()
            .map_err(|e| format!("Invalid header: {}", e))?,
    );
    headers.insert(
        ACCEPT,
        "application/vnd.github+json"
            .parse()
            .map_err(|e| format!("Invalid header: {}", e))?,
    );
    headers.insert(
        USER_AGENT,
        "PacketCode/1.0"
            .parse()
            .map_err(|e| format!("Invalid header: {}", e))?,
    );

    reqwest::Client::builder()
        .default_headers(headers)
        .build()
        .map_err(|e| format!("Failed to build HTTP client: {}", e))
}

async fn github_response_text(resp: reqwest::Response) -> Result<String, String> {
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
pub async fn github_set_token(
    auth: State<'_, GitHubAuthState>,
    token: String,
) -> Result<(), String> {
    let trimmed = token.trim();
    if trimmed.is_empty() {
        return Err("GitHub token cannot be empty".to_string());
    }
    let mut guard = auth.token.write().await;
    *guard = Some(trimmed.to_string());
    Ok(())
}

#[tauri::command]
pub async fn github_clear_token(auth: State<'_, GitHubAuthState>) -> Result<(), String> {
    let mut guard = auth.token.write().await;
    *guard = None;
    Ok(())
}

#[tauri::command]
pub async fn github_has_token(auth: State<'_, GitHubAuthState>) -> Result<bool, String> {
    let guard = auth.token.read().await;
    Ok(guard.is_some())
}

async fn github_client_from_state(auth: &GitHubAuthState) -> Result<reqwest::Client, String> {
    let token = auth
        .token
        .read()
        .await
        .clone()
        .ok_or_else(|| "GitHub token not set. Connect first.".to_string())?;
    github_client(&token)
}

async fn github_get_issue_with_client(
    client: &reqwest::Client,
    owner: &str,
    repo: &str,
    issue_number: u32,
) -> Result<String, String> {
    let url = format!(
        "https://api.github.com/repos/{}/{}/issues/{}",
        owner, repo, issue_number
    );
    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;
    github_response_text(resp).await
}

#[tauri::command]
pub async fn github_list_repos(auth: State<'_, GitHubAuthState>) -> Result<String, String> {
    let client = github_client_from_state(auth.inner()).await?;
    let resp = client
        .get("https://api.github.com/user/repos?sort=updated&per_page=30")
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;
    github_response_text(resp).await
}

#[tauri::command]
pub async fn github_list_issues(
    auth: State<'_, GitHubAuthState>,
    owner: String,
    repo: String,
) -> Result<String, String> {
    let client = github_client_from_state(auth.inner()).await?;
    let url = format!(
        "https://api.github.com/repos/{}/{}/issues?state=open&per_page=50",
        owner, repo
    );
    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| format!("Request failed: {}", e))?;
    github_response_text(resp).await
}

#[tauri::command]
pub async fn github_get_issue(
    auth: State<'_, GitHubAuthState>,
    owner: String,
    repo: String,
    issue_number: u32,
) -> Result<String, String> {
    let client = github_client_from_state(auth.inner()).await?;
    github_get_issue_with_client(&client, &owner, &repo, issue_number).await
}

#[tauri::command]
pub async fn github_create_pr(
    auth: State<'_, GitHubAuthState>,
    owner: String,
    repo: String,
    title: String,
    body: String,
    head: String,
    base: String,
) -> Result<String, String> {
    let client = github_client_from_state(auth.inner()).await?;
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
    github_response_text(resp).await
}

#[tauri::command]
pub async fn github_investigate_issue(
    auth: State<'_, GitHubAuthState>,
    project_path: String,
    owner: String,
    repo: String,
    issue_number: u32,
) -> Result<String, String> {
    let client = github_client_from_state(auth.inner()).await?;

    let issue_json = github_get_issue_with_client(&client, &owner, &repo, issue_number).await?;
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

    crate::claude::binary::run_claude(&prompt, Some(&project_path)).await
}

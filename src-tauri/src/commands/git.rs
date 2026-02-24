use std::process::Command;
use super::shared::hide_window;

fn git_command(args: &[&str], cwd: &str) -> Result<std::process::Output, String> {
    let mut cmd = Command::new("git");
    cmd.args(args).current_dir(cwd);
    hide_window(&mut cmd);
    cmd.output()
        .map_err(|e| format!("Failed to run git: {}", e))
}

#[tauri::command]
pub async fn get_git_branch(project_path: String) -> Result<String, String> {
    let output = git_command(&["rev-parse", "--abbrev-ref", "HEAD"], &project_path)?;
    if output.status.success() {
        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    } else {
        Err("Not a git repository or git not found".to_string())
    }
}

#[tauri::command]
pub async fn get_git_status(project_path: String) -> Result<String, String> {
    let output = git_command(&["status", "--short"], &project_path)?;
    if output.status.success() {
        Ok(String::from_utf8_lossy(&output.stdout).to_string())
    } else {
        Err("Failed to get git status".to_string())
    }
}

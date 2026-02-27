use std::process::Command;
use super::shared::hide_window;

fn git_command(args: &[&str], cwd: &str) -> Result<std::process::Output, String> {
    let mut cmd = Command::new("git");
    cmd.args(args).current_dir(cwd);
    hide_window(&mut cmd);
    cmd.output()
        .map_err(|e| format!("Failed to run git: {}", e))
}

fn git_command_result(args: &[&str], cwd: &str) -> Result<String, String> {
    let output = git_command(args, cwd)?;
    if output.status.success() {
        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    } else {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        Err(if stderr.is_empty() {
            format!("git {} failed", args[0])
        } else {
            stderr
        })
    }
}

#[tauri::command]
pub async fn get_git_branch(project_path: String) -> Result<String, String> {
    git_command_result(&["rev-parse", "--abbrev-ref", "HEAD"], &project_path)
        .map_err(|_| "Not a git repository or git not found".to_string())
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

#[tauri::command]
pub async fn git_commit(
    project_path: String,
    message: String,
    stage_all: bool,
) -> Result<String, String> {
    if stage_all {
        git_command_result(&["add", "-A"], &project_path)?;
    }
    git_command_result(&["commit", "-m", &message], &project_path)
}

#[tauri::command]
pub async fn git_push(project_path: String) -> Result<String, String> {
    git_command_result(&["push"], &project_path)
}

#[tauri::command]
pub async fn git_pull(project_path: String) -> Result<String, String> {
    git_command_result(&["pull"], &project_path)
}

#[tauri::command]
pub async fn git_create_branch(
    project_path: String,
    branch_name: String,
    checkout: bool,
) -> Result<String, String> {
    if checkout {
        git_command_result(&["checkout", "-b", &branch_name], &project_path)
    } else {
        git_command_result(&["branch", &branch_name], &project_path)
    }
}

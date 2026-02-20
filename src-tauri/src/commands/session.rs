use std::sync::Arc;
use tauri::{AppHandle, Emitter, State};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::sync::Mutex;
use uuid::Uuid;

use crate::claude::messages::SessionOutputEvent;
use crate::session::manager::SharedSessionManager;
use crate::session::types::{SessionInfo, SessionState, SessionStatus};

#[tauri::command]
pub async fn create_session(
    app: AppHandle,
    manager: State<'_, SharedSessionManager>,
    project_path: String,
    prompt: String,
    model: Option<String>,
    resume_session_id: Option<String>,
) -> Result<String, String> {
    let session_id = Uuid::new_v4().to_string();

    let mut session = SessionState::new(
        session_id.clone(),
        project_path.clone(),
        model.clone(),
    );

    // Build the command arguments
    let mut args: Vec<String> = vec![
        "-p".to_string(),
        prompt.clone(),
        "--output-format".to_string(),
        "stream-json".to_string(),
        "--verbose".to_string(),
    ];

    if let Some(ref m) = model {
        args.push("--model".to_string());
        args.push(m.clone());
    }

    if let Some(ref resume_id) = resume_session_id {
        args.push("--resume".to_string());
        args.push(resume_id.clone());
    }

    // Spawn Claude CLI process
    let mut cmd = tokio::process::Command::new("claude");
    cmd.args(&args)
        .current_dir(&project_path)
        .stdin(std::process::Stdio::piped())
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped());

    // Windows: hide console window
    #[cfg(windows)]
    {
        #[allow(unused_imports)]
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000);
    }

    let mut child = cmd.spawn().map_err(|e| {
        format!("Failed to spawn Claude CLI: {}. Is claude installed and on PATH?", e)
    })?;

    let pid = child.id();
    session.info.pid = pid;
    session.info.status = SessionStatus::Running;

    // Take stdin for later writing
    let stdin = child.stdin.take();
    if let Some(stdin) = stdin {
        session.stdin_handle = Some(Arc::new(Mutex::new(stdin)));
    }

    // Take stdout for reading
    let stdout = child.stdout.take().ok_or("Failed to capture stdout")?;
    let stderr = child.stderr.take();

    // Store the child process
    session.child = Some(Arc::new(Mutex::new(child)));

    // Store session in manager
    {
        let mut mgr = manager.lock().await;
        mgr.add_session(session);
    }

    let sid = session_id.clone();
    let app_handle = app.clone();
    let mgr_clone = manager.inner().clone();

    // Spawn async task to read stdout JSONL
    tokio::spawn(async move {
        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();

        while let Ok(Some(line)) = lines.next_line().await {
            let line = line.trim().to_string();
            if line.is_empty() {
                continue;
            }

            // Try to extract session_id and cost from result messages
            if let Ok(parsed) = serde_json::from_str::<serde_json::Value>(&line) {
                if parsed.get("type").and_then(|t| t.as_str()) == Some("result") {
                    let mut mgr = mgr_clone.lock().await;
                    if let Some(sess) = mgr.get_session_mut(&sid) {
                        if let Some(cost) = parsed.get("cost_usd").and_then(|c| c.as_f64()) {
                            sess.info.cost_usd = cost;
                        }
                        if let Some(claude_sid) = parsed.get("session_id").and_then(|s| s.as_str()) {
                            sess.info.claude_session_id = Some(claude_sid.to_string());
                        }
                        if let Some(turns) = parsed.get("num_turns").and_then(|n| n.as_u64()) {
                            sess.info.num_turns = turns as u32;
                        }
                        sess.info.status = SessionStatus::Idle;
                    }
                }
            }

            let event = SessionOutputEvent {
                session_id: sid.clone(),
                raw_json: line,
            };

            let _ = app_handle.emit("session:output", &event);
        }

        // Process ended — mark session as terminated
        let mut mgr = mgr_clone.lock().await;
        if let Some(sess) = mgr.get_session_mut(&sid) {
            if sess.info.status == SessionStatus::Running {
                sess.info.status = SessionStatus::Terminated;
            }
        }

        let _ = app_handle.emit("session:ended", &sid);
    });

    // Spawn task to read stderr (for debug logging)
    if let Some(stderr) = stderr {
        let sid_err = session_id.clone();
        let app_err = app.clone();
        tokio::spawn(async move {
            let reader = BufReader::new(stderr);
            let mut lines = reader.lines();
            while let Ok(Some(line)) = lines.next_line().await {
                let _ = app_err.emit("session:stderr", serde_json::json!({
                    "session_id": sid_err,
                    "line": line
                }));
            }
        });
    }

    Ok(session_id)
}

#[tauri::command]
pub async fn send_input(
    manager: State<'_, SharedSessionManager>,
    session_id: String,
    input: String,
) -> Result<(), String> {
    let mgr = manager.lock().await;
    let session = mgr.get_session(&session_id)
        .ok_or_else(|| format!("Session {} not found", session_id))?;

    let stdin_handle = session.stdin_handle.clone()
        .ok_or("Session has no stdin handle")?;

    let mut stdin = stdin_handle.lock().await;
    stdin.write_all(input.as_bytes()).await
        .map_err(|e| format!("Failed to write to stdin: {}", e))?;
    stdin.write_all(b"\n").await
        .map_err(|e| format!("Failed to write newline: {}", e))?;
    stdin.flush().await
        .map_err(|e| format!("Failed to flush stdin: {}", e))?;

    Ok(())
}

#[tauri::command]
pub async fn kill_session(
    manager: State<'_, SharedSessionManager>,
    session_id: String,
) -> Result<(), String> {
    let mut mgr = manager.lock().await;
    if let Some(session) = mgr.get_session_mut(&session_id) {
        if let Some(child) = &session.child {
            let mut child = child.lock().await;
            let _ = child.kill().await;
        }
        session.info.status = SessionStatus::Terminated;
        session.child = None;
        session.stdin_handle = None;
        Ok(())
    } else {
        Err(format!("Session {} not found", session_id))
    }
}

#[tauri::command]
pub async fn list_sessions(
    manager: State<'_, SharedSessionManager>,
) -> Result<Vec<SessionInfo>, String> {
    let mgr = manager.lock().await;
    Ok(mgr.list_sessions().iter().map(|s| s.info.clone()).collect())
}

#[tauri::command]
pub async fn get_session_info(
    manager: State<'_, SharedSessionManager>,
    session_id: String,
) -> Result<SessionInfo, String> {
    let mgr = manager.lock().await;
    mgr.get_session(&session_id)
        .map(|s| s.info.clone())
        .ok_or_else(|| format!("Session {} not found", session_id))
}

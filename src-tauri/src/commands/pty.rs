use std::collections::HashMap;
use std::io::{Read, Write};
use std::sync::{Arc, Mutex};
use std::thread;

use portable_pty::{native_pty_system, CommandBuilder, MasterPty, PtySize};
use serde::Serialize;
use tauri::{AppHandle, Emitter, State};
use uuid::Uuid;

/// Data emitted to the frontend for each chunk of PTY output
#[derive(Clone, Serialize)]
pub struct PtyOutput {
    pub session_id: String,
    pub data: String,
}

/// Info about a running PTY session
#[derive(Clone, Serialize)]
pub struct PtySessionInfo {
    pub id: String,
    pub project_path: String,
    pub pid: Option<u32>,
    pub alive: bool,
}

/// Internal state for one PTY session
struct PtySession {
    info: PtySessionInfo,
    writer: Box<dyn Write + Send>,
    master: Box<dyn MasterPty + Send>,
    kill_flag: Arc<std::sync::atomic::AtomicBool>,
}

/// Manages all PTY sessions
pub struct PtyManager {
    sessions: HashMap<String, PtySession>,
}

impl PtyManager {
    pub fn new() -> Self {
        Self {
            sessions: HashMap::new(),
        }
    }
}

pub type SharedPtyManager = Arc<Mutex<PtyManager>>;

pub fn create_shared_pty_manager() -> SharedPtyManager {
    Arc::new(Mutex::new(PtyManager::new()))
}

#[tauri::command]
pub fn create_pty_session(
    app: AppHandle,
    manager: State<'_, SharedPtyManager>,
    project_path: String,
    cols: u16,
    rows: u16,
    command: String,
    args: Option<Vec<String>>,
) -> Result<String, String> {
    let session_id = Uuid::new_v4().to_string();

    let pty_system = native_pty_system();

    let pair = pty_system
        .openpty(PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        })
        .map_err(|e| format!("Failed to open PTY: {}", e))?;

    // Build the command: launch the specified CLI interactively
    // On Windows, npm global installs use .cmd wrappers (e.g. codex.cmd, claude.cmd)
    // CommandBuilder doesn't resolve .cmd like a shell does, so we must add the extension.
    let resolved_command = if cfg!(windows) && !command.ends_with(".cmd") && !command.ends_with(".exe") {
        format!("{}.cmd", command)
    } else {
        command.clone()
    };
    let mut cmd = CommandBuilder::new(&resolved_command);
    cmd.cwd(&project_path);

    // Append any extra arguments (e.g. --model)
    if let Some(extra_args) = &args {
        for arg in extra_args {
            cmd.arg(arg);
        }
    }

    // Clear env vars that make Claude think it's inside another session
    if command == "claude" {
        cmd.env_remove("CLAUDECODE");
        cmd.env_remove("CLAUDE_CODE_ENTRYPOINT");
        // Tell statusline.ps1 to suppress terminal output (PacketCode has its own native status bar)
        cmd.env("PACKETCODE", "1");
    }

    // Spawn the child process in the PTY
    let child = pair
        .slave
        .spawn_command(cmd)
        .map_err(|e| format!("Failed to spawn {} in PTY: {}. Is {} installed?", command, e, command))?;

    let pid = child.process_id();

    let writer = pair
        .master
        .take_writer()
        .map_err(|e| format!("Failed to take PTY writer: {}", e))?;

    let mut reader = pair
        .master
        .try_clone_reader()
        .map_err(|e| format!("Failed to clone PTY reader: {}", e))?;

    let kill_flag = Arc::new(std::sync::atomic::AtomicBool::new(false));

    let info = PtySessionInfo {
        id: session_id.clone(),
        project_path: project_path.clone(),
        pid,
        alive: true,
    };

    let session = PtySession {
        info: info.clone(),
        writer,
        master: pair.master,
        kill_flag: kill_flag.clone(),
    };

    {
        let mut mgr = manager.lock().map_err(|e| format!("Lock error: {}", e))?;
        mgr.sessions.insert(session_id.clone(), session);
    }

    // Spawn a thread to read PTY output and emit events
    let sid = session_id.clone();
    let app_handle = app.clone();
    let mgr_ref = manager.inner().clone();

    thread::spawn(move || {
        let mut buf = [0u8; 4096];
        loop {
            if kill_flag.load(std::sync::atomic::Ordering::Relaxed) {
                break;
            }

            match reader.read(&mut buf) {
                Ok(0) => break, // EOF — process exited
                Ok(n) => {
                    // Convert to string, replacing invalid UTF-8
                    let data = String::from_utf8_lossy(&buf[..n]).to_string();
                    let event = PtyOutput {
                        session_id: sid.clone(),
                        data,
                    };
                    let _ = app_handle.emit("pty:output", &event);
                }
                Err(e) => {
                    // On Windows, ERROR_BROKEN_PIPE means the child exited
                    let err_str = e.to_string();
                    if err_str.contains("broken pipe")
                        || err_str.contains("The pipe has been ended")
                        || e.kind() == std::io::ErrorKind::BrokenPipe
                    {
                        break;
                    }
                    // Transient errors — retry
                    thread::sleep(std::time::Duration::from_millis(10));
                }
            }
        }

        // Mark session as dead
        if let Ok(mut mgr) = mgr_ref.lock() {
            if let Some(session) = mgr.sessions.get_mut(&sid) {
                session.info.alive = false;
            }
        }

        let _ = app_handle.emit("pty:exit", &sid);
    });

    Ok(session_id)
}

#[tauri::command]
pub fn write_pty(
    manager: State<'_, SharedPtyManager>,
    session_id: String,
    data: String,
) -> Result<(), String> {
    let mut mgr = manager.lock().map_err(|e| format!("Lock error: {}", e))?;
    let session = mgr
        .sessions
        .get_mut(&session_id)
        .ok_or_else(|| format!("PTY session {} not found", session_id))?;

    session
        .writer
        .write_all(data.as_bytes())
        .map_err(|e| format!("Failed to write to PTY: {}", e))?;
    session
        .writer
        .flush()
        .map_err(|e| format!("Failed to flush PTY: {}", e))?;

    Ok(())
}

#[tauri::command]
pub fn resize_pty(
    manager: State<'_, SharedPtyManager>,
    session_id: String,
    cols: u16,
    rows: u16,
) -> Result<(), String> {
    let mgr = manager.lock().map_err(|e| format!("Lock error: {}", e))?;
    let session = mgr
        .sessions
        .get(&session_id)
        .ok_or_else(|| format!("PTY session {} not found", session_id))?;

    session
        .master
        .resize(PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        })
        .map_err(|e| format!("Failed to resize PTY: {}", e))?;

    Ok(())
}

#[tauri::command]
pub fn kill_pty(
    manager: State<'_, SharedPtyManager>,
    session_id: String,
) -> Result<(), String> {
    let mut mgr = manager.lock().map_err(|e| format!("Lock error: {}", e))?;
    if let Some(session) = mgr.sessions.get_mut(&session_id) {
        session
            .kill_flag
            .store(true, std::sync::atomic::Ordering::Relaxed);
        session.info.alive = false;
        // Drop the writer to close the PTY input, which signals the child
        // We replace with a dummy that does nothing
    }

    Ok(())
}

#[tauri::command]
pub fn list_pty_sessions(
    manager: State<'_, SharedPtyManager>,
) -> Result<Vec<PtySessionInfo>, String> {
    let mgr = manager.lock().map_err(|e| format!("Lock error: {}", e))?;
    Ok(mgr.sessions.values().map(|s| s.info.clone()).collect())
}

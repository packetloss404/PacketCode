use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::process::Child;
use tokio::sync::Mutex;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum SessionStatus {
    #[serde(rename = "idle")]
    Idle,
    #[serde(rename = "running")]
    Running,
    #[serde(rename = "waiting_input")]
    WaitingInput,
    #[serde(rename = "error")]
    Error,
    #[serde(rename = "terminated")]
    Terminated,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionInfo {
    pub id: String,
    pub claude_session_id: Option<String>,
    pub project_path: String,
    pub model: Option<String>,
    pub status: SessionStatus,
    pub pid: Option<u32>,
    pub cost_usd: f64,
    pub total_tokens: u64,
    pub num_turns: u32,
}

/// Internal state for a running session. Not directly serializable due to Child handle.
pub struct SessionState {
    pub info: SessionInfo,
    pub child: Option<Arc<Mutex<Child>>>,
    pub stdin_handle: Option<Arc<Mutex<tokio::process::ChildStdin>>>,
}

impl SessionState {
    pub fn new(id: String, project_path: String, model: Option<String>) -> Self {
        Self {
            info: SessionInfo {
                id,
                claude_session_id: None,
                project_path,
                model,
                status: SessionStatus::Idle,
                pid: None,
                cost_usd: 0.0,
                total_tokens: 0,
                num_turns: 0,
            },
            child: None,
            stdin_handle: None,
        }
    }
}

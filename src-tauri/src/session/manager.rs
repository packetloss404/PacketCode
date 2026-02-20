use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::Mutex;

use super::types::SessionState;

/// Manages all active Claude sessions.
/// Stored as Tauri managed state via Arc<Mutex<>>.
pub struct SessionManager {
    pub sessions: HashMap<String, SessionState>,
}

impl SessionManager {
    pub fn new() -> Self {
        Self {
            sessions: HashMap::new(),
        }
    }

    pub fn add_session(&mut self, session: SessionState) {
        self.sessions.insert(session.info.id.clone(), session);
    }

    pub fn get_session(&self, id: &str) -> Option<&SessionState> {
        self.sessions.get(id)
    }

    pub fn get_session_mut(&mut self, id: &str) -> Option<&mut SessionState> {
        self.sessions.get_mut(id)
    }

    #[allow(dead_code)]
    pub fn remove_session(&mut self, id: &str) -> Option<SessionState> {
        self.sessions.remove(id)
    }

    pub fn list_sessions(&self) -> Vec<&SessionState> {
        self.sessions.values().collect()
    }
}

/// Type alias used in Tauri managed state
pub type SharedSessionManager = Arc<Mutex<SessionManager>>;

pub fn create_shared_manager() -> SharedSessionManager {
    Arc::new(Mutex::new(SessionManager::new()))
}

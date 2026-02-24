use crate::commands::shared::home_dir;
use super::helpers::{now_epoch_seconds, STALE_SECONDS};
use serde::Serialize;
use std::fs;
use std::path::PathBuf;
use std::time::SystemTime;

#[derive(Debug, Serialize, Clone)]
pub struct StatusLineData {
    pub session_id: String,
    pub model: String,
    pub cwd: String,
    pub dir_name: String,
    pub context_percent: u32,
    pub context_current_k: u32,
    pub context_max_k: u32,
    pub git_branch: String,
    pub cost_usd: f64,
    pub cost_display: String,
    pub duration_minutes: u32,
    pub context_icon: String,
    pub timestamp: u64,
}

#[tauri::command]
pub fn read_statusline_states() -> Vec<StatusLineData> {
    let home = match home_dir() {
        Some(h) => h,
        None => return vec![],
    };

    let state_dir = PathBuf::from(&home).join(".claude").join("statusline-state");
    let entries = match fs::read_dir(&state_dir) {
        Ok(e) => e,
        Err(_) => return vec![],
    };

    let now = now_epoch_seconds();
    let mut results = Vec::new();

    for entry in entries.flatten() {
        let path = entry.path();

        let name = match path.file_name().and_then(|n| n.to_str()) {
            Some(n) => n.to_string(),
            None => continue,
        };
        if !name.ends_with(".json") || name.ends_with(".tmp") {
            continue;
        }

        if let Ok(meta) = fs::metadata(&path) {
            if let Ok(modified) = meta.modified() {
                if let Ok(age) = SystemTime::now().duration_since(modified) {
                    if age.as_secs() > STALE_SECONDS {
                        continue;
                    }
                }
            }
        }

        let content = match fs::read_to_string(&path) {
            Ok(c) => c,
            Err(_) => continue,
        };

        let parsed: serde_json::Value = match serde_json::from_str(&content) {
            Ok(v) => v,
            Err(_) => continue,
        };

        if let Some(ts) = parsed.get("timestamp").and_then(|v| v.as_u64()) {
            if now.saturating_sub(ts) > STALE_SECONDS {
                continue;
            }
        }

        let raw_model = parsed.get("model").and_then(|v| v.as_str()).unwrap_or("Unknown").trim();
        let model = raw_model
            .strip_suffix(" for Cursor")
            .or_else(|| raw_model.strip_suffix(" for cursor"))
            .unwrap_or(raw_model)
            .trim();
        let model = if model.is_empty() {
            "Claude Code".to_string()
        } else {
            model.to_string()
        };

        let data = StatusLineData {
            session_id: parsed.get("session_id").and_then(|v| v.as_str()).unwrap_or("unknown").to_string(),
            model,
            cwd: parsed.get("cwd").and_then(|v| v.as_str()).unwrap_or("").to_string(),
            dir_name: parsed.get("dir_name").and_then(|v| v.as_str()).unwrap_or("").to_string(),
            context_percent: parsed.get("context_percent").and_then(|v| v.as_u64()).unwrap_or(0) as u32,
            context_current_k: parsed.get("context_current_k").and_then(|v| v.as_u64()).unwrap_or(0) as u32,
            context_max_k: parsed.get("context_max_k").and_then(|v| v.as_u64()).unwrap_or(0) as u32,
            git_branch: parsed.get("git_branch").and_then(|v| v.as_str()).unwrap_or("-").to_string(),
            cost_usd: parsed.get("cost_usd").and_then(|v| v.as_f64()).unwrap_or(0.0),
            cost_display: parsed.get("cost_display").and_then(|v| v.as_str()).unwrap_or("$0.00").to_string(),
            duration_minutes: parsed.get("duration_minutes").and_then(|v| v.as_u64()).unwrap_or(0) as u32,
            context_icon: parsed.get("context_icon").and_then(|v| v.as_str()).unwrap_or("").to_string(),
            timestamp: parsed.get("timestamp").and_then(|v| v.as_u64()).unwrap_or(0),
        };

        results.push(data);
    }

    results
}

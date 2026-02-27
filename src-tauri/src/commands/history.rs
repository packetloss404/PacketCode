use crate::commands::shared::home_dir;
use serde::{Deserialize, Serialize};
use std::fs;
use std::io::{BufRead, BufReader};
use std::path::PathBuf;

#[derive(Debug, Serialize, Clone)]
pub struct HistoryEntry {
    pub display: String,
    pub timestamp: f64,
    pub project: String,
    #[serde(rename = "sessionId")]
    pub session_id: String,
}

/// Raw JSONL entry shape from ~/.claude/projects/*/history.jsonl
#[derive(Debug, Deserialize)]
struct RawHistoryEntry {
    #[serde(default)]
    display: Option<String>,
    #[serde(default, alias = "message")]
    message: Option<String>,
    #[serde(default)]
    timestamp: Option<f64>,
    #[serde(default)]
    project: Option<String>,
    #[serde(default, rename = "sessionId")]
    session_id: Option<String>,
}

#[tauri::command]
pub fn read_prompt_history() -> String {
    let home = match home_dir() {
        Some(h) => h,
        None => return "[]".to_string(),
    };

    let mut entries: Vec<HistoryEntry> = Vec::new();

    // Try the main history file
    let main_history = PathBuf::from(&home).join(".claude").join("history.jsonl");
    if main_history.exists() {
        read_jsonl_file(&main_history, "", &mut entries);
    }

    // Also scan per-project history files
    let projects_dir = PathBuf::from(&home).join(".claude").join("projects");
    if let Ok(project_entries) = fs::read_dir(&projects_dir) {
        for project_entry in project_entries.flatten() {
            let project_path = project_entry.path();
            if !project_path.is_dir() {
                continue;
            }

            let project_name = project_path
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("unknown")
                .to_string();

            // Look for history.jsonl in the project directory
            let history_file = project_path.join("history.jsonl");
            if history_file.exists() {
                read_jsonl_file(&history_file, &project_name, &mut entries);
            }
        }
    }

    // Sort by timestamp descending (newest first)
    entries.sort_by(|a, b| {
        b.timestamp
            .partial_cmp(&a.timestamp)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    // Cap at 2000 entries to avoid huge payloads
    entries.truncate(2000);

    serde_json::to_string(&entries).unwrap_or_else(|_| "[]".to_string())
}

fn read_jsonl_file(path: &PathBuf, default_project: &str, entries: &mut Vec<HistoryEntry>) {
    let file = match fs::File::open(path) {
        Ok(f) => f,
        Err(_) => return,
    };

    let reader = BufReader::new(file);
    for line in reader.lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => continue,
        };

        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }

        if let Ok(raw) = serde_json::from_str::<RawHistoryEntry>(trimmed) {
            let display = raw.display.or(raw.message).unwrap_or_default();
            if display.is_empty() {
                continue;
            }

            entries.push(HistoryEntry {
                display,
                timestamp: raw.timestamp.unwrap_or(0.0),
                project: raw.project.unwrap_or_else(|| default_project.to_string()),
                session_id: raw.session_id.unwrap_or_default(),
            });
        }
    }
}

use serde::Serialize;
use std::fs;
use std::io::{Read, Seek, SeekFrom};
use std::path::PathBuf;
use std::time::{SystemTime, UNIX_EPOCH};

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
    let home = match std::env::var("USERPROFILE")
        .or_else(|_| std::env::var("HOME"))
    {
        Ok(h) => h,
        Err(_) => return vec![],
    };

    let state_dir = PathBuf::from(&home)
        .join(".claude")
        .join("statusline-state");

    let entries = match fs::read_dir(&state_dir) {
        Ok(e) => e,
        Err(_) => return vec![],
    };

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0);

    let mut results = Vec::new();

    for entry in entries.flatten() {
        let path = entry.path();

        // Skip non-json and tmp files
        let name = match path.file_name().and_then(|n| n.to_str()) {
            Some(n) => n.to_string(),
            None => continue,
        };
        if !name.ends_with(".json") || name.ends_with(".tmp") {
            continue;
        }

        // Skip stale files (>5 minutes old by filesystem mtime)
        if let Ok(meta) = fs::metadata(&path) {
            if let Ok(modified) = meta.modified() {
                if let Ok(age) = SystemTime::now().duration_since(modified) {
                    if age.as_secs() > 300 {
                        continue;
                    }
                }
            }
        }

        // Read and parse
        let content = match fs::read_to_string(&path) {
            Ok(c) => c,
            Err(_) => continue,
        };

        let parsed: serde_json::Value = match serde_json::from_str(&content) {
            Ok(v) => v,
            Err(_) => continue,
        };

        // Also skip by timestamp field if present (>5 min stale)
        if let Some(ts) = parsed.get("timestamp").and_then(|v| v.as_u64()) {
            if now.saturating_sub(ts) > 300 {
                continue;
            }
        }

        let data = StatusLineData {
            session_id: parsed
                .get("session_id")
                .and_then(|v| v.as_str())
                .unwrap_or("unknown")
                .to_string(),
            model: parsed
                .get("model")
                .and_then(|v| v.as_str())
                .unwrap_or("Unknown")
                .to_string(),
            cwd: parsed
                .get("cwd")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string(),
            dir_name: parsed
                .get("dir_name")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string(),
            context_percent: parsed
                .get("context_percent")
                .and_then(|v| v.as_u64())
                .unwrap_or(0) as u32,
            context_current_k: parsed
                .get("context_current_k")
                .and_then(|v| v.as_u64())
                .unwrap_or(0) as u32,
            context_max_k: parsed
                .get("context_max_k")
                .and_then(|v| v.as_u64())
                .unwrap_or(0) as u32,
            git_branch: parsed
                .get("git_branch")
                .and_then(|v| v.as_str())
                .unwrap_or("-")
                .to_string(),
            cost_usd: parsed
                .get("cost_usd")
                .and_then(|v| v.as_f64())
                .unwrap_or(0.0),
            cost_display: parsed
                .get("cost_display")
                .and_then(|v| v.as_str())
                .unwrap_or("$0.00")
                .to_string(),
            duration_minutes: parsed
                .get("duration_minutes")
                .and_then(|v| v.as_u64())
                .unwrap_or(0) as u32,
            context_icon: parsed
                .get("context_icon")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string(),
            timestamp: parsed
                .get("timestamp")
                .and_then(|v| v.as_u64())
                .unwrap_or(0),
        };

        results.push(data);
    }

    results
}

#[derive(Debug, Serialize, Clone)]
pub struct CodexStatusLineData {
    pub session_id: String,
    pub model: String,
    pub reasoning_effort: String,
    pub cwd: String,
    pub input_tokens: u64,
    pub output_tokens: u64,
    pub reasoning_tokens: u64,
    pub cached_tokens: u64,
    pub total_tokens: u64,
    pub context_window: u64,
    pub context_percent: u32,
    pub rate_limit_primary_pct: f64,
    pub rate_limit_secondary_pct: f64,
    pub cli_version: String,
    pub timestamp: u64,
}

/// Read Codex config.toml for model and reasoning_effort
fn read_codex_config(home: &str) -> (String, String) {
    let config_path = PathBuf::from(home).join(".codex").join("config.toml");
    let content = match fs::read_to_string(&config_path) {
        Ok(c) => c,
        Err(_) => return ("unknown".to_string(), "medium".to_string()),
    };

    let parsed: toml::Value = match content.parse() {
        Ok(v) => v,
        Err(_) => return ("unknown".to_string(), "medium".to_string()),
    };

    let model = parsed
        .get("model")
        .and_then(|v| v.as_str())
        .unwrap_or("unknown")
        .to_string();

    // Codex uses "model_reasoning_effort" in config.toml
    let effort = parsed
        .get("model_reasoning_effort")
        .or_else(|| parsed.get("reasoning_effort"))
        .or_else(|| parsed.get("reasoning").and_then(|r| r.get("effort")))
        .and_then(|v| v.as_str())
        .unwrap_or("medium")
        .to_string();

    (model, effort)
}

/// Parse an ISO 8601 timestamp string to Unix epoch seconds
fn iso_to_epoch(ts: &str) -> u64 {
    // Handle format like "2026-02-19T22:30:32.356Z"
    // Simple parser: extract date/time parts
    let ts = ts.trim_end_matches('Z');
    let parts: Vec<&str> = ts.split('T').collect();
    if parts.len() != 2 {
        return 0;
    }
    let date_parts: Vec<u64> = parts[0].split('-').filter_map(|s| s.parse().ok()).collect();
    if date_parts.len() != 3 {
        return 0;
    }
    let time_str = parts[1].split('.').next().unwrap_or("00:00:00");
    let time_parts: Vec<u64> = time_str.split(':').filter_map(|s| s.parse().ok()).collect();
    if time_parts.len() != 3 {
        return 0;
    }

    let (year, month, day) = (date_parts[0], date_parts[1], date_parts[2]);
    let (hour, min, sec) = (time_parts[0], time_parts[1], time_parts[2]);

    // Days from epoch (1970-01-01) — simplified calculation
    let mut days: u64 = 0;
    for y in 1970..year {
        days += if y % 4 == 0 && (y % 100 != 0 || y % 400 == 0) { 366 } else { 365 };
    }
    let month_days = [0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
    let is_leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
    for m in 1..month {
        days += month_days[m as usize];
        if m == 2 && is_leap {
            days += 1;
        }
    }
    days += day - 1;

    days * 86400 + hour * 3600 + min * 60 + sec
}

/// Tail-read the last `n` bytes of a file and return the lines
fn tail_read_lines(path: &PathBuf, n: u64) -> Vec<String> {
    let mut file = match fs::File::open(path) {
        Ok(f) => f,
        Err(_) => return vec![],
    };

    let metadata = match file.metadata() {
        Ok(m) => m,
        Err(_) => return vec![],
    };

    let file_size = metadata.len();
    let seek_pos = if file_size > n { file_size - n } else { 0 };

    if file.seek(SeekFrom::Start(seek_pos)).is_err() {
        return vec![];
    }

    let mut buf = String::new();
    if file.read_to_string(&mut buf).is_err() {
        return vec![];
    }

    let mut lines: Vec<String> = buf.lines().map(|l| l.to_string()).collect();

    // If we seeked into the middle of a line, drop the first partial line
    if seek_pos > 0 && !lines.is_empty() {
        lines.remove(0);
    }

    lines
}

/// Recursively collect JSONL files from the sessions directory
fn collect_jsonl_files(dir: &PathBuf, files: &mut Vec<PathBuf>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            collect_jsonl_files(&path, files);
        } else if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
            if name.ends_with(".jsonl") {
                files.push(path);
            }
        }
    }
}

#[tauri::command]
pub fn read_codex_statusline_states() -> Vec<CodexStatusLineData> {
    let home = match std::env::var("USERPROFILE")
        .or_else(|_| std::env::var("HOME"))
    {
        Ok(h) => h,
        Err(_) => return vec![],
    };

    let (config_model, config_effort) = read_codex_config(&home);

    let sessions_dir = PathBuf::from(&home).join(".codex").join("sessions");
    if !sessions_dir.exists() {
        return vec![];
    }

    // Collect all JSONL files
    let mut jsonl_files = Vec::new();
    collect_jsonl_files(&sessions_dir, &mut jsonl_files);

    // Filter to files modified in the last 5 minutes
    let mut recent_files = Vec::new();
    for path in jsonl_files {
        if let Ok(meta) = fs::metadata(&path) {
            if let Ok(modified) = meta.modified() {
                if let Ok(age) = SystemTime::now().duration_since(modified) {
                    if age.as_secs() <= 300 {
                        recent_files.push(path);
                    }
                }
            }
        }
    }

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0);

    let mut results = Vec::new();

    for path in recent_files {
        // Read the first line for session_meta
        let first_line = match fs::read_to_string(&path) {
            Ok(content) => content.lines().next().unwrap_or("").to_string(),
            Err(_) => continue,
        };

        let meta_parsed: serde_json::Value = match serde_json::from_str(&first_line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        // Actual format: {"type":"session_meta","payload":{"id":"...","cwd":"...","cli_version":"..."}}
        let event_type = meta_parsed
            .get("type")
            .and_then(|v| v.as_str())
            .unwrap_or("");

        let payload = meta_parsed.get("payload");

        let (session_id, cwd, cli_version) = if event_type == "session_meta" {
            let p = match payload {
                Some(p) => p,
                None => continue,
            };
            let sid = p.get("id")
                .or_else(|| p.get("session_id"))
                .and_then(|v| v.as_str())
                .unwrap_or("unknown")
                .to_string();
            let c = p.get("cwd")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let ver = p.get("cli_version")
                .or_else(|| p.get("version"))
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            (sid, c, ver)
        } else {
            // Fallback: try payload or top-level
            let p = payload.unwrap_or(&meta_parsed);
            let sid = p.get("id")
                .or_else(|| p.get("session_id"))
                .and_then(|v| v.as_str())
                .unwrap_or("unknown")
                .to_string();
            let c = p.get("cwd")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            (sid, c, String::new())
        };

        // Skip if no CWD (can't match to a pane)
        if cwd.is_empty() {
            continue;
        }

        // Tail-read the last 16KB to find the most recent token_count event
        let tail_lines = tail_read_lines(&path, 16384);

        let mut latest_token_payload: Option<serde_json::Value> = None;
        let mut latest_timestamp: u64 = 0;
        let mut latest_model: Option<String> = None;

        // Scan from end to find the most recent token_count and turn_context events
        for line in tail_lines.iter().rev() {
            let parsed: serde_json::Value = match serde_json::from_str(line) {
                Ok(v) => v,
                Err(_) => continue,
            };

            let top_type = parsed.get("type").and_then(|v| v.as_str()).unwrap_or("");

            // Actual format: {"type":"event_msg","payload":{"type":"token_count","info":{...},"rate_limits":{...}}}
            if top_type == "event_msg" {
                let inner_payload = match parsed.get("payload") {
                    Some(p) => p,
                    None => continue,
                };
                let inner_type = inner_payload.get("type").and_then(|v| v.as_str()).unwrap_or("");

                if inner_type == "token_count" && latest_token_payload.is_none() {
                    // Parse ISO timestamp from top-level
                    let ts_str = parsed.get("timestamp")
                        .and_then(|v| v.as_str())
                        .unwrap_or("");
                    let ts = iso_to_epoch(ts_str);

                    latest_timestamp = ts;
                    latest_token_payload = Some(inner_payload.clone());
                }
            }

            // Also grab model from turn_context events
            if top_type == "turn_context" && latest_model.is_none() {
                if let Some(p) = parsed.get("payload") {
                    if let Some(m) = p.get("model").and_then(|v| v.as_str()) {
                        latest_model = Some(m.to_string());
                    }
                }
            }

            // Stop early once we have both
            if latest_token_payload.is_some() && latest_model.is_some() {
                break;
            }
        }

        // Use file mtime as fallback timestamp
        let file_timestamp = fs::metadata(&path)
            .ok()
            .and_then(|m| m.modified().ok())
            .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
            .map(|d| d.as_secs())
            .unwrap_or(0);

        let timestamp = if latest_timestamp > 0 {
            latest_timestamp
        } else {
            file_timestamp
        };

        // Extract token data from the payload
        // Format: payload.info.total_token_usage.{input_tokens, output_tokens, ...}
        // Context window: payload.info.model_context_window
        // Rate limits: payload.rate_limits.primary.used_percent
        let (input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens,
             context_window, rate_limit_primary, rate_limit_secondary) =
            if let Some(ref p) = latest_token_payload {
                let info = p.get("info");
                let usage = info.and_then(|i| i.get("total_token_usage"));

                let inp = usage
                    .and_then(|u| u.get("input_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                let out = usage
                    .and_then(|u| u.get("output_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                let reas = usage
                    .and_then(|u| u.get("reasoning_output_tokens"))
                    .or_else(|| usage.and_then(|u| u.get("reasoning_tokens")))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                let cached = usage
                    .and_then(|u| u.get("cached_input_tokens"))
                    .or_else(|| usage.and_then(|u| u.get("cached_tokens")))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                let total = usage
                    .and_then(|u| u.get("total_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(inp + out);
                let ctx_win = info
                    .and_then(|i| i.get("model_context_window"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(200_000);

                let rate_limits = p.get("rate_limits");
                let rl_primary = rate_limits
                    .and_then(|r| r.get("primary"))
                    .and_then(|r| r.get("used_percent"))
                    .and_then(|v| v.as_f64())
                    .unwrap_or(0.0);
                let rl_secondary = rate_limits
                    .and_then(|r| r.get("secondary"))
                    .and_then(|r| r.get("used_percent"))
                    .and_then(|v| v.as_f64())
                    .unwrap_or(0.0);

                (inp, out, reas, cached, total, ctx_win, rl_primary, rl_secondary)
            } else {
                (0, 0, 0, 0, 0, 200_000u64, 0.0, 0.0)
            };

        let context_percent = if context_window > 0 {
            ((total_tokens as f64 / context_window as f64) * 100.0).round() as u32
        } else {
            0
        };

        // Use model from turn_context, then config fallback
        let model = latest_model
            .unwrap_or_else(|| config_model.clone());

        let data = CodexStatusLineData {
            session_id,
            model,
            reasoning_effort: config_effort.clone(),
            cwd,
            input_tokens,
            output_tokens,
            reasoning_tokens,
            cached_tokens,
            total_tokens,
            context_window,
            context_percent,
            rate_limit_primary_pct: rate_limit_primary,
            rate_limit_secondary_pct: rate_limit_secondary,
            cli_version,
            timestamp: if timestamp > 0 { timestamp } else { now },
        };

        results.push(data);
    }

    results
}

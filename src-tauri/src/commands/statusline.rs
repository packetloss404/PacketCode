use serde::Serialize;
use std::collections::{HashMap, HashSet};
use std::fs;
use std::io::{BufRead, BufReader, Read, Seek, SeekFrom};
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};
use std::time::{SystemTime, UNIX_EPOCH};

const STALE_SECONDS: u64 = 300;
const CODEX_TAIL_BYTES: u64 = 16_384;
const CODEX_FULL_SCAN_INTERVAL_SECONDS: u64 = 30;

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

fn now_epoch_seconds() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

#[tauri::command]
pub fn read_statusline_states() -> Vec<StatusLineData> {
    let home = match std::env::var("USERPROFILE").or_else(|_| std::env::var("HOME")) {
        Ok(h) => h,
        Err(_) => return vec![],
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

        // Skip non-json and tmp files
        let name = match path.file_name().and_then(|n| n.to_str()) {
            Some(n) => n.to_string(),
            None => continue,
        };
        if !name.ends_with(".json") || name.ends_with(".tmp") {
            continue;
        }

        // Skip stale files by filesystem mtime
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

        // Skip by timestamp if present
        if let Some(ts) = parsed.get("timestamp").and_then(|v| v.as_u64()) {
            if now.saturating_sub(ts) > STALE_SECONDS {
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

#[derive(Clone, Default)]
struct CachedCodexSessionMeta {
    session_id: String,
    cwd: String,
    cli_version: String,
}

#[derive(Clone, Default)]
struct CachedCodexFile {
    modified_epoch: u64,
    len: u64,
    meta: Option<CachedCodexSessionMeta>,
    data: Option<CodexStatusLineData>,
}

#[derive(Default)]
struct CodexStatusCache {
    files: HashMap<String, CachedCodexFile>,
    last_full_scan_epoch: u64,
}

static CODEX_STATUS_CACHE: OnceLock<Mutex<CodexStatusCache>> = OnceLock::new();

fn codex_status_cache() -> &'static Mutex<CodexStatusCache> {
    CODEX_STATUS_CACHE.get_or_init(|| Mutex::new(CodexStatusCache::default()))
}

#[derive(Debug)]
struct CodexTailSnapshot {
    latest_timestamp: u64,
    latest_model: Option<String>,
    input_tokens: u64,
    output_tokens: u64,
    reasoning_tokens: u64,
    cached_tokens: u64,
    total_tokens: u64,
    context_window: u64,
    rate_limit_primary_pct: f64,
    rate_limit_secondary_pct: f64,
    has_token_count: bool,
}

impl Default for CodexTailSnapshot {
    fn default() -> Self {
        Self {
            latest_timestamp: 0,
            latest_model: None,
            input_tokens: 0,
            output_tokens: 0,
            reasoning_tokens: 0,
            cached_tokens: 0,
            total_tokens: 0,
            context_window: 200_000,
            rate_limit_primary_pct: 0.0,
            rate_limit_secondary_pct: 0.0,
            has_token_count: false,
        }
    }
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

    // Days from epoch (1970-01-01) -- simplified calculation
    let mut days: u64 = 0;
    for y in 1970..year {
        days += if y % 4 == 0 && (y % 100 != 0 || y % 400 == 0) {
            366
        } else {
            365
        };
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

    days * 86_400 + hour * 3_600 + min * 60 + sec
}

/// Tail-read the last `n` bytes of a file and return the lines
fn tail_read_lines(path: &Path, n: u64) -> Vec<String> {
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

fn read_first_line(path: &Path) -> Option<String> {
    let file = fs::File::open(path).ok()?;
    let mut reader = BufReader::new(file);
    let mut line = String::new();
    let bytes = reader.read_line(&mut line).ok()?;
    if bytes == 0 {
        return None;
    }
    Some(line.trim_end_matches(&['\n', '\r'][..]).to_string())
}

/// Recursively collect JSONL files from the sessions directory
fn collect_jsonl_files(dir: &Path, files: &mut Vec<PathBuf>) {
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

fn metadata_modified_epoch(metadata: &fs::Metadata) -> u64 {
    metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

fn parse_codex_session_meta(first_line: &str) -> Option<CachedCodexSessionMeta> {
    let parsed: serde_json::Value = serde_json::from_str(first_line).ok()?;
    let event_type = parsed.get("type").and_then(|v| v.as_str()).unwrap_or("");
    let payload = parsed.get("payload");

    let (session_id, cwd, cli_version) = if event_type == "session_meta" {
        let p = payload?;
        let sid = p
            .get("id")
            .or_else(|| p.get("session_id"))
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_string();
        let c = p
            .get("cwd")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let ver = p
            .get("cli_version")
            .or_else(|| p.get("version"))
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        (sid, c, ver)
    } else {
        // Fallback: try payload or top-level
        let p = payload.unwrap_or(&parsed);
        let sid = p
            .get("id")
            .or_else(|| p.get("session_id"))
            .and_then(|v| v.as_str())
            .unwrap_or("unknown")
            .to_string();
        let c = p
            .get("cwd")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let ver = p
            .get("cli_version")
            .or_else(|| p.get("version"))
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        (sid, c, ver)
    };

    Some(CachedCodexSessionMeta {
        session_id,
        cwd,
        cli_version,
    })
}

fn parse_codex_tail_snapshot(path: &Path) -> CodexTailSnapshot {
    let tail_lines = tail_read_lines(path, CODEX_TAIL_BYTES);
    let mut snapshot = CodexTailSnapshot::default();

    // Scan from the end to find the most recent token_count and model context.
    for line in tail_lines.iter().rev() {
        let parsed: serde_json::Value = match serde_json::from_str(line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        let top_type = parsed.get("type").and_then(|v| v.as_str()).unwrap_or("");

        if !snapshot.has_token_count && top_type == "event_msg" {
            let inner_payload = match parsed.get("payload") {
                Some(p) => p,
                None => continue,
            };
            let inner_type = inner_payload
                .get("type")
                .and_then(|v| v.as_str())
                .unwrap_or("");

            if inner_type == "token_count" {
                snapshot.has_token_count = true;

                let ts_str = parsed
                    .get("timestamp")
                    .and_then(|v| v.as_str())
                    .unwrap_or("");
                snapshot.latest_timestamp = iso_to_epoch(ts_str);

                let info = inner_payload.get("info");
                let usage = info.and_then(|i| i.get("total_token_usage"));

                let input_tokens = usage
                    .and_then(|u| u.get("input_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                let output_tokens = usage
                    .and_then(|u| u.get("output_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);

                snapshot.input_tokens = input_tokens;
                snapshot.output_tokens = output_tokens;
                snapshot.reasoning_tokens = usage
                    .and_then(|u| u.get("reasoning_output_tokens"))
                    .or_else(|| usage.and_then(|u| u.get("reasoning_tokens")))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                snapshot.cached_tokens = usage
                    .and_then(|u| u.get("cached_input_tokens"))
                    .or_else(|| usage.and_then(|u| u.get("cached_tokens")))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0);
                snapshot.total_tokens = usage
                    .and_then(|u| u.get("total_tokens"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(input_tokens + output_tokens);
                snapshot.context_window = info
                    .and_then(|i| i.get("model_context_window"))
                    .and_then(|v| v.as_u64())
                    .unwrap_or(200_000);

                let rate_limits = inner_payload.get("rate_limits");
                snapshot.rate_limit_primary_pct = rate_limits
                    .and_then(|r| r.get("primary"))
                    .and_then(|r| r.get("used_percent"))
                    .and_then(|v| v.as_f64())
                    .unwrap_or(0.0);
                snapshot.rate_limit_secondary_pct = rate_limits
                    .and_then(|r| r.get("secondary"))
                    .and_then(|r| r.get("used_percent"))
                    .and_then(|v| v.as_f64())
                    .unwrap_or(0.0);
            }
        }

        if snapshot.latest_model.is_none() && top_type == "turn_context" {
            if let Some(payload) = parsed.get("payload") {
                if let Some(model) = payload.get("model").and_then(|v| v.as_str()) {
                    snapshot.latest_model = Some(model.to_string());
                }
            }
        }

        if snapshot.has_token_count && snapshot.latest_model.is_some() {
            break;
        }
    }

    snapshot
}

fn should_full_scan(cache: &CodexStatusCache, now: u64) -> bool {
    cache.files.is_empty()
        || now.saturating_sub(cache.last_full_scan_epoch) >= CODEX_FULL_SCAN_INTERVAL_SECONDS
}

#[tauri::command]
pub fn read_codex_statusline_states() -> Vec<CodexStatusLineData> {
    let home = match std::env::var("USERPROFILE").or_else(|_| std::env::var("HOME")) {
        Ok(h) => h,
        Err(_) => return vec![],
    };

    let (config_model, config_effort) = read_codex_config(&home);
    let sessions_dir = PathBuf::from(&home).join(".codex").join("sessions");
    if !sessions_dir.exists() {
        return vec![];
    }

    let now = now_epoch_seconds();

    // Snapshot cache with bounded full-file discovery cadence.
    let snapshot = {
        let mut cache = match codex_status_cache().lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };

        if should_full_scan(&cache, now) {
            let mut discovered = Vec::new();
            collect_jsonl_files(&sessions_dir, &mut discovered);

            let discovered_set: HashSet<String> = discovered
                .into_iter()
                .map(|p| p.to_string_lossy().to_string())
                .collect();

            cache.files.retain(|path, _| discovered_set.contains(path));
            for path in discovered_set {
                cache.files.entry(path).or_default();
            }

            cache.last_full_scan_epoch = now;
        }

        cache.files.clone()
    };

    if snapshot.is_empty() {
        return vec![];
    }

    let mut results = Vec::new();
    let mut next_cache: HashMap<String, CachedCodexFile> = HashMap::with_capacity(snapshot.len());

    for (path_str, cached) in snapshot {
        let path = PathBuf::from(&path_str);
        let metadata = match fs::metadata(&path) {
            Ok(meta) => meta,
            Err(_) => continue,
        };

        let modified_epoch = metadata_modified_epoch(&metadata);
        if now.saturating_sub(modified_epoch) > STALE_SECONDS {
            continue;
        }

        let len = metadata.len();
        let unchanged = cached.modified_epoch == modified_epoch && cached.len == len && cached.data.is_some();

        if unchanged {
            if let Some(data) = cached.data.clone() {
                if now.saturating_sub(data.timestamp) <= STALE_SECONDS {
                    results.push(data);
                }
            }
            next_cache.insert(path_str, cached);
            continue;
        }

        // Re-read metadata if file rotated/truncated, otherwise reuse cached parsed session_meta.
        let meta = if cached.meta.is_some() && len >= cached.len {
            cached.meta.clone()
        } else {
            read_first_line(&path)
                .as_deref()
                .and_then(parse_codex_session_meta)
        };

        let mut data: Option<CodexStatusLineData> = None;
        if let Some(session_meta) = meta.as_ref() {
            if !session_meta.cwd.is_empty() {
                let tail = parse_codex_tail_snapshot(&path);
                let timestamp = if tail.latest_timestamp > 0 {
                    tail.latest_timestamp
                } else {
                    modified_epoch
                };

                let context_percent = if tail.context_window > 0 {
                    ((tail.total_tokens as f64 / tail.context_window as f64) * 100.0).round() as u32
                } else {
                    0
                };

                let current = CodexStatusLineData {
                    session_id: if session_meta.session_id.is_empty() {
                        "unknown".to_string()
                    } else {
                        session_meta.session_id.clone()
                    },
                    model: tail.latest_model.unwrap_or_else(|| config_model.clone()),
                    reasoning_effort: config_effort.clone(),
                    cwd: session_meta.cwd.clone(),
                    input_tokens: tail.input_tokens,
                    output_tokens: tail.output_tokens,
                    reasoning_tokens: tail.reasoning_tokens,
                    cached_tokens: tail.cached_tokens,
                    total_tokens: tail.total_tokens,
                    context_window: tail.context_window,
                    context_percent,
                    rate_limit_primary_pct: tail.rate_limit_primary_pct,
                    rate_limit_secondary_pct: tail.rate_limit_secondary_pct,
                    cli_version: session_meta.cli_version.clone(),
                    timestamp,
                };

                if now.saturating_sub(current.timestamp) <= STALE_SECONDS {
                    results.push(current.clone());
                }
                data = Some(current);
            }
        }

        next_cache.insert(
            path_str,
            CachedCodexFile {
                modified_epoch,
                len,
                meta,
                data,
            },
        );
    }

    {
        let mut cache = match codex_status_cache().lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };
        cache.files = next_cache;
    }

    results
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_test_dir(prefix: &str) -> PathBuf {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or(0);
        let dir = std::env::temp_dir().join(format!("packetcode-{}-{}", prefix, unique));
        fs::create_dir_all(&dir).expect("failed to create temp test directory");
        dir
    }

    #[test]
    fn iso_to_epoch_parses_basic_valid_values() {
        assert_eq!(iso_to_epoch("1970-01-01T00:00:00Z"), 0);
        assert_eq!(iso_to_epoch("1970-01-02T00:00:00Z"), 86_400);
        assert_eq!(iso_to_epoch("1970-01-02T00:00:00.123Z"), 86_400);
    }

    #[test]
    fn iso_to_epoch_returns_zero_for_invalid_format() {
        assert_eq!(iso_to_epoch("not-a-timestamp"), 0);
        assert_eq!(iso_to_epoch("2026-02-24"), 0);
    }

    #[test]
    fn read_first_line_trims_newline_chars() {
        let dir = temp_test_dir("statusline-read-first-line");
        let path = dir.join("line.txt");
        fs::write(&path, "first line\r\nsecond line\r\n").expect("failed to write fixture");

        let first = read_first_line(&path);
        assert_eq!(first.as_deref(), Some("first line"));

        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn parse_codex_session_meta_reads_session_meta_payload() {
        let line = r#"{"type":"session_meta","payload":{"id":"sid-123","cwd":"C:/repo","cli_version":"1.2.3"}}"#;
        let parsed = parse_codex_session_meta(line).expect("expected session meta");

        assert_eq!(parsed.session_id, "sid-123");
        assert_eq!(parsed.cwd, "C:/repo");
        assert_eq!(parsed.cli_version, "1.2.3");
    }

    #[test]
    fn parse_codex_session_meta_supports_fallback_shape() {
        let line = r#"{"id":"sid-999","cwd":"D:/proj","cli_version":"2.0.0"}"#;
        let parsed = parse_codex_session_meta(line).expect("expected fallback metadata");

        assert_eq!(parsed.session_id, "sid-999");
        assert_eq!(parsed.cwd, "D:/proj");
        assert_eq!(parsed.cli_version, "2.0.0");
    }

    #[test]
    fn parse_codex_tail_snapshot_reads_latest_token_and_model() {
        let dir = temp_test_dir("statusline-tail-snapshot");
        let path = dir.join("session.jsonl");

        let lines = [
            r#"{"type":"session_meta","payload":{"id":"sid","cwd":"C:/repo","cli_version":"1.0.0"}}"#,
            r#"{"type":"turn_context","payload":{"model":"old-model"}}"#,
            r#"{"type":"event_msg","timestamp":"2026-02-24T10:00:00.000Z","payload":{"type":"token_count","info":{"model_context_window":1000,"total_token_usage":{"input_tokens":10,"output_tokens":5,"reasoning_output_tokens":2,"cached_input_tokens":3,"total_tokens":15}},"rate_limits":{"primary":{"used_percent":12.5},"secondary":{"used_percent":7.5}}}}"#,
            r#"{"type":"turn_context","payload":{"model":"new-model"}}"#,
            r#"{"type":"event_msg","timestamp":"2026-02-24T10:05:00.000Z","payload":{"type":"token_count","info":{"model_context_window":2000,"total_token_usage":{"input_tokens":20,"output_tokens":8,"reasoning_tokens":4,"cached_tokens":6,"total_tokens":28}},"rate_limits":{"primary":{"used_percent":22.0},"secondary":{"used_percent":11.0}}}}"#,
        ]
        .join("\n");

        fs::write(&path, lines).expect("failed to write fixture");

        let snapshot = parse_codex_tail_snapshot(&path);
        assert!(snapshot.has_token_count);
        assert_eq!(snapshot.latest_model.as_deref(), Some("new-model"));
        assert_eq!(snapshot.input_tokens, 20);
        assert_eq!(snapshot.output_tokens, 8);
        assert_eq!(snapshot.reasoning_tokens, 4);
        assert_eq!(snapshot.cached_tokens, 6);
        assert_eq!(snapshot.total_tokens, 28);
        assert_eq!(snapshot.context_window, 2000);
        assert!((snapshot.rate_limit_primary_pct - 22.0).abs() < f64::EPSILON);
        assert!((snapshot.rate_limit_secondary_pct - 11.0).abs() < f64::EPSILON);

        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn should_full_scan_respects_cache_age_and_contents() {
        let empty_cache = CodexStatusCache::default();
        assert!(should_full_scan(&empty_cache, 100));

        let mut cache = CodexStatusCache {
            files: HashMap::new(),
            last_full_scan_epoch: 100,
        };
        cache
            .files
            .insert("file.jsonl".to_string(), CachedCodexFile::default());

        assert!(!should_full_scan(&cache, 110));
        assert!(should_full_scan(&cache, 131));
    }
}

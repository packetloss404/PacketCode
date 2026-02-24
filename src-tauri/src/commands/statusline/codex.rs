use crate::commands::shared::home_dir;
use super::helpers::*;
use serde::Serialize;
use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};

const CODEX_TAIL_BYTES: u64 = 16_384;
const CODEX_FULL_SCAN_INTERVAL_SECONDS: u64 = 30;

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

    let model = parsed.get("model").and_then(|v| v.as_str()).unwrap_or("unknown").to_string();
    let effort = parsed
        .get("model_reasoning_effort")
        .or_else(|| parsed.get("reasoning_effort"))
        .or_else(|| parsed.get("reasoning").and_then(|r| r.get("effort")))
        .and_then(|v| v.as_str())
        .unwrap_or("medium")
        .to_string();

    (model, effort)
}

fn parse_codex_session_meta(first_line: &str) -> Option<CachedCodexSessionMeta> {
    let parsed: serde_json::Value = serde_json::from_str(first_line).ok()?;
    let event_type = parsed.get("type").and_then(|v| v.as_str()).unwrap_or("");
    let payload = parsed.get("payload");

    let source = if event_type == "session_meta" {
        payload?
    } else {
        payload.unwrap_or(&parsed)
    };

    let session_id = source
        .get("id")
        .or_else(|| source.get("session_id"))
        .and_then(|v| v.as_str())
        .unwrap_or("unknown")
        .to_string();
    let cwd = source.get("cwd").and_then(|v| v.as_str()).unwrap_or("").to_string();
    let cli_version = source
        .get("cli_version")
        .or_else(|| source.get("version"))
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();

    Some(CachedCodexSessionMeta { session_id, cwd, cli_version })
}

fn parse_codex_tail_snapshot(path: &Path) -> CodexTailSnapshot {
    let tail_lines = tail_read_lines(path, CODEX_TAIL_BYTES);
    let mut snapshot = CodexTailSnapshot::default();

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
            let inner_type = inner_payload.get("type").and_then(|v| v.as_str()).unwrap_or("");

            if inner_type == "token_count" {
                snapshot.has_token_count = true;

                let ts_str = parsed.get("timestamp").and_then(|v| v.as_str()).unwrap_or("");
                snapshot.latest_timestamp = iso_to_epoch(ts_str);

                let info = inner_payload.get("info");
                let usage = info.and_then(|i| i.get("total_token_usage"));

                let input_tokens = usage.and_then(|u| u.get("input_tokens")).and_then(|v| v.as_u64()).unwrap_or(0);
                let output_tokens = usage.and_then(|u| u.get("output_tokens")).and_then(|v| v.as_u64()).unwrap_or(0);

                snapshot.input_tokens = input_tokens;
                snapshot.output_tokens = output_tokens;
                snapshot.reasoning_tokens = usage
                    .and_then(|u| u.get("reasoning_output_tokens"))
                    .or_else(|| usage.and_then(|u| u.get("reasoning_tokens")))
                    .and_then(|v| v.as_u64()).unwrap_or(0);
                snapshot.cached_tokens = usage
                    .and_then(|u| u.get("cached_input_tokens"))
                    .or_else(|| usage.and_then(|u| u.get("cached_tokens")))
                    .and_then(|v| v.as_u64()).unwrap_or(0);
                snapshot.total_tokens = usage
                    .and_then(|u| u.get("total_tokens"))
                    .and_then(|v| v.as_u64()).unwrap_or(input_tokens + output_tokens);
                snapshot.context_window = info
                    .and_then(|i| i.get("model_context_window"))
                    .and_then(|v| v.as_u64()).unwrap_or(200_000);

                let rate_limits = inner_payload.get("rate_limits");
                snapshot.rate_limit_primary_pct = rate_limits
                    .and_then(|r| r.get("primary")).and_then(|r| r.get("used_percent"))
                    .and_then(|v| v.as_f64()).unwrap_or(0.0);
                snapshot.rate_limit_secondary_pct = rate_limits
                    .and_then(|r| r.get("secondary")).and_then(|r| r.get("used_percent"))
                    .and_then(|v| v.as_f64()).unwrap_or(0.0);
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
    let home = match home_dir() {
        Some(h) => h,
        None => return vec![],
    };

    let (config_model, config_effort) = read_codex_config(&home);
    let sessions_dir = PathBuf::from(&home).join(".codex").join("sessions");
    if !sessions_dir.exists() {
        return vec![];
    }

    let now = now_epoch_seconds();

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

        let meta = if cached.meta.is_some() && len >= cached.len {
            cached.meta.clone()
        } else {
            read_first_line(&path).as_deref().and_then(parse_codex_session_meta)
        };

        let mut data: Option<CodexStatusLineData> = None;
        if let Some(session_meta) = meta.as_ref() {
            if !session_meta.cwd.is_empty() {
                let tail = parse_codex_tail_snapshot(&path);
                let timestamp = if tail.latest_timestamp > 0 { tail.latest_timestamp } else { modified_epoch };

                let context_percent = if tail.context_window > 0 {
                    ((tail.total_tokens as f64 / tail.context_window as f64) * 100.0).round() as u32
                } else {
                    0
                };

                let current = CodexStatusLineData {
                    session_id: if session_meta.session_id.is_empty() { "unknown".to_string() } else { session_meta.session_id.clone() },
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

        next_cache.insert(path_str, CachedCodexFile { modified_epoch, len, meta, data });
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
pub mod tests_support {
    use super::*;
    use std::collections::HashMap;

    pub fn parse_codex_session_meta_test(line: &str) -> Option<(String, String, String)> {
        parse_codex_session_meta(line).map(|m| (m.session_id, m.cwd, m.cli_version))
    }

    pub fn parse_codex_tail_snapshot_test(path: &std::path::Path) -> (Option<String>, u64, u64, u64, u64, u64, bool) {
        let s = parse_codex_tail_snapshot(path);
        (s.latest_model, s.input_tokens, s.output_tokens, s.reasoning_tokens, s.cached_tokens, s.total_tokens, s.has_token_count)
    }

    pub fn should_full_scan_test_empty(now: u64) -> bool {
        let cache = CodexStatusCache::default();
        should_full_scan(&cache, now)
    }

    pub fn should_full_scan_test_recent(now: u64) -> bool {
        let mut cache = CodexStatusCache {
            files: HashMap::new(),
            last_full_scan_epoch: 100,
        };
        cache.files.insert("file.jsonl".to_string(), CachedCodexFile::default());
        should_full_scan(&cache, now)
    }
}

use std::fs;
use std::io::{BufRead, BufReader, Read, Seek, SeekFrom};
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

pub const STALE_SECONDS: u64 = 300;

pub fn now_epoch_seconds() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

pub fn metadata_modified_epoch(metadata: &fs::Metadata) -> u64 {
    metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// Parse an ISO 8601 timestamp string to Unix epoch seconds.
pub fn iso_to_epoch(ts: &str) -> u64 {
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

    days * 86_400 + hour * 3_600 + min * 60 + sec
}

/// Tail-read the last `n` bytes of a file and return the lines.
pub fn tail_read_lines(path: &Path, n: u64) -> Vec<String> {
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
    if seek_pos > 0 && !lines.is_empty() {
        lines.remove(0);
    }
    lines
}

pub fn read_first_line(path: &Path) -> Option<String> {
    let file = fs::File::open(path).ok()?;
    let mut reader = BufReader::new(file);
    let mut line = String::new();
    let bytes = reader.read_line(&mut line).ok()?;
    if bytes == 0 {
        return None;
    }
    Some(line.trim_end_matches(&['\n', '\r'][..]).to_string())
}

/// Recursively collect JSONL files from a directory.
pub fn collect_jsonl_files(dir: &Path, files: &mut Vec<PathBuf>) {
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

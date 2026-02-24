mod helpers;
pub mod claude;
pub mod codex;

#[cfg(test)]
mod tests {
    use super::helpers::*;
    use std::fs;
    use std::path::PathBuf;
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
        let parsed = super::codex::tests_support::parse_codex_session_meta_test(line);
        let parsed = parsed.expect("expected session meta");

        assert_eq!(parsed.0, "sid-123");
        assert_eq!(parsed.1, "C:/repo");
        assert_eq!(parsed.2, "1.2.3");
    }

    #[test]
    fn parse_codex_session_meta_supports_fallback_shape() {
        let line = r#"{"id":"sid-999","cwd":"D:/proj","cli_version":"2.0.0"}"#;
        let parsed = super::codex::tests_support::parse_codex_session_meta_test(line);
        let parsed = parsed.expect("expected fallback metadata");

        assert_eq!(parsed.0, "sid-999");
        assert_eq!(parsed.1, "D:/proj");
        assert_eq!(parsed.2, "2.0.0");
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

        let snapshot = super::codex::tests_support::parse_codex_tail_snapshot_test(&path);
        assert!(snapshot.6); // has_token_count
        assert_eq!(snapshot.0.as_deref(), Some("new-model")); // latest_model
        assert_eq!(snapshot.1, 20); // input_tokens
        assert_eq!(snapshot.2, 8);  // output_tokens
        assert_eq!(snapshot.3, 4);  // reasoning_tokens
        assert_eq!(snapshot.4, 6);  // cached_tokens
        assert_eq!(snapshot.5, 28); // total_tokens

        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn should_full_scan_respects_cache_age_and_contents() {
        let result = super::codex::tests_support::should_full_scan_test_empty(100);
        assert!(result);

        let result2 = super::codex::tests_support::should_full_scan_test_recent(110);
        assert!(!result2);

        let result3 = super::codex::tests_support::should_full_scan_test_recent(131);
        assert!(result3);
    }
}

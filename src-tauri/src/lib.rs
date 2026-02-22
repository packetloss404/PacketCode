mod claude;
mod commands;
mod session;

use commands::pty::create_shared_pty_manager;
use session::manager::create_shared_manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .manage(create_shared_manager())
        .manage(create_shared_pty_manager())
        .invoke_handler(tauri::generate_handler![
            // PTY-based sessions (primary)
            commands::pty::create_pty_session,
            commands::pty::write_pty,
            commands::pty::resize_pty,
            commands::pty::kill_pty,
            commands::pty::list_pty_sessions,
            // Legacy JSONL sessions (kept for flexibility)
            commands::session::create_session,
            commands::session::send_input,
            commands::session::kill_session,
            commands::session::list_sessions,
            commands::session::get_session_info,
            // Git
            commands::git::get_git_branch,
            commands::git::get_git_status,
            // Code quality
            commands::code_quality::analyze_code_quality,
            // Filesystem
            commands::fs::list_directory,
            // Status line
            commands::statusline::read_statusline_states,
            commands::statusline::read_codex_statusline_states,
            // Spec parsing
            commands::spec::parse_spec_to_tickets,
            // Insights chat
            commands::insights::ask_insights,
            // Ideation scanner
            commands::ideation::generate_ideas,
            // GitHub integration
            commands::github::github_list_repos,
            commands::github::github_list_issues,
            commands::github::github_get_issue,
            commands::github::github_create_pr,
            commands::github::github_investigate_issue,
            // Memory layer
            commands::memory::scan_codebase_memory,
            commands::memory::summarize_session,
            commands::memory::extract_patterns,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

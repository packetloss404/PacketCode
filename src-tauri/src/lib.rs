mod claude;
mod commands;

use commands::github::create_github_auth_state;
use commands::pty::create_shared_pty_manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .manage(create_github_auth_state())
        .manage(create_shared_pty_manager())
        .invoke_handler(tauri::generate_handler![
            // PTY-based sessions (primary)
            commands::pty::create_pty_session,
            commands::pty::write_pty,
            commands::pty::resize_pty,
            commands::pty::kill_pty,
            commands::pty::list_pty_sessions,
            // Git
            commands::git::get_git_branch,
            commands::git::get_git_status,
            commands::git::git_commit,
            commands::git::git_push,
            commands::git::git_pull,
            commands::git::git_create_branch,
            // Code quality
            commands::code_quality::analyze_code_quality,
            // Filesystem
            commands::fs::list_directory,
            // Status line
            commands::statusline::claude::read_statusline_states,
            commands::statusline::codex::read_codex_statusline_states,
            // Spec parsing
            commands::spec::parse_spec_to_tickets,
            // Insights chat
            commands::insights::ask_insights,
            commands::insights::ask_insights_stream,
            // Ideation scanner
            commands::ideation::generate_ideas,
            // GitHub integration
            commands::github::github_set_token,
            commands::github::github_clear_token,
            commands::github::github_has_token,
            commands::github::github_list_repos,
            commands::github::github_list_issues,
            commands::github::github_get_issue,
            commands::github::github_create_pr,
            commands::github::github_list_prs,
            commands::github::github_get_pr_diff,
            commands::github::github_investigate_issue,
            // Memory layer
            commands::memory::scan_codebase_memory,
            commands::memory::summarize_session,
            commands::memory::extract_patterns,
            // Prompt history
            commands::history::read_prompt_history,
            // Usage analytics
            commands::analytics::read_usage_analytics,
            // MCP server management
            commands::mcp::read_mcp_servers,
            commands::mcp::write_mcp_server,
            commands::mcp::delete_mcp_server,
            // Project scaffolding
            commands::scaffold::scaffold_project,
            commands::scaffold::check_scaffold_tools,
            // Deploy pipeline
            commands::deploy::read_deploy_config,
            commands::deploy::create_deploy_config,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

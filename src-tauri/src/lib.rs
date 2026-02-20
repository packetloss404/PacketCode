mod claude;
mod commands;
mod session;

use session::manager::create_shared_manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .manage(create_shared_manager())
        .invoke_handler(tauri::generate_handler![
            commands::session::create_session,
            commands::session::send_input,
            commands::session::kill_session,
            commands::session::list_sessions,
            commands::session::get_session_info,
            commands::git::get_git_branch,
            commands::git::get_git_status,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

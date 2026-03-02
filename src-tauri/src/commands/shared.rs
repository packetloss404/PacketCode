/// Windows CREATE_NO_WINDOW flag — prevents flashing console windows for background processes.
#[cfg(windows)]
pub const CREATE_NO_WINDOW: u32 = 0x08000000;

/// Apply platform-specific flags to hide console windows on Windows (no-op on other platforms).
#[cfg(windows)]
pub fn hide_window(cmd: &mut std::process::Command) {
    use std::os::windows::process::CommandExt;
    cmd.creation_flags(CREATE_NO_WINDOW);
}

#[cfg(not(windows))]
pub fn hide_window(_cmd: &mut std::process::Command) {}

/// Apply platform-specific flags to hide console windows for tokio async commands.
#[cfg(windows)]
pub fn hide_window_async(cmd: &mut tokio::process::Command) {
    #[allow(unused_imports)]
    use std::os::windows::process::CommandExt;
    cmd.creation_flags(CREATE_NO_WINDOW);
}

#[cfg(not(windows))]
pub fn hide_window_async(_cmd: &mut tokio::process::Command) {}

/// Resolve the user's home directory (USERPROFILE on Windows, HOME on Unix).
pub fn home_dir() -> Option<String> {
    std::env::var("USERPROFILE")
        .or_else(|_| std::env::var("HOME"))
        .ok()
}

/// Lock a Mutex, converting PoisonError to a String for Tauri command returns.
pub fn lock_mutex<T>(mutex: &std::sync::Mutex<T>) -> Result<std::sync::MutexGuard<'_, T>, String> {
    mutex.lock().map_err(|e| {
        tracing::error!("Mutex poisoned: {}", e);
        format!("Lock error: {}", e)
    })
}

/// Directories to always skip when traversing the file tree.
pub const SKIP_DIRS: &[&str] = &[
    ".git",
    "node_modules",
    "target",
    "dist",
    "build",
    ".next",
    "__pycache__",
    ".venv",
    "venv",
    ".idea",
    ".vscode",
    "coverage",
    ".turbo",
    ".cache",
    ".parcel-cache",
    "vendor",
    "pkg",
    ".svelte-kit",
];

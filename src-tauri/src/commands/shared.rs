/// Resolve the user's home directory (USERPROFILE on Windows, HOME on Unix).
pub fn home_dir() -> Option<String> {
    std::env::var("USERPROFILE")
        .or_else(|_| std::env::var("HOME"))
        .ok()
}

/// Lock a Mutex, converting PoisonError to a String for Tauri command returns.
pub fn lock_mutex<T>(mutex: &std::sync::Mutex<T>) -> Result<std::sync::MutexGuard<'_, T>, String> {
    mutex.lock().map_err(|e| format!("Lock error: {}", e))
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

use serde::Serialize;
use std::fs;
use std::path::Path;

#[derive(Clone, Serialize)]
pub struct DirEntry {
    pub name: String,
    pub path: String,
    pub is_dir: bool,
    pub size: u64,
    pub extension: Option<String>,
}

/// Directories to skip in the explorer
const SKIP_DIRS: &[&str] = &[
    ".git", "node_modules", "target", "dist", "build", ".next",
    "__pycache__", ".venv", "venv", ".idea", ".vscode", "coverage",
    ".turbo", ".cache", ".parcel-cache", "pkg", ".svelte-kit",
];

#[tauri::command]
pub async fn list_directory(dir_path: String) -> Result<Vec<DirEntry>, String> {
    let path = Path::new(&dir_path);
    if !path.is_dir() {
        return Err(format!("Not a directory: {}", dir_path));
    }

    let mut entries: Vec<DirEntry> = Vec::new();

    let read_dir = fs::read_dir(path)
        .map_err(|e| format!("Failed to read directory: {}", e))?;

    for entry in read_dir.flatten() {
        let file_name = entry.file_name().to_string_lossy().to_string();

        // Skip hidden files/dirs (starting with .) except a few useful ones
        if file_name.starts_with('.') && !matches!(file_name.as_str(), ".env" | ".env.local" | ".gitignore" | ".eslintrc" | ".prettierrc") {
            continue;
        }

        let metadata = match entry.metadata() {
            Ok(m) => m,
            Err(_) => continue,
        };

        let is_dir = metadata.is_dir();

        // Skip ignored directories
        if is_dir && SKIP_DIRS.contains(&file_name.as_str()) {
            continue;
        }

        let full_path = entry.path().to_string_lossy().to_string();
        let extension = if !is_dir {
            entry.path().extension().map(|e| e.to_string_lossy().to_string())
        } else {
            None
        };

        entries.push(DirEntry {
            name: file_name,
            path: full_path,
            is_dir,
            size: if is_dir { 0 } else { metadata.len() },
            extension,
        });
    }

    // Sort: directories first, then alphabetical
    entries.sort_by(|a, b| {
        if a.is_dir == b.is_dir {
            a.name.to_lowercase().cmp(&b.name.to_lowercase())
        } else if a.is_dir {
            std::cmp::Ordering::Less
        } else {
            std::cmp::Ordering::Greater
        }
    });

    Ok(entries)
}

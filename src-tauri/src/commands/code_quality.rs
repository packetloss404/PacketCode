use serde::Serialize;
use std::collections::HashMap;
use std::fs;
use std::path::Path;

#[derive(Clone, Serialize)]
pub struct LanguageStats {
    pub name: String,
    pub extension: String,
    pub files: u32,
    pub code_lines: u32,
    pub comment_lines: u32,
    pub blank_lines: u32,
    pub total_lines: u32,
}

#[derive(Clone, Serialize)]
pub struct FileComplexity {
    pub path: String,
    pub language: String,
    pub lines: u32,
    pub complexity: u32,
}

#[derive(Clone, Serialize)]
pub struct CodeQualityReport {
    pub total_files: u32,
    pub total_code_lines: u32,
    pub total_lines: u32,
    pub total_comment_lines: u32,
    pub total_blank_lines: u32,
    pub language_count: u32,
    pub languages: Vec<LanguageStats>,
    pub avg_complexity: f64,
    pub test_files: u32,
    pub test_lines: u32,
    pub top_complex_files: Vec<FileComplexity>,
    pub comment_ratio: f64,
    pub test_ratio: f64,
    pub org_score: u32,
}

/// Directories to always skip
const SKIP_DIRS: &[&str] = &[
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
];

fn get_language(ext: &str) -> Option<&'static str> {
    match ext {
        "ts" | "tsx" => Some("typescript"),
        "js" | "jsx" | "mjs" | "cjs" => Some("javascript"),
        "rs" => Some("rust"),
        "py" => Some("python"),
        "go" => Some("go"),
        "java" => Some("java"),
        "c" | "h" => Some("c"),
        "cpp" | "cc" | "cxx" | "hpp" => Some("cpp"),
        "cs" => Some("csharp"),
        "rb" => Some("ruby"),
        "php" => Some("php"),
        "swift" => Some("swift"),
        "kt" | "kts" => Some("kotlin"),
        "lua" => Some("lua"),
        "sh" | "bash" | "zsh" => Some("shell"),
        "ps1" => Some("powershell"),
        "sql" => Some("sql"),
        "html" | "htm" => Some("html"),
        "css" | "scss" | "sass" | "less" => Some("css"),
        "json" => Some("json"),
        "yaml" | "yml" => Some("yaml"),
        "toml" => Some("toml"),
        "xml" => Some("xml"),
        "md" | "mdx" => Some("markdown"),
        "vue" => Some("vue"),
        "svelte" => Some("svelte"),
        "dart" => Some("dart"),
        "r" | "R" => Some("r"),
        "ex" | "exs" => Some("elixir"),
        "zig" => Some("zig"),
        _ => None,
    }
}

fn is_comment_lang(lang: &str) -> bool {
    !matches!(lang, "json" | "yaml" | "toml" | "xml" | "markdown" | "html")
}

/// Count complexity keywords in a line for supported languages
fn line_complexity(line: &str, lang: &str) -> u32 {
    if matches!(lang, "json" | "yaml" | "toml" | "xml" | "markdown" | "html" | "css" | "sql") {
        return 0;
    }
    let trimmed = line.trim();
    let mut score: u32 = 0;
    // Simple keyword-based complexity: each branch/loop adds 1
    let keywords = ["if ", "if(", "else ", "else{", "for ", "for(",
                     "while ", "while(", "switch ", "switch(",
                     "match ", "match{", "case ", "catch ", "catch(",
                     "? ", "&&", "||", "try ", "try{"];
    for kw in &keywords {
        if trimmed.contains(kw) {
            score += 1;
        }
    }
    score
}

fn is_test_file(path: &str) -> bool {
    let lower = path.to_lowercase();
    lower.contains("test") || lower.contains("spec") || lower.contains("__tests__")
}

fn analyze_file(path: &Path, lang: &str) -> (u32, u32, u32, u32, u32) {
    // Returns: (code_lines, comment_lines, blank_lines, total_lines, complexity)
    let content = match fs::read_to_string(path) {
        Ok(c) => c,
        Err(_) => return (0, 0, 0, 0, 0),
    };

    let mut code = 0u32;
    let mut comments = 0u32;
    let mut blanks = 0u32;
    let mut total = 0u32;
    let mut complexity = 0u32;
    let mut in_block_comment = false;
    let can_comment = is_comment_lang(lang);

    for line in content.lines() {
        total += 1;
        let trimmed = line.trim();

        if trimmed.is_empty() {
            blanks += 1;
            continue;
        }

        if can_comment {
            // Block comments
            if in_block_comment {
                comments += 1;
                if trimmed.contains("*/") {
                    in_block_comment = false;
                }
                continue;
            }

            if trimmed.starts_with("/*") {
                comments += 1;
                if !trimmed.contains("*/") {
                    in_block_comment = true;
                }
                continue;
            }

            // Line comments
            if trimmed.starts_with("//") || trimmed.starts_with('#') && matches!(lang, "python" | "ruby" | "shell" | "r" | "yaml" | "toml") {
                comments += 1;
                continue;
            }

            // Python/Rust doc comments
            if (trimmed.starts_with("///") || trimmed.starts_with("//!")) && matches!(lang, "rust") {
                comments += 1;
                continue;
            }
        }

        code += 1;
        complexity += line_complexity(line, lang);
    }

    (code, comments, blanks, total, complexity)
}

/// Calculate organization score based on directory structure heuristics
fn calc_org_score(files: &[(String, String)]) -> u32 {
    if files.is_empty() {
        return 50;
    }

    let mut score: f64 = 50.0;

    // Check for source directory organization
    let has_src = files.iter().any(|(p, _)| p.contains("/src/") || p.contains("\\src\\") || p.starts_with("src/") || p.starts_with("src\\"));
    if has_src { score += 10.0; }

    // Check for config files at root
    let has_config = files.iter().any(|(p, _)| {
        let name = p.rsplit(|c| c == '/' || c == '\\').next().unwrap_or("");
        matches!(name, "package.json" | "Cargo.toml" | "pyproject.toml" | "go.mod" | "tsconfig.json")
    });
    if has_config { score += 5.0; }

    // Check for test organization
    let has_test_dir = files.iter().any(|(p, _)| {
        p.contains("/tests/") || p.contains("\\tests\\") || p.contains("/__tests__/") || p.contains("\\__tests__\\")
    });
    if has_test_dir { score += 10.0; }

    // Check for consistent naming (no mixed case styles in same dir)
    let has_readme = files.iter().any(|(p, _)| {
        let name = p.rsplit(|c| c == '/' || c == '\\').next().unwrap_or("").to_lowercase();
        name == "readme.md" || name == "readme"
    });
    if has_readme { score += 5.0; }

    // Check average directory depth (shallow = better organized)
    let avg_depth: f64 = files.iter().map(|(p, _)| {
        p.matches('/').count() + p.matches('\\').count()
    }).sum::<usize>() as f64 / files.len() as f64;

    if avg_depth < 3.0 { score += 10.0; }
    else if avg_depth < 5.0 { score += 5.0; }

    // Check for types/interfaces directory
    let has_types = files.iter().any(|(p, _)| p.contains("/types/") || p.contains("\\types\\"));
    if has_types { score += 5.0; }

    // Check for components directory
    let has_components = files.iter().any(|(p, _)| p.contains("/components/") || p.contains("\\components\\"));
    if has_components { score += 5.0; }

    score.min(100.0) as u32
}

fn walk_dir(dir: &Path, base: &Path, files: &mut Vec<(String, String)>) {
    let entries = match fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        let file_name = entry.file_name().to_string_lossy().to_string();

        if path.is_dir() {
            if SKIP_DIRS.contains(&file_name.as_str()) || file_name.starts_with('.') {
                continue;
            }
            walk_dir(&path, base, files);
        } else if path.is_file() {
            if let Some(ext) = path.extension() {
                let ext_str = ext.to_string_lossy().to_lowercase();
                if let Some(lang) = get_language(&ext_str) {
                    let rel_path = path.strip_prefix(base)
                        .unwrap_or(&path)
                        .to_string_lossy()
                        .to_string();
                    files.push((rel_path, lang.to_string()));
                }
            }
        }
    }
}

#[tauri::command]
pub async fn analyze_code_quality(project_path: String) -> Result<CodeQualityReport, String> {
    let base = Path::new(&project_path);
    if !base.is_dir() {
        return Err(format!("Path is not a directory: {}", project_path));
    }

    // Collect all recognized files
    let mut files: Vec<(String, String)> = Vec::new();
    walk_dir(base, base, &mut files);

    // Per-language aggregation
    let mut lang_map: HashMap<String, LanguageStats> = HashMap::new();
    let mut all_complexities: Vec<FileComplexity> = Vec::new();
    let mut total_complexity: u64 = 0;
    let mut complexity_file_count: u32 = 0;
    let mut test_files: u32 = 0;
    let mut test_lines: u32 = 0;

    for (rel_path, lang) in &files {
        let full_path = base.join(rel_path);
        let (code, comments, blanks, total, complexity) = analyze_file(&full_path, lang);

        let ext = full_path.extension()
            .map(|e| e.to_string_lossy().to_string())
            .unwrap_or_default();

        let entry = lang_map.entry(lang.clone()).or_insert(LanguageStats {
            name: lang.clone(),
            extension: ext.clone(),
            files: 0,
            code_lines: 0,
            comment_lines: 0,
            blank_lines: 0,
            total_lines: 0,
        });

        entry.files += 1;
        entry.code_lines += code;
        entry.comment_lines += comments;
        entry.blank_lines += blanks;
        entry.total_lines += total;

        if is_test_file(rel_path) {
            test_files += 1;
            test_lines += total;
        }

        if complexity > 0 || is_comment_lang(lang) {
            total_complexity += complexity as u64;
            complexity_file_count += 1;
            all_complexities.push(FileComplexity {
                path: rel_path.clone(),
                language: lang.clone(),
                lines: total,
                complexity,
            });
        }
    }

    // Sort complexities descending, take top 20
    all_complexities.sort_by(|a, b| b.complexity.cmp(&a.complexity));
    let top_complex: Vec<FileComplexity> = all_complexities.into_iter().take(20).collect();

    // Aggregate totals
    let mut languages: Vec<LanguageStats> = lang_map.into_values().collect();
    languages.sort_by(|a, b| b.total_lines.cmp(&a.total_lines));

    let total_files: u32 = languages.iter().map(|l| l.files).sum();
    let total_code: u32 = languages.iter().map(|l| l.code_lines).sum();
    let total_comments: u32 = languages.iter().map(|l| l.comment_lines).sum();
    let total_blanks: u32 = languages.iter().map(|l| l.blank_lines).sum();
    let total_lines: u32 = languages.iter().map(|l| l.total_lines).sum();
    let language_count = languages.len() as u32;

    let avg_complexity = if complexity_file_count > 0 {
        total_complexity as f64 / complexity_file_count as f64
    } else {
        0.0
    };

    let comment_ratio = if total_code + total_comments > 0 {
        total_comments as f64 / (total_code + total_comments) as f64
    } else {
        0.0
    };

    let test_ratio = if total_files > 0 {
        test_files as f64 / total_files as f64
    } else {
        0.0
    };

    let org_score = calc_org_score(&files);

    Ok(CodeQualityReport {
        total_files,
        total_code_lines: total_code,
        total_lines,
        total_comment_lines: total_comments,
        total_blank_lines: total_blanks,
        language_count,
        languages,
        avg_complexity,
        test_files,
        test_lines,
        top_complex_files: top_complex,
        comment_ratio,
        test_ratio,
        org_score,
    })
}

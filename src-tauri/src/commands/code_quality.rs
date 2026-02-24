use super::shared::SKIP_DIRS;
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
    let normalized = path.replace('\\', "/").to_lowercase();
    if normalized.starts_with("tests/")
        || normalized.starts_with("__tests__/")
        || normalized.contains("/tests/")
        || normalized.contains("/__tests__/")
    {
        return true;
    }

    let file_name = normalized.rsplit('/').next().unwrap_or("");
    file_name.contains(".test.") || file_name.contains(".spec.")
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
            if trimmed.starts_with("//") || (trimmed.starts_with('#') && matches!(lang, "python" | "ruby" | "shell" | "r" | "yaml" | "toml")) {
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

#[cfg(test)]
mod tests {
    use super::*;
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

    fn normalize(path: &str) -> String {
        path.replace('\\', "/")
    }

    #[test]
    fn is_test_file_matches_strict_test_patterns() {
        assert!(is_test_file("src/foo.test.ts"));
        assert!(is_test_file("src/foo.spec.tsx"));
        assert!(is_test_file("tests/integration.ts"));
        assert!(is_test_file("src\\__tests__\\suite.ts"));
    }

    #[test]
    fn is_test_file_rejects_substring_false_positives() {
        assert!(!is_test_file("src/specification.ts"));
        assert!(!is_test_file("src/latestest.ts"));
        assert!(!is_test_file("src/testimony.ts"));
    }

    #[test]
    fn line_complexity_ignores_non_code_languages() {
        assert_eq!(line_complexity(r#"{"if":true}"#, "json"), 0);
        assert_eq!(line_complexity("if (x) {}", "css"), 0);
    }

    #[test]
    fn analyze_file_counts_code_comments_and_complexity() {
        let dir = temp_test_dir("code-quality-analyze-file");
        let path = dir.join("sample.rs");
        fs::write(
            &path,
            "// comment\nlet x = 1;\nif x > 0 {\n}\n",
        )
        .expect("failed to write fixture");

        let (code, comments, blanks, total, complexity) = analyze_file(&path, "rust");
        assert_eq!(code, 3);
        assert_eq!(comments, 1);
        assert_eq!(blanks, 0);
        assert_eq!(total, 4);
        assert_eq!(complexity, 1);

        let _ = fs::remove_dir_all(dir);
    }

    #[test]
    fn walk_dir_skips_known_directories() {
        let dir = temp_test_dir("code-quality-walk-dir");
        fs::create_dir_all(dir.join("src")).expect("failed to create src dir");
        fs::create_dir_all(dir.join("node_modules/pkg")).expect("failed to create node_modules dir");

        fs::write(dir.join("src/app.ts"), "export const x = 1;\n").expect("failed to write app.ts");
        fs::write(dir.join("node_modules/pkg/index.ts"), "export const y = 2;\n")
            .expect("failed to write node_modules fixture");

        let mut files = Vec::new();
        walk_dir(&dir, &dir, &mut files);
        let normalized: Vec<String> = files.iter().map(|(p, _)| normalize(p)).collect();

        assert!(normalized.iter().any(|p| p.ends_with("src/app.ts")));
        assert!(!normalized.iter().any(|p| p.contains("node_modules")));

        let _ = fs::remove_dir_all(dir);
    }

    #[tokio::test]
    async fn analyze_code_quality_counts_only_strict_test_files() {
        let dir = temp_test_dir("code-quality-report-tests");
        fs::create_dir_all(dir.join("src")).expect("failed to create src dir");
        fs::create_dir_all(dir.join("tests")).expect("failed to create tests dir");

        fs::write(dir.join("src/main.ts"), "export const main = true;\n")
            .expect("failed to write main.ts");
        fs::write(dir.join("src/specification.ts"), "export const spec = true;\n")
            .expect("failed to write specification.ts");
        fs::write(dir.join("src/app.test.ts"), "describe('x', () => {});\n")
            .expect("failed to write app.test.ts");
        fs::write(dir.join("tests/integration.ts"), "describe('i', () => {});\n")
            .expect("failed to write tests/integration.ts");

        let report = analyze_code_quality(dir.to_string_lossy().to_string())
            .await
            .expect("analysis should succeed");

        // 4 recognized TypeScript files; only *.test.* and /tests/ should count as test files.
        assert_eq!(report.total_files, 4);
        assert_eq!(report.test_files, 2);

        let _ = fs::remove_dir_all(dir);
    }
}

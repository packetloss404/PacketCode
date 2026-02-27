use serde::Serialize;
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::process::Command;

#[derive(Clone, Serialize)]
pub struct ScaffoldResult {
    pub success: bool,
    pub project_path: String,
    pub message: String,
}

fn tool_exists(name: &str) -> bool {
    #[cfg(target_os = "windows")]
    {
        Command::new("where").arg(name).output().map(|o| o.status.success()).unwrap_or(false)
    }
    #[cfg(not(target_os = "windows"))]
    {
        Command::new("which").arg(name).output().map(|o| o.status.success()).unwrap_or(false)
    }
}

#[tauri::command]
pub async fn check_scaffold_tools() -> Result<HashMap<String, bool>, String> {
    let mut map = HashMap::new();
    map.insert("node".to_string(), tool_exists("node"));
    map.insert("cargo".to_string(), tool_exists("cargo"));
    map.insert("python".to_string(), tool_exists("python") || tool_exists("python3"));
    Ok(map)
}

#[tauri::command]
pub async fn scaffold_project(
    parent_dir: String,
    project_name: String,
    template: String,
) -> Result<ScaffoldResult, String> {
    let project_path = Path::new(&parent_dir).join(&project_name);
    let project_path_str = project_path.to_string_lossy().to_string();

    match template.as_str() {
        "nextjs" => {
            let output = Command::new("npx")
                .args(["create-next-app@latest", &project_name, "--ts", "--use-pnpm", "--no-git", "--eslint", "--no-tailwind", "--src-dir", "--no-app", "--import-alias", "@/*"])
                .current_dir(&parent_dir)
                .output()
                .map_err(|e| format!("Failed to run npx: {}", e))?;

            if output.status.success() {
                Ok(ScaffoldResult {
                    success: true,
                    project_path: project_path_str,
                    message: "Next.js project created successfully".to_string(),
                })
            } else {
                let stderr = String::from_utf8_lossy(&output.stderr);
                Ok(ScaffoldResult {
                    success: false,
                    project_path: project_path_str,
                    message: format!("Next.js scaffold failed: {}", stderr),
                })
            }
        }
        "react-vite" => {
            let output = Command::new("npx")
                .args(["create-vite@latest", &project_name, "--template", "react-ts"])
                .current_dir(&parent_dir)
                .output()
                .map_err(|e| format!("Failed to run npx: {}", e))?;

            if output.status.success() {
                Ok(ScaffoldResult {
                    success: true,
                    project_path: project_path_str,
                    message: "React + Vite project created successfully".to_string(),
                })
            } else {
                let stderr = String::from_utf8_lossy(&output.stderr);
                Ok(ScaffoldResult {
                    success: false,
                    project_path: project_path_str,
                    message: format!("React + Vite scaffold failed: {}", stderr),
                })
            }
        }
        "rust-cli" => {
            fs::create_dir_all(&project_path).map_err(|e| e.to_string())?;
            let output = Command::new("cargo")
                .args(["init", "--name", &project_name])
                .current_dir(&project_path)
                .output()
                .map_err(|e| format!("Failed to run cargo: {}", e))?;

            if output.status.success() {
                Ok(ScaffoldResult {
                    success: true,
                    project_path: project_path_str,
                    message: "Rust CLI project created successfully".to_string(),
                })
            } else {
                let stderr = String::from_utf8_lossy(&output.stderr);
                Ok(ScaffoldResult {
                    success: false,
                    project_path: project_path_str,
                    message: format!("Rust scaffold failed: {}", stderr),
                })
            }
        }
        "python-fastapi" => {
            fs::create_dir_all(&project_path).map_err(|e| e.to_string())?;

            let main_py = r#"from fastapi import FastAPI

app = FastAPI()


@app.get("/")
def read_root():
    return {"message": "Hello, World!"}


@app.get("/health")
def health_check():
    return {"status": "ok"}
"#;
            let requirements = "fastapi>=0.100.0\nuvicorn[standard]>=0.23.0\n";
            let gitignore = "__pycache__/\n*.pyc\n.venv/\n.env\n";

            fs::write(project_path.join("main.py"), main_py).map_err(|e| e.to_string())?;
            fs::write(project_path.join("requirements.txt"), requirements).map_err(|e| e.to_string())?;
            fs::write(project_path.join(".gitignore"), gitignore).map_err(|e| e.to_string())?;

            Ok(ScaffoldResult {
                success: true,
                project_path: project_path_str,
                message: "Python FastAPI project created successfully".to_string(),
            })
        }
        "node-express" => {
            fs::create_dir_all(&project_path).map_err(|e| e.to_string())?;

            let index_js = r#"const express = require('express');
const app = express();
const port = process.env.PORT || 3000;

app.use(express.json());

app.get('/', (req, res) => {
  res.json({ message: 'Hello, World!' });
});

app.get('/health', (req, res) => {
  res.json({ status: 'ok' });
});

app.listen(port, () => {
  console.log(`Server running on port ${port}`);
});
"#;
            let package_json = serde_json::json!({
                "name": project_name,
                "version": "1.0.0",
                "main": "index.js",
                "scripts": {
                    "start": "node index.js",
                    "dev": "node --watch index.js"
                },
                "dependencies": {
                    "express": "^4.18.0"
                }
            });
            let gitignore = "node_modules/\n.env\n";

            fs::write(
                project_path.join("package.json"),
                serde_json::to_string_pretty(&package_json).map_err(|e| e.to_string())?,
            )
            .map_err(|e| e.to_string())?;
            fs::write(project_path.join("index.js"), index_js).map_err(|e| e.to_string())?;
            fs::write(project_path.join(".gitignore"), gitignore).map_err(|e| e.to_string())?;

            Ok(ScaffoldResult {
                success: true,
                project_path: project_path_str,
                message: "Node.js Express project created successfully".to_string(),
            })
        }
        "blank" => {
            fs::create_dir_all(&project_path).map_err(|e| e.to_string())?;

            let readme = format!("# {}\n\nA new project.\n", project_name);
            let gitignore = "node_modules/\n.env\n*.log\n";

            fs::write(project_path.join("README.md"), readme).map_err(|e| e.to_string())?;
            fs::write(project_path.join(".gitignore"), gitignore).map_err(|e| e.to_string())?;

            Ok(ScaffoldResult {
                success: true,
                project_path: project_path_str,
                message: "Blank project created successfully".to_string(),
            })
        }
        _ => Err(format!("Unknown template: {}", template)),
    }
}

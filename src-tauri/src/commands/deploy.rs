use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;

#[derive(Clone, Serialize, Deserialize)]
pub struct DeployConfig {
    pub name: String,
    pub command: String,
    #[serde(default)]
    pub env: std::collections::HashMap<String, String>,
}

#[derive(Clone, Serialize)]
pub struct DeployConfigFile {
    pub configs: Vec<DeployConfig>,
    pub source: String, // "packetcode.deploy.json", "package.json", "auto-detected"
}

#[tauri::command]
pub async fn read_deploy_config(project_path: String) -> Result<DeployConfigFile, String> {
    let base = Path::new(&project_path);

    // 1. Check packetcode.deploy.json
    let deploy_file = base.join("packetcode.deploy.json");
    if deploy_file.exists() {
        let content = fs::read_to_string(&deploy_file).map_err(|e| e.to_string())?;
        let configs: Vec<DeployConfig> =
            serde_json::from_str(&content).map_err(|e| e.to_string())?;
        return Ok(DeployConfigFile {
            configs,
            source: "packetcode.deploy.json".to_string(),
        });
    }

    // 2. Check package.json scripts
    let package_json = base.join("package.json");
    if package_json.exists() {
        let content = fs::read_to_string(&package_json).map_err(|e| e.to_string())?;
        let parsed: serde_json::Value =
            serde_json::from_str(&content).map_err(|e| e.to_string())?;
        let mut configs = Vec::new();

        if let Some(scripts) = parsed.get("scripts").and_then(|v| v.as_object()) {
            for key in ["build", "deploy", "start"] {
                if let Some(cmd) = scripts.get(key).and_then(|v| v.as_str()) {
                    configs.push(DeployConfig {
                        name: format!("npm run {}", key),
                        command: format!("npm run {}", key),
                        env: Default::default(),
                    });
                    let _ = cmd; // used via format above
                }
            }
        }

        if !configs.is_empty() {
            return Ok(DeployConfigFile {
                configs,
                source: "package.json".to_string(),
            });
        }
    }

    // 3. Auto-detect platform configs
    let mut configs = Vec::new();

    if base.join("vercel.json").exists() {
        configs.push(DeployConfig {
            name: "Vercel Deploy".to_string(),
            command: "npx vercel --prod".to_string(),
            env: Default::default(),
        });
    }

    if base.join("netlify.toml").exists() {
        configs.push(DeployConfig {
            name: "Netlify Deploy".to_string(),
            command: "npx netlify deploy --prod".to_string(),
            env: Default::default(),
        });
    }

    if base.join("Dockerfile").exists() {
        configs.push(DeployConfig {
            name: "Docker Build".to_string(),
            command: "docker build -t app .".to_string(),
            env: Default::default(),
        });
    }

    if !configs.is_empty() {
        return Ok(DeployConfigFile {
            configs,
            source: "auto-detected".to_string(),
        });
    }

    Ok(DeployConfigFile {
        configs: Vec::new(),
        source: "none".to_string(),
    })
}

#[tauri::command]
pub async fn create_deploy_config(
    project_path: String,
    configs: Vec<DeployConfig>,
) -> Result<(), String> {
    let file_path = Path::new(&project_path).join("packetcode.deploy.json");
    let pretty = serde_json::to_string_pretty(&configs).map_err(|e| e.to_string())?;
    fs::write(&file_path, pretty).map_err(|e| e.to_string())?;
    Ok(())
}

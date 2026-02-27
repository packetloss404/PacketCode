use serde::Serialize;
use serde_json::Value;
use std::fs;
use std::path::PathBuf;

#[derive(Clone, Serialize)]
pub struct McpServerConfig {
    pub command: String,
    pub args: Vec<String>,
    pub env: std::collections::HashMap<String, String>,
}

#[derive(Clone, Serialize)]
pub struct McpServerEntry {
    pub name: String,
    pub config: McpServerConfig,
    pub scope: String, // "global" or "project"
    pub disabled: bool,
}

fn global_settings_path() -> PathBuf {
    let home = dirs_next().unwrap_or_else(|| PathBuf::from("."));
    home.join(".claude").join("settings.json")
}

fn dirs_next() -> Option<PathBuf> {
    #[cfg(target_os = "windows")]
    {
        std::env::var("USERPROFILE").ok().map(PathBuf::from)
    }
    #[cfg(not(target_os = "windows"))]
    {
        std::env::var("HOME").ok().map(PathBuf::from)
    }
}

fn project_mcp_path(project_path: &str) -> PathBuf {
    PathBuf::from(project_path).join(".mcp.json")
}

fn read_json_file(path: &PathBuf) -> Value {
    match fs::read_to_string(path) {
        Ok(content) => serde_json::from_str(&content).unwrap_or(Value::Object(Default::default())),
        Err(_) => Value::Object(Default::default()),
    }
}

fn extract_servers(json: &Value, scope: &str) -> Vec<McpServerEntry> {
    let servers_key = if scope == "global" { "mcpServers" } else { "mcpServers" };
    let servers = match json.get(servers_key) {
        Some(Value::Object(map)) => map,
        _ => return Vec::new(),
    };

    servers
        .iter()
        .map(|(name, val)| {
            let command = val
                .get("command")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let args = val
                .get("args")
                .and_then(|v| v.as_array())
                .map(|arr| {
                    arr.iter()
                        .filter_map(|a| a.as_str().map(|s| s.to_string()))
                        .collect()
                })
                .unwrap_or_default();
            let env = val
                .get("env")
                .and_then(|v| v.as_object())
                .map(|obj| {
                    obj.iter()
                        .filter_map(|(k, v)| v.as_str().map(|s| (k.clone(), s.to_string())))
                        .collect()
                })
                .unwrap_or_default();
            let disabled = val
                .get("disabled")
                .and_then(|v| v.as_bool())
                .unwrap_or(false);

            McpServerEntry {
                name: name.clone(),
                config: McpServerConfig { command, args, env },
                scope: scope.to_string(),
                disabled,
            }
        })
        .collect()
}

#[tauri::command]
pub async fn read_mcp_servers(project_path: String) -> Result<Vec<McpServerEntry>, String> {
    let mut entries = Vec::new();

    // Global servers from ~/.claude/settings.json
    let global_path = global_settings_path();
    let global_json = read_json_file(&global_path);
    entries.extend(extract_servers(&global_json, "global"));

    // Project servers from .mcp.json
    let proj_path = project_mcp_path(&project_path);
    let proj_json = read_json_file(&proj_path);
    entries.extend(extract_servers(&proj_json, "project"));

    Ok(entries)
}

#[tauri::command]
pub async fn write_mcp_server(
    project_path: String,
    name: String,
    command: String,
    args: Vec<String>,
    env: std::collections::HashMap<String, String>,
    scope: String,
) -> Result<(), String> {
    let file_path = if scope == "global" {
        global_settings_path()
    } else {
        project_mcp_path(&project_path)
    };

    let mut json = read_json_file(&file_path);

    // Ensure mcpServers object exists
    if json.get("mcpServers").is_none() {
        json.as_object_mut()
            .ok_or("Invalid JSON structure")?
            .insert("mcpServers".to_string(), Value::Object(Default::default()));
    }

    let server_val = serde_json::json!({
        "command": command,
        "args": args,
        "env": env,
    });

    json.as_object_mut()
        .ok_or("Invalid JSON structure")?
        .get_mut("mcpServers")
        .ok_or("Missing mcpServers key")?
        .as_object_mut()
        .ok_or("mcpServers is not an object")?
        .insert(name, server_val);

    // Ensure parent directory exists
    if let Some(parent) = file_path.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }

    let pretty = serde_json::to_string_pretty(&json).map_err(|e| e.to_string())?;
    fs::write(&file_path, pretty).map_err(|e| e.to_string())?;

    Ok(())
}

#[tauri::command]
pub async fn delete_mcp_server(
    project_path: String,
    name: String,
    scope: String,
) -> Result<(), String> {
    let file_path = if scope == "global" {
        global_settings_path()
    } else {
        project_mcp_path(&project_path)
    };

    let mut json = read_json_file(&file_path);

    if let Some(servers) = json
        .as_object_mut()
        .and_then(|o| o.get_mut("mcpServers"))
        .and_then(|v| v.as_object_mut())
    {
        servers.remove(&name);
    }

    let pretty = serde_json::to_string_pretty(&json).map_err(|e| e.to_string())?;
    fs::write(&file_path, pretty).map_err(|e| e.to_string())?;

    Ok(())
}

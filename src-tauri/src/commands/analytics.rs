use crate::commands::shared::home_dir;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Serialize, Clone)]
pub struct ModelUsage {
    pub model: String,
    pub sessions: u32,
    #[serde(rename = "inputTokens")]
    pub input_tokens: u64,
    #[serde(rename = "outputTokens")]
    pub output_tokens: u64,
    #[serde(rename = "costUsd")]
    pub cost_usd: f64,
}

#[derive(Debug, Serialize, Clone)]
pub struct DailyCost {
    pub date: String,
    #[serde(rename = "costUsd")]
    pub cost_usd: f64,
}

#[derive(Debug, Serialize, Clone)]
pub struct AnalyticsData {
    #[serde(rename = "totalCostUsd")]
    pub total_cost_usd: f64,
    #[serde(rename = "totalSessions")]
    pub total_sessions: u32,
    #[serde(rename = "totalInputTokens")]
    pub total_input_tokens: u64,
    #[serde(rename = "totalOutputTokens")]
    pub total_output_tokens: u64,
    #[serde(rename = "modelUsage")]
    pub model_usage: Vec<ModelUsage>,
    #[serde(rename = "dailyCosts")]
    pub daily_costs: Vec<DailyCost>,
}

/// Shape of ~/.claude/cost-tally.json entries
#[derive(Debug, Deserialize)]
struct CostTallyEntry {
    #[serde(default)]
    model: Option<String>,
    #[serde(default)]
    cost: Option<f64>,
    #[serde(default, alias = "costUsd")]
    cost_usd: Option<f64>,
    #[serde(default)]
    date: Option<String>,
    #[serde(default, alias = "inputTokens")]
    input_tokens: Option<u64>,
    #[serde(default, alias = "outputTokens")]
    output_tokens: Option<u64>,
    #[serde(default)]
    sessions: Option<u32>,
}

#[tauri::command]
pub fn read_usage_analytics() -> String {
    let home = match home_dir() {
        Some(h) => h,
        None => return empty_analytics(),
    };

    let claude_dir = PathBuf::from(&home).join(".claude");

    // Try reading cost-tally.json
    let cost_tally_path = claude_dir.join("cost-tally.json");
    let cost_entries = read_cost_tally(&cost_tally_path);

    // Aggregate data
    let mut total_cost: f64 = 0.0;
    let mut total_sessions: u32 = 0;
    let mut total_input: u64 = 0;
    let mut total_output: u64 = 0;

    let mut model_map: HashMap<String, ModelUsage> = HashMap::new();
    let mut daily_map: HashMap<String, f64> = HashMap::new();

    for entry in &cost_entries {
        let cost = entry.cost_usd.or(entry.cost).unwrap_or(0.0);
        let model = entry.model.clone().unwrap_or_else(|| "unknown".to_string());
        let input = entry.input_tokens.unwrap_or(0);
        let output = entry.output_tokens.unwrap_or(0);
        let sessions = entry.sessions.unwrap_or(1);

        total_cost += cost;
        total_sessions += sessions;
        total_input += input;
        total_output += output;

        let usage = model_map.entry(model.clone()).or_insert(ModelUsage {
            model,
            sessions: 0,
            input_tokens: 0,
            output_tokens: 0,
            cost_usd: 0.0,
        });
        usage.sessions += sessions;
        usage.input_tokens += input;
        usage.output_tokens += output;
        usage.cost_usd += cost;

        if let Some(date) = &entry.date {
            *daily_map.entry(date.clone()).or_insert(0.0) += cost;
        }
    }

    // Also try reading stats-cache.json for additional data
    let stats_path = claude_dir.join("stats-cache.json");
    if let Ok(contents) = fs::read_to_string(&stats_path) {
        if let Ok(stats) = serde_json::from_str::<serde_json::Value>(&contents) {
            // Extract any additional session/cost data from stats cache
            if let Some(total) = stats.get("totalCost").and_then(|v| v.as_f64()) {
                if total > total_cost {
                    total_cost = total;
                }
            }
            if let Some(count) = stats.get("totalSessions").and_then(|v| v.as_u64()) {
                if count as u32 > total_sessions {
                    total_sessions = count as u32;
                }
            }
        }
    }

    let mut model_usage: Vec<ModelUsage> = model_map.into_values().collect();
    model_usage.sort_by(|a, b| {
        b.cost_usd
            .partial_cmp(&a.cost_usd)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    let mut daily_costs: Vec<DailyCost> = daily_map
        .into_iter()
        .map(|(date, cost_usd)| DailyCost { date, cost_usd })
        .collect();
    daily_costs.sort_by(|a, b| a.date.cmp(&b.date));

    // Keep last 30 days
    if daily_costs.len() > 30 {
        daily_costs = daily_costs.split_off(daily_costs.len() - 30);
    }

    let data = AnalyticsData {
        total_cost_usd: total_cost,
        total_sessions,
        total_input_tokens: total_input,
        total_output_tokens: total_output,
        model_usage,
        daily_costs,
    };

    serde_json::to_string(&data).unwrap_or_else(|_| empty_analytics())
}

fn read_cost_tally(path: &PathBuf) -> Vec<CostTallyEntry> {
    let contents = match fs::read_to_string(path) {
        Ok(c) => c,
        Err(_) => return vec![],
    };

    // Could be an array or an object with entries
    if let Ok(entries) = serde_json::from_str::<Vec<CostTallyEntry>>(&contents) {
        return entries;
    }

    // Try as a map of date -> entry
    if let Ok(map) = serde_json::from_str::<HashMap<String, serde_json::Value>>(&contents) {
        let mut entries = Vec::new();
        for (date, value) in map {
            if let Ok(mut entry) = serde_json::from_value::<CostTallyEntry>(value) {
                if entry.date.is_none() {
                    entry.date = Some(date);
                }
                entries.push(entry);
            }
        }
        return entries;
    }

    vec![]
}

fn empty_analytics() -> String {
    r#"{"totalCostUsd":0,"totalSessions":0,"totalInputTokens":0,"totalOutputTokens":0,"modelUsage":[],"dailyCosts":[]}"#.to_string()
}

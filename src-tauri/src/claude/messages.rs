use serde::{Deserialize, Serialize};

/// Represents a parsed line from Claude CLI's stream-json output.
/// The CLI outputs JSONL where each line is a message event.
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ClaudeMessage {
    #[serde(rename = "system")]
    System {
        subtype: Option<String>,
        #[serde(flatten)]
        data: serde_json::Value,
    },

    #[serde(rename = "assistant")]
    Assistant {
        subtype: Option<String>,
        message: Option<serde_json::Value>,
        #[serde(flatten)]
        data: serde_json::Value,
    },

    #[serde(rename = "user")]
    User {
        subtype: Option<String>,
        message: Option<serde_json::Value>,
        #[serde(flatten)]
        data: serde_json::Value,
    },

    #[serde(rename = "result")]
    Result {
        subtype: Option<String>,
        result: Option<String>,
        cost_usd: Option<f64>,
        duration_ms: Option<u64>,
        duration_api_ms: Option<u64>,
        num_turns: Option<u32>,
        session_id: Option<String>,
        #[serde(flatten)]
        data: serde_json::Value,
    },
}

/// Lightweight wrapper for emitting to the frontend.
/// We forward the raw JSON line so the frontend parser handles rendering logic.
#[derive(Debug, Clone, Serialize)]
pub struct SessionOutputEvent {
    pub session_id: String,
    pub raw_json: String,
}

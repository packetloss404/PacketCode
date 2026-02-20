import type { ParsedMessage } from "@/types/messages";

let messageCounter = 0;

function generateId(): string {
  return `msg_${Date.now()}_${++messageCounter}`;
}

/**
 * Parse a raw JSONL line from Claude CLI's --output-format stream-json.
 *
 * Actual format from the CLI:
 *
 *   {"type":"system","subtype":"init","session_id":"...","tools":[...], ...}
 *   {"type":"assistant","subtype":"message_start","message":{"role":"assistant","content":[],...}}
 *   {"type":"assistant","subtype":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
 *   {"type":"assistant","subtype":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}
 *   {"type":"assistant","subtype":"content_block_stop","index":0}
 *   {"type":"assistant","subtype":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{...}}
 *   {"type":"assistant","subtype":"message_stop"}
 *   {"type":"result","subtype":"success","cost_usd":0.001,"session_id":"...","num_turns":1,"result":"..."}
 *
 * Tool use:
 *   {"type":"assistant","subtype":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_...","name":"Bash","input":{}}}
 *   {"type":"assistant","subtype":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}
 *   {"type":"assistant","subtype":"content_block_stop","index":1}
 *   {"type":"user","subtype":"tool_result","tool_use_id":"toolu_...","content":[{"type":"text","text":"output"}]}
 */
export function parseClaudeJsonLine(line: string): ParsedMessage[] {
  let raw: Record<string, unknown>;
  try {
    raw = JSON.parse(line);
  } catch {
    // Not valid JSON — emit as raw text (e.g. stderr leakage)
    if (line.trim()) {
      return [
        {
          id: generateId(),
          type: "error",
          role: "system",
          timestamp: Date.now(),
          content: line,
        },
      ];
    }
    return [];
  }

  const type = raw.type as string | undefined;
  const subtype = raw.subtype as string | undefined;

  // ── system messages ──────────────────────────────────────────────
  if (type === "system") {
    if (subtype === "init") {
      const sid = raw.session_id as string | undefined;
      const cwd = raw.cwd as string | undefined;
      return [
        {
          id: generateId(),
          type: "system",
          role: "system",
          timestamp: Date.now(),
          content: `Session initialized${sid ? ` (${sid.slice(0, 8)}...)` : ""}${cwd ? ` in ${cwd}` : ""}`,
          raw,
        },
      ];
    }
    // Other system messages (e.g. permission prompts)
    return [
      {
        id: generateId(),
        type: "system",
        role: "system",
        timestamp: Date.now(),
        content: JSON.stringify(raw),
        raw,
      },
    ];
  }

  // ── assistant messages ───────────────────────────────────────────
  if (type === "assistant") {
    // content_block_start: beginning of a text/tool_use/thinking block
    if (subtype === "content_block_start") {
      const block = raw.content_block as Record<string, unknown> | undefined;
      if (!block) return [];

      const blockType = block.type as string;

      if (blockType === "text") {
        // Initial text block — usually empty, content comes via deltas
        const text = block.text as string | undefined;
        if (text) {
          return [
            {
              id: generateId(),
              type: "text",
              role: "assistant",
              timestamp: Date.now(),
              content: text,
              isStreaming: true,
            },
          ];
        }
        return [];
      }

      if (blockType === "tool_use") {
        return [
          {
            id: generateId(),
            type: "tool_use",
            role: "assistant",
            timestamp: Date.now(),
            content: (block.name as string) || "Unknown tool",
            toolName: block.name as string,
            toolInput: (block.input as Record<string, unknown>) || {},
            isStreaming: true,
          },
        ];
      }

      if (blockType === "thinking") {
        const text = block.thinking as string || block.text as string || "";
        if (text) {
          return [
            {
              id: generateId(),
              type: "thinking",
              role: "assistant",
              timestamp: Date.now(),
              content: text,
              isStreaming: true,
            },
          ];
        }
        return [];
      }

      return [];
    }

    // content_block_delta: streaming chunk
    if (subtype === "content_block_delta") {
      const delta = raw.delta as Record<string, unknown> | undefined;
      if (!delta) return [];

      const deltaType = delta.type as string;

      if (deltaType === "text_delta") {
        const text = delta.text as string;
        if (text) {
          return [
            {
              id: generateId(),
              type: "text",
              role: "assistant",
              timestamp: Date.now(),
              content: text,
              isStreaming: true,
            },
          ];
        }
      }

      if (deltaType === "input_json_delta") {
        const partial = delta.partial_json as string;
        if (partial) {
          return [
            {
              id: generateId(),
              type: "tool_use",
              role: "assistant",
              timestamp: Date.now(),
              content: partial,
              isStreaming: true,
            },
          ];
        }
      }

      if (deltaType === "thinking_delta") {
        const text = delta.thinking as string || delta.text as string;
        if (text) {
          return [
            {
              id: generateId(),
              type: "thinking",
              role: "assistant",
              timestamp: Date.now(),
              content: text,
              isStreaming: true,
            },
          ];
        }
      }

      return [];
    }

    // content_block_stop, message_start, message_delta, message_stop
    // These are lifecycle events — no UI output needed
    if (
      subtype === "content_block_stop" ||
      subtype === "message_start" ||
      subtype === "message_stop"
    ) {
      return [];
    }

    if (subtype === "message_delta") {
      // Could extract stop_reason here if needed
      return [];
    }

    return [];
  }

  // ── user messages (tool results) ─────────────────────────────────
  if (type === "user") {
    if (subtype === "tool_result") {
      const content = raw.content;
      let text = "";
      if (typeof content === "string") {
        text = content;
      } else if (Array.isArray(content)) {
        text = (content as Array<Record<string, unknown>>)
          .map((c) => (c.text as string) || "")
          .filter(Boolean)
          .join("\n");
      }
      return [
        {
          id: generateId(),
          type: "tool_result",
          role: "user",
          timestamp: Date.now(),
          content: text || "(no output)",
          toolName: (raw.tool_use_id as string) || undefined,
        },
      ];
    }
    return [];
  }

  // ── result message (session complete) ────────────────────────────
  if (type === "result") {
    return [
      {
        id: generateId(),
        type: "result",
        role: "system",
        timestamp: Date.now(),
        content: (raw.result as string) || "Session completed",
        costUsd: raw.cost_usd as number | undefined,
        sessionId: raw.session_id as string | undefined,
        raw,
      },
    ];
  }

  // ── fallback: unknown message type ───────────────────────────────
  return [];
}

import type {
  RawClaudeMessage,
  ParsedMessage,
  ContentBlock,
} from "@/types/messages";

let messageCounter = 0;

function generateId(): string {
  return `msg_${Date.now()}_${++messageCounter}`;
}

/**
 * Parse a raw JSONL line from Claude CLI into renderable messages.
 * Each line can produce zero or more ParsedMessages.
 */
export function parseClaudeJsonLine(line: string): ParsedMessage[] {
  let raw: RawClaudeMessage;
  try {
    raw = JSON.parse(line);
  } catch {
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

  const messages: ParsedMessage[] = [];
  const type = raw.type;
  const subtype = raw.subtype;

  if (type === "system") {
    // System init messages
    if (subtype === "init") {
      messages.push({
        id: generateId(),
        type: "system",
        role: "system",
        timestamp: Date.now(),
        content: `Session initialized${raw.session_id ? ` (${raw.session_id})` : ""}`,
        raw,
      });
    }
    return messages;
  }

  if (type === "assistant") {
    if (subtype === "message_start" || subtype === "message_delta") {
      // Ignore message lifecycle events — content comes in content_block events
      return messages;
    }

    // Content block events
    if (subtype === "content_block_start" && raw.message?.content) {
      for (const block of raw.message.content) {
        messages.push(...parseContentBlock(block, true));
      }
      return messages;
    }

    if (subtype === "content_block_delta") {
      // Streaming delta — raw has index and delta
      const delta = (raw as Record<string, unknown>).delta as ContentBlock | undefined;
      if (delta) {
        if (delta.type === "text_delta" && delta.text) {
          messages.push({
            id: generateId(),
            type: "text",
            role: "assistant",
            timestamp: Date.now(),
            content: delta.text,
            isStreaming: true,
          });
        } else if (delta.type === "input_json_delta" && delta.partial_json) {
          messages.push({
            id: generateId(),
            type: "tool_use",
            role: "assistant",
            timestamp: Date.now(),
            content: delta.partial_json,
            isStreaming: true,
          });
        } else if (delta.type === "thinking_delta" && delta.text) {
          messages.push({
            id: generateId(),
            type: "thinking",
            role: "assistant",
            timestamp: Date.now(),
            content: delta.text,
            isStreaming: true,
          });
        }
      }
      return messages;
    }

    // Full message with content blocks
    if (raw.message?.content) {
      for (const block of raw.message.content) {
        messages.push(...parseContentBlock(block, false));
      }
      return messages;
    }
  }

  if (type === "user") {
    // Tool results come as user messages
    if (raw.message?.content) {
      for (const block of raw.message.content) {
        if (block.type === "tool_result") {
          const resultContent =
            typeof block.content === "string"
              ? block.content
              : Array.isArray(block.content)
                ? block.content
                    .map((c) => c.text || "")
                    .join("\n")
                : "";
          messages.push({
            id: generateId(),
            type: "tool_result",
            role: "user",
            timestamp: Date.now(),
            content: resultContent,
            toolName: block.id || undefined,
          });
        }
      }
    }
    return messages;
  }

  if (type === "result") {
    messages.push({
      id: generateId(),
      type: "result",
      role: "system",
      timestamp: Date.now(),
      content: raw.result || "Session completed",
      costUsd: raw.cost_usd,
      sessionId: raw.session_id || undefined,
      raw,
    });
    return messages;
  }

  // Fallback: emit raw as text
  if (Object.keys(raw).length > 0) {
    messages.push({
      id: generateId(),
      type: "text",
      role: "system",
      timestamp: Date.now(),
      content: JSON.stringify(raw, null, 2),
      raw,
    });
  }

  return messages;
}

function parseContentBlock(
  block: ContentBlock,
  isStreaming: boolean
): ParsedMessage[] {
  const messages: ParsedMessage[] = [];

  if (block.type === "text" && block.text) {
    messages.push({
      id: generateId(),
      type: "text",
      role: "assistant",
      timestamp: Date.now(),
      content: block.text,
      isStreaming,
    });
  } else if (block.type === "tool_use") {
    messages.push({
      id: generateId(),
      type: "tool_use",
      role: "assistant",
      timestamp: Date.now(),
      content: block.name || "Unknown tool",
      toolName: block.name,
      toolInput: block.input,
      isStreaming,
    });
  } else if (block.type === "thinking" && block.text) {
    messages.push({
      id: generateId(),
      type: "thinking",
      role: "assistant",
      timestamp: Date.now(),
      content: block.text,
      isStreaming,
    });
  }

  return messages;
}

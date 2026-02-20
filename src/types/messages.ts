export type ParsedMessageType =
  | "text"
  | "tool_use"
  | "tool_result"
  | "system"
  | "result"
  | "error"
  | "thinking";

export interface ParsedMessage {
  id: string;
  type: ParsedMessageType;
  role: "assistant" | "user" | "system";
  timestamp: number;
  content: string;
  toolName?: string;
  toolInput?: Record<string, unknown>;
  isStreaming?: boolean;
  costUsd?: number;
  sessionId?: string;
  raw?: Record<string, unknown>;
}

export interface SessionOutputEvent {
  session_id: string;
  raw_json: string;
}

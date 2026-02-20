export interface RawClaudeMessage {
  type: string;
  subtype?: string;
  message?: {
    role?: string;
    content?: ContentBlock[];
    model?: string;
    usage?: {
      input_tokens?: number;
      output_tokens?: number;
      cache_read_input_tokens?: number;
      cache_creation_input_tokens?: number;
    };
  };
  result?: string;
  cost_usd?: number;
  duration_ms?: number;
  session_id?: string;
  num_turns?: number;
  [key: string]: unknown;
}

export interface ContentBlock {
  type: string;
  text?: string;
  id?: string;
  name?: string;
  input?: Record<string, unknown>;
  content?: string | ContentBlock[];
  partial_json?: string;
}

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
  raw?: RawClaudeMessage;
}

export interface SessionOutputEvent {
  session_id: string;
  raw_json: string;
}

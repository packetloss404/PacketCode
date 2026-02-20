export type SessionStatus =
  | "idle"
  | "running"
  | "waiting_input"
  | "error"
  | "terminated";

export interface SessionInfo {
  id: string;
  claude_session_id: string | null;
  project_path: string;
  model: string | null;
  status: SessionStatus;
  pid: number | null;
  cost_usd: number;
  total_tokens: number;
  num_turns: number;
}

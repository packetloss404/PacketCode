export interface StatusLineData {
  session_id: string;
  model: string;
  cwd: string;
  dir_name: string;
  context_percent: number;
  context_current_k: number;
  context_max_k: number;
  git_branch: string;
  cost_usd: number;
  cost_display: string;
  duration_minutes: number;
  context_icon: string;
  timestamp: number;
}

export interface CodexStatusLineData {
  session_id: string;
  model: string;
  reasoning_effort: string;
  cwd: string;
  input_tokens: number;
  output_tokens: number;
  reasoning_tokens: number;
  cached_tokens: number;
  total_tokens: number;
  context_window: number;
  context_percent: number;
  rate_limit_primary_pct: number;
  rate_limit_secondary_pct: number;
  cli_version: string;
  timestamp: number;
}

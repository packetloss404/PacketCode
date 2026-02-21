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

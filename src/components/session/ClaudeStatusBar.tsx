import { GitBranch, Database, DollarSign, Clock } from "lucide-react";
import { useStatusLineForCwd } from "@/hooks/useStatusLine";

interface ClaudeStatusBarProps {
  projectPath: string;
}

export function ClaudeStatusBar({ projectPath }: ClaudeStatusBarProps) {
  const data = useStatusLineForCwd(projectPath);

  if (!data) {
    return null;
  }

  const nowSec = Math.floor(Date.now() / 1000);
  const isStale = nowSec - data.timestamp > 30;

  // Context % color coding
  let contextColor = "#56d364"; // green
  if (data.context_percent >= 80) {
    contextColor = "#f85149"; // red
  } else if (data.context_percent >= 60) {
    contextColor = "#f0b400"; // amber
  }

  // Duration display
  let durationDisplay: string;
  if (data.duration_minutes < 60) {
    durationDisplay = `${data.duration_minutes}m`;
  } else {
    const h = Math.floor(data.duration_minutes / 60);
    const m = data.duration_minutes % 60;
    durationDisplay = `${h}h${m}m`;
  }

  return (
    <div
      className="flex items-center gap-3 px-3 text-[11px] bg-bg-secondary border-t border-bg-border select-none"
      style={{ height: 20, minHeight: 20, opacity: isStale ? 0.5 : 1 }}
    >
      {/* Model */}
      <span style={{ color: "#58a6ff" }}>{data.model}</span>

      {/* Context % */}
      <span className="flex items-center gap-1">
        <Database size={10} style={{ color: contextColor }} />
        <span style={{ color: contextColor }}>{data.context_percent}%</span>
        <span className="text-text-muted">
          ({data.context_current_k}K/{data.context_max_k}K)
        </span>
      </span>

      {/* Git branch */}
      {data.git_branch && data.git_branch !== "-" && (
        <span className="flex items-center gap-1" style={{ color: "#bc8cff" }}>
          <GitBranch size={10} />
          {data.git_branch}
        </span>
      )}

      {/* Cost */}
      <span className="flex items-center gap-1" style={{ color: "#f0b400" }}>
        <DollarSign size={10} />
        {data.cost_display}
      </span>

      {/* Duration */}
      <span className="flex items-center gap-1 text-text-muted">
        <Clock size={10} />
        {durationDisplay}
      </span>
    </div>
  );
}

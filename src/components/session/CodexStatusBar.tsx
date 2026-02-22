import { Database, Gauge, Zap } from "lucide-react";
import { useCodexStatusLineForCwd } from "@/hooks/useCodexStatusLine";

interface CodexStatusBarProps {
  projectPath: string;
}

export function CodexStatusBar({ projectPath }: CodexStatusBarProps) {
  const data = useCodexStatusLineForCwd(projectPath);

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

  // Rate limit color coding
  let rateLimitColor = "#56d364"; // green
  if (data.rate_limit_primary_pct >= 80) {
    rateLimitColor = "#f85149"; // red
  } else if (data.rate_limit_primary_pct >= 60) {
    rateLimitColor = "#f0b400"; // amber
  }

  // Token display in K
  const totalK = Math.round(data.total_tokens / 1000);
  const contextK = Math.round(data.context_window / 1000);

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
          ({totalK}K/{contextK}K)
        </span>
      </span>

      {/* Rate limit */}
      {data.rate_limit_primary_pct > 0 && (
        <span className="flex items-center gap-1">
          <Gauge size={10} style={{ color: rateLimitColor }} />
          <span style={{ color: rateLimitColor }}>
            {Math.round(data.rate_limit_primary_pct)}%
          </span>
        </span>
      )}

      {/* Reasoning effort */}
      {data.reasoning_effort && data.reasoning_effort !== "medium" && (
        <span className="flex items-center gap-1" style={{ color: "#bc8cff" }}>
          <Zap size={10} />
          {data.reasoning_effort}
        </span>
      )}

      {/* CLI version */}
      {data.cli_version && (
        <span className="text-text-muted">v{data.cli_version}</span>
      )}
    </div>
  );
}

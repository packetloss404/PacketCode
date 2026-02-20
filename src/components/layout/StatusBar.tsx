import { useSessionStore } from "@/stores/sessionStore";
import { useLayoutStore } from "@/stores/layoutStore";

export function StatusBar() {
  const sessions = useSessionStore((s) => s.sessions);
  const panes = useLayoutStore((s) => s.panes);

  // Count running sessions
  const runningSessions = Array.from(sessions.values()).filter(
    (s) => s.status === "running"
  ).length;

  // Total cost across all sessions
  const totalCost = Array.from(sessions.values()).reduce(
    (sum, s) => sum + s.costUsd,
    0
  );

  return (
    <div className="flex items-center h-6 px-3 bg-bg-secondary border-t border-bg-border text-[10px] text-text-muted gap-4">
      <span>
        {panes.length} pane{panes.length !== 1 ? "s" : ""}
      </span>
      {runningSessions > 0 && (
        <span className="text-accent-green">
          {runningSessions} running
        </span>
      )}
      {totalCost > 0 && (
        <span className="text-accent-amber">
          Total: ${totalCost.toFixed(4)}
        </span>
      )}
      <div className="flex-1" />
      <span>Ctrl+Enter to send</span>
      <span>Ctrl+\\ to split</span>
    </div>
  );
}

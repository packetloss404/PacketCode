import { useTabStore } from "@/stores/tabStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore } from "@/stores/appStore";

export function StatusBar() {
  const tabs = useTabStore((s) => s.tabs);
  const panes = useLayoutStore((s) => s.panes);
  const activeView = useAppStore((s) => s.activeView);

  // Count active sessions
  const activeSessions = tabs.filter(
    (t) => t.status === "running" || t.status === "thinking" || t.status === "starting"
  ).length;

  const totalSessions = tabs.length;

  const isCliView = activeView === "claude" || activeView === "codex";
  const claudeCount = panes.filter((p) => p.cliCommand === "claude").length;
  const codexCount = panes.filter((p) => p.cliCommand === "codex").length;

  return (
    <div className="flex items-center h-6 px-3 bg-bg-secondary border-t border-bg-border text-[10px] text-text-muted gap-4">
      <span>
        {claudeCount} claude
      </span>
      <span>
        {codexCount} codex
      </span>
      <span>
        {totalSessions} session{totalSessions !== 1 ? "s" : ""}
        {activeSessions > 0 && (
          <span className="text-accent-green ml-1">
            ({activeSessions} active)
          </span>
        )}
      </span>
      <div className="flex-1" />
      {isCliView && (
        <>
          <span>Ctrl+\\ split</span>
          <span>Ctrl+1-{panes.length > 4 ? 4 : panes.length} panes</span>
        </>
      )}
      <span>Ctrl+Shift+1-5 views</span>
    </div>
  );
}

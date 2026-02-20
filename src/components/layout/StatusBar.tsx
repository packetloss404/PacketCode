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

  return (
    <div className="flex items-center h-6 px-3 bg-bg-secondary border-t border-bg-border text-[10px] text-text-muted gap-4">
      <span>
        {panes.length} pane{panes.length !== 1 ? "s" : ""}
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
      {activeView === "claude" && (
        <>
          <span>Ctrl+\\ split</span>
          <span>Ctrl+1-4 panes</span>
        </>
      )}
      <span>Ctrl+Shift+1-4 views</span>
    </div>
  );
}

import { Plus, X, Link } from "lucide-react";
import { useTabStore, type SessionTab } from "@/stores/tabStore";
import { useActivityStore } from "@/stores/activityStore";

interface SessionTabBarProps {
  cliType?: "claude" | "codex";
}

export function SessionTabBar({ cliType = "claude" }: SessionTabBarProps) {
  const tabs = useTabStore((s) => s.tabs);
  const activeTabId = useTabStore((s) => s.activeTabId);
  const setActiveTab = useTabStore((s) => s.setActiveTab);
  const removeTab = useTabStore((s) => s.removeTab);

  const eventName = cliType === "codex" ? "packetcode:new-codex-session" : "packetcode:new-session";

  return (
    <div className="flex items-center h-8 bg-bg-secondary border-b border-bg-border overflow-x-auto">
      <div className="flex items-center h-full min-w-0">
        {tabs.map((tab) => (
          <TabItem
            key={tab.id}
            tab={tab}
            isActive={tab.id === activeTabId}
            onActivate={() => setActiveTab(tab.id)}
            onClose={() => removeTab(tab.id)}
            closable={tabs.length > 1}
          />
        ))}
      </div>
      <button
        onClick={() => {
          window.dispatchEvent(new CustomEvent(eventName));
        }}
        className="flex items-center justify-center w-7 h-7 ml-1 text-text-muted hover:text-accent-green hover:bg-bg-hover rounded transition-colors flex-shrink-0"
        title="New session"
      >
        <Plus size={14} />
      </button>
    </div>
  );
}

function TabItem({
  tab,
  isActive,
  onActivate,
  onClose,
  closable,
}: {
  tab: SessionTab;
  isActive: boolean;
  onActivate: () => void;
  onClose: () => void;
  closable: boolean;
}) {
  const statusColor = getStatusColor(tab.status);
  const activity = useActivityStore((s) => s.activities[tab.id]);
  const activityTooltip = activity?.currentTool
    ? `${activity.currentTool}${activity.currentFile ? `: ${activity.currentFile}` : ""}`
    : undefined;

  return (
    <div
      className={`group flex items-center gap-2 h-full px-3 border-r border-bg-border cursor-pointer transition-colors min-w-0 max-w-[220px] ${
        isActive
          ? "bg-bg-primary text-text-primary border-b-2 border-b-accent-green"
          : "text-text-secondary hover:bg-bg-hover hover:text-text-primary"
      }`}
      onClick={onActivate}
      title={activityTooltip}
    >
      {/* Status dot */}
      <div
        className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${statusColor} ${
          tab.status === "thinking" || tab.status === "running"
            ? "animate-pulse"
            : tab.status === "waiting_approval"
              ? "animate-pulse"
              : ""
        }`}
      />

      {/* Session name */}
      <span className="text-[11px] truncate">{tab.name}</span>

      {/* Ticket badge */}
      {tab.ticketId && (
        <span className="flex items-center gap-0.5 text-[9px] px-1 py-0 bg-accent-purple/20 text-accent-purple rounded flex-shrink-0">
          <Link size={8} />
          {tab.ticketId}
        </span>
      )}

      {/* Status label */}
      <span className="text-[10px] text-text-muted truncate flex-shrink-0">
        {tab.statusLabel}
      </span>

      {/* Close button */}
      {closable && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onClose();
          }}
          className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
          title="Close session"
        >
          <X size={10} />
        </button>
      )}
    </div>
  );
}

function getStatusColor(status: SessionTab["status"]): string {
  switch (status) {
    case "idle":
      return "bg-text-muted";
    case "starting":
      return "bg-accent-amber";
    case "thinking":
      return "bg-accent-blue";
    case "running":
      return "bg-accent-green";
    case "waiting_approval":
      return "bg-accent-amber";
    case "waiting_input":
      return "bg-accent-amber";
    case "done":
      return "bg-accent-purple";
    case "error":
      return "bg-accent-red";
    default:
      return "bg-text-muted";
  }
}

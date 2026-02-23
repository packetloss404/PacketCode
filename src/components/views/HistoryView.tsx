import { Clock } from "lucide-react";
import { useTabStore } from "@/stores/tabStore";

export function HistoryView() {
  const tabs = useTabStore((s) => s.tabs);

  // Show past sessions (done or error status)
  const pastSessions = tabs.filter(
    (t) => t.status === "done" || t.status === "error"
  );
  const activeSessions = tabs.filter(
    (t) => t.status !== "done" && t.status !== "error"
  );

  return (
    <div className="flex flex-col h-full bg-bg-primary p-4">
      <div className="flex items-center gap-2 mb-4">
        <Clock size={16} className="text-accent-purple" />
        <h2 className="text-sm font-semibold text-text-primary">
          Session History
        </h2>
      </div>

      {/* Active sessions */}
      {activeSessions.length > 0 && (
        <div className="mb-6">
          <h3 className="text-[10px] text-text-muted uppercase tracking-wider mb-2">
            Active Sessions
          </h3>
          <div className="flex flex-col gap-1">
            {activeSessions.map((session) => (
              <SessionRow key={session.id} session={session} />
            ))}
          </div>
        </div>
      )}

      {/* Past sessions */}
      <div>
        <h3 className="text-[10px] text-text-muted uppercase tracking-wider mb-2">
          Completed
        </h3>
        {pastSessions.length === 0 ? (
          <p className="text-xs text-text-muted">No completed sessions yet.</p>
        ) : (
          <div className="flex flex-col gap-1">
            {pastSessions.map((session) => (
              <SessionRow key={session.id} session={session} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function SessionRow({
  session,
}: {
  session: ReturnType<typeof useTabStore.getState>["tabs"][0];
}) {
  const formatTime = (ms: number) => {
    const date = new Date(ms);
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  };

  return (
    <div className="flex items-center gap-3 px-3 py-2 bg-bg-secondary border border-bg-border rounded hover:border-text-muted/30 transition-colors">
      <div
        className={`w-2 h-2 rounded-full flex-shrink-0 ${
          session.status === "done"
            ? "bg-accent-green"
            : session.status === "error"
              ? "bg-accent-red"
              : session.status === "running" || session.status === "thinking"
                ? "bg-accent-blue animate-pulse"
                : "bg-text-muted"
        }`}
      />
      <div className="flex-1 min-w-0">
        <p className="text-xs text-text-primary truncate">{session.name}</p>
        <p className="text-[10px] text-text-muted">
          {formatTime(session.startedAt)} &middot; {session.statusLabel}
        </p>
      </div>
      <span className="text-[10px] text-text-muted flex-shrink-0">
        {session.projectPath.split(/[/\\]/).pop()}
      </span>
    </div>
  );
}

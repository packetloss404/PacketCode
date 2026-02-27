import { useEffect, useCallback, useState } from "react";
import { Clock, Search, Copy, Check, Filter, RefreshCw } from "lucide-react";
import { useTabStore } from "@/stores/tabStore";
import { useHistoryStore, type HistoryEntry } from "@/stores/historyStore";

type HistoryTab = "sessions" | "prompts";

export function HistoryView() {
  const [activeTab, setActiveTab] = useState<HistoryTab>("prompts");

  return (
    <div className="flex flex-col h-full bg-bg-primary">
      {/* Header */}
      <div className="flex items-center gap-2 px-4 pt-4 pb-2">
        <Clock size={16} className="text-accent-purple" />
        <h2 className="text-sm font-semibold text-text-primary">
          Session History
        </h2>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 px-4 pb-3">
        <TabButton
          label="Prompt History"
          active={activeTab === "prompts"}
          onClick={() => setActiveTab("prompts")}
        />
        <TabButton
          label="Active Sessions"
          active={activeTab === "sessions"}
          onClick={() => setActiveTab("sessions")}
        />
      </div>

      {activeTab === "prompts" ? <PromptHistoryTab /> : <ActiveSessionsTab />}
    </div>
  );
}

function TabButton({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1 text-[11px] font-medium rounded transition-colors ${
        active
          ? "bg-accent-purple/20 text-accent-purple"
          : "text-text-muted hover:text-text-secondary hover:bg-bg-hover"
      }`}
    >
      {label}
    </button>
  );
}

// --- Prompt History Tab (from ~/.claude) ---

function PromptHistoryTab() {
  const entries = useHistoryStore((s) => s.entries);
  const loading = useHistoryStore((s) => s.loading);
  const error = useHistoryStore((s) => s.error);
  const searchQuery = useHistoryStore((s) => s.searchQuery);
  const projectFilter = useHistoryStore((s) => s.projectFilter);
  const setSearchQuery = useHistoryStore((s) => s.setSearchQuery);
  const setProjectFilter = useHistoryStore((s) => s.setProjectFilter);
  const load = useHistoryStore((s) => s.load);
  const filteredEntries = useHistoryStore((s) => s.filteredEntries);
  const uniqueProjects = useHistoryStore((s) => s.uniqueProjects);

  useEffect(() => {
    if (entries.length === 0 && !loading) {
      load();
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const filtered = filteredEntries();
  const projects = uniqueProjects();

  return (
    <div className="flex flex-col flex-1 overflow-hidden px-4">
      {/* Search and filter bar */}
      <div className="flex items-center gap-2 mb-3">
        <div className="flex-1 flex items-center gap-1.5 px-2.5 py-1.5 bg-bg-secondary border border-bg-border rounded text-xs">
          <Search size={12} className="text-text-muted flex-shrink-0" />
          <input
            type="text"
            placeholder="Search prompts..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="flex-1 bg-transparent text-text-primary placeholder:text-text-muted outline-none text-xs"
          />
        </div>

        {projects.length > 0 && (
          <div className="flex items-center gap-1.5 px-2.5 py-1.5 bg-bg-secondary border border-bg-border rounded text-xs">
            <Filter size={12} className="text-text-muted flex-shrink-0" />
            <select
              value={projectFilter || ""}
              onChange={(e) => setProjectFilter(e.target.value || null)}
              className="bg-transparent text-text-secondary outline-none text-xs cursor-pointer"
            >
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </div>
        )}

        <button
          onClick={load}
          disabled={loading}
          className="p-1.5 text-text-muted hover:text-text-primary transition-colors disabled:opacity-50"
          title="Refresh"
        >
          <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
        </button>
      </div>

      {/* Results count */}
      <p className="text-[10px] text-text-muted mb-2">
        {filtered.length} prompt{filtered.length !== 1 ? "s" : ""}
        {entries.length > 0 && filtered.length !== entries.length
          ? ` (of ${entries.length})`
          : ""}
      </p>

      {error && (
        <div className="text-[10px] text-accent-amber bg-accent-amber/10 rounded px-2 py-1 mb-2">
          {error}
        </div>
      )}

      {loading && entries.length === 0 && (
        <p className="text-xs text-text-muted">Loading history...</p>
      )}

      {/* Scrollable list */}
      <div className="flex-1 overflow-y-auto space-y-1 pb-4">
        {filtered.map((entry, i) => (
          <PromptRow key={`${entry.sessionId}-${entry.timestamp}-${i}`} entry={entry} />
        ))}
        {!loading && filtered.length === 0 && entries.length > 0 && (
          <p className="text-xs text-text-muted pt-2">No matching prompts.</p>
        )}
        {!loading && entries.length === 0 && !error && (
          <p className="text-xs text-text-muted pt-2">
            No prompt history found. History is read from ~/.claude/
          </p>
        )}
      </div>
    </div>
  );
}

function PromptRow({ entry }: { entry: HistoryEntry }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(entry.display).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [entry.display]);

  const formatTimestamp = (ts: number) => {
    if (!ts) return "";
    const d = new Date(ts * 1000);
    const now = new Date();
    const isToday = d.toDateString() === now.toDateString();
    if (isToday) {
      return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    }
    return d.toLocaleDateString([], { month: "short", day: "numeric" }) +
      " " +
      d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  };

  return (
    <div className="group flex items-start gap-2 px-3 py-2 bg-bg-secondary border border-bg-border rounded hover:border-text-muted/30 transition-colors">
      <div className="flex-1 min-w-0">
        <p className="text-xs text-text-primary leading-relaxed line-clamp-3">
          {entry.display}
        </p>
        <div className="flex items-center gap-2 mt-1">
          <span className="text-[9px] text-text-muted">
            {formatTimestamp(entry.timestamp)}
          </span>
          {entry.project && (
            <span className="text-[9px] px-1.5 py-0 rounded bg-accent-purple/15 text-accent-purple">
              {entry.project}
            </span>
          )}
        </div>
      </div>
      <button
        onClick={handleCopy}
        className="p-1 text-text-muted hover:text-text-primary opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
        title="Copy prompt"
      >
        {copied ? <Check size={12} className="text-accent-green" /> : <Copy size={12} />}
      </button>
    </div>
  );
}

// --- Active Sessions Tab (from tabStore) ---

function ActiveSessionsTab() {
  const tabs = useTabStore((s) => s.tabs);

  const activeSessions = tabs.filter(
    (t) => t.status !== "done" && t.status !== "error"
  );
  const pastSessions = tabs.filter(
    (t) => t.status === "done" || t.status === "error"
  );

  return (
    <div className="flex-1 overflow-y-auto px-4 pb-4">
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
              : session.status === "waiting_approval"
                ? "bg-accent-amber animate-pulse"
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

import { useState, useMemo, useEffect } from "react";
import { Target, Plus, Search, Trash2, X, ChevronDown, Play } from "lucide-react";
import { useFlightStore } from "@/stores/flightStore";
import { useIssueStore } from "@/stores/issueStore";
import { useAppStore } from "@/stores/appStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { relativeTime } from "@/lib/time";
import { FLIGHT_STATUS_CONFIG, FLIGHT_PRIORITY_COLORS, ISSUE_STATUS_COLORS, ISSUE_STATUS_LABELS } from "@/lib/flight-colors";
import type { Flight, FlightStatus, FlightPriority } from "@/types/flight";
import type { IssueStatus } from "@/stores/issueStore";

const ALL_STATUSES: FlightStatus[] = ["draft", "active", "blocked", "needs_human", "done", "failed"];
const ALL_PRIORITIES: FlightPriority[] = ["low", "medium", "high", "critical"];

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function FlightsView() {
  const flights = useFlightStore((s) => s.flights);
  const activeFlightId = useFlightStore((s) => s.activeFlightId);
  const [selectedId, setSelectedId] = useState<string | null>(activeFlightId);
  const [search, setSearch] = useState("");

  useEffect(() => {
    if (activeFlightId) {
      setSelectedId(activeFlightId);
    }
  }, [activeFlightId]);
  const [statusFilter, setStatusFilter] = useState<FlightStatus | "all">("all");
  const [showCreate, setShowCreate] = useState(false);

  const filtered = useMemo(() => {
    let list = flights;
    if (statusFilter !== "all") {
      list = list.filter((f) => f.status === statusFilter);
    }
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(
        (f) =>
          f.title.toLowerCase().includes(q) ||
          f.objective.toLowerCase().includes(q)
      );
    }
    return list.sort((a, b) => b.updatedAt - a.updatedAt);
  }, [flights, search, statusFilter]);

  const selected = useMemo(
    () => flights.find((f) => f.id === selectedId) ?? null,
    [flights, selectedId]
  );

  function handleCreated(id: string) {
    setSelectedId(id);
    setShowCreate(false);
  }

  return (
    <div className="flex flex-1 h-full overflow-hidden bg-bg-primary">
      {/* Left panel — flight list */}
      <div className="flex flex-col w-[280px] min-w-[280px] border-r border-bg-border bg-bg-secondary">
        {/* Header */}
        <div className="flex items-center justify-between px-3 pt-3 pb-2">
          <div className="flex items-center gap-2">
            <Target size={16} className="text-accent-green" />
            <h2 className="text-sm font-semibold text-text-primary">Flights</h2>
            <span className="text-[10px] text-text-muted bg-bg-elevated px-1.5 py-0.5 rounded-full">
              {flights.length}
            </span>
          </div>
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="flex items-center gap-1 px-2 py-1 text-[11px] text-accent-green hover:bg-accent-green/10 rounded transition-colors"
          >
            <Plus size={12} />
            New Flight
          </button>
        </div>

        {/* Create form */}
        {showCreate && (
          <CreateFlightForm
            onCreated={handleCreated}
            onCancel={() => setShowCreate(false)}
          />
        )}

        {/* Search + filter */}
        <div className="px-3 pb-2 space-y-1.5">
          <div className="flex items-center gap-1.5 px-2 py-1 bg-bg-elevated rounded border border-bg-border">
            <Search size={12} className="text-text-muted" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search flights…"
              className="flex-1 bg-transparent text-xs text-text-primary placeholder:text-text-muted outline-none"
            />
          </div>
          <StatusFilterDropdown value={statusFilter} onChange={setStatusFilter} />
        </div>

        {/* Flight list */}
        <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-1">
          {filtered.length === 0 && (
            <p className="text-[11px] text-text-muted px-2 py-4 text-center">
              No flights match your filters. Try clearing the search or status filter.
            </p>
          )}
          {filtered.map((f) => (
            <FlightCard
              key={f.id}
              flight={f}
              isSelected={f.id === selectedId}
              onClick={() => setSelectedId(f.id)}
            />
          ))}
        </div>
      </div>

      {/* Right panel — detail or empty state */}
      <div className="flex flex-col flex-1 overflow-hidden">
        {selected ? (
          <FlightDetail
            flight={selected}
            onDeleted={() => setSelectedId(null)}
          />
        ) : (
          <EmptyState hasFlights={flights.length > 0} />
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Status filter dropdown
// ---------------------------------------------------------------------------

function StatusFilterDropdown({
  value,
  onChange,
}: {
  value: FlightStatus | "all";
  onChange: (v: FlightStatus | "all") => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 px-2 py-1 text-[11px] text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors w-full"
      >
        <ChevronDown size={10} className="text-text-muted" />
        <span>{value === "all" ? "All statuses" : FLIGHT_STATUS_CONFIG[value].label}</span>
      </button>
      {open && (
        <div className="absolute top-full left-0 mt-0.5 w-full bg-bg-elevated border border-bg-border rounded shadow-lg z-20 py-0.5">
          <button
            onClick={() => { onChange("all"); setOpen(false); }}
            className={`block w-full text-left px-2 py-1 text-[11px] hover:bg-bg-hover transition-colors ${
              value === "all" ? "text-accent-green" : "text-text-secondary"
            }`}
          >
            All statuses
          </button>
          {ALL_STATUSES.map((s) => (
            <button
              key={s}
              onClick={() => { onChange(s); setOpen(false); }}
              className={`block w-full text-left px-2 py-1 text-[11px] hover:bg-bg-hover transition-colors ${
                value === s ? "text-accent-green" : "text-text-secondary"
              }`}
            >
              {FLIGHT_STATUS_CONFIG[s].label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Flight card (left panel list item)
// ---------------------------------------------------------------------------

function FlightCard({
  flight,
  isSelected,
  onClick,
}: {
  flight: Flight;
  isSelected: boolean;
  onClick: () => void;
}) {
  const sc = FLIGHT_STATUS_CONFIG[flight.status];
  const pc = FLIGHT_PRIORITY_COLORS[flight.priority];

  return (
    <button
      onClick={onClick}
      className={`w-full text-left px-2.5 py-2 rounded transition-colors border ${
        isSelected
          ? "border-accent-green/50 bg-bg-hover"
          : "border-transparent hover:bg-bg-hover"
      }`}
    >
      <p className="text-xs text-text-primary truncate">{flight.title || "Untitled"}</p>
      <div className="flex items-center gap-2 mt-1">
        <span className={`inline-flex items-center gap-1 text-[10px] ${sc.text}`}>
          <span className={`w-1.5 h-1.5 rounded-full ${sc.dot}`} />
          {FLIGHT_STATUS_CONFIG[flight.status].label}
        </span>
        <span className={`text-[10px] ${pc}`}>{flight.priority}</span>
        <span className="text-[10px] text-text-muted">{flight.issueIds.length} issues</span>
      </div>
      <p className="text-[10px] text-text-muted mt-0.5">{relativeTime(flight.updatedAt)}</p>
    </button>
  );
}

// ---------------------------------------------------------------------------
// Create flight form
// ---------------------------------------------------------------------------

function CreateFlightForm({
  onCreated,
  onCancel,
}: {
  onCreated: (id: string) => void;
  onCancel: () => void;
}) {
  const addFlight = useFlightStore((s) => s.addFlight);
  const [title, setTitle] = useState("");
  const [objective, setObjective] = useState("");
  const [priority, setPriority] = useState<FlightPriority>("medium");

  function handleCreate() {
    if (!title.trim()) return;
    const f = addFlight({
      title: title.trim(),
      objective: objective.trim(),
      priority,
      status: "draft",
      issueIds: [],
      linkedSessionIds: [],
    });
    onCreated(f.id);
  }

  return (
    <div className="mx-3 mb-2 p-2 bg-bg-elevated rounded border border-bg-border space-y-1.5">
      <input
        type="text"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Flight title"
        className="w-full bg-bg-primary text-xs text-text-primary placeholder:text-text-muted px-2 py-1 rounded border border-bg-border outline-none focus:border-accent-green/50"
        autoFocus
      />
      <textarea
        value={objective}
        onChange={(e) => setObjective(e.target.value)}
        placeholder="Objective (optional)"
        rows={2}
        className="w-full bg-bg-primary text-xs text-text-primary placeholder:text-text-muted px-2 py-1 rounded border border-bg-border outline-none focus:border-accent-green/50 resize-none"
      />
      <div className="flex items-center gap-2">
        <select
          value={priority}
          onChange={(e) => setPriority(e.target.value as FlightPriority)}
          className="bg-bg-primary text-xs text-text-primary border border-bg-border rounded px-1.5 py-0.5 outline-none"
        >
          {ALL_PRIORITIES.map((p) => (
            <option key={p} value={p}>{p.charAt(0).toUpperCase() + p.slice(1)}</option>
          ))}
        </select>
        <div className="flex-1" />
        <button
          onClick={onCancel}
          className="px-2 py-0.5 text-[11px] text-text-muted hover:text-text-primary transition-colors"
        >
          Cancel
        </button>
        <button
          onClick={handleCreate}
          disabled={!title.trim()}
          className="px-2 py-0.5 text-[11px] text-accent-green hover:bg-accent-green/10 rounded transition-colors disabled:opacity-40"
        >
          Create Flight
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Flight detail (right panel)
// ---------------------------------------------------------------------------

function FlightDetail({
  flight,
  onDeleted,
}: {
  flight: Flight;
  onDeleted: () => void;
}) {
  const deleteFlight = useFlightStore((s) => s.deleteFlight);
  const updateFlight = useFlightStore((s) => s.updateFlight);
  const computeFlightStatus = useFlightStore((s) => s.computeFlightStatus);
  const linkSessionToFlight = useFlightStore((s) => s.linkSessionToFlight);
  const issues = useIssueStore((s) => s.issues);

  const linkedIssues = useMemo(
    () => issues.filter((i) => flight.issueIds.includes(i.id)),
    [issues, flight.issueIds]
  );

  const computedStatus = computeFlightStatus(flight.id);
  const csc = FLIGHT_STATUS_CONFIG[computedStatus];

  // Issue rollup counts
  const rollup = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const i of linkedIssues) {
      counts[i.status] = (counts[i.status] || 0) + 1;
    }
    return counts;
  }, [linkedIssues]);

  function handleLaunchSession(cli: "claude" | "codex") {
    // Build context-rich prompt from flight + issues
    const lines: string[] = [];
    lines.push(`Work on this flight:`);
    lines.push(``);
    lines.push(`## ${flight.title}`);
    if (flight.objective) {
      lines.push(``);
      lines.push(flight.objective);
    }
    lines.push(``);
    lines.push(`Priority: ${flight.priority}`);

    if (linkedIssues.length > 0) {
      lines.push(``);
      lines.push(`### Linked Issues (${linkedIssues.length})`);
      for (const issue of linkedIssues) {
        const statusStr = ISSUE_STATUS_LABELS[issue.status];
        lines.push(``);
        lines.push(`- **${issue.ticketId}: ${issue.title}** [${statusStr}]`);
        if (issue.description) {
          lines.push(`  ${issue.description}`);
        }
        const criteria = issue.acceptanceCriteria;
        if (criteria && criteria.length > 0) {
          for (const c of criteria) {
            lines.push(`  - [${c.checked ? "x" : " "}] ${c.text}`);
          }
        }
      }
    }

    const prompt = lines.join("\n");

    // Switch to session view and open a new pane
    useAppStore.getState().setActiveView(cli);
    const paneId = useLayoutStore.getState().addPane({ cliCommand: cli });

    // Watch for the pane's sessionId to be set, then link it to the flight.
    // This replaces a fragile fixed timeout with an event-driven approach.
    const unsub = useLayoutStore.subscribe((state) => {
      const pane = state.panes.find((p) => p.id === paneId);
      if (pane?.sessionId) {
        linkSessionToFlight(flight.id, pane.sessionId);
        unsub();
      }
    });
    // Safety: clean up the subscription after 30s if the session never starts
    setTimeout(() => unsub(), 30_000);

    // Inject the prompt via the same custom event pattern used by "Work on this issue"
    setTimeout(() => {
      window.dispatchEvent(
        new CustomEvent("packetcode:issue-prompt", { detail: { prompt } })
      );
    }, 1500);
  }

  function handleDelete() {
    if (!window.confirm(`Delete flight "${flight.title}"? This cannot be undone.`)) return;
    deleteFlight(flight.id);
    onDeleted();
  }

  return (
    <div className="flex flex-col flex-1 overflow-y-auto">
      {/* Header — editable */}
      <div className="px-4 pt-4 pb-3 border-b border-bg-border space-y-2">
        <input
          type="text"
          value={flight.title}
          onChange={(e) => updateFlight(flight.id, { title: e.target.value })}
          className="w-full text-sm font-semibold text-text-primary bg-transparent border-b border-transparent hover:border-bg-border focus:border-accent-green focus:outline-none truncate"
          placeholder="Flight title"
        />
        <textarea
          value={flight.objective}
          onChange={(e) => updateFlight(flight.id, { objective: e.target.value })}
          rows={2}
          className="w-full text-xs text-text-secondary bg-transparent border border-transparent hover:border-bg-border focus:border-accent-green focus:outline-none resize-none rounded px-1 py-0.5"
          placeholder="Objective (optional)"
        />
        <div className="flex items-center gap-2">
          <select
            value={flight.status}
            onChange={(e) => updateFlight(flight.id, { status: e.target.value as FlightStatus })}
            className="bg-bg-primary text-[11px] text-text-secondary border border-bg-border rounded px-1.5 py-0.5 outline-none focus:border-accent-green"
          >
            {ALL_STATUSES.map((s) => (
              <option key={s} value={s}>{FLIGHT_STATUS_CONFIG[s].label}</option>
            ))}
          </select>
          <select
            value={flight.priority}
            onChange={(e) => updateFlight(flight.id, { priority: e.target.value as FlightPriority })}
            className="bg-bg-primary text-[11px] text-text-secondary border border-bg-border rounded px-1.5 py-0.5 outline-none focus:border-accent-green"
          >
            {ALL_PRIORITIES.map((p) => (
              <option key={p} value={p}>{p.charAt(0).toUpperCase() + p.slice(1)}</option>
            ))}
          </select>
          {linkedIssues.length > 0 && (
            <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] ${csc.bg} ${csc.text}`}>
              <span className={`w-1.5 h-1.5 rounded-full ${csc.dot}`} />
              Rollup: {FLIGHT_STATUS_CONFIG[computedStatus].label}
            </span>
          )}
        </div>
      </div>

      {/* Status Rollup */}
      {linkedIssues.length > 0 && (
        <div className="px-4 py-2 border-b border-bg-border">
          <div className="flex items-center gap-2 mb-1.5">
            <span className="text-[11px] font-medium text-text-secondary">Status Rollup</span>
            <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] ${csc.bg} ${csc.text}`}>
              <span className={`w-1.5 h-1.5 rounded-full ${csc.dot}`} />
              {FLIGHT_STATUS_CONFIG[computedStatus].label}
            </span>
          </div>
          <div className="flex items-center gap-3 flex-wrap">
            {(["todo", "in_progress", "qa", "done", "blocked", "needs_human"] as IssueStatus[]).map((s) =>
              rollup[s] ? (
                <span key={s} className="inline-flex items-center gap-1 text-[10px] text-text-muted">
                  <span className={`w-1.5 h-1.5 rounded-full ${ISSUE_STATUS_COLORS[s]}`} />
                  {ISSUE_STATUS_LABELS[s]} {rollup[s]}
                </span>
              ) : null
            )}
          </div>
        </div>
      )}

      {/* Launch session */}
      <div className="px-4 py-2 border-b border-bg-border">
        <span className="text-[11px] font-medium text-text-secondary mb-1.5 block">Launch Session</span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => handleLaunchSession("claude")}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] text-accent-green bg-accent-green/10 border border-accent-green/20 rounded hover:bg-accent-green/20 transition-colors"
          >
            <Play size={11} />
            Claude
          </button>
          <button
            onClick={() => handleLaunchSession("codex")}
            className="flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] text-accent-blue bg-accent-blue/10 border border-accent-blue/20 rounded hover:bg-accent-blue/20 transition-colors"
          >
            <Play size={11} />
            Codex
          </button>
          <span className="text-[10px] text-text-muted ml-1">with flight context</span>
        </div>
      </div>

      {/* Linked Issues */}
      <div className="px-4 py-3 border-b border-bg-border">
        <div className="flex items-center justify-between mb-2">
          <span className="text-[11px] font-medium text-text-secondary">
            Linked Issues ({linkedIssues.length})
          </span>
          <AssignIssueButton flightId={flight.id} />
        </div>
        {linkedIssues.length === 0 ? (
          <p className="text-[10px] text-text-muted py-2">No issues linked yet.</p>
        ) : (
          <div className="space-y-0.5">
            {linkedIssues.map((issue) => (
              <IssueRow key={issue.id} issue={issue} flightId={flight.id} />
            ))}
          </div>
        )}
      </div>

      {/* Linked Sessions */}
      <div className="px-4 py-3 border-b border-bg-border">
        <span className="text-[11px] font-medium text-text-secondary">
          Sessions ({flight.linkedSessionIds.length})
        </span>
        {flight.linkedSessionIds.length === 0 ? (
          <p className="text-[10px] text-text-muted py-2">No sessions linked yet. Launch a session above.</p>
        ) : (
          <div className="mt-1.5 space-y-0.5">
            {flight.linkedSessionIds.map((sid) => (
              <div key={sid} className="flex items-center gap-2 py-1 group">
                <span className="w-1.5 h-1.5 rounded-full bg-accent-blue shrink-0" />
                <span className="text-[10px] text-text-secondary font-mono truncate flex-1">{sid}</span>
                <button
                  onClick={() => {
                    useFlightStore.getState().unlinkSessionFromFlight(flight.id, sid);
                  }}
                  className="opacity-0 group-hover:opacity-100 p-0.5 text-text-muted hover:text-accent-red transition-all"
                  title="Unlink session"
                >
                  <X size={10} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Delete */}
      <div className="px-4 py-3 mt-auto">
        <button
          onClick={handleDelete}
          className="flex items-center gap-1.5 text-[11px] text-accent-red hover:bg-accent-red/10 px-2 py-1 rounded transition-colors"
        >
          <Trash2 size={12} />
          Delete flight
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Issue row inside flight detail
// ---------------------------------------------------------------------------

function IssueRow({
  issue,
  flightId,
}: {
  issue: { id: string; ticketId: string; title: string; status: IssueStatus; priority: string };
  flightId: string;
}) {
  const removeIssueFromFlight = useFlightStore((s) => s.removeIssueFromFlight);
  const assignToFlight = useIssueStore((s) => s.assignToFlight);

  const statusColor = ISSUE_STATUS_COLORS[issue.status] ?? "bg-text-muted";
  const pc = FLIGHT_PRIORITY_COLORS[issue.priority as FlightPriority] ?? "text-text-muted";

  function handleRemove() {
    removeIssueFromFlight(flightId, issue.id);
    assignToFlight(issue.id, null);
  }

  return (
    <div className="flex items-center gap-2 py-1.5 group">
      <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${statusColor}`} />
      <span className="text-[10px] text-text-muted font-mono w-14 shrink-0">{issue.ticketId}</span>
      <span className="text-xs text-text-primary truncate flex-1">{issue.title}</span>
      <span className={`text-[10px] shrink-0 ${pc}`}>{issue.priority}</span>
      <button
        onClick={handleRemove}
        className="opacity-0 group-hover:opacity-100 p-0.5 text-text-muted hover:text-accent-red transition-all"
        title="Remove from flight"
      >
        <X size={10} />
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Assign issue button + dropdown
// ---------------------------------------------------------------------------

function AssignIssueButton({ flightId }: { flightId: string }) {
  const [open, setOpen] = useState(false);
  const issues = useIssueStore((s) => s.issues);
  const addIssueToFlight = useFlightStore((s) => s.addIssueToFlight);
  const assignToFlight = useIssueStore((s) => s.assignToFlight);

  const unassigned = useMemo(
    () => issues.filter((i) => i.flightId == null),
    [issues]
  );

  function handleAssign(issueId: string) {
    addIssueToFlight(flightId, issueId);
    assignToFlight(issueId, flightId);
    setOpen(false);
  }

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1 text-[10px] text-accent-green hover:bg-accent-green/10 px-1.5 py-0.5 rounded transition-colors"
      >
        <Plus size={10} />
        Assign
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-0.5 w-56 max-h-48 overflow-y-auto bg-bg-elevated border border-bg-border rounded shadow-lg z-30 py-0.5">
          {unassigned.length === 0 ? (
            <p className="text-[10px] text-text-muted px-2 py-2 text-center">No unassigned issues</p>
          ) : (
            unassigned.map((issue) => (
              <button
                key={issue.id}
                onClick={() => handleAssign(issue.id)}
                className="flex items-center gap-2 w-full text-left px-2 py-1.5 text-[11px] text-text-secondary hover:bg-bg-hover transition-colors"
              >
                <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${ISSUE_STATUS_COLORS[issue.status]}`} />
                <span className="text-text-muted font-mono text-[10px] w-14 shrink-0">{issue.ticketId}</span>
                <span className="truncate flex-1">{issue.title}</span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

function EmptyState({ hasFlights }: { hasFlights: boolean }) {
  return (
    <div className="flex flex-col items-center justify-center flex-1 gap-3">
      <Target size={32} className="text-text-muted" />
      <p className="text-xs text-text-muted text-center max-w-[240px]">
        {hasFlights
          ? "Select a flight or create one."
          : "No flights yet. Create your first flight to organize your work."}
      </p>
    </div>
  );
}

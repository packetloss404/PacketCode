import { useState, useMemo } from "react";
import { Radio, ChevronDown, ChevronRight, Target, AlertTriangle } from "lucide-react";
import { useFlightStore } from "@/stores/flightStore";
import { useIssueStore } from "@/stores/issueStore";
import { useAppStore } from "@/stores/appStore";
import { relativeTime } from "@/lib/time";
import { FLIGHT_STATUS_CONFIG, FLIGHT_PRIORITY_COLORS } from "@/lib/flight-colors";
import type { Flight, FlightStatus } from "@/types/flight";

function handleFlightClick(flightId: string) {
  useFlightStore.getState().setActiveFlight(flightId);
  useAppStore.getState().setActiveView("flights");
}

// ── StatusStrip ──────────────────────────────────────────────────────

function StatusStrip({ statusCounts, total }: { statusCounts: Record<FlightStatus, number>; total: number }) {
  const statuses: FlightStatus[] = ["active", "blocked", "needs_human", "done", "draft", "failed"];
  return (
    <div className="flex items-center gap-2 px-4 py-2 bg-bg-secondary border-b border-bg-border">
      <span className="text-[11px] text-text-muted mr-1">
        {total} flight{total !== 1 ? "s" : ""}
      </span>
      <div className="w-px h-4 bg-bg-border" />
      {statuses.map((s) => {
        const cfg = FLIGHT_STATUS_CONFIG[s];
        if (statusCounts[s] === 0) return null;
        return (
          <span key={s} className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium ${cfg.bg} ${cfg.text}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${cfg.dot}`} />
            {cfg.label} {statusCounts[s]}
          </span>
        );
      })}
    </div>
  );
}

// ── AttentionCard ────────────────────────────────────────────────────

function AttentionCard({ flight, status }: { flight: Flight; status: FlightStatus }) {
  const issues = useIssueStore((s) => s.issues);
  const linkedIssues = issues.filter((i) => flight.issueIds.includes(i.id));
  const concerningIssues = linkedIssues.filter((i) => i.status === "blocked" || i.status === "needs_human");
  const cfg = FLIGHT_STATUS_CONFIG[status];
  const borderColor = status === "blocked" ? "border-accent-red" : "border-accent-amber";

  return (
    <button
      onClick={() => handleFlightClick(flight.id)}
      className={`w-full text-left border-l-2 ${borderColor} bg-bg-elevated rounded-r px-3 py-2 hover:bg-bg-hover transition-colors`}
    >
      <div className="flex items-center gap-2 mb-1">
        <span className={`w-2 h-2 rounded-full ${cfg.dot}`} />
        <span className="text-xs font-medium text-text-primary truncate flex-1">{flight.title}</span>
        <span className={`text-[10px] font-medium ${FLIGHT_PRIORITY_COLORS[flight.priority] || "text-text-muted"}`}>
          {flight.priority}
        </span>
      </div>
      <div className="flex items-center gap-3 text-[10px] text-text-muted">
        <span>{linkedIssues.length} issue{linkedIssues.length !== 1 ? "s" : ""}</span>
        {concerningIssues.length > 0 && (
          <span className="flex items-center gap-0.5 text-accent-amber">
            <AlertTriangle size={9} />
            {concerningIssues.length} need attention
          </span>
        )}
        <span className="ml-auto">{relativeTime(flight.updatedAt)}</span>
      </div>
      {concerningIssues.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {concerningIssues.map((issue) => (
            <span
              key={issue.id}
              className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[9px] ${
                issue.status === "blocked" ? "bg-accent-red/10 text-accent-red" : "bg-accent-amber/10 text-accent-amber"
              }`}
            >
              {issue.ticketId}: {issue.status === "blocked" ? "blocked" : "needs human"}
            </span>
          ))}
        </div>
      )}
    </button>
  );
}

// ── AttentionQueue ───────────────────────────────────────────────────

function AttentionQueue({ attentionFlights }: { attentionFlights: { flight: Flight; status: FlightStatus }[] }) {
  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-2 mb-2">
        <AlertTriangle size={12} className="text-accent-amber" />
        <span className="text-xs font-semibold text-text-primary">Attention Queue</span>
        <span className="text-[10px] text-text-muted">({attentionFlights.length})</span>
      </div>
      {attentionFlights.length === 0 ? (
        <div className="text-[11px] text-text-muted px-2 py-3 text-center bg-bg-elevated rounded">
          All clear — no flights need attention
        </div>
      ) : (
        <div className="flex flex-col gap-1.5">
          {attentionFlights.map(({ flight, status }) => (
            <AttentionCard key={flight.id} flight={flight} status={status} />
          ))}
        </div>
      )}
    </div>
  );
}

// ── ActiveFlightsSection ─────────────────────────────────────────────

function ActiveFlightsSection({ activeFlights }: { activeFlights: { flight: Flight; status: FlightStatus }[] }) {
  const issues = useIssueStore((s) => s.issues);
  if (activeFlights.length === 0) return null;

  return (
    <div className="px-4 py-3 border-t border-bg-border">
      <div className="flex items-center gap-2 mb-2">
        <Target size={12} className="text-accent-blue" />
        <span className="text-xs font-semibold text-text-primary">Active Flights</span>
        <span className="text-[10px] text-text-muted">({activeFlights.length})</span>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
        {activeFlights.map(({ flight }) => {
          const linkedIssues = issues.filter((i) => flight.issueIds.includes(i.id));
          const doneCount = linkedIssues.filter((i) => i.status === "done").length;
          return (
            <button
              key={flight.id}
              onClick={() => handleFlightClick(flight.id)}
              className="text-left bg-bg-elevated rounded px-3 py-2 hover:bg-bg-hover transition-colors border border-bg-border"
            >
              <div className="flex items-center gap-2 mb-1">
                <span className="w-1.5 h-1.5 rounded-full bg-accent-blue" />
                <span className="text-xs font-medium text-text-primary truncate flex-1">{flight.title}</span>
              </div>
              <div className="flex items-center gap-3 text-[10px] text-text-muted">
                <span className={FLIGHT_PRIORITY_COLORS[flight.priority] || "text-text-muted"}>{flight.priority}</span>
                <span>{doneCount}/{linkedIssues.length} issues done</span>
                <span>{flight.linkedSessionIds.length} session{flight.linkedSessionIds.length !== 1 ? "s" : ""}</span>
                <span className="ml-auto">{relativeTime(flight.updatedAt)}</span>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ── FlightRow ────────────────────────────────────────────────────────

function FlightRow({ flight, status }: { flight: Flight; status: FlightStatus }) {
  const cfg = FLIGHT_STATUS_CONFIG[status];
  return (
    <button
      onClick={() => handleFlightClick(flight.id)}
      className="flex items-center gap-3 w-full text-left px-3 py-1.5 hover:bg-bg-hover transition-colors rounded text-[11px]"
    >
      <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${cfg.dot}`} />
      <span className="text-text-primary truncate flex-1 min-w-0">{flight.title}</span>
      <span className={`flex-shrink-0 ${FLIGHT_PRIORITY_COLORS[flight.priority] || "text-text-muted"}`}>{flight.priority}</span>
      <span className="flex-shrink-0 text-text-muted w-16 text-right">{flight.issueIds.length} issue{flight.issueIds.length !== 1 ? "s" : ""}</span>
      <span className="flex-shrink-0 text-text-muted w-20 text-right">{flight.linkedSessionIds.length} {flight.linkedSessionIds.length === 1 ? "session" : "sessions"}</span>
      <span className="flex-shrink-0 text-text-muted w-14 text-right">{relativeTime(flight.updatedAt)}</span>
    </button>
  );
}

// ── FlightGroupSection ───────────────────────────────────────────────

function FlightGroupSection({ statusKey, flights }: { statusKey: FlightStatus; flights: Flight[] }) {
  const [open, setOpen] = useState(true);
  const cfg = FLIGHT_STATUS_CONFIG[statusKey];

  if (flights.length === 0) return null;

  return (
    <div className="mb-1">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 w-full px-4 py-1.5 text-left hover:bg-bg-hover transition-colors"
      >
        {open ? <ChevronDown size={10} className="text-text-muted" /> : <ChevronRight size={10} className="text-text-muted" />}
        <span className={`w-1.5 h-1.5 rounded-full ${cfg.dot}`} />
        <span className={`text-[11px] font-medium ${cfg.text}`}>{cfg.label}</span>
        <span className="text-[10px] text-text-muted">({flights.length})</span>
      </button>
      {open && (
        <div className="px-2">
          {flights.map((f) => (
            <FlightRow key={f.id} flight={f} status={statusKey} />
          ))}
        </div>
      )}
    </div>
  );
}

// ── FlightDeckView (main) ────────────────────────────────────────────

export function FlightDeckView() {
  const flights = useFlightStore((s) => s.flights);
  const computeFlightStatus = useFlightStore((s) => s.computeFlightStatus);

  // Compute status for every flight
  const flightsByStatus = useMemo(() => {
    const map: Record<FlightStatus, { flight: Flight; status: FlightStatus }[]> = {
      draft: [],
      active: [],
      blocked: [],
      needs_human: [],
      done: [],
      failed: [],
    };
    for (const f of flights) {
      const status = computeFlightStatus(f.id);
      map[status].push({ flight: f, status });
    }
    return map;
  }, [flights, computeFlightStatus]);

  const statusCounts = useMemo(() => {
    const counts: Record<FlightStatus, number> = { draft: 0, active: 0, blocked: 0, needs_human: 0, done: 0, failed: 0 };
    for (const key of Object.keys(counts) as FlightStatus[]) {
      counts[key] = flightsByStatus[key].length;
    }
    return counts;
  }, [flightsByStatus]);

  const attentionFlights = useMemo(
    () => [...flightsByStatus.blocked, ...flightsByStatus.needs_human],
    [flightsByStatus]
  );

  const activeFlights = flightsByStatus.active;

  // Groups for "All Flights" section — exclude active, blocked, needs_human (already shown above)
  const remainingGroups: FlightStatus[] = ["done", "draft", "failed"];

  if (flights.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center flex-1 gap-3 text-text-muted">
        <Radio size={32} />
        <span className="text-sm font-medium">No flights to supervise</span>
        <span className="text-xs">Create flights from the Flights view to see them here.</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <StatusStrip statusCounts={statusCounts} total={flights.length} />

      <div className="flex-1 overflow-y-auto">
        <AttentionQueue attentionFlights={attentionFlights} />
        <ActiveFlightsSection activeFlights={activeFlights} />

        {/* All Flights — remaining groups */}
        <div className="px-4 py-3 border-t border-bg-border">
          <span className="text-xs font-semibold text-text-primary mb-2 block">All Flights</span>
          {remainingGroups.map((statusKey) => (
            <FlightGroupSection
              key={statusKey}
              statusKey={statusKey}
              flights={flightsByStatus[statusKey].map((e) => e.flight)}
            />
          ))}
          {remainingGroups.every((s) => flightsByStatus[s].length === 0) && (
            <div className="text-[11px] text-text-muted px-2 py-2">No other flights.</div>
          )}
        </div>
      </div>
    </div>
  );
}

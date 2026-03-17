import { useState, useMemo } from "react";
import { Radio, ChevronDown, ChevronRight, Target, AlertTriangle } from "lucide-react";
import { useMissionStore } from "@/stores/missionStore";
import { useIssueStore } from "@/stores/issueStore";
import { useAppStore } from "@/stores/appStore";
import type { Mission, MissionStatus } from "@/types/mission";

// ── Helpers ──────────────────────────────────────────────────────────

function relativeTime(ts: number): string {
  const diff = Date.now() - ts;
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

const STATUS_CONFIG: Record<MissionStatus, { dot: string; bg: string; text: string; label: string }> = {
  draft: { dot: "bg-text-muted", bg: "bg-text-muted/10", text: "text-text-muted", label: "Draft" },
  active: { dot: "bg-accent-blue", bg: "bg-accent-blue/10", text: "text-accent-blue", label: "Active" },
  blocked: { dot: "bg-accent-red", bg: "bg-accent-red/10", text: "text-accent-red", label: "Blocked" },
  needs_human: { dot: "bg-accent-amber", bg: "bg-accent-amber/10", text: "text-accent-amber", label: "Needs Human" },
  done: { dot: "bg-accent-green", bg: "bg-accent-green/10", text: "text-accent-green", label: "Done" },
  failed: { dot: "bg-accent-red", bg: "bg-accent-red/10", text: "text-accent-red", label: "Failed" },
};

const PRIORITY_COLORS: Record<string, string> = {
  critical: "text-accent-red",
  high: "text-accent-amber",
  medium: "text-accent-blue",
  low: "text-text-muted",
};

function handleMissionClick(missionId: string) {
  useMissionStore.getState().setActiveMission(missionId);
  useAppStore.getState().setActiveView("missions");
}

// ── StatusStrip ──────────────────────────────────────────────────────

function StatusStrip({ statusCounts, total }: { statusCounts: Record<MissionStatus, number>; total: number }) {
  const statuses: MissionStatus[] = ["active", "blocked", "needs_human", "done", "draft", "failed"];
  return (
    <div className="flex items-center gap-2 px-4 py-2 bg-bg-secondary border-b border-bg-border">
      <span className="text-[11px] text-text-muted mr-1">
        {total} mission{total !== 1 ? "s" : ""}
      </span>
      <div className="w-px h-4 bg-bg-border" />
      {statuses.map((s) => {
        const cfg = STATUS_CONFIG[s];
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

function AttentionCard({ mission, status }: { mission: Mission; status: MissionStatus }) {
  const issues = useIssueStore((s) => s.issues);
  const linkedIssues = issues.filter((i) => mission.issueIds.includes(i.id));
  const concerningIssues = linkedIssues.filter((i) => i.status === "blocked" || i.status === "needs_human");
  const cfg = STATUS_CONFIG[status];
  const borderColor = status === "blocked" ? "border-accent-red" : "border-accent-amber";

  return (
    <button
      onClick={() => handleMissionClick(mission.id)}
      className={`w-full text-left border-l-2 ${borderColor} bg-bg-elevated rounded-r px-3 py-2 hover:bg-bg-hover transition-colors`}
    >
      <div className="flex items-center gap-2 mb-1">
        <span className={`w-2 h-2 rounded-full ${cfg.dot}`} />
        <span className="text-xs font-medium text-text-primary truncate flex-1">{mission.title}</span>
        <span className={`text-[10px] font-medium ${PRIORITY_COLORS[mission.priority] || "text-text-muted"}`}>
          {mission.priority}
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
        <span className="ml-auto">{relativeTime(mission.updatedAt)}</span>
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

function AttentionQueue({ attentionMissions }: { attentionMissions: { mission: Mission; status: MissionStatus }[] }) {
  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-2 mb-2">
        <AlertTriangle size={12} className="text-accent-amber" />
        <span className="text-xs font-semibold text-text-primary">Attention Queue</span>
        <span className="text-[10px] text-text-muted">({attentionMissions.length})</span>
      </div>
      {attentionMissions.length === 0 ? (
        <div className="text-[11px] text-text-muted px-2 py-3 text-center bg-bg-elevated rounded">
          ✓ All clear — no missions need attention
        </div>
      ) : (
        <div className="flex flex-col gap-1.5">
          {attentionMissions.map(({ mission, status }) => (
            <AttentionCard key={mission.id} mission={mission} status={status} />
          ))}
        </div>
      )}
    </div>
  );
}

// ── ActiveMissionsSection ────────────────────────────────────────────

function ActiveMissionsSection({ activeMissions }: { activeMissions: { mission: Mission; status: MissionStatus }[] }) {
  const issues = useIssueStore((s) => s.issues);
  if (activeMissions.length === 0) return null;

  return (
    <div className="px-4 py-3 border-t border-bg-border">
      <div className="flex items-center gap-2 mb-2">
        <Target size={12} className="text-accent-blue" />
        <span className="text-xs font-semibold text-text-primary">Active Missions</span>
        <span className="text-[10px] text-text-muted">({activeMissions.length})</span>
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
        {activeMissions.map(({ mission }) => {
          const linkedIssues = issues.filter((i) => mission.issueIds.includes(i.id));
          const doneCount = linkedIssues.filter((i) => i.status === "done").length;
          return (
            <button
              key={mission.id}
              onClick={() => handleMissionClick(mission.id)}
              className="text-left bg-bg-elevated rounded px-3 py-2 hover:bg-bg-hover transition-colors border border-bg-border"
            >
              <div className="flex items-center gap-2 mb-1">
                <span className="w-1.5 h-1.5 rounded-full bg-accent-blue" />
                <span className="text-xs font-medium text-text-primary truncate flex-1">{mission.title}</span>
              </div>
              <div className="flex items-center gap-3 text-[10px] text-text-muted">
                <span className={PRIORITY_COLORS[mission.priority] || "text-text-muted"}>{mission.priority}</span>
                <span>{doneCount}/{linkedIssues.length} issues done</span>
                <span>{mission.linkedSessionIds.length} session{mission.linkedSessionIds.length !== 1 ? "s" : ""}</span>
                <span className="ml-auto">{relativeTime(mission.updatedAt)}</span>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ── MissionRow ───────────────────────────────────────────────────────

function MissionRow({ mission, status }: { mission: Mission; status: MissionStatus }) {
  const cfg = STATUS_CONFIG[status];
  return (
    <button
      onClick={() => handleMissionClick(mission.id)}
      className="flex items-center gap-3 w-full text-left px-3 py-1.5 hover:bg-bg-hover transition-colors rounded text-[11px]"
    >
      <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${cfg.dot}`} />
      <span className="text-text-primary truncate flex-1 min-w-0">{mission.title}</span>
      <span className={`flex-shrink-0 ${PRIORITY_COLORS[mission.priority] || "text-text-muted"}`}>{mission.priority}</span>
      <span className="flex-shrink-0 text-text-muted w-16 text-right">{mission.issueIds.length} issue{mission.issueIds.length !== 1 ? "s" : ""}</span>
      <span className="flex-shrink-0 text-text-muted w-16 text-right">{mission.linkedSessionIds.length} sess</span>
      <span className="flex-shrink-0 text-text-muted w-14 text-right">{relativeTime(mission.updatedAt)}</span>
    </button>
  );
}

// ── MissionGroupSection ──────────────────────────────────────────────

function MissionGroupSection({ statusKey, missions }: { statusKey: MissionStatus; missions: Mission[] }) {
  const [open, setOpen] = useState(true);
  const cfg = STATUS_CONFIG[statusKey];

  if (missions.length === 0) return null;

  return (
    <div className="mb-1">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 w-full px-4 py-1.5 text-left hover:bg-bg-hover transition-colors"
      >
        {open ? <ChevronDown size={10} className="text-text-muted" /> : <ChevronRight size={10} className="text-text-muted" />}
        <span className={`w-1.5 h-1.5 rounded-full ${cfg.dot}`} />
        <span className={`text-[11px] font-medium ${cfg.text}`}>{cfg.label}</span>
        <span className="text-[10px] text-text-muted">({missions.length})</span>
      </button>
      {open && (
        <div className="px-2">
          {missions.map((m) => (
            <MissionRow key={m.id} mission={m} status={statusKey} />
          ))}
        </div>
      )}
    </div>
  );
}

// ── MissionControlView (main) ────────────────────────────────────────

export function MissionControlView() {
  const missions = useMissionStore((s) => s.missions);
  const computeMissionStatus = useMissionStore((s) => s.computeMissionStatus);

  // Compute status for every mission
  const missionsByStatus = useMemo(() => {
    const map: Record<MissionStatus, { mission: Mission; status: MissionStatus }[]> = {
      draft: [],
      active: [],
      blocked: [],
      needs_human: [],
      done: [],
      failed: [],
    };
    for (const m of missions) {
      const status = computeMissionStatus(m.id);
      map[status].push({ mission: m, status });
    }
    return map;
  }, [missions, computeMissionStatus]);

  const statusCounts = useMemo(() => {
    const counts: Record<MissionStatus, number> = { draft: 0, active: 0, blocked: 0, needs_human: 0, done: 0, failed: 0 };
    for (const key of Object.keys(counts) as MissionStatus[]) {
      counts[key] = missionsByStatus[key].length;
    }
    return counts;
  }, [missionsByStatus]);

  const attentionMissions = useMemo(
    () => [...missionsByStatus.blocked, ...missionsByStatus.needs_human],
    [missionsByStatus]
  );

  const activeMissions = missionsByStatus.active;

  // Groups for "All Missions" section — exclude active, blocked, needs_human (already shown above)
  const remainingGroups: MissionStatus[] = ["done", "draft", "failed"];

  if (missions.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center flex-1 gap-3 text-text-muted">
        <Radio size={32} />
        <span className="text-sm font-medium">No missions to supervise</span>
        <span className="text-xs">Create missions from the Missions view to see them here.</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <StatusStrip statusCounts={statusCounts} total={missions.length} />

      <div className="flex-1 overflow-y-auto">
        <AttentionQueue attentionMissions={attentionMissions} />
        <ActiveMissionsSection activeMissions={activeMissions} />

        {/* All Missions — remaining groups */}
        <div className="px-4 py-3 border-t border-bg-border">
          <span className="text-xs font-semibold text-text-primary mb-2 block">All Missions</span>
          {remainingGroups.map((statusKey) => (
            <MissionGroupSection
              key={statusKey}
              statusKey={statusKey}
              missions={missionsByStatus[statusKey].map((e) => e.mission)}
            />
          ))}
          {remainingGroups.every((s) => missionsByStatus[s].length === 0) && (
            <div className="text-[11px] text-text-muted px-2 py-2">No other missions.</div>
          )}
        </div>
      </div>
    </div>
  );
}

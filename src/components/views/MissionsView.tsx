import { useState, useMemo, useEffect } from "react";
import { Target, Plus, Search, Trash2, X, ChevronDown, Play } from "lucide-react";
import { useMissionStore } from "@/stores/missionStore";
import { useIssueStore } from "@/stores/issueStore";
import { useAppStore } from "@/stores/appStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { relativeTime } from "@/lib/time";
import { MISSION_STATUS_CONFIG, MISSION_PRIORITY_COLORS, ISSUE_STATUS_COLORS, ISSUE_STATUS_LABELS } from "@/lib/mission-colors";
import type { Mission, MissionStatus, MissionPriority } from "@/types/mission";
import type { IssueStatus } from "@/stores/issueStore";

const ALL_STATUSES: MissionStatus[] = ["draft", "active", "blocked", "needs_human", "done", "failed"];
const ALL_PRIORITIES: MissionPriority[] = ["low", "medium", "high", "critical"];

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function MissionsView() {
  const missions = useMissionStore((s) => s.missions);
  const activeMissionId = useMissionStore((s) => s.activeMissionId);
  const [selectedId, setSelectedId] = useState<string | null>(activeMissionId);
  const [search, setSearch] = useState("");

  useEffect(() => {
    if (activeMissionId) {
      setSelectedId(activeMissionId);
    }
  }, [activeMissionId]);
  const [statusFilter, setStatusFilter] = useState<MissionStatus | "all">("all");
  const [showCreate, setShowCreate] = useState(false);

  const filtered = useMemo(() => {
    let list = missions;
    if (statusFilter !== "all") {
      list = list.filter((m) => m.status === statusFilter);
    }
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(
        (m) =>
          m.title.toLowerCase().includes(q) ||
          m.objective.toLowerCase().includes(q)
      );
    }
    return list.sort((a, b) => b.updatedAt - a.updatedAt);
  }, [missions, search, statusFilter]);

  const selected = useMemo(
    () => missions.find((m) => m.id === selectedId) ?? null,
    [missions, selectedId]
  );

  function handleCreated(id: string) {
    setSelectedId(id);
    setShowCreate(false);
  }

  return (
    <div className="flex flex-1 h-full overflow-hidden bg-bg-primary">
      {/* Left panel — mission list */}
      <div className="flex flex-col w-[280px] min-w-[280px] border-r border-bg-border bg-bg-secondary">
        {/* Header */}
        <div className="flex items-center justify-between px-3 pt-3 pb-2">
          <div className="flex items-center gap-2">
            <Target size={16} className="text-accent-green" />
            <h2 className="text-sm font-semibold text-text-primary">Missions</h2>
            <span className="text-[10px] text-text-muted bg-bg-elevated px-1.5 py-0.5 rounded-full">
              {missions.length}
            </span>
          </div>
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="flex items-center gap-1 px-2 py-1 text-[11px] text-accent-green hover:bg-accent-green/10 rounded transition-colors"
          >
            <Plus size={12} />
            New Mission
          </button>
        </div>

        {/* Create form */}
        {showCreate && (
          <CreateMissionForm
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
              placeholder="Search missions…"
              className="flex-1 bg-transparent text-xs text-text-primary placeholder:text-text-muted outline-none"
            />
          </div>
          <StatusFilterDropdown value={statusFilter} onChange={setStatusFilter} />
        </div>

        {/* Mission list */}
        <div className="flex-1 overflow-y-auto px-2 pb-2 space-y-1">
          {filtered.length === 0 && (
            <p className="text-[11px] text-text-muted px-2 py-4 text-center">
              No missions match your filters. Try clearing the search or status filter.
            </p>
          )}
          {filtered.map((m) => (
            <MissionCard
              key={m.id}
              mission={m}
              isSelected={m.id === selectedId}
              onClick={() => setSelectedId(m.id)}
            />
          ))}
        </div>
      </div>

      {/* Right panel — detail or empty state */}
      <div className="flex flex-col flex-1 overflow-hidden">
        {selected ? (
          <MissionDetail
            mission={selected}
            onDeleted={() => setSelectedId(null)}
          />
        ) : (
          <EmptyState hasMissions={missions.length > 0} />
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
  value: MissionStatus | "all";
  onChange: (v: MissionStatus | "all") => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 px-2 py-1 text-[11px] text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors w-full"
      >
        <ChevronDown size={10} className="text-text-muted" />
        <span>{value === "all" ? "All statuses" : MISSION_STATUS_CONFIG[value].label}</span>
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
              {MISSION_STATUS_CONFIG[s].label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Mission card (left panel list item)
// ---------------------------------------------------------------------------

function MissionCard({
  mission,
  isSelected,
  onClick,
}: {
  mission: Mission;
  isSelected: boolean;
  onClick: () => void;
}) {
  const sc = MISSION_STATUS_CONFIG[mission.status];
  const pc = MISSION_PRIORITY_COLORS[mission.priority];

  return (
    <button
      onClick={onClick}
      className={`w-full text-left px-2.5 py-2 rounded transition-colors border ${
        isSelected
          ? "border-accent-green/50 bg-bg-hover"
          : "border-transparent hover:bg-bg-hover"
      }`}
    >
      <p className="text-xs text-text-primary truncate">{mission.title || "Untitled"}</p>
      <div className="flex items-center gap-2 mt-1">
        <span className={`inline-flex items-center gap-1 text-[10px] ${sc.text}`}>
          <span className={`w-1.5 h-1.5 rounded-full ${sc.dot}`} />
          {MISSION_STATUS_CONFIG[mission.status].label}
        </span>
        <span className={`text-[10px] ${pc}`}>{mission.priority}</span>
        <span className="text-[10px] text-text-muted">{mission.issueIds.length} issues</span>
      </div>
      <p className="text-[10px] text-text-muted mt-0.5">{relativeTime(mission.updatedAt)}</p>
    </button>
  );
}

// ---------------------------------------------------------------------------
// Create mission form
// ---------------------------------------------------------------------------

function CreateMissionForm({
  onCreated,
  onCancel,
}: {
  onCreated: (id: string) => void;
  onCancel: () => void;
}) {
  const addMission = useMissionStore((s) => s.addMission);
  const [title, setTitle] = useState("");
  const [objective, setObjective] = useState("");
  const [priority, setPriority] = useState<MissionPriority>("medium");

  function handleCreate() {
    if (!title.trim()) return;
    const m = addMission({
      title: title.trim(),
      objective: objective.trim(),
      priority,
      status: "draft",
      issueIds: [],
      linkedSessionIds: [],
    });
    onCreated(m.id);
  }

  return (
    <div className="mx-3 mb-2 p-2 bg-bg-elevated rounded border border-bg-border space-y-1.5">
      <input
        type="text"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Mission title"
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
          onChange={(e) => setPriority(e.target.value as MissionPriority)}
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
          Create Mission
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Mission detail (right panel)
// ---------------------------------------------------------------------------

function MissionDetail({
  mission,
  onDeleted,
}: {
  mission: Mission;
  onDeleted: () => void;
}) {
  const deleteMission = useMissionStore((s) => s.deleteMission);
  const updateMission = useMissionStore((s) => s.updateMission);
  const computeMissionStatus = useMissionStore((s) => s.computeMissionStatus);
  const linkSessionToMission = useMissionStore((s) => s.linkSessionToMission);
  const issues = useIssueStore((s) => s.issues);

  const linkedIssues = useMemo(
    () => issues.filter((i) => mission.issueIds.includes(i.id)),
    [issues, mission.issueIds]
  );

  const computedStatus = computeMissionStatus(mission.id);
  const csc = MISSION_STATUS_CONFIG[computedStatus];

  // Issue rollup counts
  const rollup = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const i of linkedIssues) {
      counts[i.status] = (counts[i.status] || 0) + 1;
    }
    return counts;
  }, [linkedIssues]);

  function handleLaunchSession(cli: "claude" | "codex") {
    // Build context-rich prompt from mission + issues
    const lines: string[] = [];
    lines.push(`Work on this mission:`);
    lines.push(``);
    lines.push(`## ${mission.title}`);
    if (mission.objective) {
      lines.push(``);
      lines.push(mission.objective);
    }
    lines.push(``);
    lines.push(`Priority: ${mission.priority}`);

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

    // Link this session to the mission after a short delay
    // (the pane needs time to create the PTY session)
    setTimeout(() => {
      // Find the pane to get its sessionId
      const pane = useLayoutStore.getState().panes.find((p) => p.id === paneId);
      if (pane?.sessionId) {
        linkSessionToMission(mission.id, pane.sessionId);
      }
    }, 2000);

    // Inject the prompt via the same custom event pattern used by "Work on this issue"
    setTimeout(() => {
      window.dispatchEvent(
        new CustomEvent("packetcode:issue-prompt", { detail: { prompt } })
      );
    }, 1500);
  }

  function handleDelete() {
    if (!window.confirm(`Delete mission "${mission.title}"? This cannot be undone.`)) return;
    deleteMission(mission.id);
    onDeleted();
  }

  return (
    <div className="flex flex-col flex-1 overflow-y-auto">
      {/* Header — editable */}
      <div className="px-4 pt-4 pb-3 border-b border-bg-border space-y-2">
        <input
          type="text"
          value={mission.title}
          onChange={(e) => updateMission(mission.id, { title: e.target.value })}
          className="w-full text-sm font-semibold text-text-primary bg-transparent border-b border-transparent hover:border-bg-border focus:border-accent-green focus:outline-none truncate"
          placeholder="Mission title"
        />
        <textarea
          value={mission.objective}
          onChange={(e) => updateMission(mission.id, { objective: e.target.value })}
          rows={2}
          className="w-full text-xs text-text-secondary bg-transparent border border-transparent hover:border-bg-border focus:border-accent-green focus:outline-none resize-none rounded px-1 py-0.5"
          placeholder="Objective (optional)"
        />
        <div className="flex items-center gap-2">
          <select
            value={mission.status}
            onChange={(e) => updateMission(mission.id, { status: e.target.value as MissionStatus })}
            className="bg-bg-primary text-[11px] text-text-secondary border border-bg-border rounded px-1.5 py-0.5 outline-none focus:border-accent-green"
          >
            {ALL_STATUSES.map((s) => (
              <option key={s} value={s}>{MISSION_STATUS_CONFIG[s].label}</option>
            ))}
          </select>
          <select
            value={mission.priority}
            onChange={(e) => updateMission(mission.id, { priority: e.target.value as MissionPriority })}
            className="bg-bg-primary text-[11px] text-text-secondary border border-bg-border rounded px-1.5 py-0.5 outline-none focus:border-accent-green"
          >
            {ALL_PRIORITIES.map((p) => (
              <option key={p} value={p}>{p.charAt(0).toUpperCase() + p.slice(1)}</option>
            ))}
          </select>
          {linkedIssues.length > 0 && (
            <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] ${csc.bg} ${csc.text}`}>
              <span className={`w-1.5 h-1.5 rounded-full ${csc.dot}`} />
              Rollup: {MISSION_STATUS_CONFIG[computedStatus].label}
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
              {MISSION_STATUS_CONFIG[computedStatus].label}
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
          <span className="text-[10px] text-text-muted ml-1">with mission context</span>
        </div>
      </div>

      {/* Linked Issues */}
      <div className="px-4 py-3 border-b border-bg-border">
        <div className="flex items-center justify-between mb-2">
          <span className="text-[11px] font-medium text-text-secondary">
            Linked Issues ({linkedIssues.length})
          </span>
          <AssignIssueButton missionId={mission.id} />
        </div>
        {linkedIssues.length === 0 ? (
          <p className="text-[10px] text-text-muted py-2">No issues linked yet.</p>
        ) : (
          <div className="space-y-0.5">
            {linkedIssues.map((issue) => (
              <IssueRow key={issue.id} issue={issue} missionId={mission.id} />
            ))}
          </div>
        )}
      </div>

      {/* Linked Sessions */}
      <div className="px-4 py-3 border-b border-bg-border">
        <span className="text-[11px] font-medium text-text-secondary">
          Sessions ({mission.linkedSessionIds.length})
        </span>
        {mission.linkedSessionIds.length === 0 ? (
          <p className="text-[10px] text-text-muted py-2">No sessions linked yet. Launch a session above.</p>
        ) : (
          <div className="mt-1.5 space-y-0.5">
            {mission.linkedSessionIds.map((sid) => (
              <div key={sid} className="flex items-center gap-2 py-1 group">
                <span className="w-1.5 h-1.5 rounded-full bg-accent-blue shrink-0" />
                <span className="text-[10px] text-text-secondary font-mono truncate flex-1">{sid}</span>
                <button
                  onClick={() => {
                    useMissionStore.getState().unlinkSessionFromMission(mission.id, sid);
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
          Delete mission
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Issue row inside mission detail
// ---------------------------------------------------------------------------

function IssueRow({
  issue,
  missionId,
}: {
  issue: { id: string; ticketId: string; title: string; status: IssueStatus; priority: string };
  missionId: string;
}) {
  const removeIssueFromMission = useMissionStore((s) => s.removeIssueFromMission);
  const assignToMission = useIssueStore((s) => s.assignToMission);

  const statusColor = ISSUE_STATUS_COLORS[issue.status] ?? "bg-text-muted";
  const pc = MISSION_PRIORITY_COLORS[issue.priority as MissionPriority] ?? "text-text-muted";

  function handleRemove() {
    removeIssueFromMission(missionId, issue.id);
    assignToMission(issue.id, null);
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
        title="Remove from mission"
      >
        <X size={10} />
      </button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Assign issue button + dropdown
// ---------------------------------------------------------------------------

function AssignIssueButton({ missionId }: { missionId: string }) {
  const [open, setOpen] = useState(false);
  const issues = useIssueStore((s) => s.issues);
  const addIssueToMission = useMissionStore((s) => s.addIssueToMission);
  const assignToMission = useIssueStore((s) => s.assignToMission);

  const unassigned = useMemo(
    () => issues.filter((i) => i.missionId == null),
    [issues]
  );

  function handleAssign(issueId: string) {
    addIssueToMission(missionId, issueId);
    assignToMission(issueId, missionId);
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

function EmptyState({ hasMissions }: { hasMissions: boolean }) {
  return (
    <div className="flex flex-col items-center justify-center flex-1 gap-3">
      <Target size={32} className="text-text-muted" />
      <p className="text-xs text-text-muted text-center max-w-[240px]">
        {hasMissions
          ? "Select a mission or create one."
          : "No missions yet. Create your first mission to organize your work."}
      </p>
    </div>
  );
}

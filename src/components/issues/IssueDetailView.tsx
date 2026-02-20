import { useState } from "react";
import {
  X,
  ChevronDown,
  ChevronRight,
  Plus,
  Trash2,
  Play,
} from "lucide-react";
import {
  useIssueStore,
  type Issue,
  type IssueStatus,
} from "@/stores/issueStore";
import { useAppStore } from "@/stores/appStore";
import { useLayoutStore } from "@/stores/layoutStore";

interface IssueDetailViewProps {
  issueId: string;
  onClose: () => void;
}

const STATUS_BUTTONS: { key: IssueStatus; label: string }[] = [
  { key: "todo", label: "To Do" },
  { key: "in_progress", label: "In Progress" },
  { key: "qa", label: "QA" },
  { key: "done", label: "Done" },
  { key: "blocked", label: "Blocked" },
  { key: "needs_human", label: "Needs Human" },
];

const LABEL_COLORS: Record<string, { bg: string; text: string }> = {
  api: { bg: "bg-accent-green/20", text: "text-accent-green" },
  frontend: { bg: "bg-accent-amber/20", text: "text-accent-amber" },
  working: { bg: "bg-accent-green/20", text: "text-accent-green" },
  bug: { bg: "bg-accent-red/20", text: "text-accent-red" },
  feature: { bg: "bg-accent-blue/20", text: "text-accent-blue" },
  enhancement: { bg: "bg-accent-blue/20", text: "text-accent-blue" },
  refactor: { bg: "bg-accent-purple/20", text: "text-accent-purple" },
  docs: { bg: "bg-text-muted/20", text: "text-text-secondary" },
  devops: { bg: "bg-accent-amber/20", text: "text-accent-amber" },
  mvp: { bg: "bg-accent-green/20", text: "text-accent-green" },
};

function getLabelColor(label: string): { bg: string; text: string } {
  return LABEL_COLORS[label.toLowerCase()] || { bg: "bg-bg-elevated", text: "text-text-muted" };
}

function getStatusButtonColor(status: IssueStatus, isActive: boolean): string {
  if (!isActive) return "bg-bg-primary border-bg-border text-text-muted hover:bg-bg-hover hover:text-text-secondary";
  switch (status) {
    case "todo": return "bg-text-muted/20 border-text-muted/40 text-text-primary";
    case "in_progress": return "bg-accent-blue/20 border-accent-blue/40 text-accent-blue";
    case "qa": return "bg-accent-amber/20 border-accent-amber/40 text-accent-amber";
    case "done": return "bg-accent-green/20 border-accent-green/40 text-accent-green";
    case "blocked": return "bg-accent-red/20 border-accent-red/40 text-accent-red";
    case "needs_human": return "bg-accent-purple/20 border-accent-purple/40 text-accent-purple";
    default: return "bg-bg-elevated border-bg-border text-text-primary";
  }
}

function getPriorityLabel(priority: string): { text: string; cls: string } {
  switch (priority) {
    case "critical": return { text: "Critical", cls: "text-accent-red" };
    case "high": return { text: "High", cls: "text-accent-amber" };
    case "medium": return { text: "Medium", cls: "text-accent-blue" };
    case "low": return { text: "Low", cls: "text-text-muted" };
    default: return { text: priority, cls: "text-text-muted" };
  }
}

function formatDate(timestamp: number): string {
  const d = new Date(timestamp);
  return d.toLocaleDateString("en-US", {
    month: "numeric",
    day: "numeric",
    year: "numeric",
  }) + " at " + d.toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: true,
  });
}

export function IssueDetailView({ issueId, onClose }: IssueDetailViewProps) {
  const issues = useIssueStore((s) => s.issues);
  const moveIssue = useIssueStore((s) => s.moveIssue);
  const toggleCriterion = useIssueStore((s) => s.toggleCriterion);
  const addCriterion = useIssueStore((s) => s.addCriterion);
  const removeCriterion = useIssueStore((s) => s.removeCriterion);
  const addBlockedBy = useIssueStore((s) => s.addBlockedBy);
  const removeBlockedBy = useIssueStore((s) => s.removeBlockedBy);
  const addBlocks = useIssueStore((s) => s.addBlocks);
  const removeBlocks = useIssueStore((s) => s.removeBlocks);

  const [showDepGraph, setShowDepGraph] = useState(false);
  const [newCriterionText, setNewCriterionText] = useState("");
  const [showAddBlockedBy, setShowAddBlockedBy] = useState(false);
  const [showAddBlocks, setShowAddBlocks] = useState(false);

  const foundIssue = issues.find((i) => i.id === issueId);
  if (!foundIssue) return null;
  const issue = foundIssue;

  const priorityInfo = getPriorityLabel(issue.priority);
  const checkedCount = issue.acceptanceCriteria.filter((c) => c.checked).length;
  const totalCriteria = issue.acceptanceCriteria.length;

  // Resolve issue IDs to issue objects
  const blockedByIssues = issue.blockedBy
    .map((id) => issues.find((i) => i.id === id))
    .filter(Boolean) as Issue[];
  const blocksIssues = issue.blocks
    .map((id) => issues.find((i) => i.id === id))
    .filter(Boolean) as Issue[];

  // Issues available to add as dependencies (exclude self and already-linked)
  const availableForBlockedBy = issues.filter(
    (i) => i.id !== issue.id && !issue.blockedBy.includes(i.id)
  );
  const availableForBlocks = issues.filter(
    (i) => i.id !== issue.id && !issue.blocks.includes(i.id)
  );

  function handleAddCriterion() {
    if (!newCriterionText.trim()) return;
    addCriterion(issueId, newCriterionText.trim());
    setNewCriterionText("");
  }

  function handleWorkOnIssue() {
    // Build the prompt from the issue
    const lines: string[] = [];
    lines.push(`Work on this issue:`);
    lines.push(``);
    lines.push(`## ${issue.ticketId}: ${issue.title}`);
    if (issue.description) {
      lines.push(``);
      lines.push(issue.description);
    }
    if (issue.acceptanceCriteria.length > 0) {
      lines.push(``);
      lines.push(`### Acceptance Criteria`);
      for (const c of issue.acceptanceCriteria) {
        lines.push(`- [${c.checked ? "x" : " "}] ${c.text}`);
      }
    }
    if (issue.labels.length > 0) {
      lines.push(``);
      lines.push(`Labels: ${issue.labels.join(", ")}`);
    }
    if (issue.priority) {
      lines.push(`Priority: ${issue.priority}`);
    }

    const prompt = lines.join("\n");

    // Switch to Claude view
    useAppStore.getState().setActiveView("claude");

    // Add a new pane (which auto-starts a session)
    useLayoutStore.getState().addPane();

    // Dispatch event with the prompt data — TerminalPane will pick this up
    // We need a small delay to let the session start, then write the prompt
    setTimeout(() => {
      window.dispatchEvent(
        new CustomEvent("packetcode:issue-prompt", { detail: { prompt, issueId: issue.id } })
      );
    }, 1500);

    onClose();
  }

  // Build dependency graph text representation
  function buildDepGraph(): string[] {
    const lines: string[] = [];
    lines.push(`${issue.ticketId}`);
    if (blockedByIssues.length > 0) {
      for (const b of blockedByIssues) {
        const marker = b.status === "done" ? "[done]" : "[pending]";
        lines.push(`  <- ${b.ticketId} ${marker}`);
      }
    }
    if (blocksIssues.length > 0) {
      for (const b of blocksIssues) {
        const marker = b.status === "done" ? "[done]" : "[pending]";
        lines.push(`  -> ${b.ticketId} ${marker}`);
      }
    }
    return lines;
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
      <div className="bg-bg-secondary border border-bg-border rounded-lg w-[560px] max-h-[85vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-start justify-between px-5 pt-4 pb-3 border-b border-bg-border">
          <div className="flex-1 min-w-0">
            {/* Title */}
            <h2 className={`text-sm font-semibold text-text-primary leading-snug ${issue.status === "done" ? "line-through opacity-60" : ""}`}>
              {issue.ticketId}: {issue.title}
            </h2>

            {/* Meta row: priority, date, labels */}
            <div className="flex items-center gap-2 mt-2 flex-wrap">
              <span className={`text-[11px] font-medium ${priorityInfo.cls}`}>
                {priorityInfo.text}
              </span>
              <span className="text-[10px] text-text-muted">
                {formatDate(issue.createdAt)}
              </span>
              {issue.labels.map((label) => {
                const color = getLabelColor(label);
                return (
                  <span
                    key={label}
                    className={`text-[9px] px-1.5 py-0.5 rounded font-medium ${color.bg} ${color.text}`}
                  >
                    {label}
                  </span>
                );
              })}
              {issue.epic && (
                <span className="text-[9px] px-1.5 py-0.5 bg-accent-purple/15 text-accent-purple rounded font-medium">
                  {issue.epic}
                </span>
              )}
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-text-muted hover:text-text-primary transition-colors flex-shrink-0 ml-3"
          >
            <X size={16} />
          </button>
        </div>

        <div className="px-5 py-4 flex flex-col gap-4">
          {/* Description */}
          {issue.description && (
            <p className="text-xs text-text-secondary leading-relaxed">
              {issue.description}
            </p>
          )}

          {/* Status buttons row */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1.5 uppercase tracking-wider">
              Status
            </label>
            <div className="flex flex-wrap gap-1.5">
              {STATUS_BUTTONS.map((s) => (
                <button
                  key={s.key}
                  onClick={() => moveIssue(issueId, s.key)}
                  className={`px-2.5 py-1 text-[11px] rounded border transition-colors ${getStatusButtonColor(s.key, issue.status === s.key)}`}
                >
                  {s.label}
                </button>
              ))}
            </div>
          </div>

          {/* Work on this issue button */}
          <button
            onClick={handleWorkOnIssue}
            className="flex items-center justify-center gap-2 py-2.5 bg-accent-green/15 border border-accent-green/30 rounded-lg text-accent-green text-xs font-medium hover:bg-accent-green/25 transition-colors"
          >
            <Play size={14} />
            Work on this issue
          </button>

          {/* Blocked By */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-[10px] text-text-muted uppercase tracking-wider">
                Blocked By ({blockedByIssues.length} issue{blockedByIssues.length !== 1 ? "s" : ""})
              </label>
              <button
                onClick={() => setShowAddBlockedBy(!showAddBlockedBy)}
                className="p-0.5 text-text-muted hover:text-accent-green transition-colors"
              >
                <Plus size={11} />
              </button>
            </div>
            {blockedByIssues.length > 0 ? (
              <div className="flex flex-col gap-1">
                {blockedByIssues.map((b) => (
                  <div
                    key={b.id}
                    className="flex items-center gap-2 group"
                  >
                    <span
                      className={`text-[11px] ${b.status === "done" ? "line-through text-text-muted" : "text-text-secondary"}`}
                    >
                      {b.ticketId}: {b.title}
                    </span>
                    {b.status === "done" && (
                      <span className="text-[9px] text-accent-green">done</span>
                    )}
                    <button
                      onClick={() => removeBlockedBy(issueId, b.id)}
                      className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity ml-auto"
                    >
                      <X size={10} />
                    </button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-[10px] text-text-muted">No blockers</p>
            )}
            {showAddBlockedBy && availableForBlockedBy.length > 0 && (
              <select
                className="mt-1.5 w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
                defaultValue=""
                onChange={(e) => {
                  if (e.target.value) {
                    addBlockedBy(issueId, e.target.value);
                    setShowAddBlockedBy(false);
                  }
                }}
              >
                <option value="" disabled>Select blocking issue...</option>
                {availableForBlockedBy.map((i) => (
                  <option key={i.id} value={i.id}>{i.ticketId}: {i.title}</option>
                ))}
              </select>
            )}
          </div>

          {/* Blocks */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-[10px] text-text-muted uppercase tracking-wider">
                Blocks ({blocksIssues.length} issue{blocksIssues.length !== 1 ? "s" : ""})
              </label>
              <button
                onClick={() => setShowAddBlocks(!showAddBlocks)}
                className="p-0.5 text-text-muted hover:text-accent-green transition-colors"
              >
                <Plus size={11} />
              </button>
            </div>
            {blocksIssues.length > 0 ? (
              <div className="flex flex-col gap-1">
                {blocksIssues.map((b) => (
                  <div
                    key={b.id}
                    className="flex items-center gap-2 group"
                  >
                    <span
                      className={`text-[11px] ${b.status === "done" ? "line-through text-text-muted" : "text-text-secondary"}`}
                    >
                      {b.ticketId}: {b.title}
                    </span>
                    {b.status === "done" && (
                      <span className="text-[9px] text-accent-green">done</span>
                    )}
                    <button
                      onClick={() => removeBlocks(issueId, b.id)}
                      className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity ml-auto"
                    >
                      <X size={10} />
                    </button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-[10px] text-text-muted">No downstream issues</p>
            )}
            {showAddBlocks && availableForBlocks.length > 0 && (
              <select
                className="mt-1.5 w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
                defaultValue=""
                onChange={(e) => {
                  if (e.target.value) {
                    addBlocks(issueId, e.target.value);
                    setShowAddBlocks(false);
                  }
                }}
              >
                <option value="" disabled>Select issue to block...</option>
                {availableForBlocks.map((i) => (
                  <option key={i.id} value={i.id}>{i.ticketId}: {i.title}</option>
                ))}
              </select>
            )}
          </div>

          {/* Dependency Graph (collapsible) */}
          {(blockedByIssues.length > 0 || blocksIssues.length > 0) && (
            <div>
              <button
                onClick={() => setShowDepGraph(!showDepGraph)}
                className="flex items-center gap-1 text-[10px] text-text-muted uppercase tracking-wider hover:text-text-secondary transition-colors"
              >
                {showDepGraph ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
                Dependency Graph
              </button>
              {showDepGraph && (
                <div className="mt-1.5 p-2 bg-bg-primary rounded border border-bg-border">
                  <pre className="text-[10px] text-text-secondary font-mono leading-relaxed">
                    {buildDepGraph().join("\n")}
                  </pre>
                </div>
              )}
            </div>
          )}

          {/* Acceptance Criteria */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1.5 uppercase tracking-wider">
              Acceptance Criteria ({checkedCount}/{totalCriteria} complete)
            </label>
            {issue.acceptanceCriteria.length > 0 ? (
              <div className="flex flex-col gap-1.5">
                {issue.acceptanceCriteria.map((criterion) => (
                  <div key={criterion.id} className="flex items-start gap-2 group">
                    <input
                      type="checkbox"
                      checked={criterion.checked}
                      onChange={() => toggleCriterion(issueId, criterion.id)}
                      className="mt-0.5 accent-[#00ff41] cursor-pointer"
                    />
                    <span
                      className={`text-[11px] leading-snug flex-1 ${
                        criterion.checked ? "line-through text-text-muted" : "text-text-secondary"
                      }`}
                    >
                      {criterion.text}
                    </span>
                    <button
                      onClick={() => removeCriterion(issueId, criterion.id)}
                      className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                    >
                      <Trash2 size={10} />
                    </button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-[10px] text-text-muted">No acceptance criteria defined</p>
            )}

            {/* Add criterion input */}
            <div className="flex items-center gap-1.5 mt-2">
              <input
                type="text"
                value={newCriterionText}
                onChange={(e) => setNewCriterionText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    handleAddCriterion();
                  }
                }}
                placeholder="Add criterion..."
                className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
              />
              <button
                onClick={handleAddCriterion}
                disabled={!newCriterionText.trim()}
                className="p-1 text-text-muted hover:text-accent-green transition-colors disabled:opacity-30"
              >
                <Plus size={14} />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

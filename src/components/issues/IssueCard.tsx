import { Trash2, Link, CheckSquare } from "lucide-react";
import { useIssueStore, type Issue } from "@/stores/issueStore";
import { getLabelColor } from "@/lib/colors";

interface IssueCardProps {
  issue: Issue;
  onDragStart: (e: React.DragEvent) => void;
  onClick: () => void;
  isDragging?: boolean;
}

export function IssueCard({ issue, onDragStart, onClick, isDragging }: IssueCardProps) {
  const deleteIssue = useIssueStore((s) => s.deleteIssue);

  const timeAgo = getTimeAgo(issue.updatedAt);
  const checkedCount = issue.acceptanceCriteria.filter((c) => c.checked).length;
  const totalCriteria = issue.acceptanceCriteria.length;
  const depCount = issue.blockedBy.length + issue.blocks.length;

  return (
    <div
      draggable
      onDragStart={onDragStart}
      onClick={onClick}
      className={`group bg-bg-primary border border-bg-border rounded-md p-2.5 cursor-pointer hover:border-text-muted/30 transition-all ${
        isDragging ? "opacity-50 scale-[0.97] shadow-lg ring-1 ring-accent-green/30" : ""
      }`}
    >
      {/* Header row: ticket ID + priority */}
      <div className="flex items-center justify-between gap-1 mb-1">
        <div className="flex items-center gap-1.5">
          <span className="text-[10px] text-text-muted font-mono">
            {issue.ticketId}:
          </span>
          <PriorityBadge priority={issue.priority} />
        </div>
        <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0">
          <button
            onClick={(e) => {
              e.stopPropagation();
              deleteIssue(issue.id);
            }}
            className="p-0.5 text-text-muted hover:text-accent-red transition-colors"
            title="Delete"
          >
            <Trash2 size={10} />
          </button>
        </div>
      </div>

      {/* Title */}
      <p className={`text-xs text-text-primary leading-snug mb-1 ${issue.status === "done" ? "line-through opacity-60" : ""}`}>
        {issue.title}
      </p>

      {/* Description preview */}
      {issue.description && (
        <p className="text-[10px] text-text-muted leading-relaxed truncate mb-1.5">
          {issue.description}
        </p>
      )}

      {/* Labels (colored badges) */}
      {(issue.labels.length > 0 || issue.epic) && (
        <div className="flex flex-wrap gap-1 mt-1.5">
          {issue.epic && (
            <span className="text-[9px] px-1.5 py-0.5 bg-accent-purple/15 text-accent-purple rounded font-medium">
              {issue.epic}
            </span>
          )}
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
        </div>
      )}

      {/* Footer: criteria progress + deps + session link + time ago */}
      <div className="flex items-center justify-between mt-1.5 gap-2">
        <div className="flex items-center gap-2">
          {issue.sessionId && (
            <span className="flex items-center gap-0.5 text-[9px] text-accent-blue">
              <Link size={8} />
              linked
            </span>
          )}
          {totalCriteria > 0 && (
            <span className="flex items-center gap-0.5 text-[9px] text-text-muted">
              <CheckSquare size={8} />
              {checkedCount}/{totalCriteria}
            </span>
          )}
          {depCount > 0 && (
            <span className="text-[9px] text-text-muted">
              {depCount} dep{depCount !== 1 ? "s" : ""}
            </span>
          )}
        </div>
        <span className="text-[9px] text-text-muted">{timeAgo}</span>
      </div>
    </div>
  );
}

function PriorityBadge({ priority }: { priority: Issue["priority"] }) {
  const config: Record<string, { letter: string; class: string }> = {
    critical: { letter: "C", class: "bg-accent-red/20 text-accent-red" },
    high: { letter: "H", class: "bg-accent-amber/20 text-accent-amber" },
    medium: { letter: "M", class: "bg-accent-blue/20 text-accent-blue" },
    low: { letter: "L", class: "bg-bg-elevated text-text-muted" },
  };

  const { letter, class: cls } = config[priority];

  return (
    <span className={`text-[9px] w-4 h-4 flex items-center justify-center rounded font-bold ${cls}`}>
      {letter}
    </span>
  );
}

function getTimeAgo(timestamp: number): string {
  const now = Date.now();
  const diff = now - timestamp;
  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (seconds < 60) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  return `${days}d ago`;
}

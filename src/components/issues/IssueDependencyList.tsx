import { useState } from "react";
import { X, Plus } from "lucide-react";
import type { Issue } from "@/stores/issueStore";

interface IssueDependencyListProps {
  label: string;
  emptyText: string;
  selectPlaceholder: string;
  linkedIssues: Issue[];
  availableIssues: Issue[];
  onAdd: (targetId: string) => void;
  onRemove: (targetId: string) => void;
}

export function IssueDependencyList({
  label,
  emptyText,
  selectPlaceholder,
  linkedIssues,
  availableIssues,
  onAdd,
  onRemove,
}: IssueDependencyListProps) {
  const [showAdd, setShowAdd] = useState(false);

  return (
    <div>
      <div className="flex items-center justify-between mb-1.5">
        <label className="text-[10px] text-text-muted uppercase tracking-wider">
          {label} ({linkedIssues.length} issue{linkedIssues.length !== 1 ? "s" : ""})
        </label>
        <button
          onClick={() => setShowAdd(!showAdd)}
          className="p-0.5 text-text-muted hover:text-accent-green transition-colors"
        >
          <Plus size={11} />
        </button>
      </div>
      {linkedIssues.length > 0 ? (
        <div className="flex flex-col gap-1">
          {linkedIssues.map((b) => (
            <div key={b.id} className="flex items-center gap-2 group">
              <span className={`text-[11px] ${b.status === "done" ? "line-through text-text-muted" : "text-text-secondary"}`}>
                {b.ticketId}: {b.title}
              </span>
              {b.status === "done" && (
                <span className="text-[9px] text-accent-green">done</span>
              )}
              <button
                onClick={() => onRemove(b.id)}
                className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity ml-auto"
              >
                <X size={10} />
              </button>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-[10px] text-text-muted">{emptyText}</p>
      )}
      {showAdd && availableIssues.length > 0 && (
        <select
          className="mt-1.5 w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
          defaultValue=""
          onChange={(e) => {
            if (e.target.value) {
              onAdd(e.target.value);
              setShowAdd(false);
            }
          }}
        >
          <option value="" disabled>{selectPlaceholder}</option>
          {availableIssues.map((i) => (
            <option key={i.id} value={i.id}>{i.ticketId}: {i.title}</option>
          ))}
        </select>
      )}
    </div>
  );
}

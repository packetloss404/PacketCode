import { useState } from "react";
import { X } from "lucide-react";
import {
  useIssueStore,
  type IssueStatus,
  type IssuePriority,
} from "@/stores/issueStore";

interface NewIssueFormProps {
  defaultStatus: IssueStatus;
  onClose: () => void;
}

export function NewIssueForm({ defaultStatus, onClose }: NewIssueFormProps) {
  const addIssue = useIssueStore((s) => s.addIssue);
  const labels = useIssueStore((s) => s.labels);
  const epics = useIssueStore((s) => s.epics);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<IssuePriority>("medium");
  const [status, setStatus] = useState<IssueStatus>(defaultStatus);
  const [selectedLabels, setSelectedLabels] = useState<string[]>([]);
  const [epic, setEpic] = useState<string>("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;

    addIssue({
      title: title.trim(),
      description: description.trim(),
      status,
      priority,
      labels: selectedLabels,
      epic: epic || null,
      sessionId: null,
    });

    onClose();
  }

  function toggleLabel(label: string) {
    setSelectedLabels((prev) =>
      prev.includes(label) ? prev.filter((l) => l !== label) : [...prev, label]
    );
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
      <form
        onSubmit={handleSubmit}
        className="bg-bg-secondary border border-bg-border rounded-lg w-[420px] max-h-[80vh] overflow-y-auto"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-bg-border">
          <h3 className="text-sm font-semibold text-text-primary">
            New Issue
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 text-text-muted hover:text-text-primary transition-colors"
          >
            <X size={14} />
          </button>
        </div>

        <div className="p-4 flex flex-col gap-3">
          {/* Title */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
              Title
            </label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
              placeholder="Issue title..."
              autoFocus
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green resize-none"
              placeholder="Describe the issue..."
            />
          </div>

          {/* Priority + Status row */}
          <div className="flex gap-3">
            <div className="flex-1">
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Priority
              </label>
              <select
                value={priority}
                onChange={(e) => setPriority(e.target.value as IssuePriority)}
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1.5 text-xs text-text-primary focus:outline-none focus:border-accent-green"
              >
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>
            <div className="flex-1">
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Status
              </label>
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value as IssueStatus)}
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1.5 text-xs text-text-primary focus:outline-none focus:border-accent-green"
              >
                <option value="todo">To Do</option>
                <option value="in_progress">In Progress</option>
                <option value="qa">QA</option>
                <option value="done">Done</option>
                <option value="blocked">Blocked</option>
                <option value="needs_human">Needs Human</option>
              </select>
            </div>
          </div>

          {/* Labels */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
              Labels
            </label>
            <div className="flex flex-wrap gap-1.5">
              {labels.map((label) => (
                <button
                  key={label}
                  type="button"
                  onClick={() => toggleLabel(label)}
                  className={`text-[10px] px-2 py-0.5 rounded border transition-colors ${
                    selectedLabels.includes(label)
                      ? "bg-accent-green/15 border-accent-green/40 text-accent-green"
                      : "bg-bg-primary border-bg-border text-text-muted hover:text-text-secondary"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          {/* Epic */}
          {epics.length > 0 && (
            <div>
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Epic
              </label>
              <select
                value={epic}
                onChange={(e) => setEpic(e.target.value)}
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1.5 text-xs text-text-primary focus:outline-none focus:border-accent-green"
              >
                <option value="">None</option>
                {epics.map((e) => (
                  <option key={e} value={e}>
                    {e}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 px-4 py-3 border-t border-bg-border">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!title.trim()}
            className="px-3 py-1.5 text-xs bg-accent-green/15 text-accent-green border border-accent-green/30 rounded hover:bg-accent-green/25 transition-colors disabled:opacity-30"
          >
            Create Issue
          </button>
        </div>
      </form>
    </div>
  );
}

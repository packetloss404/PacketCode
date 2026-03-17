import { useState } from "react";
import { X, Plus, Trash2 } from "lucide-react";
import {
  useIssueStore,
  type IssueStatus,
  type IssuePriority,
} from "@/stores/issueStore";
import { useMissionStore } from "@/stores/missionStore";
import { generateId } from "@/lib/storage";

interface NewIssueFormProps {
  defaultStatus: IssueStatus;
  onClose: () => void;
}

export function NewIssueForm({ defaultStatus, onClose }: NewIssueFormProps) {
  const addIssue = useIssueStore((s) => s.addIssue);
  const labels = useIssueStore((s) => s.labels);
  const epics = useIssueStore((s) => s.epics);
  const issues = useIssueStore((s) => s.issues);
  const addBlockedBy = useIssueStore((s) => s.addBlockedBy);
  const addBlocks = useIssueStore((s) => s.addBlocks);

  const missions = useMissionStore((s) => s.missions);
  const addIssueToMission = useMissionStore((s) => s.addIssueToMission);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<IssuePriority>("medium");
  const [status, setStatus] = useState<IssueStatus>(defaultStatus);
  const [selectedLabels, setSelectedLabels] = useState<string[]>([]);
  const [epic, setEpic] = useState<string>("");

  // Acceptance criteria
  const [criteria, setCriteria] = useState<string[]>([]);
  const [newCriterion, setNewCriterion] = useState("");

  // Mission
  const [missionId, setMissionId] = useState<string>("");

  // Dependencies
  const [blockedByIds, setBlockedByIds] = useState<string[]>([]);
  const [blocksIds, setBlocksIds] = useState<string[]>([]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;

    const newIssue = addIssue({
      title: title.trim(),
      description: description.trim(),
      status,
      priority,
      labels: selectedLabels,
      epic: epic || null,
      sessionId: null,
      acceptanceCriteria: criteria.map((text) => ({
        id: generateId("ac", 6),
        text,
        checked: false,
      })),
      blockedBy: [],
      blocks: [],
    });

    // Set up dependencies after creation
    for (const id of blockedByIds) {
      addBlockedBy(newIssue.id, id);
    }
    for (const id of blocksIds) {
      addBlocks(newIssue.id, id);
    }

    // Assign to mission if selected
    if (missionId) {
      addIssueToMission(missionId, newIssue.id);
      useIssueStore.getState().assignToMission(newIssue.id, missionId);
    }

    onClose();
  }

  function toggleLabel(label: string) {
    setSelectedLabels((prev) =>
      prev.includes(label) ? prev.filter((l) => l !== label) : [...prev, label]
    );
  }

  function handleAddCriterion() {
    if (!newCriterion.trim()) return;
    setCriteria((prev) => [...prev, newCriterion.trim()]);
    setNewCriterion("");
  }

  function removeCriterion(index: number) {
    setCriteria((prev) => prev.filter((_, i) => i !== index));
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
      <form
        onSubmit={handleSubmit}
        className="bg-bg-secondary border border-bg-border rounded-lg w-[480px] max-h-[85vh] overflow-y-auto"
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

          {/* Mission */}
          {missions.length > 0 && (
            <div>
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Mission
              </label>
              <select
                value={missionId}
                onChange={(e) => setMissionId(e.target.value)}
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1.5 text-xs text-text-primary focus:outline-none focus:border-accent-green"
              >
                <option value="">None</option>
                {missions.map((m) => (
                  <option key={m.id} value={m.id}>{m.title}</option>
                ))}
              </select>
            </div>
          )}

          {/* Acceptance Criteria */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
              Acceptance Criteria
            </label>
            {criteria.length > 0 && (
              <div className="flex flex-col gap-1 mb-1.5">
                {criteria.map((text, idx) => (
                  <div key={idx} className="flex items-center gap-2 group">
                    <span className="text-[11px] text-text-secondary flex-1">{text}</span>
                    <button
                      type="button"
                      onClick={() => removeCriterion(idx)}
                      className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity"
                    >
                      <Trash2 size={10} />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-center gap-1.5">
              <input
                type="text"
                value={newCriterion}
                onChange={(e) => setNewCriterion(e.target.value)}
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
                type="button"
                onClick={handleAddCriterion}
                disabled={!newCriterion.trim()}
                className="p-1 text-text-muted hover:text-accent-green transition-colors disabled:opacity-30"
              >
                <Plus size={12} />
              </button>
            </div>
          </div>

          {/* Blocked By */}
          {issues.length > 0 && (
            <div>
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Blocked By
              </label>
              {blockedByIds.length > 0 && (
                <div className="flex flex-col gap-1 mb-1.5">
                  {blockedByIds.map((id) => {
                    const dep = issues.find((i) => i.id === id);
                    if (!dep) return null;
                    return (
                      <div key={id} className="flex items-center gap-2 group">
                        <span className="text-[11px] text-text-secondary">{dep.ticketId}: {dep.title}</span>
                        <button
                          type="button"
                          onClick={() => setBlockedByIds((prev) => prev.filter((x) => x !== id))}
                          className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity ml-auto"
                        >
                          <X size={10} />
                        </button>
                      </div>
                    );
                  })}
                </div>
              )}
              <select
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
                value=""
                onChange={(e) => {
                  if (e.target.value && !blockedByIds.includes(e.target.value)) {
                    setBlockedByIds((prev) => [...prev, e.target.value]);
                  }
                }}
              >
                <option value="">Add blocking issue...</option>
                {issues
                  .filter((i) => !blockedByIds.includes(i.id))
                  .map((i) => (
                    <option key={i.id} value={i.id}>{i.ticketId}: {i.title}</option>
                  ))}
              </select>
            </div>
          )}

          {/* Blocks */}
          {issues.length > 0 && (
            <div>
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Blocks
              </label>
              {blocksIds.length > 0 && (
                <div className="flex flex-col gap-1 mb-1.5">
                  {blocksIds.map((id) => {
                    const dep = issues.find((i) => i.id === id);
                    if (!dep) return null;
                    return (
                      <div key={id} className="flex items-center gap-2 group">
                        <span className="text-[11px] text-text-secondary">{dep.ticketId}: {dep.title}</span>
                        <button
                          type="button"
                          onClick={() => setBlocksIds((prev) => prev.filter((x) => x !== id))}
                          className="p-0.5 text-text-muted hover:text-accent-red opacity-0 group-hover:opacity-100 transition-opacity ml-auto"
                        >
                          <X size={10} />
                        </button>
                      </div>
                    );
                  })}
                </div>
              )}
              <select
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
                value=""
                onChange={(e) => {
                  if (e.target.value && !blocksIds.includes(e.target.value)) {
                    setBlocksIds((prev) => [...prev, e.target.value]);
                  }
                }}
              >
                <option value="">Add blocked issue...</option>
                {issues
                  .filter((i) => !blocksIds.includes(i.id))
                  .map((i) => (
                    <option key={i.id} value={i.id}>{i.ticketId}: {i.title}</option>
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

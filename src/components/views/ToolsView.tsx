import { Wrench, GitBranch, FolderOpen, Settings } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { useIssueStore } from "@/stores/issueStore";
import { useState } from "react";

export function ToolsView() {
  const projectPath = useLayoutStore((s) => s.projectPath);
  const gitBranch = useGitInfo();
  const ticketPrefix = useIssueStore((s) => s.ticketPrefix);
  const setTicketPrefix = useIssueStore((s) => s.setTicketPrefix);
  const addEpic = useIssueStore((s) => s.addEpic);
  const addLabel = useIssueStore((s) => s.addLabel);
  const [newEpic, setNewEpic] = useState("");
  const [newLabel, setNewLabel] = useState("");

  return (
    <div className="flex flex-col h-full bg-bg-primary p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <Wrench size={16} className="text-accent-amber" />
        <h2 className="text-sm font-semibold text-text-primary">Tools</h2>
      </div>

      <div className="grid grid-cols-2 gap-4 max-w-2xl">
        {/* Project Info */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
            <FolderOpen size={12} />
            Project
          </h3>
          <div className="flex flex-col gap-2 text-xs">
            <div>
              <span className="text-text-muted">Path: </span>
              <span className="text-text-secondary">{projectPath}</span>
            </div>
            {gitBranch && (
              <div className="flex items-center gap-1">
                <span className="text-text-muted">Branch: </span>
                <GitBranch size={10} className="text-accent-purple" />
                <span className="text-text-secondary">{gitBranch}</span>
              </div>
            )}
          </div>
        </div>

        {/* Issue Settings */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
            <Settings size={12} />
            Issue Settings
          </h3>
          <div className="flex flex-col gap-2">
            <div>
              <label className="text-[10px] text-text-muted uppercase tracking-wider">
                Ticket Prefix
              </label>
              <input
                type="text"
                value={ticketPrefix}
                onChange={(e) => setTicketPrefix(e.target.value.toUpperCase())}
                className="w-full bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary focus:outline-none focus:border-accent-green mt-1"
                maxLength={6}
              />
            </div>
          </div>
        </div>

        {/* Epics */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3">
            Epics
          </h3>
          <div className="flex gap-1 mb-2">
            <input
              type="text"
              value={newEpic}
              onChange={(e) => setNewEpic(e.target.value)}
              placeholder="New epic..."
              className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
              onKeyDown={(e) => {
                if (e.key === "Enter" && newEpic.trim()) {
                  addEpic(newEpic.trim());
                  setNewEpic("");
                }
              }}
            />
            <button
              onClick={() => {
                if (newEpic.trim()) {
                  addEpic(newEpic.trim());
                  setNewEpic("");
                }
              }}
              className="px-2 py-1 text-xs text-accent-green hover:bg-accent-green/15 rounded transition-colors"
            >
              Add
            </button>
          </div>
          <div className="flex flex-wrap gap-1">
            {useIssueStore.getState().epics.map((e) => (
              <span
                key={e}
                className="text-[10px] px-1.5 py-0.5 bg-accent-purple/15 text-accent-purple rounded"
              >
                {e}
              </span>
            ))}
          </div>
        </div>

        {/* Labels */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3">
            Labels
          </h3>
          <div className="flex gap-1 mb-2">
            <input
              type="text"
              value={newLabel}
              onChange={(e) => setNewLabel(e.target.value)}
              placeholder="New label..."
              className="flex-1 bg-bg-primary border border-bg-border rounded px-2 py-1 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
              onKeyDown={(e) => {
                if (e.key === "Enter" && newLabel.trim()) {
                  addLabel(newLabel.trim());
                  setNewLabel("");
                }
              }}
            />
            <button
              onClick={() => {
                if (newLabel.trim()) {
                  addLabel(newLabel.trim());
                  setNewLabel("");
                }
              }}
              className="px-2 py-1 text-xs text-accent-green hover:bg-accent-green/15 rounded transition-colors"
            >
              Add
            </button>
          </div>
          <div className="flex flex-wrap gap-1">
            {useIssueStore.getState().labels.map((l) => (
              <span
                key={l}
                className="text-[10px] px-1.5 py-0.5 bg-bg-elevated text-text-muted rounded"
              >
                {l}
              </span>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

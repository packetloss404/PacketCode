import { useState } from "react";
import { Wrench, ClipboardList } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { useIssueStore } from "@/stores/issueStore";
import { SpecImportModal } from "./SpecImportModal";
import { ProjectInfoCard } from "./tools/ProjectInfoCard";
import { IssueSettingsCard } from "./tools/IssueSettingsCard";
import { TagListCard } from "./tools/TagListCard";
import { AgentProfilesCard } from "./tools/AgentProfilesCard";
import { ModulesCard } from "./tools/ModulesCard";
import { NotificationSettingsCard } from "./tools/NotificationSettingsCard";

export function ToolsView() {
  const projectPath = useLayoutStore((s) => s.projectPath);
  const gitBranch = useGitInfo();
  const ticketPrefix = useIssueStore((s) => s.ticketPrefix);
  const setTicketPrefix = useIssueStore((s) => s.setTicketPrefix);
  const addEpic = useIssueStore((s) => s.addEpic);
  const addLabel = useIssueStore((s) => s.addLabel);
  const epics = useIssueStore((s) => s.epics);
  const labels = useIssueStore((s) => s.labels);
  const [showSpecImport, setShowSpecImport] = useState(false);

  return (
    <div className="flex flex-col h-full bg-bg-primary p-4 overflow-y-auto">
      <div className="flex items-center gap-2 mb-4">
        <Wrench size={16} className="text-accent-amber" />
        <h2 className="text-sm font-semibold text-text-primary">Tools</h2>
      </div>

      <div className="grid grid-cols-2 gap-4 max-w-2xl">
        <ProjectInfoCard projectPath={projectPath} gitBranch={gitBranch} />
        <IssueSettingsCard ticketPrefix={ticketPrefix} setTicketPrefix={setTicketPrefix} />

        <TagListCard
          title="Epics"
          items={epics}
          onAdd={addEpic}
          tagClassName="bg-accent-purple/15 text-accent-purple"
          placeholder="New epic..."
        />
        <TagListCard
          title="Labels"
          items={labels}
          onAdd={addLabel}
          tagClassName="bg-bg-elevated text-text-muted"
          placeholder="New label..."
        />

        {/* Spec2Tick */}
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
            <ClipboardList size={12} className="text-accent-green" />
            Spec2Tick
          </h3>
          <p className="text-[10px] text-text-muted mb-3">
            Paste any project spec and let Claude parse it into structured
            tickets on the Issues board.
          </p>
          <button
            onClick={() => setShowSpecImport(true)}
            className="px-3 py-1.5 text-[11px] font-medium rounded bg-accent-green text-bg-primary hover:opacity-90 transition-opacity"
          >
            Import Spec
          </button>
        </div>

        <AgentProfilesCard />
        <NotificationSettingsCard />
        <ModulesCard />
      </div>

      {showSpecImport && (
        <SpecImportModal onClose={() => setShowSpecImport(false)} />
      )}
    </div>
  );
}

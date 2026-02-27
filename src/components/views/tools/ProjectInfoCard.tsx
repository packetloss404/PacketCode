import { FolderOpen, GitBranch } from "lucide-react";
import { open } from "@tauri-apps/plugin-dialog";
import { useLayoutStore } from "../../../stores/layoutStore";

interface ProjectInfoCardProps {
  projectPath: string;
  gitBranch: string | null;
}

export function ProjectInfoCard({ projectPath, gitBranch }: ProjectInfoCardProps) {
  const setProjectPath = useLayoutStore((s) => s.setProjectPath);

  const handleBrowse = async () => {
    const selected = await open({ directory: true, title: "Select Default Project Folder" });
    if (selected) {
      setProjectPath(selected);
    }
  };

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
      <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
        <FolderOpen size={12} />
        Project
      </h3>
      <div className="flex flex-col gap-2 text-xs">
        <div className="flex items-center gap-2">
          <span className="text-text-muted">Path: </span>
          <span className="text-text-secondary truncate flex-1">{projectPath}</span>
          <button
            onClick={handleBrowse}
            className="px-2 py-0.5 text-[11px] bg-bg-tertiary hover:bg-bg-border text-text-secondary rounded transition-colors"
          >
            Browse
          </button>
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
  );
}

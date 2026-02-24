import { FolderOpen, GitBranch } from "lucide-react";

interface ProjectInfoCardProps {
  projectPath: string;
  gitBranch: string | null;
}

export function ProjectInfoCard({ projectPath, gitBranch }: ProjectInfoCardProps) {
  return (
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
  );
}

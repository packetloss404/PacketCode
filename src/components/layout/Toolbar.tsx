import { Plus, GitBranch, FolderOpen } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { open } from "@tauri-apps/plugin-dialog";

export function Toolbar() {
  const addPane = useLayoutStore((s) => s.addPane);
  const projectPath = useLayoutStore((s) => s.projectPath);
  const setProjectPath = useLayoutStore((s) => s.setProjectPath);
  const paneCount = useLayoutStore((s) => s.panes.length);
  const gitBranch = useGitInfo();

  const projectName = projectPath.split(/[/\\]/).pop() || "PacketCode";

  async function handleOpenFolder() {
    const selected = await open({
      directory: true,
      multiple: false,
      title: "Select Project Folder",
    });
    if (selected) {
      setProjectPath(selected as string);
    }
  }

  return (
    <div className="flex items-center h-9 px-3 bg-bg-tertiary border-b border-bg-border gap-2">
      {/* Left section */}
      <div className="flex items-center gap-3 flex-1">
        {/* Tab-like buttons */}
        <button className="px-2 py-1 text-xs text-accent-green bg-bg-elevated rounded hover:bg-bg-hover transition-colors">
          Claude
        </button>
        <button className="px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors">
          Tools
        </button>
        <button className="px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors">
          History
        </button>

        <div className="w-px h-4 bg-bg-border" />

        {/* Split pane button */}
        <button
          onClick={addPane}
          disabled={paneCount >= 4}
          className="flex items-center gap-1.5 px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors disabled:opacity-30"
          title="Split pane (max 4)"
        >
          <Plus size={12} />
          <span>Split</span>
        </button>
      </div>

      {/* Right section */}
      <div className="flex items-center gap-3">
        {/* Git branch */}
        {gitBranch && (
          <div className="flex items-center gap-1.5 px-2 py-0.5 bg-bg-elevated rounded text-xs">
            <GitBranch size={12} className="text-accent-purple" />
            <span className="text-text-secondary">{gitBranch}</span>
          </div>
        )}

        {/* Project name */}
        <button
          onClick={handleOpenFolder}
          className="flex items-center gap-1.5 px-2 py-0.5 bg-bg-elevated rounded text-xs text-text-secondary hover:text-text-primary transition-colors"
          title={projectPath}
        >
          <FolderOpen size={12} />
          <span>{projectName}</span>
        </button>
      </div>
    </div>
  );
}

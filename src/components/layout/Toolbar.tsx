import { Plus, GitBranch, FolderOpen } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore, type AppView } from "@/stores/appStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { open } from "@tauri-apps/plugin-dialog";

const TABS: { key: AppView; label: string }[] = [
  { key: "claude", label: "Claude" },
  { key: "issues", label: "Issues" },
  { key: "history", label: "History" },
  { key: "tools", label: "Tools" },
];

export function Toolbar() {
  const addPane = useLayoutStore((s) => s.addPane);
  const projectPath = useLayoutStore((s) => s.projectPath);
  const setProjectPath = useLayoutStore((s) => s.setProjectPath);
  const gitBranch = useGitInfo();

  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);

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
      {/* Left section — view tabs */}
      <div className="flex items-center gap-1 flex-1">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveView(tab.key)}
            className={`px-2.5 py-1 text-xs rounded transition-colors ${
              activeView === tab.key
                ? "text-accent-green bg-bg-elevated"
                : "text-text-secondary hover:text-text-primary hover:bg-bg-hover"
            }`}
          >
            {tab.label}
          </button>
        ))}

        <div className="w-px h-4 bg-bg-border ml-1" />

        {/* Split pane button — only in Claude view */}
        {activeView === "claude" && (
          <button
            onClick={addPane}
            className="flex items-center gap-1.5 px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors ml-1"
            title="New session pane"
          >
            <Plus size={12} />
            <span>Split</span>
          </button>
        )}
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

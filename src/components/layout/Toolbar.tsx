import { useState, useRef, useEffect } from "react";
import { Plus, GitBranch, FolderOpen, Diamond, Wrench, FolderTree, Sparkles } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore, type AppView } from "@/stores/appStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { open } from "@tauri-apps/plugin-dialog";
import { CodeQualityModal } from "@/components/quality/CodeQualityModal";
import { NewSessionModal } from "@/components/session/NewSessionModal";
import { FileExplorer } from "@/components/explorer/FileExplorer";

const TABS: { key: AppView; label: string }[] = [
  { key: "claude", label: "Claude" },
  { key: "codex", label: "Codex" },
  { key: "issues", label: "Issues" },
  { key: "history", label: "History" },
];

export function Toolbar() {
  const addPane = useLayoutStore((s) => s.addPane);
  const addCodexPane = useLayoutStore((s) => s.addCodexPane);
  const projectPath = useLayoutStore((s) => s.projectPath);
  const setProjectPath = useLayoutStore((s) => s.setProjectPath);
  const gitBranch = useGitInfo();
  const [showCodeQuality, setShowCodeQuality] = useState(false);
  const [newSessionCli, setNewSessionCli] = useState<"claude" | "codex" | null>(null);
  const [showToolsMenu, setShowToolsMenu] = useState(false);
  const [showExplorer, setShowExplorer] = useState(false);
  const toolsMenuRef = useRef<HTMLDivElement>(null);

  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);

  const projectName = projectPath.split(/[/\\]/).pop() || "PacketCode";

  // Close tools menu when clicking outside
  useEffect(() => {
    if (!showToolsMenu) return;
    function handleClick(e: MouseEvent) {
      if (toolsMenuRef.current && !toolsMenuRef.current.contains(e.target as Node)) {
        setShowToolsMenu(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [showToolsMenu]);

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

  function handleSplit() {
    if (activeView === "codex") {
      addCodexPane();
    } else {
      addPane();
    }
  }

  function handleTabClick(key: AppView) {
    if (key === "claude" || key === "codex") {
      setNewSessionCli(key);
    } else {
      setActiveView(key);
    }
  }

  function handleNewSessionClose() {
    if (newSessionCli) {
      setActiveView(newSessionCli);
    }
    setNewSessionCli(null);
  }

  return (
    <div className="flex items-center h-9 px-3 bg-bg-tertiary border-b border-bg-border gap-2">
      {/* Left section — view tabs + actions */}
      <div className="flex items-center gap-1 flex-1">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => handleTabClick(tab.key)}
            className={`px-2.5 py-1 text-xs rounded transition-colors ${
              activeView === tab.key
                ? "text-accent-green bg-bg-elevated"
                : "text-text-secondary hover:text-text-primary hover:bg-bg-hover"
            }`}
          >
            {tab.label}
          </button>
        ))}

        {/* Tools dropdown */}
        <div className="relative" ref={toolsMenuRef}>
          <button
            onClick={() => setShowToolsMenu(!showToolsMenu)}
            className={`px-2.5 py-1 text-xs rounded transition-colors flex items-center gap-1 ${
              activeView === "tools" || activeView === "architect"
                ? "text-accent-green bg-bg-elevated"
                : "text-text-secondary hover:text-text-primary hover:bg-bg-hover"
            }`}
          >
            <Wrench size={11} />
            Tools
          </button>

          {showToolsMenu && (
            <div className="absolute top-full left-0 mt-1 w-48 bg-bg-secondary border border-bg-border rounded-lg shadow-xl z-50 py-1">
              <button
                onClick={() => {
                  setShowExplorer(!showExplorer);
                  setShowToolsMenu(false);
                }}
                className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors text-left"
              >
                <FolderTree size={12} className="text-accent-amber" />
                Explorer
                {showExplorer && (
                  <span className="ml-auto text-[9px] text-accent-green">open</span>
                )}
              </button>
              <button
                onClick={() => {
                  setActiveView("architect");
                  setShowToolsMenu(false);
                }}
                className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors text-left"
              >
                <Sparkles size={12} className="text-accent-purple" />
                Vibe Architect
              </button>
              <button
                onClick={() => {
                  setActiveView("tools");
                  setShowToolsMenu(false);
                }}
                className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-text-secondary hover:text-text-primary hover:bg-bg-hover transition-colors text-left"
              >
                <Wrench size={12} className="text-text-muted" />
                Settings
              </button>
            </div>
          )}
        </div>

        <div className="w-px h-4 bg-bg-border ml-1" />

        {/* Split pane button — in Claude or Codex view */}
        {(activeView === "claude" || activeView === "codex") && (
          <button
            onClick={handleSplit}
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
        {/* Code Quality button */}
        <button
          onClick={() => setShowCodeQuality(true)}
          className="flex items-center gap-1.5 px-2 py-0.5 bg-bg-elevated rounded text-xs text-text-secondary hover:text-accent-amber transition-colors"
          title="Code Quality"
        >
          <Diamond size={12} className="text-accent-amber" />
          <span>Code Quality</span>
        </button>

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

      {/* Modals */}
      {showCodeQuality && (
        <CodeQualityModal onClose={() => setShowCodeQuality(false)} />
      )}
      {newSessionCli && (
        <NewSessionModal
          defaultCli={newSessionCli}
          onClose={handleNewSessionClose}
        />
      )}

      {/* Floating Explorer panel */}
      {showExplorer && (
        <FileExplorer onClose={() => setShowExplorer(false)} />
      )}
    </div>
  );
}

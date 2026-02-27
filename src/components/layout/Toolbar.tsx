import { useState, useRef, useEffect } from "react";
import { Plus, GitBranch, FolderOpen, Diamond, Wrench, FolderTree, MessageSquare, Github, Brain, User, BarChart3, Rocket, Zap, ArrowDown, ArrowUp, GitCommit, Sun, Moon, DollarSign, ClipboardList } from "lucide-react";
import { DropdownItem } from "./DropdownItem";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore, isModuleView, moduleViewId, type AppView } from "@/stores/appStore";
import { useModuleStore } from "@/stores/moduleStore";
import { getModulesSorted } from "@/modules/registry";
import { useProfileStore } from "@/stores/profileStore";
import { useGitInfo } from "@/hooks/useGitInfo";
import { open } from "@tauri-apps/plugin-dialog";
import { CodeQualityModal } from "@/components/quality/CodeQualityModal";
import { NewSessionModal } from "@/components/session/NewSessionModal";
import { SpecImportModal } from "@/components/views/SpecImportModal";

const TABS: { key: AppView; label: string }[] = [
  { key: "claude", label: "Claude" },
  { key: "codex", label: "Codex" },
  { key: "issues", label: "Issues" },
  { key: "history", label: "History" },
];

export function Toolbar() {
  const projectPath = useLayoutStore((s) => s.projectPath);
  const setProjectPath = useLayoutStore((s) => s.setProjectPath);
  const gitBranch = useGitInfo();
  const [showCodeQuality, setShowCodeQuality] = useState(false);
  const [newSessionCli, setNewSessionCli] = useState<"claude" | "codex" | null>(null);
  const [showToolsMenu, setShowToolsMenu] = useState(false);
  const [showSpecImport, setShowSpecImport] = useState(false);
  const [showProfileMenu, setShowProfileMenu] = useState(false);
  const explorerOpen = useLayoutStore((s) => s.explorerOpen);
  const toggleExplorer = useLayoutStore((s) => s.toggleExplorer);
  const theme = useAppStore((s) => s.theme);
  const setTheme = useAppStore((s) => s.setTheme);
  const toolsMenuRef = useRef<HTMLDivElement>(null);
  const profileMenuRef = useRef<HTMLDivElement>(null);

  const activeProfileId = useProfileStore((s) => s.activeProfileId);
  const profiles = useProfileStore((s) => s.profiles);
  const setActiveProfile = useProfileStore((s) => s.setActiveProfile);
  const activeProfile = profiles.find((p) => p.id === activeProfileId);

  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);
  const quickStartSession = useAppStore((s) => s.quickStartSession);
  const moduleStates = useModuleStore((s) => s.states);

  const projectName = projectPath.split(/[/\\]/).pop() || "PacketCode";

  // Close tools menu when clicking outside
  useEffect(() => {
    if (!showToolsMenu && !showProfileMenu) return;
    function handleClick(e: MouseEvent) {
      if (showToolsMenu && toolsMenuRef.current && !toolsMenuRef.current.contains(e.target as Node)) {
        setShowToolsMenu(false);
      }
      if (showProfileMenu && profileMenuRef.current && !profileMenuRef.current.contains(e.target as Node)) {
        setShowProfileMenu(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [showToolsMenu, showProfileMenu]);

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
    setNewSessionCli(activeView === "codex" ? "codex" : "claude");
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
        {/* Sessions — navigate to sessions view without opening modal */}
        <button
          onClick={() => {
            if (activeView !== "claude" && activeView !== "codex") {
              setActiveView("claude");
            }
          }}
          className={`px-2.5 py-1 text-xs rounded transition-colors ${
            activeView === "claude" || activeView === "codex"
              ? "text-accent-green bg-bg-elevated"
              : "text-text-secondary hover:text-text-primary hover:bg-bg-hover"
          }`}
        >
          Sessions
        </button>

        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => handleTabClick(tab.key)}
            className={`px-2.5 py-1 text-xs rounded transition-colors ${
              activeView === tab.key ||
              ((tab.key === "claude" || tab.key === "codex") &&
                (activeView === "claude" || activeView === "codex"))
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
              activeView === "tools" || activeView === "insights" || activeView === "github" || activeView === "memory" || activeView === "analytics" || isModuleView(activeView)
                ? "text-accent-green bg-bg-elevated"
                : "text-text-secondary hover:text-text-primary hover:bg-bg-hover"
            }`}
          >
            <Wrench size={11} />
            Tools
          </button>

          {showToolsMenu && (
            <div className="absolute top-full left-0 mt-1 w-48 bg-bg-secondary border border-bg-border rounded-lg shadow-xl z-50 py-1">
              <DropdownItem
                icon={<FolderTree size={12} className="text-accent-amber" />}
                label="Explorer"
                badge={explorerOpen ? "open" : undefined}
                onClick={() => { toggleExplorer(); setShowToolsMenu(false); }}
              />
              {getModulesSorted()
                .filter((mod) => moduleStates[mod.id]?.enabled ?? false)
                .map((mod) => {
                  const Icon = mod.icon;
                  return (
                    <DropdownItem
                      key={mod.id}
                      icon={<Icon size={12} className={mod.iconColor} />}
                      label={mod.name}
                      onClick={() => { setActiveView(moduleViewId(mod.id)); setShowToolsMenu(false); }}
                    />
                  );
                })}
              <DropdownItem icon={<MessageSquare size={12} className="text-accent-blue" />} label="Insights Chat"
                onClick={() => { setActiveView("insights"); setShowToolsMenu(false); }} />
              <DropdownItem icon={<Github size={12} className="text-text-primary" />} label="GitHub"
                onClick={() => { setActiveView("github"); setShowToolsMenu(false); }} />
              <DropdownItem icon={<Brain size={12} className="text-accent-purple" />} label="Memory"
                onClick={() => { setActiveView("memory"); setShowToolsMenu(false); }} />
              <DropdownItem icon={<BarChart3 size={12} className="text-accent-green" />} label="Analytics"
                onClick={() => { setActiveView("analytics"); setShowToolsMenu(false); }} />
              <DropdownItem icon={<DollarSign size={12} className="text-accent-amber" />} label="Cost Dashboard"
                onClick={() => { setActiveView("cost"); setShowToolsMenu(false); }} />
              <DropdownItem icon={<ClipboardList size={12} className="text-accent-green" />} label="Import Spec"
                onClick={() => { setShowSpecImport(true); setShowToolsMenu(false); }} />
              <div className="h-px bg-bg-border my-0.5" />
              <DropdownItem icon={<Wrench size={12} className="text-text-muted" />} label="Settings"
                onClick={() => { setActiveView("tools"); setShowToolsMenu(false); }} />
            </div>
          )}
        </div>

        <div className="w-px h-4 bg-bg-border ml-1" />

        {/* Quick Session + Split pane buttons — in Claude or Codex view */}
        {(activeView === "claude" || activeView === "codex") && (
          <>
            <button
              onClick={() => quickStartSession(activeView === "codex" ? "codex" : "claude")}
              className="flex items-center gap-1.5 px-2 py-1 text-xs text-accent-green hover:bg-accent-green/10 rounded transition-colors ml-1"
              title="Quick session with profile defaults"
            >
              <Zap size={12} />
              <span>Quick</span>
            </button>
            <button
              onClick={handleSplit}
              className="flex items-center gap-1.5 px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-bg-hover rounded transition-colors"
              title="New session pane (with options)"
            >
              <Plus size={12} />
              <span>Split</span>
            </button>
          </>
        )}
      </div>

      {/* Profile quick-switch */}
      <div className="relative" ref={profileMenuRef}>
        <button
          onClick={() => setShowProfileMenu(!showProfileMenu)}
          className={`flex items-center gap-1.5 px-2 py-1 text-[11px] rounded transition-colors ${
            activeProfile
              ? `${activeProfile.color} bg-bg-elevated`
              : "text-text-muted hover:text-text-primary hover:bg-bg-hover"
          }`}
          title="Agent Profile"
        >
          <User size={11} />
          <span>{activeProfile?.name || "No Profile"}</span>
        </button>

        {showProfileMenu && (
          <div className="absolute top-full right-0 mt-1 w-52 bg-bg-secondary border border-bg-border rounded-lg shadow-xl z-50 py-1">
            <button
              onClick={() => {
                setActiveProfile(null);
                setShowProfileMenu(false);
              }}
              className={`flex items-center gap-2 w-full px-3 py-1.5 text-[11px] hover:bg-bg-hover transition-colors text-left ${
                !activeProfileId ? "text-accent-green" : "text-text-secondary"
              }`}
            >
              No Profile
            </button>
            {profiles.map((p) => (
              <button
                key={p.id}
                onClick={() => {
                  setActiveProfile(p.id);
                  setShowProfileMenu(false);
                }}
                className={`flex items-center gap-2 w-full px-3 py-1.5 text-[11px] hover:bg-bg-hover transition-colors text-left ${
                  activeProfileId === p.id ? "text-accent-green" : "text-text-secondary"
                }`}
              >
                <span className={p.color}>
                  <User size={10} />
                </span>
                <span className="flex-1">{p.name}</span>
                {activeProfileId === p.id && (
                  <span className="text-[9px] text-accent-green">active</span>
                )}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Right section */}
      <div className="flex items-center gap-2">
        {/* Theme toggle */}
        <button
          onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
          className="p-1 text-text-muted hover:text-text-primary transition-colors"
          title={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}
        >
          {theme === "dark" ? <Sun size={12} /> : <Moon size={12} />}
        </button>

        {/* Cost Dashboard */}
        <button
          onClick={() => setActiveView("cost")}
          className={`flex items-center gap-1 px-2 py-0.5 rounded text-xs transition-colors ${
            activeView === "cost"
              ? "bg-bg-elevated text-accent-amber"
              : "text-text-muted hover:text-accent-amber"
          }`}
          title="Cost Dashboard"
        >
          <DollarSign size={11} />
        </button>

        {/* Deploy button */}
        <button
          onClick={() => setActiveView("deploy")}
          className={`flex items-center gap-1.5 px-2 py-0.5 rounded text-xs transition-colors ${
            activeView === "deploy"
              ? "bg-bg-elevated text-accent-amber"
              : "bg-bg-elevated text-text-secondary hover:text-accent-amber"
          }`}
          title="Deploy Pipeline"
        >
          <Rocket size={12} className="text-accent-amber" />
          <span>Deploy</span>
        </button>

        {/* Code Quality button */}
        <button
          onClick={() => setShowCodeQuality(true)}
          className="flex items-center gap-1.5 px-2 py-0.5 bg-bg-elevated rounded text-xs text-text-secondary hover:text-accent-amber transition-colors"
          title="Code Quality"
        >
          <Diamond size={12} className="text-accent-amber" />
          <span>Quality</span>
        </button>

        {/* Git branch + actions */}
        {gitBranch && (
          <div className="flex items-center gap-0.5">
            <div className="flex items-center gap-1.5 px-2 py-0.5 bg-bg-elevated rounded-l text-xs">
              <GitBranch size={12} className="text-accent-purple" />
              <span className="text-text-secondary">{gitBranch}</span>
            </div>
            <GitActionButtons />
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
      {showSpecImport && (
        <SpecImportModal onClose={() => setShowSpecImport(false)} />
      )}

    </div>
  );
}

function GitActionButtons() {
  const projectPath = useLayoutStore((s) => s.projectPath);
  const [busy, setBusy] = useState<string | null>(null);

  async function handleGitAction(action: "pull" | "push" | "commit") {
    if (busy) return;
    setBusy(action);
    try {
      const { gitPull, gitPush, gitCommit } = await import("@/lib/tauri");
      if (action === "pull") {
        await gitPull(projectPath);
      } else if (action === "push") {
        await gitPush(projectPath);
      } else if (action === "commit") {
        // Quick commit with stage all
        const message = window.prompt("Commit message:");
        if (message) {
          await gitCommit(projectPath, message, true);
        }
      }
    } catch (err) {
      console.error(`Git ${action} failed:`, err);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="flex items-center bg-bg-elevated rounded-r border-l border-bg-border">
      <button
        onClick={() => handleGitAction("pull")}
        disabled={busy !== null}
        className="p-1 text-text-muted hover:text-accent-green transition-colors disabled:opacity-40"
        title="Git Pull"
      >
        <ArrowDown size={11} />
      </button>
      <button
        onClick={() => handleGitAction("push")}
        disabled={busy !== null}
        className="p-1 text-text-muted hover:text-accent-green transition-colors disabled:opacity-40"
        title="Git Push"
      >
        <ArrowUp size={11} />
      </button>
      <button
        onClick={() => handleGitAction("commit")}
        disabled={busy !== null}
        className="p-1 text-text-muted hover:text-accent-green transition-colors disabled:opacity-40"
        title="Git Commit"
      >
        <GitCommit size={11} />
      </button>
    </div>
  );
}

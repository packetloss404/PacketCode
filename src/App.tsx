import { useEffect, useCallback } from "react";
import { TitleBar } from "@/components/layout/TitleBar";
import { Toolbar } from "@/components/layout/Toolbar";
import { PaneContainer } from "@/components/layout/PaneContainer";
import { StatusBar } from "@/components/layout/StatusBar";
import { IssueBoard } from "@/components/issues/IssueBoard";
import { HistoryView } from "@/components/views/HistoryView";
import { ToolsView } from "@/components/views/ToolsView";
import { InsightsView } from "@/components/views/InsightsView";
import { GitHubView } from "@/components/views/GitHubView";
import { MemoryView } from "@/components/views/MemoryView";
import { WelcomeScreen } from "@/components/views/WelcomeScreen";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore, getModuleId, moduleViewId } from "@/stores/appStore";
import { useModuleStore } from "@/stores/moduleStore";
import { getModule } from "@/modules/registry";
import { useStatusLinePoller } from "@/hooks/useStatusLine";
import { useCodexStatusLinePoller } from "@/hooks/useCodexStatusLine";
import type { AppView } from "@/stores/appStore";

export default function App() {
  const addPane = useLayoutStore((s) => s.addPane);
  const panes = useLayoutStore((s) => s.panes);
  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);

  // Poll Claude Code status line data
  useStatusLinePoller();
  // Poll Codex status line data
  useCodexStatusLinePoller();

  // Persist pane count to localStorage
  useEffect(() => {
    localStorage.setItem("packetcode:pane-count", String(panes.length));
  }, [panes.length]);

  // Persist project path
  useEffect(() => {
    const saved = localStorage.getItem("packetcode:project-path");
    if (saved) {
      useLayoutStore.getState().setProjectPath(saved);
    }
  }, []);

  const projectPath = useLayoutStore((s) => s.projectPath);
  useEffect(() => {
    localStorage.setItem("packetcode:project-path", projectPath);
  }, [projectPath]);

  // Guard: if active view is a disabled module, redirect to tools
  useEffect(() => {
    const modId = getModuleId(activeView);
    if (modId && !useModuleStore.getState().isEnabled(modId)) {
      setActiveView("tools");
    }
  }, [activeView, setActiveView]);

  // Listen for new session requests
  useEffect(() => {
    function handleNewSession() {
      const view = useAppStore.getState().activeView;
      if (view !== "claude" && view !== "codex") {
        useAppStore.getState().setActiveView("claude");
      }
      useLayoutStore.getState().addPane({ cliCommand: "claude" });
    }

    function handleNewCodexSession() {
      if (useAppStore.getState().activeView !== "codex") {
        useAppStore.getState().setActiveView("codex");
      }
      useLayoutStore.getState().addPane({ cliCommand: "codex" });
    }

    window.addEventListener("packetcode:new-session", handleNewSession);
    window.addEventListener("packetcode:new-codex-session", handleNewCodexSession);
    return () => {
      window.removeEventListener("packetcode:new-session", handleNewSession);
      window.removeEventListener("packetcode:new-codex-session", handleNewCodexSession);
    };
  }, []);

  // Global keyboard shortcuts
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Ctrl+\ to split pane
      if (e.ctrlKey && e.key === "\\") {
        e.preventDefault();
        const view = useAppStore.getState().activeView;
        const cli = view === "codex" ? "codex" : "claude";
        addPane({ cliCommand: cli });
      }
      // Ctrl+1/2/3/4 to switch panes
      if (e.ctrlKey && !e.shiftKey && e.key >= "1" && e.key <= "4") {
        e.preventDefault();
        const currentPanes = useLayoutStore.getState().panes;
        const idx = parseInt(e.key) - 1;
        if (idx < currentPanes.length) {
          useLayoutStore.getState().setActivePaneId(currentPanes[idx].id);
        }
      }
      // Ctrl+Shift+1/2/3/4/5/6 to switch views
      if (e.ctrlKey && e.shiftKey) {
        const viewMap: Record<string, AppView> = {
          "!": "claude",    // Shift+1
          "@": "codex",     // Shift+2
          "#": "issues",    // Shift+3
          "$": "history",   // Shift+4
          "%": "tools",     // Shift+5
        };
        // Shift+6 -> Vibe Architect (only if enabled)
        if (e.key === "^") {
          const modView = moduleViewId("vibe-architect");
          if (useModuleStore.getState().isEnabled("vibe-architect")) {
            e.preventDefault();
            setActiveView(modView);
          }
          return;
        }
        if (viewMap[e.key]) {
          e.preventDefault();
          setActiveView(viewMap[e.key]);
        }
      }
    },
    [addPane, setActiveView]
  );

  useEffect(() => {
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown]);

  const isSessionsView = activeView === "claude" || activeView === "codex";

  return (
    <ErrorBoundary fallbackMessage="PacketCode encountered an error">
      <div className="flex flex-col h-screen bg-bg-primary text-text-primary font-mono">
        <TitleBar />
        <Toolbar />
        <ErrorBoundary fallbackMessage="View error">
          {/* Welcome screen */}
          {activeView === "welcome" && (
            <div className="flex flex-col flex-1 overflow-hidden">
              <WelcomeScreen />
            </div>
          )}
          {/* Unified PaneContainer for both Claude and Codex */}
          <div
            className="flex flex-col flex-1 overflow-hidden"
            style={{ display: isSessionsView ? "flex" : "none" }}
          >
            <PaneContainer />
          </div>
          {/* Other views render conditionally */}
          <OtherViewContent activeView={activeView} />
        </ErrorBoundary>
        <StatusBar />
      </div>
    </ErrorBoundary>
  );
}

function OtherViewContent({ activeView }: { activeView: AppView }) {
  switch (activeView) {
    case "welcome":
      return null; // rendered above
    case "issues":
      return <IssueBoard />;
    case "history":
      return <HistoryView />;
    case "tools":
      return <ToolsView />;
    case "insights":
      return <InsightsView />;
    case "github":
      return <GitHubView />;
    case "memory":
      return <MemoryView />;
  }

  // Module views — dynamic lookup
  const modId = getModuleId(activeView);
  if (!modId) return null;
  const mod = getModule(modId);
  if (!mod || !useModuleStore.getState().isEnabled(modId)) return null;
  const ModComponent = mod.component;
  return (
    <ErrorBoundary fallbackMessage={`${mod.name} encountered an error`}>
      <ModComponent />
    </ErrorBoundary>
  );
}

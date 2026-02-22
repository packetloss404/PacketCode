import { useEffect, useCallback } from "react";
import { TitleBar } from "@/components/layout/TitleBar";
import { Toolbar } from "@/components/layout/Toolbar";
import { PaneContainer } from "@/components/layout/PaneContainer";
import { StatusBar } from "@/components/layout/StatusBar";
import { IssueBoard } from "@/components/issues/IssueBoard";
import { HistoryView } from "@/components/views/HistoryView";
import { ToolsView } from "@/components/views/ToolsView";
import { VibeArchitectView } from "@/components/views/VibeArchitectView";
import { InsightsView } from "@/components/views/InsightsView";
import { IdeationView } from "@/components/views/IdeationView";
import { GitHubView } from "@/components/views/GitHubView";
import { MemoryView } from "@/components/views/MemoryView";
import { WelcomeScreen } from "@/components/views/WelcomeScreen";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore } from "@/stores/appStore";
import { useStatusLinePoller } from "@/hooks/useStatusLine";
import { useCodexStatusLinePoller } from "@/hooks/useCodexStatusLine";

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
      // Ctrl+Shift+1/2/3/4/5 to switch views
      if (e.ctrlKey && e.shiftKey) {
        const viewMap: Record<string, typeof activeView> = {
          "!": "claude",    // Shift+1
          "@": "codex",     // Shift+2
          "#": "issues",    // Shift+3
          "$": "history",   // Shift+4
          "%": "tools",     // Shift+5
          "^": "architect", // Shift+6
        };
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

function OtherViewContent({ activeView }: { activeView: string }) {
  switch (activeView) {
    case "welcome":
      return null; // rendered above
    case "issues":
      return <IssueBoard />;
    case "history":
      return <HistoryView />;
    case "tools":
      return <ToolsView />;
    case "architect":
      return <VibeArchitectView />;
    case "insights":
      return <InsightsView />;
    case "ideation":
      return <IdeationView />;
    case "github":
      return <GitHubView />;
    case "memory":
      return <MemoryView />;
    default:
      return null;
  }
}

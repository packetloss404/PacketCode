import { useEffect, useCallback } from "react";
import { TitleBar } from "@/components/layout/TitleBar";
import { Toolbar } from "@/components/layout/Toolbar";
import { PaneContainer } from "@/components/layout/PaneContainer";
import { StatusBar } from "@/components/layout/StatusBar";
import { IssueBoard } from "@/components/issues/IssueBoard";
import { HistoryView } from "@/components/views/HistoryView";
import { ToolsView } from "@/components/views/ToolsView";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore } from "@/stores/appStore";

export default function App() {
  const addPane = useLayoutStore((s) => s.addPane);
  const addCodexPane = useLayoutStore((s) => s.addCodexPane);
  const panes = useLayoutStore((s) => s.panes);
  const codexPanes = useLayoutStore((s) => s.codexPanes);
  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);

  // Persist pane layout to localStorage
  useEffect(() => {
    const saved = localStorage.getItem("packetcode:pane-count");
    if (saved) {
      const count = parseInt(saved, 10);
      const current = useLayoutStore.getState().panes.length;
      for (let i = current; i < count; i++) {
        useLayoutStore.getState().addPane();
      }
    }
    const savedCodex = localStorage.getItem("packetcode:codex-pane-count");
    if (savedCodex) {
      const count = parseInt(savedCodex, 10);
      const current = useLayoutStore.getState().codexPanes.length;
      for (let i = current; i < count; i++) {
        useLayoutStore.getState().addCodexPane();
      }
    }
  }, []);

  useEffect(() => {
    localStorage.setItem("packetcode:pane-count", String(panes.length));
  }, [panes.length]);

  useEffect(() => {
    localStorage.setItem("packetcode:codex-pane-count", String(codexPanes.length));
  }, [codexPanes.length]);

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

  // Listen for new session requests from tab bars
  useEffect(() => {
    function handleNewSession() {
      const view = useAppStore.getState().activeView;
      if (view !== "claude" && view !== "codex") {
        useAppStore.getState().setActiveView("claude");
      }
      if (view === "codex") {
        useLayoutStore.getState().addCodexPane();
      } else {
        useLayoutStore.getState().addPane();
      }
    }

    function handleNewCodexSession() {
      if (useAppStore.getState().activeView !== "codex") {
        useAppStore.getState().setActiveView("codex");
      }
      useLayoutStore.getState().addCodexPane();
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
      // Ctrl+\ to split pane (in active CLI view)
      if (e.ctrlKey && e.key === "\\") {
        e.preventDefault();
        const view = useAppStore.getState().activeView;
        if (view === "codex") {
          addCodexPane();
        } else {
          addPane();
        }
      }
      // Ctrl+1/2/3/4 to switch panes
      if (e.ctrlKey && !e.shiftKey && e.key >= "1" && e.key <= "4") {
        e.preventDefault();
        const view = useAppStore.getState().activeView;
        if (view === "codex") {
          const currentPanes = useLayoutStore.getState().codexPanes;
          const idx = parseInt(e.key) - 1;
          if (idx < currentPanes.length) {
            useLayoutStore.getState().setActiveCodexPaneId(currentPanes[idx].id);
          }
        } else {
          const currentPanes = useLayoutStore.getState().panes;
          const idx = parseInt(e.key) - 1;
          if (idx < currentPanes.length) {
            useLayoutStore.getState().setActivePaneId(currentPanes[idx].id);
          }
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
        };
        if (viewMap[e.key]) {
          e.preventDefault();
          setActiveView(viewMap[e.key]);
        }
      }
    },
    [addPane, addCodexPane, setActiveView]
  );

  useEffect(() => {
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleKeyDown]);

  return (
    <ErrorBoundary fallbackMessage="PacketCode encountered an error">
      <div className="flex flex-col h-screen bg-bg-primary text-text-primary font-mono">
        <TitleBar />
        <Toolbar />
        <ErrorBoundary fallbackMessage="View error">
          {/* Claude and Codex PaneContainers always rendered, toggled via CSS */}
          <div
            className="flex flex-col flex-1 overflow-hidden"
            style={{ display: activeView === "claude" ? "flex" : "none" }}
          >
            <PaneContainer cliCommand="claude" paneSource="claude" />
          </div>
          <div
            className="flex flex-col flex-1 overflow-hidden"
            style={{ display: activeView === "codex" ? "flex" : "none" }}
          >
            <PaneContainer cliCommand="codex" paneSource="codex" />
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
    case "issues":
      return <IssueBoard />;
    case "history":
      return <HistoryView />;
    case "tools":
      return <ToolsView />;
    default:
      return null;
  }
}

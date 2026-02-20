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
  const panes = useLayoutStore((s) => s.panes);
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
  }, []);

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

  // Listen for new session request from tab bar
  useEffect(() => {
    function handleNewSession() {
      // If we're not in claude view, switch to it
      if (useAppStore.getState().activeView !== "claude") {
        useAppStore.getState().setActiveView("claude");
      }
      // Add a new pane (which auto-starts a session)
      useLayoutStore.getState().addPane();
    }

    window.addEventListener("packetcode:new-session", handleNewSession);
    return () =>
      window.removeEventListener("packetcode:new-session", handleNewSession);
  }, []);

  // Global keyboard shortcuts
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Ctrl+\ to split pane
      if (e.ctrlKey && e.key === "\\") {
        e.preventDefault();
        addPane();
      }
      // Ctrl+1/2/3/4 to switch panes
      if (e.ctrlKey && e.key >= "1" && e.key <= "4") {
        e.preventDefault();
        const currentPanes = useLayoutStore.getState().panes;
        const idx = parseInt(e.key) - 1;
        if (idx < currentPanes.length) {
          useLayoutStore.getState().setActivePaneId(currentPanes[idx].id);
        }
      }
      // Ctrl+Shift+1/2/3/4 to switch views
      if (e.ctrlKey && e.shiftKey) {
        const viewMap: Record<string, typeof activeView> = {
          "!": "claude",    // Shift+1
          "@": "issues",    // Shift+2
          "#": "history",   // Shift+3
          "$": "tools",     // Shift+4
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

  return (
    <ErrorBoundary fallbackMessage="PacketCode encountered an error">
      <div className="flex flex-col h-screen bg-bg-primary text-text-primary font-mono">
        <TitleBar />
        <Toolbar />
        <ErrorBoundary fallbackMessage="View error">
          <ViewContent activeView={activeView} />
        </ErrorBoundary>
        <StatusBar />
      </div>
    </ErrorBoundary>
  );
}

function ViewContent({ activeView }: { activeView: string }) {
  switch (activeView) {
    case "claude":
      return <PaneContainer />;
    case "issues":
      return <IssueBoard />;
    case "history":
      return <HistoryView />;
    case "tools":
      return <ToolsView />;
    default:
      return <PaneContainer />;
  }
}

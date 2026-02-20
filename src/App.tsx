import { useEffect, useCallback } from "react";
import { TitleBar } from "@/components/layout/TitleBar";
import { Toolbar } from "@/components/layout/Toolbar";
import { PaneContainer } from "@/components/layout/PaneContainer";
import { StatusBar } from "@/components/layout/StatusBar";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { useLayoutStore } from "@/stores/layoutStore";

export default function App() {
  const addPane = useLayoutStore((s) => s.addPane);
  const panes = useLayoutStore((s) => s.panes);

  // Persist pane layout to localStorage
  useEffect(() => {
    const saved = localStorage.getItem("packetcode:pane-count");
    if (saved) {
      const count = parseInt(saved, 10);
      const current = useLayoutStore.getState().panes.length;
      for (let i = current; i < count && i < 4; i++) {
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
    },
    [addPane]
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
        <ErrorBoundary fallbackMessage="Session pane error">
          <PaneContainer />
        </ErrorBoundary>
        <StatusBar />
      </div>
    </ErrorBoundary>
  );
}

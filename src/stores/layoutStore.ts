import { create } from "zustand";
import type { PaneConfig } from "@/types/layout";

interface LayoutStore {
  panes: PaneConfig[];
  activePaneId: string;
  projectPath: string;
  explorerOpen: boolean;

  setProjectPath: (path: string) => void;
  setExplorerOpen: (open: boolean) => void;
  toggleExplorer: () => void;
  addPane: (opts?: { cliCommand?: "claude" | "codex"; cliArgs?: string[]; initialPrompt?: string }) => string;
  removePane: (paneId: string) => void;
  setActivePaneId: (paneId: string) => void;
  setPaneSession: (paneId: string, sessionId: string | null) => void;
  getActivePane: () => PaneConfig | undefined;
  updatePaneSize: (paneId: string, flexSize: number) => void;
}

let paneCounter = 0;
function createPaneId(): string {
  return `pane_${++paneCounter}`;
}

export const useLayoutStore = create<LayoutStore>((set, get) => ({
  panes: [],
  activePaneId: "",
  projectPath: "D:\\projects\\PacketCode",
  explorerOpen: false,

  setProjectPath: (path) => set({ projectPath: path }),
  setExplorerOpen: (open) => set({ explorerOpen: open }),
  toggleExplorer: () => set((state) => ({ explorerOpen: !state.explorerOpen })),

  addPane: (opts) => {
    const id = createPaneId();
    set((state) => ({
      panes: [
        ...state.panes,
        {
          id,
          sessionId: null,
          cliCommand: opts?.cliCommand ?? "claude",
          cliArgs: opts?.cliArgs,
          initialPrompt: opts?.initialPrompt,
          flexSize: 1,
        },
      ],
      activePaneId: id,
    }));
    return id;
  },

  removePane: (paneId) => {
    set((state) => {
      const panes = state.panes.filter((p) => p.id !== paneId);
      const activePaneId =
        state.activePaneId === paneId
          ? (panes[panes.length - 1]?.id ?? "")
          : state.activePaneId;
      return { panes, activePaneId };
    });
  },

  setActivePaneId: (paneId) => set({ activePaneId: paneId }),

  setPaneSession: (paneId, sessionId) => {
    set((state) => ({
      panes: state.panes.map((p) =>
        p.id === paneId ? { ...p, sessionId } : p
      ),
    }));
  },

  getActivePane: () => {
    const state = get();
    return state.panes.find((p) => p.id === state.activePaneId);
  },

  updatePaneSize: (paneId, flexSize) => {
    set((state) => ({
      panes: state.panes.map((p) =>
        p.id === paneId ? { ...p, flexSize } : p
      ),
    }));
  },
}));

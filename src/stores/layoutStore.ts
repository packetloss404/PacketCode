import { create } from "zustand";
import type { PaneConfig } from "@/types/layout";

interface LayoutStore {
  panes: PaneConfig[];
  activePaneId: string;
  projectPath: string;

  setProjectPath: (path: string) => void;
  addPane: () => string;
  removePane: (paneId: string) => void;
  setActivePaneId: (paneId: string) => void;
  setPaneSession: (paneId: string, sessionId: string | null) => void;
  getActivePane: () => PaneConfig | undefined;
}

let paneCounter = 0;
function createPaneId(): string {
  return `pane_${++paneCounter}`;
}

const initialPaneId = createPaneId();

export const useLayoutStore = create<LayoutStore>((set, get) => ({
  panes: [{ id: initialPaneId, sessionId: null }],
  activePaneId: initialPaneId,
  projectPath: "D:\\projects\\PacketCode",

  setProjectPath: (path) => set({ projectPath: path }),

  addPane: () => {
    const id = createPaneId();
    set((state) => ({
      panes: [...state.panes, { id, sessionId: null }],
      activePaneId: id,
    }));
    return id;
  },

  removePane: (paneId) => {
    set((state) => {
      if (state.panes.length <= 1) return state;
      const panes = state.panes.filter((p) => p.id !== paneId);
      const activePaneId =
        state.activePaneId === paneId
          ? panes[panes.length - 1].id
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
}));

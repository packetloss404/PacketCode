import { create } from "zustand";
import type { PaneConfig } from "@/types/layout";

interface LayoutStore {
  panes: PaneConfig[];
  activePaneId: string;
  codexPanes: PaneConfig[];
  activeCodexPaneId: string;
  projectPath: string;

  setProjectPath: (path: string) => void;
  addPane: (opts?: { cliArgs?: string[]; initialPrompt?: string }) => string;
  removePane: (paneId: string) => void;
  setActivePaneId: (paneId: string) => void;
  setPaneSession: (paneId: string, sessionId: string | null) => void;
  getActivePane: () => PaneConfig | undefined;

  addCodexPane: (opts?: { cliArgs?: string[]; initialPrompt?: string }) => string;
  removeCodexPane: (paneId: string) => void;
  setActiveCodexPaneId: (paneId: string) => void;
  setCodexPaneSession: (paneId: string, sessionId: string | null) => void;
  getActiveCodexPane: () => PaneConfig | undefined;
}

let paneCounter = 0;
function createPaneId(): string {
  return `pane_${++paneCounter}`;
}

const initialPaneId = createPaneId();
const initialCodexPaneId = createPaneId();

export const useLayoutStore = create<LayoutStore>((set, get) => ({
  panes: [{ id: initialPaneId, sessionId: null }],
  activePaneId: initialPaneId,
  codexPanes: [{ id: initialCodexPaneId, sessionId: null }],
  activeCodexPaneId: initialCodexPaneId,
  projectPath: "D:\\projects\\PacketCode",

  setProjectPath: (path) => set({ projectPath: path }),

  addPane: (opts) => {
    const id = createPaneId();
    set((state) => ({
      panes: [...state.panes, { id, sessionId: null, cliArgs: opts?.cliArgs, initialPrompt: opts?.initialPrompt }],
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

  addCodexPane: (opts) => {
    const id = createPaneId();
    set((state) => ({
      codexPanes: [...state.codexPanes, { id, sessionId: null, cliArgs: opts?.cliArgs, initialPrompt: opts?.initialPrompt }],
      activeCodexPaneId: id,
    }));
    return id;
  },

  removeCodexPane: (paneId) => {
    set((state) => {
      if (state.codexPanes.length <= 1) return state;
      const codexPanes = state.codexPanes.filter((p) => p.id !== paneId);
      const activeCodexPaneId =
        state.activeCodexPaneId === paneId
          ? codexPanes[codexPanes.length - 1].id
          : state.activeCodexPaneId;
      return { codexPanes, activeCodexPaneId };
    });
  },

  setActiveCodexPaneId: (paneId) => set({ activeCodexPaneId: paneId }),

  setCodexPaneSession: (paneId, sessionId) => {
    set((state) => ({
      codexPanes: state.codexPanes.map((p) =>
        p.id === paneId ? { ...p, sessionId } : p
      ),
    }));
  },

  getActiveCodexPane: () => {
    const state = get();
    return state.codexPanes.find((p) => p.id === state.activeCodexPaneId);
  },
}));

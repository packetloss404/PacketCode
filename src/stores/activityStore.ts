import { create } from "zustand";

export interface PaneActivity {
  currentTool: string | null;
  currentFile: string | null;
  agentState: "idle" | "thinking" | "tool_use" | "responding";
  lastActivityAt: number;
}

interface ActivityStore {
  activities: Record<string, PaneActivity>;

  setActivity: (paneId: string, activity: PaneActivity) => void;
  clearActivity: (paneId: string) => void;
  getActivity: (paneId: string) => PaneActivity | undefined;
}

export const useActivityStore = create<ActivityStore>((set, get) => ({
  activities: {},

  setActivity: (paneId, activity) => {
    set((s) => ({
      activities: { ...s.activities, [paneId]: activity },
    }));
  },

  clearActivity: (paneId) => {
    set((s) => {
      const next = { ...s.activities };
      delete next[paneId];
      return { activities: next };
    });
  },

  getActivity: (paneId) => get().activities[paneId],
}));

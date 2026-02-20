import { create } from "zustand";

export type SessionStatus =
  | "idle"
  | "starting"
  | "thinking"
  | "running"
  | "done"
  | "error";

const STATUS_LABELS: Record<SessionStatus, string[]> = {
  idle: ["Idle"],
  starting: ["Warming up..."],
  thinking: ["Crunching...", "Imagining...", "Pondering...", "Brewing...", "Concocting..."],
  running: ["Working...", "Crafting...", "Building..."],
  done: ["Cogitated", "Brewed", "Baked", "Crafted"],
  error: ["Error"],
};

function pickRandom(arr: string[]): string {
  return arr[Math.floor(Math.random() * arr.length)];
}

export interface SessionTab {
  id: string;
  ptySessionId: string;
  name: string;
  ticketId: string | null;
  status: SessionStatus;
  statusLabel: string;
  startedAt: number;
  durationMs: number;
  projectPath: string;
}

interface TabStore {
  tabs: SessionTab[];
  activeTabId: string | null;

  addTab: (tab: Omit<SessionTab, "statusLabel" | "durationMs">) => void;
  removeTab: (id: string) => void;
  setActiveTab: (id: string) => void;
  updateTabStatus: (id: string, status: SessionStatus) => void;
  updateTabDuration: (id: string, durationMs: number) => void;
  updateTabName: (id: string, name: string) => void;
  setTabTicket: (id: string, ticketId: string | null) => void;
  getTab: (id: string) => SessionTab | undefined;
}

export const useTabStore = create<TabStore>((set, get) => ({
  tabs: [],
  activeTabId: null,

  addTab: (tab) => {
    const statusLabel = pickRandom(STATUS_LABELS[tab.status]);
    const newTab: SessionTab = { ...tab, statusLabel, durationMs: 0 };
    set((s) => ({
      tabs: [...s.tabs, newTab],
      activeTabId: newTab.id,
    }));
  },

  removeTab: (id) => {
    set((s) => {
      const tabs = s.tabs.filter((t) => t.id !== id);
      const activeTabId =
        s.activeTabId === id
          ? tabs.length > 0
            ? tabs[tabs.length - 1].id
            : null
          : s.activeTabId;
      return { tabs, activeTabId };
    });
  },

  setActiveTab: (id) => set({ activeTabId: id }),

  updateTabStatus: (id, status) => {
    set((s) => ({
      tabs: s.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              status,
              statusLabel:
                status === "done"
                  ? `${pickRandom(STATUS_LABELS.done)} for ${formatDuration(t.durationMs)}`
                  : pickRandom(STATUS_LABELS[status]),
            }
          : t
      ),
    }));
  },

  updateTabDuration: (id, durationMs) => {
    set((s) => ({
      tabs: s.tabs.map((t) =>
        t.id === id
          ? {
              ...t,
              durationMs,
              statusLabel:
                t.status === "thinking" || t.status === "running"
                  ? `${t.statusLabel.split("(")[0].trim()} (${formatDuration(durationMs)})`
                  : t.status === "done"
                    ? `${pickRandom(STATUS_LABELS.done)} for ${formatDuration(durationMs)}`
                    : t.statusLabel,
            }
          : t
      ),
    }));
  },

  updateTabName: (id, name) => {
    set((s) => ({
      tabs: s.tabs.map((t) => (t.id === id ? { ...t, name } : t)),
    }));
  },

  setTabTicket: (id, ticketId) => {
    set((s) => ({
      tabs: s.tabs.map((t) => (t.id === id ? { ...t, ticketId } : t)),
    }));
  },

  getTab: (id) => get().tabs.find((t) => t.id === id),
}));

function formatDuration(ms: number): string {
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rem = s % 60;
  return `${m}m ${rem}s`;
}

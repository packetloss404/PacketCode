import { create } from "zustand";
import { invoke } from "@tauri-apps/api/core";

export interface HistoryEntry {
  display: string;
  timestamp: number;
  project: string;
  sessionId: string;
}

interface HistoryStore {
  entries: HistoryEntry[];
  loading: boolean;
  error: string | null;
  searchQuery: string;
  projectFilter: string | null;

  load: () => Promise<void>;
  setSearchQuery: (q: string) => void;
  setProjectFilter: (p: string | null) => void;
  filteredEntries: () => HistoryEntry[];
  uniqueProjects: () => string[];
}

export const useHistoryStore = create<HistoryStore>((set, get) => ({
  entries: [],
  loading: false,
  error: null,
  searchQuery: "",
  projectFilter: null,

  load: async () => {
    set({ loading: true, error: null });
    try {
      const raw = await invoke<string>("read_prompt_history");
      const entries: HistoryEntry[] = JSON.parse(raw);
      // Sort newest first
      entries.sort((a, b) => b.timestamp - a.timestamp);
      set({ entries, loading: false });
    } catch (err) {
      set({
        error: err instanceof Error ? err.message : String(err),
        loading: false,
      });
    }
  },

  setSearchQuery: (searchQuery) => set({ searchQuery }),
  setProjectFilter: (projectFilter) => set({ projectFilter }),

  filteredEntries: () => {
    const { entries, searchQuery, projectFilter } = get();
    let filtered = entries;

    if (projectFilter) {
      filtered = filtered.filter((e) => e.project === projectFilter);
    }

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      filtered = filtered.filter((e) => e.display.toLowerCase().includes(q));
    }

    return filtered;
  },

  uniqueProjects: () => {
    const { entries } = get();
    const projects = new Set(entries.map((e) => e.project).filter(Boolean));
    return Array.from(projects).sort();
  },
}));

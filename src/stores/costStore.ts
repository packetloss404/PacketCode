import { create } from "zustand";
import { loadFromStorage, saveToStorage } from "@/lib/storage";
import type { CostEntry, CostSummary } from "@/types/cost";

const STORAGE_KEY = "packetcode:cost-entries";

interface CostStore {
  entries: CostEntry[];
  recordCost: (sessionId: string, cost: number, model: string) => void;
  clearEntries: () => void;
  getSummary: () => CostSummary;
}

export const useCostStore = create<CostStore>((set, get) => ({
  entries: loadFromStorage<CostEntry[]>(STORAGE_KEY, []),

  recordCost: (sessionId, cost, model) => {
    if (cost <= 0) return;
    const state = get();
    // Deduplicate — only record if cost changed for this session
    const existing = state.entries.find(
      (e) => e.sessionId === sessionId && Math.abs(e.cost - cost) < 0.001
    );
    if (existing) return;

    const entry: CostEntry = {
      sessionId,
      timestamp: Date.now(),
      cost,
      model,
    };
    const updated = [...state.entries, entry];
    // Keep last 1000 entries
    const trimmed = updated.length > 1000 ? updated.slice(-1000) : updated;
    set({ entries: trimmed });
    saveToStorage(STORAGE_KEY, trimmed);
  },

  clearEntries: () => {
    set({ entries: [] });
    saveToStorage(STORAGE_KEY, []);
  },

  getSummary: () => {
    const entries = get().entries;
    const sessionCosts = new Map<string, number>();
    const costByDay: Record<string, number> = {};
    const costByModel: Record<string, number> = {};

    for (const entry of entries) {
      // Track max cost per session (since cost accumulates)
      const prev = sessionCosts.get(entry.sessionId) || 0;
      if (entry.cost > prev) {
        sessionCosts.set(entry.sessionId, entry.cost);
      }

      // By day
      const day = new Date(entry.timestamp).toISOString().slice(0, 10);
      costByDay[day] = (costByDay[day] || 0) + entry.cost;

      // By model
      costByModel[entry.model] = (costByModel[entry.model] || 0) + entry.cost;
    }

    const totalCost = Array.from(sessionCosts.values()).reduce((a, b) => a + b, 0);

    return {
      totalCost,
      sessionCount: sessionCosts.size,
      costByDay,
      costByModel,
    };
  },
}));

import { create } from "zustand";
import type { StatusLineData } from "@/types/statusline";

function normalizeCwd(cwd: string): string {
  return cwd.replace(/\\/g, "/").toLowerCase();
}

interface StatusLineStore {
  /** Map from normalized cwd to the most recent StatusLineData */
  byCwd: Record<string, StatusLineData>;
  /** Update from a batch of status line entries */
  update: (entries: StatusLineData[]) => void;
}

export const useStatusLineStore = create<StatusLineStore>((set) => ({
  byCwd: {},
  update: (entries) =>
    set((state) => {
      const next = { ...state.byCwd };

      for (const entry of entries) {
        const key = normalizeCwd(entry.cwd);
        const existing = next[key];
        // Keep the most recent by timestamp
        if (!existing || entry.timestamp >= existing.timestamp) {
          next[key] = entry;
        }
      }

      return { byCwd: next };
    }),
}));

/** Selector hook: get status line data for a specific project path */
export function useStatusLineForCwd(cwd: string | undefined): StatusLineData | null {
  return useStatusLineStore((state) => {
    if (!cwd) return null;
    const key = normalizeCwd(cwd);
    return state.byCwd[key] ?? null;
  });
}

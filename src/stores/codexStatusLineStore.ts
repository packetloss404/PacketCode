import { create } from "zustand";
import type { CodexStatusLineData } from "@/types/statusline";

function normalizeCwd(cwd: string): string {
  return cwd.replace(/\\/g, "/").toLowerCase();
}

interface CodexStatusLineStore {
  /** Map from normalized cwd to the most recent CodexStatusLineData */
  byCwd: Record<string, CodexStatusLineData>;
  /** Update from a batch of status line entries */
  update: (entries: CodexStatusLineData[]) => void;
}

export const useCodexStatusLineStore = create<CodexStatusLineStore>((set) => ({
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

/** Selector hook: get Codex status line data for a specific project path */
export function useCodexStatusLineForCwd(cwd: string | undefined): CodexStatusLineData | null {
  return useCodexStatusLineStore((state) => {
    if (!cwd) return null;
    const key = normalizeCwd(cwd);
    return state.byCwd[key] ?? null;
  });
}

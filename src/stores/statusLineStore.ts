import { create } from "zustand";
import type { StatusLineData } from "@/types/statusline";
import {
  mergeStatusLineEntries,
  normalizeStatusLineCwd,
} from "@/stores/statusLineStoreUtils";

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
      return { byCwd: mergeStatusLineEntries(state.byCwd, entries) };
    }),
}));

/** Selector hook: get status line data for a specific project path */
export function useStatusLineForCwd(cwd: string | undefined): StatusLineData | null {
  return useStatusLineStore((state) => {
    if (!cwd) return null;
    const key = normalizeStatusLineCwd(cwd);
    return state.byCwd[key] ?? null;
  });
}

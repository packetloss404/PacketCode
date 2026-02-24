import { create } from "zustand";
import type { CodexStatusLineData } from "@/types/statusline";
import {
  mergeStatusLineEntries,
  normalizeStatusLineCwd,
} from "@/stores/statusLineStoreUtils";

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
      return { byCwd: mergeStatusLineEntries(state.byCwd, entries) };
    }),
}));

/** Selector hook: get Codex status line data for a specific project path */
export function useCodexStatusLineForCwd(cwd: string | undefined): CodexStatusLineData | null {
  return useCodexStatusLineStore((state) => {
    if (!cwd) return null;
    const key = normalizeStatusLineCwd(cwd);
    return state.byCwd[key] ?? null;
  });
}

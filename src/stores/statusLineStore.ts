import { create } from "zustand";
import type { StatusLineData, CodexStatusLineData } from "@/types/statusline";
import {
  mergeStatusLineEntries,
  normalizeStatusLineCwd,
  type StatusLineEntryBase,
} from "@/stores/statusLineStoreUtils";

interface StatusLineStoreShape<T extends StatusLineEntryBase> {
  byCwd: Record<string, T>;
  update: (entries: T[]) => void;
}

function createStatusLineStore<T extends StatusLineEntryBase>() {
  return create<StatusLineStoreShape<T>>((set) => ({
    byCwd: {},
    update: (entries) =>
      set((state) => ({ byCwd: mergeStatusLineEntries(state.byCwd, entries) })),
  }));
}

function createForCwdSelector<T extends StatusLineEntryBase>(
  useStore: ReturnType<typeof createStatusLineStore<T>>
) {
  return function useForCwd(cwd: string | undefined): T | null {
    return useStore((state) => {
      if (!cwd) return null;
      const key = normalizeStatusLineCwd(cwd);
      return state.byCwd[key] ?? null;
    });
  };
}

export const useStatusLineStore = createStatusLineStore<StatusLineData>();
export const useStatusLineForCwd = createForCwdSelector(useStatusLineStore);

export const useCodexStatusLineStore = createStatusLineStore<CodexStatusLineData>();
export const useCodexStatusLineForCwd = createForCwdSelector(useCodexStatusLineStore);

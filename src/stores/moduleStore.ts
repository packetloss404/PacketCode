import { create } from "zustand";
import type { ModuleState } from "@/types/modules";
import { moduleRegistry } from "@/modules/registry";
import { loadFromStorage, saveToStorage, removeFromStorage } from "@/lib/storage";

const STORAGE_KEY = "packetcode:modules";
const OLD_STORAGE_KEY = "packetcode:extensions";

interface ModuleStore {
  states: Record<string, ModuleState>;
  isEnabled: (id: string) => boolean;
  toggleModule: (id: string) => void;
  setEnabled: (id: string, enabled: boolean) => void;
}

function loadPersistedStates(): Record<string, ModuleState> {
  const saved = loadFromStorage<Record<string, ModuleState>>(STORAGE_KEY, {});
  if (Object.keys(saved).length > 0) return saved;

  // One-time migration from old key
  const old = loadFromStorage<Record<string, ModuleState>>(OLD_STORAGE_KEY, {});
  if (Object.keys(old).length > 0) {
    saveToStorage(STORAGE_KEY, old);
    removeFromStorage(OLD_STORAGE_KEY);
    return old;
  }
  return {};
}

function mergeWithDefaults(saved: Record<string, ModuleState>): Record<string, ModuleState> {
  const merged: Record<string, ModuleState> = {};
  for (const mod of moduleRegistry) {
    merged[mod.id] = saved[mod.id] ?? { enabled: mod.enabledByDefault };
  }
  return merged;
}

function persist(states: Record<string, ModuleState>) {
  saveToStorage(STORAGE_KEY, states);
}

export const useModuleStore = create<ModuleStore>((set, get) => ({
  states: mergeWithDefaults(loadPersistedStates()),

  isEnabled: (id: string) => get().states[id]?.enabled ?? false,

  toggleModule: (id: string) => {
    const current = get().states[id];
    if (!current) return;
    const next = { ...get().states, [id]: { enabled: !current.enabled } };
    persist(next);
    set({ states: next });
  },

  setEnabled: (id: string, enabled: boolean) => {
    const next = { ...get().states, [id]: { enabled } };
    persist(next);
    set({ states: next });
  },
}));

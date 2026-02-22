import { create } from "zustand";
import type { ExtensionState } from "@/types/extensions";
import { extensionRegistry } from "@/extensions/registry";

const STORAGE_KEY = "packetcode:extensions";

interface ExtensionStore {
  states: Record<string, ExtensionState>;
  isEnabled: (id: string) => boolean;
  toggleExtension: (id: string) => void;
  setEnabled: (id: string, enabled: boolean) => void;
}

function loadPersistedStates(): Record<string, ExtensionState> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw);
  } catch {
    // ignore
  }
  return {};
}

function mergeWithDefaults(saved: Record<string, ExtensionState>): Record<string, ExtensionState> {
  const merged: Record<string, ExtensionState> = {};
  for (const ext of extensionRegistry) {
    merged[ext.id] = saved[ext.id] ?? { enabled: ext.enabledByDefault };
  }
  return merged;
}

function persist(states: Record<string, ExtensionState>) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(states));
}

export const useExtensionStore = create<ExtensionStore>((set, get) => ({
  states: mergeWithDefaults(loadPersistedStates()),

  isEnabled: (id: string) => get().states[id]?.enabled ?? false,

  toggleExtension: (id: string) => {
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

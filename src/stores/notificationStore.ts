import { create } from "zustand";

interface NotificationPreferences {
  enabled: boolean;
  onlyWhenUnfocused: boolean;
  onApprovalNeeded: boolean;
  onSessionComplete: boolean;
  onSessionError: boolean;
}

interface NotificationStore extends NotificationPreferences {
  setEnabled: (enabled: boolean) => void;
  setOnlyWhenUnfocused: (v: boolean) => void;
  setOnApprovalNeeded: (v: boolean) => void;
  setOnSessionComplete: (v: boolean) => void;
  setOnSessionError: (v: boolean) => void;
}

const STORAGE_KEY = "packetcode:notifications";

function loadPreferences(): Partial<NotificationPreferences> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw);
  } catch {
    // ignore
  }
  return {};
}

function persist(state: NotificationPreferences) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    // ignore
  }
}

const saved = loadPreferences();

export const useNotificationStore = create<NotificationStore>((set, get) => ({
  enabled: saved.enabled ?? true,
  onlyWhenUnfocused: saved.onlyWhenUnfocused ?? true,
  onApprovalNeeded: saved.onApprovalNeeded ?? true,
  onSessionComplete: saved.onSessionComplete ?? true,
  onSessionError: saved.onSessionError ?? true,

  setEnabled: (enabled) => {
    set({ enabled });
    persist({ ...get(), enabled });
  },
  setOnlyWhenUnfocused: (onlyWhenUnfocused) => {
    set({ onlyWhenUnfocused });
    persist({ ...get(), onlyWhenUnfocused });
  },
  setOnApprovalNeeded: (onApprovalNeeded) => {
    set({ onApprovalNeeded });
    persist({ ...get(), onApprovalNeeded });
  },
  setOnSessionComplete: (onSessionComplete) => {
    set({ onSessionComplete });
    persist({ ...get(), onSessionComplete });
  },
  setOnSessionError: (onSessionError) => {
    set({ onSessionError });
    persist({ ...get(), onSessionError });
  },
}));

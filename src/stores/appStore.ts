import { create } from "zustand";

interface AppStore {
  gitBranch: string | null;
  claudeVersion: string | null;
  isMaximized: boolean;
  setGitBranch: (branch: string | null) => void;
  setClaudeVersion: (version: string | null) => void;
  setIsMaximized: (maximized: boolean) => void;
}

export const useAppStore = create<AppStore>((set) => ({
  gitBranch: null,
  claudeVersion: null,
  isMaximized: false,
  setGitBranch: (branch) => set({ gitBranch: branch }),
  setClaudeVersion: (version) => set({ claudeVersion: version }),
  setIsMaximized: (maximized) => set({ isMaximized: maximized }),
}));

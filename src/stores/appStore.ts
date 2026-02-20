import { create } from "zustand";

export type AppView = "claude" | "codex" | "issues" | "history" | "tools";

interface AppStore {
  activeView: AppView;
  gitBranch: string | null;
  claudeVersion: string | null;
  isMaximized: boolean;
  setActiveView: (view: AppView) => void;
  setGitBranch: (branch: string | null) => void;
  setClaudeVersion: (version: string | null) => void;
  setIsMaximized: (maximized: boolean) => void;
}

export const useAppStore = create<AppStore>((set) => ({
  activeView: "claude",
  gitBranch: null,
  claudeVersion: null,
  isMaximized: false,
  setActiveView: (view) => set({ activeView: view }),
  setGitBranch: (branch) => set({ gitBranch: branch }),
  setClaudeVersion: (version) => set({ claudeVersion: version }),
  setIsMaximized: (maximized) => set({ isMaximized: maximized }),
}));

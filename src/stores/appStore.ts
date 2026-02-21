import { create } from "zustand";

export type AppView = "welcome" | "claude" | "codex" | "issues" | "history" | "tools" | "architect";

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
  activeView: "welcome",
  gitBranch: null,
  claudeVersion: null,
  isMaximized: false,
  setActiveView: (view) => set({ activeView: view }),
  setGitBranch: (branch) => set({ gitBranch: branch }),
  setClaudeVersion: (version) => set({ claudeVersion: version }),
  setIsMaximized: (maximized) => set({ isMaximized: maximized }),
}));

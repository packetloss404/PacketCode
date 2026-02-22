import { create } from "zustand";

export type CoreView = "welcome" | "claude" | "codex" | "issues" | "history" | "tools" | "insights" | "github" | "memory";
export type AppView = CoreView | `ext:${string}`;

export function isExtensionView(view: AppView): boolean {
  return view.startsWith("ext:");
}

export function getExtensionId(view: AppView): string | null {
  return view.startsWith("ext:") ? view.slice(4) : null;
}

export function extensionViewId(id: string): AppView {
  return `ext:${id}` as AppView;
}

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

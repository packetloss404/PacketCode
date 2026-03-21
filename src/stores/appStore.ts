import { create } from "zustand";

export type CoreView = "welcome" | "claude" | "codex" | "issues" | "flights" | "flight_deck" | "history" | "tools" | "insights" | "github" | "memory" | "analytics" | "deploy" | "cost";
export type AppView = CoreView | `mod:${string}`;

export function isModuleView(view: AppView): boolean {
  return view.startsWith("mod:");
}

export function getModuleId(view: AppView): string | null {
  return view.startsWith("mod:") ? view.slice(4) : null;
}

export function moduleViewId(id: string): AppView {
  return `mod:${id}` as AppView;
}

interface AppStore {
  activeView: AppView;
  gitBranch: string | null;
  claudeVersion: string | null;
  isMaximized: boolean;
  commandPaletteOpen: boolean;
  theme: "dark" | "light";
  setActiveView: (view: AppView) => void;
  setGitBranch: (branch: string | null) => void;
  setClaudeVersion: (version: string | null) => void;
  setIsMaximized: (maximized: boolean) => void;
  setCommandPaletteOpen: (open: boolean) => void;
  setTheme: (theme: "dark" | "light") => void;
  quickStartSession: (cli?: "claude" | "codex") => void;
}

export const useAppStore = create<AppStore>((set) => ({
  activeView: "welcome",
  gitBranch: null,
  claudeVersion: null,
  isMaximized: false,
  commandPaletteOpen: false,
  theme: "dark",
  setActiveView: (view) => set({ activeView: view }),
  setGitBranch: (branch) => set({ gitBranch: branch }),
  setClaudeVersion: (version) => set({ claudeVersion: version }),
  setIsMaximized: (maximized) => set({ isMaximized: maximized }),
  setCommandPaletteOpen: (open) => set({ commandPaletteOpen: open }),
  setTheme: (theme) => set({ theme }),
  quickStartSession: async (cli = "claude") => {
    // Dynamic imports to avoid circular deps at module init time
    const { useProfileStore } = await import("@/stores/profileStore");
    const { useMemoryStore } = await import("@/stores/memoryStore");
    const { useLayoutStore } = await import("@/stores/layoutStore");

    const profileStore = useProfileStore.getState();
    const memoryStore = useMemoryStore.getState();
    const layoutStore = useLayoutStore.getState();

    const profile = profileStore.activeProfileId
      ? profileStore.profiles.find((p: { id: string }) => p.id === profileStore.activeProfileId)
      : null;

    const args: string[] = [];
    if (profile?.defaultModel) {
      args.push("--model", profile.defaultModel);
    }

    let prompt = "";
    if (profile?.systemPrompt) {
      prompt += profile.systemPrompt + "\n\n";
    }

    const memoryContext = memoryStore.getContextForSession();
    if (memoryContext.trim()) {
      prompt += memoryContext + "\n\n";
    }

    set({ activeView: cli });
    layoutStore.addPane({
      cliCommand: cli,
      cliArgs: args.length > 0 ? args : undefined,
      initialPrompt: prompt.trim() || undefined,
    });
  },
}));

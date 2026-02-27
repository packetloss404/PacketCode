import { create } from "zustand";
import type { McpServerEntry } from "@/types/mcp";
import { readMcpServers, writeMcpServer, deleteMcpServer } from "@/lib/tauri";
import { useLayoutStore } from "@/stores/layoutStore";

interface McpStore {
  servers: McpServerEntry[];
  loading: boolean;
  error: string | null;

  fetchServers: () => Promise<void>;
  addServer: (
    name: string,
    command: string,
    args: string[],
    env: Record<string, string>,
    scope: "global" | "project"
  ) => Promise<void>;
  updateServer: (
    name: string,
    command: string,
    args: string[],
    env: Record<string, string>,
    scope: "global" | "project"
  ) => Promise<void>;
  removeServer: (name: string, scope: "global" | "project") => Promise<void>;
}

export const useMcpStore = create<McpStore>((set, get) => ({
  servers: [],
  loading: false,
  error: null,

  fetchServers: async () => {
    set({ loading: true, error: null });
    try {
      const projectPath = useLayoutStore.getState().projectPath;
      const servers = await readMcpServers(projectPath);
      set({ servers, loading: false });
    } catch (e) {
      set({ error: String(e), loading: false });
    }
  },

  addServer: async (name, command, args, env, scope) => {
    const projectPath = useLayoutStore.getState().projectPath;
    await writeMcpServer(projectPath, name, command, args, env, scope);
    await get().fetchServers();
  },

  updateServer: async (name, command, args, env, scope) => {
    const projectPath = useLayoutStore.getState().projectPath;
    await writeMcpServer(projectPath, name, command, args, env, scope);
    await get().fetchServers();
  },

  removeServer: async (name, scope) => {
    const projectPath = useLayoutStore.getState().projectPath;
    await deleteMcpServer(projectPath, name, scope);
    await get().fetchServers();
  },
}));

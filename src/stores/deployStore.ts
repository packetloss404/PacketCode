import { create } from "zustand";
import type { DeployConfig, DeployRun, DeployStatus } from "@/types/deploy";
import { readDeployConfig, createDeployConfig } from "@/lib/tauri";
import { useLayoutStore } from "@/stores/layoutStore";

interface DeployStore {
  configs: DeployConfig[];
  configSource: string;
  loading: boolean;
  error: string | null;
  runs: DeployRun[];
  activeRunId: string | null;

  fetchConfigs: () => Promise<void>;
  saveConfigs: (configs: DeployConfig[]) => Promise<void>;
  addConfig: (config: DeployConfig) => Promise<void>;
  removeConfig: (name: string) => Promise<void>;
  startRun: (config: DeployConfig, sessionId: string) => void;
  finishRun: (runId: string, status: DeployStatus) => void;
  setActiveRunId: (id: string | null) => void;
}

let runCounter = 0;

export const useDeployStore = create<DeployStore>((set, get) => ({
  configs: [],
  configSource: "none",
  loading: false,
  error: null,
  runs: [],
  activeRunId: null,

  fetchConfigs: async () => {
    set({ loading: true, error: null });
    try {
      const projectPath = useLayoutStore.getState().projectPath;
      const result = await readDeployConfig(projectPath);
      set({ configs: result.configs, configSource: result.source, loading: false });
    } catch (e) {
      set({ error: String(e), loading: false });
    }
  },

  saveConfigs: async (configs) => {
    const projectPath = useLayoutStore.getState().projectPath;
    await createDeployConfig(projectPath, configs);
    set({ configs, configSource: "packetcode.deploy.json" });
  },

  addConfig: async (config) => {
    const next = [...get().configs, config];
    await get().saveConfigs(next);
  },

  removeConfig: async (name) => {
    const next = get().configs.filter((c) => c.name !== name);
    await get().saveConfigs(next);
  },

  startRun: (config, sessionId) => {
    const id = `deploy_${++runCounter}_${Date.now()}`;
    const run: DeployRun = {
      id,
      configName: config.name,
      command: config.command,
      status: "running",
      startedAt: Date.now(),
      finishedAt: null,
      sessionId,
    };
    set((s) => ({
      runs: [run, ...s.runs].slice(0, 10),
      activeRunId: id,
    }));
  },

  finishRun: (runId, status) => {
    set((s) => ({
      runs: s.runs.map((r) =>
        r.id === runId ? { ...r, status, finishedAt: Date.now() } : r
      ),
    }));
  },

  setActiveRunId: (id) => set({ activeRunId: id }),
}));

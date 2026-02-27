import { create } from "zustand";
import { invoke } from "@tauri-apps/api/core";

export interface ModelUsage {
  model: string;
  sessions: number;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
}

export interface DailyCost {
  date: string;
  costUsd: number;
}

export interface AnalyticsData {
  totalCostUsd: number;
  totalSessions: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  modelUsage: ModelUsage[];
  dailyCosts: DailyCost[];
}

interface AnalyticsStore {
  data: AnalyticsData | null;
  loading: boolean;
  error: string | null;

  load: () => Promise<void>;
}

const EMPTY_DATA: AnalyticsData = {
  totalCostUsd: 0,
  totalSessions: 0,
  totalInputTokens: 0,
  totalOutputTokens: 0,
  modelUsage: [],
  dailyCosts: [],
};

export const useAnalyticsStore = create<AnalyticsStore>((set) => ({
  data: null,
  loading: false,
  error: null,

  load: async () => {
    set({ loading: true, error: null });
    try {
      const raw = await invoke<string>("read_usage_analytics");
      const data: AnalyticsData = JSON.parse(raw);
      set({ data, loading: false });
    } catch (err) {
      set({
        data: EMPTY_DATA,
        error: err instanceof Error ? err.message : String(err),
        loading: false,
      });
    }
  },
}));

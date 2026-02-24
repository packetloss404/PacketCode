import { create } from "zustand";
import {
  githubClearToken,
  githubCreatePr,
  githubHasToken,
  githubInvestigateIssue,
  githubListIssues,
  githubListRepos,
  githubSetToken,
} from "@/lib/tauri";
import type { GitHubRepo, GitHubIssue, GitHubConfig } from "@/types/github";

const STORAGE_KEY = "packetcode:github";

interface LoadedConfig {
  config: GitHubConfig;
  legacyToken: string | null;
}

function loadConfig(): LoadedConfig {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (!saved) {
      return { config: { selectedRepo: null }, legacyToken: null };
    }
    const parsed = JSON.parse(saved) as {
      token?: unknown;
      selectedRepo?: unknown;
    };
    const selectedRepo =
      parsed.selectedRepo &&
      typeof parsed.selectedRepo === "object" &&
      "owner" in parsed.selectedRepo &&
      "repo" in parsed.selectedRepo
        ? (parsed.selectedRepo as { owner: string; repo: string })
        : null;
    const legacyToken =
      typeof parsed.token === "string" && parsed.token.trim()
        ? parsed.token.trim()
        : null;
    return { config: { selectedRepo }, legacyToken };
  } catch {
    // ignore
    return { config: { selectedRepo: null }, legacyToken: null };
  }
}

function saveConfig(config: GitHubConfig) {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({
      selectedRepo: config.selectedRepo,
    })
  );
}

interface GitHubStore {
  config: GitHubConfig;
  isConnected: boolean;
  isInitializing: boolean;
  repos: GitHubRepo[];
  issues: GitHubIssue[];
  isLoading: boolean;
  error: string | null;
  investigation: string | null;
  isInvestigating: boolean;

  initializeAuth: () => Promise<void>;
  connect: (token: string) => Promise<void>;
  disconnect: () => Promise<void>;
  fetchRepos: () => Promise<void>;
  selectRepo: (owner: string, repo: string) => void;
  fetchIssues: () => Promise<void>;
  investigateIssue: (projectPath: string, issueNumber: number) => Promise<void>;
  createPR: (
    title: string,
    body: string,
    head: string,
    base: string
  ) => Promise<string>;
  clearError: () => void;
  clearInvestigation: () => void;
}

const loaded = loadConfig();
let pendingLegacyToken = loaded.legacyToken;

export const useGitHubStore = create<GitHubStore>((set, get) => ({
  config: loaded.config,
  isConnected: false,
  isInitializing: false,
  repos: [],
  issues: [],
  isLoading: false,
  error: null,
  investigation: null,
  isInvestigating: false,

  initializeAuth: async () => {
    if (get().isInitializing) return;
    set({ isInitializing: true, error: null });
    try {
      let hasToken = await githubHasToken();
      if (!hasToken && pendingLegacyToken) {
        await githubSetToken(pendingLegacyToken);
        hasToken = true;
      }

      // One-time migration: rewrite persisted config without token.
      if (pendingLegacyToken) {
        pendingLegacyToken = null;
        saveConfig(get().config);
      }

      set({
        isConnected: hasToken,
        isInitializing: false,
      });
    } catch (e) {
      set({
        isConnected: false,
        isInitializing: false,
        error: String(e),
      });
    }
  },

  connect: async (token) => {
    const trimmed = token.trim();
    if (!trimmed) return;
    set({ isLoading: true, error: null });
    try {
      await githubSetToken(trimmed);
      pendingLegacyToken = null;
      set({
        isConnected: true,
        isLoading: false,
        repos: [],
        issues: [],
      });
    } catch (e) {
      set({
        isConnected: false,
        isLoading: false,
        error: String(e),
      });
    }
  },

  disconnect: async () => {
    set({ isLoading: true, error: null });
    try {
      await githubClearToken();
      set({
        isConnected: false,
        isLoading: false,
        repos: [],
        issues: [],
      });
    } catch (e) {
      set({
        isLoading: false,
        error: String(e),
      });
    }
  },

  fetchRepos: async () => {
    if (!get().isConnected) return;
    set({ isLoading: true, error: null });
    try {
      const json = await githubListRepos();
      const repos: GitHubRepo[] = JSON.parse(json);
      set({ repos, isLoading: false });
    } catch (e) {
      const message = String(e);
      set({
        isConnected: message.toLowerCase().includes("token not set")
          ? false
          : get().isConnected,
        error: message,
        isLoading: false,
      });
    }
  },

  selectRepo: (owner, repo) => {
    const config = { ...get().config, selectedRepo: { owner, repo } };
    saveConfig(config);
    set({ config, issues: [] });
  },

  fetchIssues: async () => {
    const { config } = get();
    if (!get().isConnected || !config.selectedRepo) return;
    set({ isLoading: true, error: null });
    try {
      const json = await githubListIssues(
        config.selectedRepo.owner,
        config.selectedRepo.repo
      );
      const issues: GitHubIssue[] = JSON.parse(json);
      set({ issues, isLoading: false });
    } catch (e) {
      const message = String(e);
      set({
        isConnected: message.toLowerCase().includes("token not set")
          ? false
          : get().isConnected,
        error: message,
        isLoading: false,
      });
    }
  },

  investigateIssue: async (projectPath, issueNumber) => {
    const { config } = get();
    if (!get().isConnected || !config.selectedRepo) return;
    set({ isInvestigating: true, investigation: null });
    try {
      const result = await githubInvestigateIssue(
        projectPath,
        config.selectedRepo.owner,
        config.selectedRepo.repo,
        issueNumber
      );
      set({ investigation: result, isInvestigating: false });
    } catch (e) {
      const message = String(e);
      set({
        isConnected: message.toLowerCase().includes("token not set")
          ? false
          : get().isConnected,
        error: message,
        isInvestigating: false,
        investigation: null,
      });
    }
  },

  createPR: async (title, body, head, base) => {
    const { config } = get();
    if (!get().isConnected || !config.selectedRepo)
      throw new Error("No repo selected");
    set({ isLoading: true, error: null });
    try {
      const json = await githubCreatePr(
        config.selectedRepo.owner,
        config.selectedRepo.repo,
        title,
        body,
        head,
        base
      );
      set({ isLoading: false });
      return json;
    } catch (e) {
      const message = String(e);
      set({
        isConnected: message.toLowerCase().includes("token not set")
          ? false
          : get().isConnected,
        error: message,
        isLoading: false,
      });
      throw e;
    }
  },

  clearError: () => set({ error: null }),
  clearInvestigation: () => set({ investigation: null }),
}));

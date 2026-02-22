import { create } from "zustand";
import { invoke } from "@tauri-apps/api/core";
import type { GitHubRepo, GitHubIssue, GitHubConfig } from "@/types/github";

function loadConfig(): GitHubConfig {
  try {
    const saved = localStorage.getItem("packetcode:github");
    if (saved) return JSON.parse(saved);
  } catch {
    // ignore
  }
  return { token: "", selectedRepo: null };
}

function saveConfig(config: GitHubConfig) {
  localStorage.setItem("packetcode:github", JSON.stringify(config));
}

interface GitHubStore {
  config: GitHubConfig;
  repos: GitHubRepo[];
  issues: GitHubIssue[];
  isLoading: boolean;
  error: string | null;
  investigation: string | null;
  isInvestigating: boolean;

  setToken: (token: string) => void;
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

export const useGitHubStore = create<GitHubStore>((set, get) => ({
  config: loadConfig(),
  repos: [],
  issues: [],
  isLoading: false,
  error: null,
  investigation: null,
  isInvestigating: false,

  setToken: (token) => {
    const config = { ...get().config, token };
    saveConfig(config);
    set({ config, repos: [], issues: [], error: null });
  },

  fetchRepos: async () => {
    const { config } = get();
    if (!config.token) return;
    set({ isLoading: true, error: null });
    try {
      const json = await invoke<string>("github_list_repos", {
        token: config.token,
      });
      const repos: GitHubRepo[] = JSON.parse(json);
      set({ repos, isLoading: false });
    } catch (e) {
      set({ error: String(e), isLoading: false });
    }
  },

  selectRepo: (owner, repo) => {
    const config = { ...get().config, selectedRepo: { owner, repo } };
    saveConfig(config);
    set({ config, issues: [] });
  },

  fetchIssues: async () => {
    const { config } = get();
    if (!config.token || !config.selectedRepo) return;
    set({ isLoading: true, error: null });
    try {
      const json = await invoke<string>("github_list_issues", {
        token: config.token,
        owner: config.selectedRepo.owner,
        repo: config.selectedRepo.repo,
      });
      const issues: GitHubIssue[] = JSON.parse(json);
      set({ issues, isLoading: false });
    } catch (e) {
      set({ error: String(e), isLoading: false });
    }
  },

  investigateIssue: async (projectPath, issueNumber) => {
    const { config } = get();
    if (!config.token || !config.selectedRepo) return;
    set({ isInvestigating: true, investigation: null });
    try {
      const result = await invoke<string>("github_investigate_issue", {
        projectPath,
        token: config.token,
        owner: config.selectedRepo.owner,
        repo: config.selectedRepo.repo,
        issueNumber: issueNumber,
      });
      set({ investigation: result, isInvestigating: false });
    } catch (e) {
      set({
        error: String(e),
        isInvestigating: false,
        investigation: null,
      });
    }
  },

  createPR: async (title, body, head, base) => {
    const { config } = get();
    if (!config.token || !config.selectedRepo)
      throw new Error("No repo selected");
    set({ isLoading: true, error: null });
    try {
      const json = await invoke<string>("github_create_pr", {
        token: config.token,
        owner: config.selectedRepo.owner,
        repo: config.selectedRepo.repo,
        title,
        body,
        head,
        base,
      });
      set({ isLoading: false });
      return json;
    } catch (e) {
      set({ error: String(e), isLoading: false });
      throw e;
    }
  },

  clearError: () => set({ error: null }),
  clearInvestigation: () => set({ investigation: null }),
}));

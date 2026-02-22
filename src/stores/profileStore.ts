import { create } from "zustand";
import type { AgentProfile } from "@/types/profiles";

const BUILTIN_PROFILES: AgentProfile[] = [
  {
    id: "auto",
    name: "Auto (Optimized)",
    description: "Balanced default with no extra system prompt",
    icon: "Zap",
    color: "text-accent-green",
    systemPrompt: "",
    defaultModel: "",
    isBuiltin: true,
  },
  {
    id: "speed-runner",
    name: "Speed Runner",
    description: "Fast, code-first responses with minimal explanation",
    icon: "Rocket",
    color: "text-accent-amber",
    systemPrompt:
      "Be concise. Minimal explanation. Code-first. Skip confirmations. Output code immediately without lengthy preambles.",
    defaultModel: "",
    isBuiltin: true,
  },
  {
    id: "thorough-reviewer",
    name: "Thorough Reviewer",
    description: "Step-by-step reasoning with edge case analysis",
    icon: "Search",
    color: "text-accent-blue",
    systemPrompt:
      "Think step-by-step. Explain reasoning. Consider edge cases. Review before executing. Be thorough in analysis.",
    defaultModel: "",
    isBuiltin: true,
  },
  {
    id: "security-auditor",
    name: "Security Auditor",
    description: "Focus on vulnerabilities and security best practices",
    icon: "Shield",
    color: "text-accent-red",
    systemPrompt:
      "Focus on security vulnerabilities, injection risks, auth issues. Flag OWASP top 10. Review for XSS, CSRF, SQLi, and other common attack vectors.",
    defaultModel: "",
    isBuiltin: true,
  },
  {
    id: "refactor-pro",
    name: "Refactor Pro",
    description: "Clean code advocate with DRY/SOLID focus",
    icon: "RefreshCw",
    color: "text-accent-purple",
    systemPrompt:
      "Focus on clean code, DRY, SOLID principles. Suggest refactoring opportunities. Improve readability and maintainability.",
    defaultModel: "",
    isBuiltin: true,
  },
];

function loadProfiles(): AgentProfile[] {
  try {
    const saved = localStorage.getItem("packetcode:profiles");
    if (saved) {
      const userProfiles: AgentProfile[] = JSON.parse(saved);
      return [...BUILTIN_PROFILES, ...userProfiles.filter((p) => !p.isBuiltin)];
    }
  } catch {
    // ignore
  }
  return [...BUILTIN_PROFILES];
}

function saveUserProfiles(profiles: AgentProfile[]) {
  const userOnly = profiles.filter((p) => !p.isBuiltin);
  localStorage.setItem("packetcode:profiles", JSON.stringify(userOnly));
}

interface ProfileStore {
  profiles: AgentProfile[];
  activeProfileId: string | null;
  addProfile: (profile: Omit<AgentProfile, "id" | "isBuiltin">) => void;
  updateProfile: (id: string, updates: Partial<AgentProfile>) => void;
  deleteProfile: (id: string) => void;
  setActiveProfile: (id: string | null) => void;
  getProfile: (id: string) => AgentProfile | undefined;
}

export const useProfileStore = create<ProfileStore>((set, get) => ({
  profiles: loadProfiles(),
  activeProfileId: localStorage.getItem("packetcode:active-profile") || null,

  addProfile: (profile) => {
    const newProfile: AgentProfile = {
      ...profile,
      id: `custom-${Date.now()}`,
      isBuiltin: false,
    };
    set((s) => {
      const profiles = [...s.profiles, newProfile];
      saveUserProfiles(profiles);
      return { profiles };
    });
  },

  updateProfile: (id, updates) => {
    set((s) => {
      const profiles = s.profiles.map((p) =>
        p.id === id ? { ...p, ...updates, id: p.id, isBuiltin: p.isBuiltin } : p
      );
      saveUserProfiles(profiles);
      return { profiles };
    });
  },

  deleteProfile: (id) => {
    const profile = get().profiles.find((p) => p.id === id);
    if (profile?.isBuiltin) return;
    set((s) => {
      const profiles = s.profiles.filter((p) => p.id !== id);
      saveUserProfiles(profiles);
      const activeProfileId = s.activeProfileId === id ? null : s.activeProfileId;
      return { profiles, activeProfileId };
    });
  },

  setActiveProfile: (id) => {
    if (id) {
      localStorage.setItem("packetcode:active-profile", id);
    } else {
      localStorage.removeItem("packetcode:active-profile");
    }
    set({ activeProfileId: id });
  },

  getProfile: (id) => get().profiles.find((p) => p.id === id),
}));

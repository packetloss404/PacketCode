import { create } from "zustand";
import { loadFromStorage, saveToStorage, generateId as genId } from "@/lib/storage";
import type { Mission, MissionStatus } from "@/types/mission";
import { useIssueStore } from "@/stores/issueStore";

type MissionState = {
  missions: Mission[];
  activeMissionId: string | null;
};

const DEFAULT_MISSION_STATE: MissionState = {
  missions: [],
  activeMissionId: null,
};

const generateMissionId = () => genId("mission");

// Migrate old missions that lack new fields
function migrateMission(mission: Mission): Mission {
  return {
    ...mission,
    id: mission.id || generateMissionId(),
    title: mission.title || "",
    objective: mission.objective || "",
    status: mission.status || "draft",
    priority: mission.priority || "medium",
    issueIds: mission.issueIds || [],
    linkedSessionIds: mission.linkedSessionIds || [],
    createdAt: mission.createdAt || Date.now(),
    updatedAt: mission.updatedAt || Date.now(),
  };
}

function loadState(): MissionState {
  const parsed = loadFromStorage<MissionState>("packetcode:missions", DEFAULT_MISSION_STATE);
  return { ...parsed, missions: (parsed.missions || []).map(migrateMission) };
}

function saveState(state: MissionState) {
  saveToStorage("packetcode:missions", state);
}

interface MissionStore {
  missions: Mission[];
  activeMissionId: string | null;

  addMission: (mission: Omit<Mission, "id" | "createdAt" | "updatedAt">) => Mission;
  updateMission: (id: string, updates: Partial<Mission>) => void;
  deleteMission: (id: string) => void;
  setActiveMission: (id: string | null) => void;
  getActiveMission: () => Mission | null;
  addIssueToMission: (missionId: string, issueId: string) => void;
  removeIssueFromMission: (missionId: string, issueId: string) => void;
  linkSessionToMission: (missionId: string, sessionId: string) => void;
  unlinkSessionFromMission: (missionId: string, sessionId: string) => void;
  computeMissionStatus: (missionId: string) => MissionStatus;
  getMissionForIssue: (issueId: string) => Mission | null;
}

const initial = loadState();

export const useMissionStore = create<MissionStore>((set, get) => ({
  ...initial,

  addMission: (mission) => {
    const state = get();
    const newMission: Mission = {
      ...mission,
      id: generateMissionId(),
      createdAt: Date.now(),
      updatedAt: Date.now(),
    };
    const newState: MissionState = {
      missions: [...state.missions, newMission],
      activeMissionId: state.activeMissionId,
    };
    set({ missions: newState.missions });
    saveState(newState);
    return newMission;
  },

  updateMission: (id, updates) => {
    set((s) => {
      const missions = s.missions.map((m) =>
        m.id === id ? { ...m, ...updates, updatedAt: Date.now() } : m
      );
      saveState({ missions, activeMissionId: s.activeMissionId });
      return { missions };
    });
  },

  deleteMission: (id) => {
    set((s) => {
      const missions = s.missions.filter((m) => m.id !== id);
      const activeMissionId = s.activeMissionId === id ? null : s.activeMissionId;
      saveState({ missions, activeMissionId });
      return { missions, activeMissionId };
    });
  },

  setActiveMission: (id) => {
    set((s) => {
      saveState({ missions: s.missions, activeMissionId: id });
      return { activeMissionId: id };
    });
  },

  getActiveMission: () => {
    const { missions, activeMissionId } = get();
    if (!activeMissionId) return null;
    return missions.find((m) => m.id === activeMissionId) ?? null;
  },

  addIssueToMission: (missionId, issueId) => {
    set((s) => {
      const missions = s.missions.map((m) =>
        m.id === missionId && !m.issueIds.includes(issueId)
          ? { ...m, issueIds: [...m.issueIds, issueId], updatedAt: Date.now() }
          : m
      );
      saveState({ missions, activeMissionId: s.activeMissionId });
      return { missions };
    });
  },

  removeIssueFromMission: (missionId, issueId) => {
    set((s) => {
      const missions = s.missions.map((m) =>
        m.id === missionId
          ? { ...m, issueIds: m.issueIds.filter((id) => id !== issueId), updatedAt: Date.now() }
          : m
      );
      saveState({ missions, activeMissionId: s.activeMissionId });
      return { missions };
    });
  },

  linkSessionToMission: (missionId, sessionId) => {
    set((s) => {
      const missions = s.missions.map((m) =>
        m.id === missionId && !m.linkedSessionIds.includes(sessionId)
          ? { ...m, linkedSessionIds: [...m.linkedSessionIds, sessionId], updatedAt: Date.now() }
          : m
      );
      saveState({ missions, activeMissionId: s.activeMissionId });
      return { missions };
    });
  },

  unlinkSessionFromMission: (missionId, sessionId) => {
    set((s) => {
      const missions = s.missions.map((m) =>
        m.id === missionId
          ? { ...m, linkedSessionIds: m.linkedSessionIds.filter((id) => id !== sessionId), updatedAt: Date.now() }
          : m
      );
      saveState({ missions, activeMissionId: s.activeMissionId });
      return { missions };
    });
  },

  computeMissionStatus: (missionId) => {
    const mission = get().missions.find((m) => m.id === missionId);
    if (!mission || mission.issueIds.length === 0) return "draft";
    const { issues } = useIssueStore.getState();
    const linked = issues.filter((i) => mission.issueIds.includes(i.id));
    if (linked.length === 0) return "draft";
    if (linked.some((i) => i.status === "needs_human")) return "needs_human";
    if (linked.some((i) => i.status === "blocked")) return "blocked";
    if (linked.every((i) => i.status === "done")) return "done";
    if (linked.some((i) => i.status === "in_progress" || i.status === "qa")) return "active";
    return "draft";
  },

  getMissionForIssue: (issueId) => {
    return get().missions.find((m) => m.issueIds.includes(issueId)) ?? null;
  },
}));

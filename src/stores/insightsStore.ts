import { create } from "zustand";
import type { InsightsMessage, InsightsSession } from "@/types/insights";
import { askInsights } from "@/lib/tauri";
import { useLayoutStore } from "@/stores/layoutStore";
import { loadFromStorage, saveToStorage, generateId } from "@/lib/storage";

const STORAGE_KEY = "packetcode:insights-sessions";

interface InsightsStore {
  sessions: InsightsSession[];
  activeSessionId: string | null;
  isLoading: boolean;

  createSession: () => void;
  switchSession: (id: string) => void;
  deleteSession: (id: string) => void;
  renameSession: (id: string, title: string) => void;
  sendMessage: (content: string) => Promise<void>;
}

function loadSessions(): InsightsSession[] {
  return loadFromStorage<InsightsSession[]>(STORAGE_KEY, []);
}

function saveSessions(sessions: InsightsSession[]) {
  saveToStorage(STORAGE_KEY, sessions);
}

export const useInsightsStore = create<InsightsStore>((set, get) => ({
  sessions: loadSessions(),
  activeSessionId: null,
  isLoading: false,

  createSession: () => {
    const session: InsightsSession = {
      id: generateId("ins"),
      title: "New Chat",
      messages: [],
      createdAt: Date.now(),
      updatedAt: Date.now(),
    };
    const sessions = [session, ...get().sessions];
    set({ sessions, activeSessionId: session.id });
    saveSessions(sessions);
  },

  switchSession: (id) => {
    set({ activeSessionId: id });
  },

  deleteSession: (id) => {
    const state = get();
    const sessions = state.sessions.filter((s) => s.id !== id);
    const activeSessionId = state.activeSessionId === id ? null : state.activeSessionId;
    set({ sessions, activeSessionId });
    saveSessions(sessions);
  },

  renameSession: (id, title) => {
    const sessions = get().sessions.map((s) =>
      s.id === id ? { ...s, title, updatedAt: Date.now() } : s
    );
    set({ sessions });
    saveSessions(sessions);
  },

  sendMessage: async (content) => {
    const state = get();
    let sessionId = state.activeSessionId;

    // Auto-create session if none active
    if (!sessionId) {
      get().createSession();
      sessionId = get().activeSessionId!;
    }

    const userMsg: InsightsMessage = {
      id: generateId("msg"),
      role: "user",
      content,
      timestamp: Date.now(),
    };

    // Append user message
    let sessions = get().sessions.map((s) =>
      s.id === sessionId
        ? {
            ...s,
            messages: [...s.messages, userMsg],
            title: s.messages.length === 0 ? content.slice(0, 50) : s.title,
            updatedAt: Date.now(),
          }
        : s
    );
    set({ sessions, isLoading: true });
    saveSessions(sessions);

    try {
      const session = sessions.find((s) => s.id === sessionId)!;
      const projectPath = useLayoutStore.getState().projectPath;

      const messagesForApi = session.messages.map((m) => ({
        role: m.role,
        content: m.content,
      }));

      const response = await askInsights(projectPath, messagesForApi);

      const assistantMsg: InsightsMessage = {
        id: generateId("msg"),
        role: "assistant",
        content: response,
        timestamp: Date.now(),
      };

      sessions = get().sessions.map((s) =>
        s.id === sessionId
          ? { ...s, messages: [...s.messages, assistantMsg], updatedAt: Date.now() }
          : s
      );
      set({ sessions, isLoading: false });
      saveSessions(sessions);
    } catch (err) {
      const errorMsg: InsightsMessage = {
        id: generateId("msg"),
        role: "assistant",
        content: `Error: ${err instanceof Error ? err.message : String(err)}`,
        timestamp: Date.now(),
      };

      sessions = get().sessions.map((s) =>
        s.id === sessionId
          ? { ...s, messages: [...s.messages, errorMsg], updatedAt: Date.now() }
          : s
      );
      set({ sessions, isLoading: false });
      saveSessions(sessions);
    }
  },
}));

import { create } from "zustand";
import type { InsightsMessage, InsightsSession } from "@/types/insights";
import { askInsightsStream } from "@/lib/tauri";
import { useLayoutStore } from "@/stores/layoutStore";
import { loadFromStorage, saveToStorage, generateId } from "@/lib/storage";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";

const STORAGE_KEY = "packetcode:insights-sessions";

interface InsightsStore {
  sessions: InsightsSession[];
  activeSessionId: string | null;
  isLoading: boolean;
  includeSessionContext: boolean;
  streamingContent: string;

  createSession: () => void;
  switchSession: (id: string) => void;
  deleteSession: (id: string) => void;
  renameSession: (id: string, title: string) => void;
  sendMessage: (content: string) => Promise<void>;
  setIncludeSessionContext: (include: boolean) => void;
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
  includeSessionContext: true,
  streamingContent: "",

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

  setIncludeSessionContext: (include) => {
    set({ includeSessionContext: include });
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
    set({ sessions, isLoading: true, streamingContent: "" });
    saveSessions(sessions);

    // Build session context if enabled
    let sessionContext: string | undefined;
    if (get().includeSessionContext) {
      const layoutStore = useLayoutStore.getState();
      const activePaneId = layoutStore.activePaneId;
      if (activePaneId) {
        const pane = layoutStore.panes.find((p) => p.id === activePaneId);
        if (pane) {
          sessionContext = `Active session: ${pane.cliCommand} (pane ${pane.id})`;
        }
      }
    }

    try {
      const session = sessions.find((s) => s.id === sessionId)!;
      const projectPath = useLayoutStore.getState().projectPath;

      const messagesForApi = session.messages.map((m) => ({
        role: m.role,
        content: m.content,
      }));

      // Set up event listeners for streaming
      let accumulated = "";
      const unlistenChunk: UnlistenFn = await listen<string>("insights:chunk", (event) => {
        accumulated += event.payload + "\n";
        set({ streamingContent: accumulated });
      });

      const donePromise = new Promise<boolean>((resolve) => {
        listen<boolean>("insights:done", (event) => {
          resolve(event.payload);
        }).then((unlisten) => {
          // Store unlisten for cleanup after done fires
          setTimeout(() => unlisten(), 100);
        });
      });

      // Start streaming
      await askInsightsStream(projectPath, messagesForApi, sessionContext);

      // Wait for completion
      await donePromise;
      unlistenChunk();

      const assistantMsg: InsightsMessage = {
        id: generateId("msg"),
        role: "assistant",
        content: accumulated.trim(),
        timestamp: Date.now(),
      };

      sessions = get().sessions.map((s) =>
        s.id === sessionId
          ? { ...s, messages: [...s.messages, assistantMsg], updatedAt: Date.now() }
          : s
      );
      set({ sessions, isLoading: false, streamingContent: "" });
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
      set({ sessions, isLoading: false, streamingContent: "" });
      saveSessions(sessions);
    }
  },
}));

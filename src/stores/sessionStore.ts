import { create } from "zustand";
import type { ParsedMessage } from "@/types/messages";
import type { SessionStatus } from "@/types/session";

export interface SessionData {
  id: string;
  messages: ParsedMessage[];
  status: SessionStatus;
  projectPath: string;
  model: string | null;
  claudeSessionId: string | null;
  costUsd: number;
  numTurns: number;
  error: string | null;
}

interface SessionStore {
  sessions: Map<string, SessionData>;
  createSessionData: (
    id: string,
    projectPath: string,
    model?: string
  ) => void;
  addMessages: (sessionId: string, messages: ParsedMessage[]) => void;
  updateStatus: (sessionId: string, status: SessionStatus) => void;
  updateSessionMeta: (
    sessionId: string,
    meta: Partial<Pick<SessionData, "claudeSessionId" | "costUsd" | "numTurns">>
  ) => void;
  setError: (sessionId: string, error: string) => void;
  removeSession: (sessionId: string) => void;
  clearSession: (sessionId: string) => void;
}

export const useSessionStore = create<SessionStore>((set) => ({
  sessions: new Map(),

  createSessionData: (id, projectPath, model) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      sessions.set(id, {
        id,
        messages: [],
        status: "running",
        projectPath,
        model: model || null,
        claudeSessionId: null,
        costUsd: 0,
        numTurns: 0,
        error: null,
      });
      return { sessions };
    });
  },

  addMessages: (sessionId, messages) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      const session = sessions.get(sessionId);
      if (session) {
        sessions.set(sessionId, {
          ...session,
          messages: [...session.messages, ...messages],
        });
      }
      return { sessions };
    });
  },

  updateStatus: (sessionId, status) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      const session = sessions.get(sessionId);
      if (session) {
        sessions.set(sessionId, { ...session, status });
      }
      return { sessions };
    });
  },

  updateSessionMeta: (sessionId, meta) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      const session = sessions.get(sessionId);
      if (session) {
        sessions.set(sessionId, { ...session, ...meta });
      }
      return { sessions };
    });
  },

  setError: (sessionId, error) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      const session = sessions.get(sessionId);
      if (session) {
        sessions.set(sessionId, {
          ...session,
          error,
          status: "error",
        });
      }
      return { sessions };
    });
  },

  removeSession: (sessionId) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      sessions.delete(sessionId);
      return { sessions };
    });
  },

  clearSession: (sessionId) => {
    set((state) => {
      const sessions = new Map(state.sessions);
      const session = sessions.get(sessionId);
      if (session) {
        sessions.set(sessionId, {
          ...session,
          messages: [],
          status: "idle",
          error: null,
          costUsd: 0,
          numTurns: 0,
          // Keep claudeSessionId for --resume
        });
      }
      return { sessions };
    });
  },
}));

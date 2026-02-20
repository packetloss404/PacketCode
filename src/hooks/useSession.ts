import { useCallback, useEffect, useRef } from "react";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { useSessionStore } from "@/stores/sessionStore";
import { useLayoutStore } from "@/stores/layoutStore";
import * as tauri from "@/lib/tauri";
import { parseClaudeJsonLine } from "@/lib/messageParser";
import type { SessionOutputEvent } from "@/types/messages";

export function useSession(paneId: string) {
  const pane = useLayoutStore((s) => s.panes.find((p) => p.id === paneId));
  const sessionId = pane?.sessionId ?? null;
  const session = useSessionStore((s) =>
    sessionId ? s.sessions.get(sessionId) : undefined
  );

  // Track unlisteners for cleanup
  const unlistenRef = useRef<UnlistenFn[]>([]);

  // Set up event listeners when sessionId changes
  useEffect(() => {
    if (!sessionId) return;

    let cancelled = false;

    async function setupListeners() {
      const outputUnlisten = await listen<SessionOutputEvent>(
        "session:output",
        (event) => {
          if (cancelled) return;
          if (event.payload.session_id !== sessionId) return;

          const parsed = parseClaudeJsonLine(event.payload.raw_json);
          if (parsed.length > 0) {
            useSessionStore.getState().addMessages(sessionId, parsed);

            // Check for result messages to update meta
            for (const msg of parsed) {
              if (msg.type === "result") {
                const store = useSessionStore.getState();
                if (msg.costUsd != null) {
                  store.updateSessionMeta(sessionId, {
                    costUsd: msg.costUsd,
                  });
                }
                if (msg.sessionId) {
                  store.updateSessionMeta(sessionId, {
                    claudeSessionId: msg.sessionId,
                  });
                }
                store.updateStatus(sessionId, "idle");
              }
            }
          }
        }
      );

      const endedUnlisten = await listen<string>(
        "session:ended",
        (event) => {
          if (cancelled) return;
          if (event.payload === sessionId) {
            const store = useSessionStore.getState();
            const sess = store.sessions.get(sessionId);
            // Only mark terminated if not already idle (successful completion)
            if (sess && sess.status === "running") {
              store.updateStatus(sessionId, "terminated");
            }
          }
        }
      );

      const stderrUnlisten = await listen<{ session_id: string; line: string }>(
        "session:stderr",
        (event) => {
          if (cancelled) return;
          if (event.payload.session_id !== sessionId) return;
          // stderr lines are debug info — ignore unless it looks like an error
          const line = event.payload.line;
          if (
            line.toLowerCase().includes("error") ||
            line.toLowerCase().includes("fatal") ||
            line.toLowerCase().includes("panic")
          ) {
            useSessionStore.getState().setError(sessionId, line);
          }
        }
      );

      if (!cancelled) {
        unlistenRef.current = [outputUnlisten, endedUnlisten, stderrUnlisten];
      } else {
        outputUnlisten();
        endedUnlisten();
        stderrUnlisten();
      }
    }

    setupListeners();

    return () => {
      cancelled = true;
      for (const fn of unlistenRef.current) {
        fn();
      }
      unlistenRef.current = [];
    };
  }, [sessionId]);

  // Start a new session or resume an existing one
  const startSession = useCallback(
    async (prompt: string, model?: string) => {
      const projectPath = useLayoutStore.getState().projectPath;

      // Check if there's a previous Claude session ID to resume
      const currentSessionId = useLayoutStore
        .getState()
        .panes.find((p) => p.id === paneId)?.sessionId;
      let resumeId: string | undefined;
      if (currentSessionId) {
        const existingSession = useSessionStore
          .getState()
          .sessions.get(currentSessionId);
        if (existingSession?.claudeSessionId) {
          resumeId = existingSession.claudeSessionId;
        }
      }

      try {
        const newSessionId = await tauri.createSession(
          projectPath,
          prompt,
          model,
          resumeId
        );
        useSessionStore
          .getState()
          .createSessionData(newSessionId, projectPath, model);
        useLayoutStore.getState().setPaneSession(paneId, newSessionId);
        return newSessionId;
      } catch (err) {
        // Surface the error visibly
        const errorMsg =
          err instanceof Error ? err.message : String(err);

        // If there's already a session, set its error
        const sid = useLayoutStore
          .getState()
          .panes.find((p) => p.id === paneId)?.sessionId;
        if (sid) {
          useSessionStore.getState().setError(sid, errorMsg);
        } else {
          // Create a temporary session to show the error
          const tempId = `error_${Date.now()}`;
          useSessionStore
            .getState()
            .createSessionData(tempId, projectPath, model);
          useSessionStore.getState().setError(tempId, errorMsg);
          useLayoutStore.getState().setPaneSession(paneId, tempId);
        }
        throw err;
      }
    },
    [paneId]
  );

  const sendInput = useCallback(
    async (input: string) => {
      const sid = useLayoutStore
        .getState()
        .panes.find((p) => p.id === paneId)?.sessionId;
      if (!sid) return;
      try {
        await tauri.sendInput(sid, input);
      } catch (err) {
        const errorMsg =
          err instanceof Error ? err.message : String(err);
        useSessionStore.getState().setError(sid, errorMsg);
      }
    },
    [paneId]
  );

  const killSession = useCallback(async () => {
    const sid = useLayoutStore
      .getState()
      .panes.find((p) => p.id === paneId)?.sessionId;
    if (!sid) return;
    try {
      await tauri.killSession(sid);
      useSessionStore.getState().updateStatus(sid, "terminated");
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : String(err);
      useSessionStore.getState().setError(sid, errorMsg);
    }
  }, [paneId]);

  const resetSession = useCallback(() => {
    const sid = useLayoutStore
      .getState()
      .panes.find((p) => p.id === paneId)?.sessionId;
    if (sid) {
      useSessionStore.getState().clearSession(sid);
    }
    useLayoutStore.getState().setPaneSession(paneId, null);
  }, [paneId]);

  return {
    session,
    sessionId,
    startSession,
    sendInput,
    killSession,
    resetSession,
  };
}

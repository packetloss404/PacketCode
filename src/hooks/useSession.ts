import { useCallback, useEffect } from "react";
import { listen } from "@tauri-apps/api/event";
import { useSessionStore } from "@/stores/sessionStore";
import { useLayoutStore } from "@/stores/layoutStore";
import * as tauri from "@/lib/tauri";
import { parseClaudeJsonLine } from "@/lib/messageParser";
import type { SessionOutputEvent } from "@/types/messages";

export function useSession(paneId: string) {
  const sessionStore = useSessionStore();
  const layoutStore = useLayoutStore();
  const pane = layoutStore.panes.find((p) => p.id === paneId);
  const sessionId = pane?.sessionId ?? null;
  const session = sessionId
    ? sessionStore.sessions.get(sessionId)
    : undefined;

  // Listen for session output events
  useEffect(() => {
    if (!sessionId) return;

    const unlisten = listen<SessionOutputEvent>(
      "session:output",
      (event) => {
        if (event.payload.session_id !== sessionId) return;

        const parsed = parseClaudeJsonLine(event.payload.raw_json);
        if (parsed.length > 0) {
          sessionStore.addMessages(sessionId, parsed);

          // Check for result messages to update meta
          for (const msg of parsed) {
            if (msg.type === "result") {
              if (msg.costUsd != null) {
                sessionStore.updateSessionMeta(sessionId, {
                  costUsd: msg.costUsd,
                });
              }
              if (msg.sessionId) {
                sessionStore.updateSessionMeta(sessionId, {
                  claudeSessionId: msg.sessionId,
                });
              }
              sessionStore.updateStatus(sessionId, "idle");
            }
          }
        }
      }
    );

    return () => {
      unlisten.then((fn) => fn());
    };
  }, [sessionId]);

  // Listen for session ended events
  useEffect(() => {
    if (!sessionId) return;

    const unlisten = listen<string>("session:ended", (event) => {
      if (event.payload === sessionId) {
        sessionStore.updateStatus(sessionId, "terminated");
      }
    });

    return () => {
      unlisten.then((fn) => fn());
    };
  }, [sessionId]);

  const startSession = useCallback(
    async (prompt: string, model?: string) => {
      const projectPath = layoutStore.projectPath;
      try {
        const newSessionId = await tauri.createSession(
          projectPath,
          prompt,
          model
        );
        sessionStore.createSessionData(newSessionId, projectPath, model);
        layoutStore.setPaneSession(paneId, newSessionId);
        return newSessionId;
      } catch (err) {
        console.error("Failed to start session:", err);
        throw err;
      }
    },
    [paneId]
  );

  const sendInput = useCallback(
    async (input: string) => {
      if (!sessionId) return;
      try {
        await tauri.sendInput(sessionId, input);
      } catch (err) {
        console.error("Failed to send input:", err);
      }
    },
    [sessionId]
  );

  const killSession = useCallback(async () => {
    if (!sessionId) return;
    try {
      await tauri.killSession(sessionId);
      sessionStore.updateStatus(sessionId, "terminated");
    } catch (err) {
      console.error("Failed to kill session:", err);
    }
  }, [sessionId]);

  return {
    session,
    sessionId,
    startSession,
    sendInput,
    killSession,
  };
}

import { useCallback } from "react";
import { X, RotateCcw } from "lucide-react";
import { useSession } from "@/hooks/useSession";
import { SessionOutput } from "./SessionOutput";
import { SessionInput } from "./SessionInput";
import { useLayoutStore } from "@/stores/layoutStore";

interface SessionPaneProps {
  paneId: string;
  onClose?: () => void;
  showCloseButton?: boolean;
}

export function SessionPane({
  paneId,
  onClose,
  showCloseButton = false,
}: SessionPaneProps) {
  const { session, sessionId, startSession, sendInput, killSession } =
    useSession(paneId);
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId);

  const handleSubmit = useCallback(
    async (prompt: string) => {
      if (session && (session.status === "running" || session.status === "waiting_input")) {
        // Send as follow-up input
        await sendInput(prompt);
      } else {
        // Start new session
        await startSession(prompt);
      }
    },
    [session, sendInput, startSession]
  );

  const status = session?.status ?? null;
  const messages = session?.messages ?? [];

  return (
    <div
      className="flex flex-col h-full bg-bg-primary"
      onClick={() => setActivePaneId(paneId)}
    >
      {/* Pane Header */}
      <div className="flex items-center justify-between px-3 py-1.5 bg-bg-secondary border-b border-bg-border">
        <div className="flex items-center gap-2">
          <div
            className={`w-2 h-2 rounded-full ${
              status === "running"
                ? "bg-accent-green animate-pulse"
                : status === "idle"
                  ? "bg-accent-amber"
                  : status === "error"
                    ? "bg-accent-red"
                    : "bg-text-muted"
            }`}
          />
          <span className="text-text-secondary text-xs">
            {sessionId
              ? `Session ${sessionId.slice(0, 8)}`
              : "New Session"}
          </span>
          {session?.costUsd ? (
            <span className="text-accent-amber text-[10px]">
              ${session.costUsd.toFixed(4)}
            </span>
          ) : null}
        </div>
        <div className="flex items-center gap-1">
          {session && session.status === "terminated" && (
            <button
              onClick={() => {
                // Clear and allow new session
              }}
              className="p-1 text-text-muted hover:text-text-primary transition-colors"
              title="New session"
            >
              <RotateCcw size={12} />
            </button>
          )}
          {showCloseButton && onClose && (
            <button
              onClick={onClose}
              className="p-1 text-text-muted hover:text-accent-red transition-colors"
              title="Close pane"
            >
              <X size={12} />
            </button>
          )}
        </div>
      </div>

      {/* Output */}
      <SessionOutput messages={messages} />

      {/* Input */}
      <SessionInput
        onSubmit={handleSubmit}
        onKill={killSession}
        status={status}
      />
    </div>
  );
}

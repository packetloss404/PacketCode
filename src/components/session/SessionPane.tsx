import { useCallback } from "react";
import { X, RotateCcw, AlertTriangle } from "lucide-react";
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
  const {
    session,
    sessionId,
    startSession,
    sendInput,
    killSession,
    resetSession,
  } = useSession(paneId);
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId);

  const handleSubmit = useCallback(
    async (prompt: string) => {
      try {
        if (
          session &&
          (session.status === "running" ||
            session.status === "waiting_input")
        ) {
          // Active session — send follow-up input to stdin
          await sendInput(prompt);
        } else {
          // No active session or session finished — start new (with resume if available)
          await startSession(prompt);
        }
      } catch {
        // Error is already surfaced in the store by useSession
      }
    },
    [session, sendInput, startSession]
  );

  const status = session?.status ?? null;
  const messages = session?.messages ?? [];
  const error = session?.error ?? null;

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
          {session &&
            (session.status === "terminated" ||
              session.status === "idle" ||
              session.status === "error") && (
              <button
                onClick={resetSession}
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

      {/* Error banner */}
      {error && (
        <div className="flex items-start gap-2 px-3 py-2 bg-accent-red/10 border-b border-accent-red/30 text-accent-red text-xs">
          <AlertTriangle size={14} className="flex-shrink-0 mt-0.5" />
          <div className="selectable flex-1">
            <p className="font-medium mb-0.5">Error</p>
            <p className="text-accent-red/80 break-all">{error}</p>
          </div>
        </div>
      )}

      {/* Output */}
      <SessionOutput messages={messages} />

      {/* Input */}
      <SessionInput
        onSubmit={handleSubmit}
        onKill={killSession}
        status={status}
        placeholder={
          session?.claudeSessionId && status === "idle"
            ? "Continue conversation... (Ctrl+Enter to send)"
            : "Enter a prompt... (Ctrl+Enter to send)"
        }
      />
    </div>
  );
}

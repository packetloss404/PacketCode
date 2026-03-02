import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { createPtySession, writePty, resizePty, killPty } from "@/lib/tauri";
import {
  X,
  RotateCcw,
  Plus,
  ShieldCheck,
  ShieldX,
  XCircle,
  FileEdit,
  Brain,
  TerminalSquare,
} from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useTabStore } from "@/stores/tabStore";
import { useActivityStore } from "@/stores/activityStore";
import { usePtyStateDetector, type PtyDetectorState } from "@/hooks/usePtyStateDetector";
import {
  notifyApprovalNeeded,
  notifySessionComplete,
  notifySessionError,
} from "@/lib/notifications";
import { ClaudeStatusBar } from "@/components/session/ClaudeStatusBar";
import { CodexStatusBar } from "@/components/session/CodexStatusBar";
import "@xterm/xterm/css/xterm.css";

interface TerminalPaneProps {
  paneId: string;
  onClose?: () => void;
  showCloseButton?: boolean;
  cliCommand?: string;
  cliArgs?: string[];
  initialPrompt?: string;
}

interface PtyOutput {
  session_id: string;
  data: string;
}

let sessionCounter = 0;

export function TerminalPane({
  paneId,
  onClose,
  showCloseButton = false,
  cliCommand = "claude",
  cliArgs,
  initialPrompt: _initialPrompt,
}: TerminalPaneProps) {
  const termRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const tabIdRef = useRef<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const unlistenersRef = useRef<UnlistenFn[]>([]);

  const [alive, setAlive] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showApproval, setShowApproval] = useState(false);
  const [activityInfo, setActivityInfo] = useState<{
    tool: string | null;
    file: string | null;
    state: PtyDetectorState["agentState"];
  }>({ tool: null, file: null, state: "idle" });

  const projectPath = useLayoutStore((s) => s.projectPath);
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId);

  // PTY state detector — handles approval detection, tool use, and activity
  const handleStateChange = useCallback(
    (prev: PtyDetectorState, next: PtyDetectorState) => {
      const tabId = tabIdRef.current;
      const sessionId = sessionIdRef.current;

      // Update approval overlay
      setShowApproval(next.needsApproval);

      // Update activity info for the strip
      setActivityInfo({
        tool: next.currentTool,
        file: next.currentFile,
        state: next.agentState,
      });

      // Update activity store for tab tooltips
      if (tabId) {
        useActivityStore.getState().setActivity(tabId, {
          currentTool: next.currentTool,
          currentFile: next.currentFile,
          agentState: next.agentState,
          lastActivityAt: next.lastActivityAt,
        });
      }

      // Tab status sync
      if (tabId) {
        if (next.needsApproval && !prev.needsApproval) {
          useTabStore.getState().updateTabStatus(tabId, "waiting_approval");
          // Notify
          const tab = useTabStore.getState().getTab(tabId);
          if (sessionId && tab) {
            notifyApprovalNeeded(sessionId, tab.name);
          }
        } else if (!next.needsApproval && prev.needsApproval) {
          // Restore to running when approval is handled
          useTabStore.getState().updateTabStatus(tabId, "running");
        }
      }
    },
    []
  );

  // We need to re-init detector when session changes. Use a state to force re-render.
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);

  // PTY state detector — subscribes to PTY output and detects approval/tool/thinking states
  const detectorResult = usePtyStateDetector({
    sessionId: currentSessionId,
    onStateChange: handleStateChange,
  });

  // Initialize xterm
  useEffect(() => {
    if (!termRef.current) return;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily:
        "'JetBrains Mono', 'Cascadia Code', 'Fira Code', 'Consolas', monospace",
      theme: {
        background: "#0d1117",
        foreground: "#c9d1d9",
        cursor: "#00ff41",
        cursorAccent: "#0d1117",
        selectionBackground: "#30363d",
        selectionForeground: "#c9d1d9",
        black: "#484f58",
        red: "#f85149",
        green: "#00ff41",
        yellow: "#f0b400",
        blue: "#58a6ff",
        magenta: "#bc8cff",
        cyan: "#39c5cf",
        white: "#c9d1d9",
        brightBlack: "#6e7681",
        brightRed: "#ffa198",
        brightGreen: "#56d364",
        brightYellow: "#e3b341",
        brightBlue: "#79c0ff",
        brightMagenta: "#d2a8ff",
        brightCyan: "#56d4dd",
        brightWhite: "#f0f6fc",
      },
      allowProposedApi: true,
      scrollback: 10000,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();

    term.loadAddon(fitAddon);
    term.loadAddon(webLinksAddon);
    term.open(termRef.current);

    // Initial fit
    try {
      fitAddon.fit();
    } catch {
      // Container might not be sized yet
    }

    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    // Handle user input — forward to PTY
    term.onData((data) => {
      const sid = sessionIdRef.current;
      if (sid) {
        writePty(sid, data).catch(() => {});
        // Clear approval state on any user input
        detectorResult.clearApproval();
      }
    });

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      try {
        fitAddon.fit();
      } catch {
        // ignore
      }
    });
    resizeObserver.observe(termRef.current);

    term.onResize(({ cols, rows }) => {
      const sid = sessionIdRef.current;
      if (sid) {
        resizePty(sid, cols, rows).catch(() => {});
      }
    });

    return () => {
      resizeObserver.disconnect();
      term.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Start duration timer for a tab
  const startDurationTimer = useCallback((tabId: string) => {
    // Clear any existing timer
    if (timerRef.current) {
      clearInterval(timerRef.current);
    }

    const startTime = Date.now();
    timerRef.current = setInterval(() => {
      const elapsed = Date.now() - startTime;
      useTabStore.getState().updateTabDuration(tabId, elapsed);
    }, 1000);
  }, []);

  const stopDurationTimer = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  // Start a PTY session
  const startSession = useCallback(async () => {
    const term = xtermRef.current;
    const fitAddon = fitAddonRef.current;
    if (!term || !fitAddon) return;

    // Clean up previous listeners
    for (const fn of unlistenersRef.current) {
      fn();
    }
    unlistenersRef.current = [];
    stopDurationTimer();

    setError(null);
    setShowApproval(false);
    setActivityInfo({ tool: null, file: null, state: "idle" });
    term.clear();

    try {
      fitAddon.fit();
    } catch {
      // ignore
    }

    const cols = term.cols;
    const rows = term.rows;

    // Create tab entry
    sessionCounter++;
    const tabId = `tab_${Date.now()}_${sessionCounter}`;
    tabIdRef.current = tabId;

    try {
      // Update tab to starting
      useTabStore.getState().addTab({
        id: tabId,
        ptySessionId: "",
        name: `Session ${sessionCounter}`,
        ticketId: null,
        status: "starting",
        startedAt: Date.now(),
        projectPath,
      });

      const sessionId = await createPtySession(
        projectPath,
        cols,
        rows,
        cliCommand,
        cliArgs || null,
      );

      sessionIdRef.current = sessionId;
      setCurrentSessionId(sessionId);
      setAlive(true);

      // Update tab with PTY session ID and set to running
      useTabStore.getState().updateTabStatus(tabId, "running");
      // Store the ptySessionId on the tab
      useTabStore.setState((s) => ({
        tabs: s.tabs.map((t) =>
          t.id === tabId ? { ...t, ptySessionId: sessionId } : t
        ),
      }));

      // Start tracking duration
      startDurationTimer(tabId);

      // Listen for PTY output
      const outputUnlisten = await listen<PtyOutput>("pty:output", (event) => {
        if (event.payload.session_id === sessionId) {
          term.write(event.payload.data);
        }
      });

      // Listen for PTY exit
      const exitUnlisten = await listen<string>("pty:exit", (event) => {
        if (event.payload === sessionId) {
          setAlive(false);
          setShowApproval(false);
          setCurrentSessionId(null);
          term.write("\r\n\x1b[90m[Session ended]\x1b[0m\r\n");
          useTabStore.getState().updateTabStatus(tabId, "done");
          stopDurationTimer();

          // Notify session complete
          const tab = useTabStore.getState().getTab(tabId);
          if (tab) {
            notifySessionComplete(sessionId, tab.name);
          }

          // Clear activity
          useActivityStore.getState().clearActivity(tabId);
        }
      });

      unlistenersRef.current = [outputUnlisten, exitUnlisten];

      // Do not auto-paste any initial prompt here — only the Issues panel ("Work on this issue") should inject text via packetcode:issue-prompt
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
      const label = cliCommand.charAt(0).toUpperCase() + cliCommand.slice(1);
      term.write(`\x1b[31mFailed to start ${label}: ${msg}\x1b[0m\r\n`);
      term.write(
        `\x1b[90mMake sure '${cliCommand}' is installed and on your PATH.\x1b[0m\r\n`
      );
      useTabStore.getState().updateTabStatus(tabId, "error");
      stopDurationTimer();

      // Notify session error
      notifySessionError(tabId, `Session ${sessionCounter}`);
    }
  }, [projectPath, cliCommand, cliArgs, startDurationTimer, stopDurationTimer]);

  // Auto-start session on mount
  useEffect(() => {
    // Small delay to let xterm render and get proper dimensions
    const timer = setTimeout(() => {
      startSession();
    }, 200);
    return () => clearTimeout(timer);
  }, [startSession]);

  // Listen for issue prompt events (from "Work on this issue" button)
  useEffect(() => {
    function handleIssuePrompt(e: Event) {
      const detail = (e as CustomEvent).detail;
      if (!detail?.prompt) return;
      const sid = sessionIdRef.current;
      if (!sid) return;

      // Write the prompt to the PTY session
      const prompt = detail.prompt as string;
      writePty(sid, prompt + "\n").catch(() => {});

      // Link the issue to this tab
      if (detail.issueId) {
        const tid = tabIdRef.current;
        if (tid) {
          useTabStore.getState().setTabTicket(tid, detail.issueId);
        }
      }

      // Remove the listener after handling — only the latest pane should handle it
      window.removeEventListener("packetcode:issue-prompt", handleIssuePrompt);
    }

    window.addEventListener("packetcode:issue-prompt", handleIssuePrompt);
    return () => window.removeEventListener("packetcode:issue-prompt", handleIssuePrompt);
  }, []);

  // Clean up listeners and timer on unmount
  useEffect(() => {
    return () => {
      for (const fn of unlistenersRef.current) {
        fn();
      }
      stopDurationTimer();
      // Kill the PTY session
      const sid = sessionIdRef.current;
      if (sid) {
        killPty(sid).catch(() => {});
      }
      // Remove tab and activity
      const tid = tabIdRef.current;
      if (tid) {
        useTabStore.getState().removeTab(tid);
        useActivityStore.getState().clearActivity(tid);
      }
    };
  }, [stopDurationTimer]);

  const handleKill = useCallback(async () => {
    const sid = sessionIdRef.current;
    if (sid) {
      await killPty(sid).catch(() => {});
      setAlive(false);
    }
    setShowApproval(false);
    setCurrentSessionId(null);
    stopDurationTimer();
    const tid = tabIdRef.current;
    if (tid) {
      useTabStore.getState().updateTabStatus(tid, "done");
      useActivityStore.getState().clearActivity(tid);
    }
  }, [stopDurationTimer]);

  const handleRestart = useCallback(async () => {
    // Kill existing session
    const sid = sessionIdRef.current;
    if (sid) {
      await killPty(sid).catch(() => {});
    }
    sessionIdRef.current = null;
    setAlive(false);
    setShowApproval(false);
    setCurrentSessionId(null);
    stopDurationTimer();

    // Remove old tab and activity
    const tid = tabIdRef.current;
    if (tid) {
      useTabStore.getState().removeTab(tid);
      useActivityStore.getState().clearActivity(tid);
    }
    tabIdRef.current = null;

    // Start fresh
    await startSession();
  }, [startSession, stopDurationTimer]);

  // Quick action handlers
  const handleApprove = useCallback(() => {
    const sid = sessionIdRef.current;
    if (sid) {
      writePty(sid, "y\n").catch(() => {});
    }
    setShowApproval(false);
    detectorResult.clearApproval();
  }, [detectorResult]);

  const handleDeny = useCallback(() => {
    const sid = sessionIdRef.current;
    if (sid) {
      writePty(sid, "n\n").catch(() => {});
    }
    setShowApproval(false);
    detectorResult.clearApproval();
  }, [detectorResult]);

  const handleAbort = useCallback(() => {
    const sid = sessionIdRef.current;
    if (sid) {
      writePty(sid, "\x03").catch(() => {});
    }
    setShowApproval(false);
    detectorResult.clearApproval();
  }, [detectorResult]);

  // Activity strip visibility — hide when idle for >10s
  const showActivityStrip =
    alive && activityInfo.state !== "idle" && activityInfo.tool !== null;

  return (
    <div
      className="flex flex-col h-full bg-bg-primary"
      onClick={() => setActivePaneId(paneId)}
    >
      {/* Pane header */}
      <div className="flex items-center justify-between px-3 py-1 bg-bg-secondary border-b border-bg-border">
        <div className="flex items-center gap-2">
          <div
            className={`w-2 h-2 rounded-full ${
              showApproval
                ? "bg-accent-amber animate-pulse"
                : alive
                  ? "bg-accent-green animate-pulse"
                  : error
                    ? "bg-accent-red"
                    : "bg-text-muted"
            }`}
          />
          <span
            className="text-[10px] px-2 py-0.5 rounded-full text-white font-medium"
            style={{
              backgroundColor: cliCommand === "claude" ? "#f0b400" : "#58a6ff",
            }}
          >
            {cliCommand === "claude" ? "Claude" : "Codex"}
          </span>
        </div>
        <div className="flex items-center gap-1">
          {!alive && (
            <button
              onClick={handleRestart}
              className="p-1 text-text-muted hover:text-accent-green transition-colors"
              title={`New ${cliCommand.charAt(0).toUpperCase() + cliCommand.slice(1)} session`}
            >
              <Plus size={12} />
            </button>
          )}
          {alive && (
            <button
              onClick={handleRestart}
              className="p-1 text-text-muted hover:text-text-primary transition-colors"
              title="Restart session"
            >
              <RotateCcw size={12} />
            </button>
          )}
          {showCloseButton && onClose && (
            <button
              onClick={() => {
                handleKill();
                onClose();
              }}
              className="p-1 text-text-muted hover:text-accent-red transition-colors"
              title="Close pane"
            >
              <X size={12} />
            </button>
          )}
        </div>
      </div>

      {/* Terminal */}
      <div className="relative flex-1 overflow-hidden">
        <div
          ref={termRef}
          className="h-full overflow-hidden"
          style={{ padding: "4px" }}
        />

        {/* Quick Actions overlay — shown when approval needed */}
        {showApproval && alive && (
          <div className="absolute bottom-0 left-0 right-0 flex items-center gap-2 px-3 py-1.5 bg-accent-amber/10 border-t border-accent-amber/30 backdrop-blur-sm">
            <ShieldCheck size={12} className="text-accent-amber flex-shrink-0" />
            <span className="text-[11px] text-accent-amber font-medium flex-1">
              Approval needed
            </span>
            <button
              onClick={handleApprove}
              className="flex items-center gap-1 px-2.5 py-1 text-[10px] font-medium rounded bg-accent-green/20 text-accent-green hover:bg-accent-green/30 transition-colors"
            >
              <ShieldCheck size={10} />
              Allow (y)
            </button>
            <button
              onClick={handleDeny}
              className="flex items-center gap-1 px-2.5 py-1 text-[10px] font-medium rounded bg-accent-red/20 text-accent-red hover:bg-accent-red/30 transition-colors"
            >
              <ShieldX size={10} />
              Deny (n)
            </button>
            <button
              onClick={handleAbort}
              className="flex items-center gap-1 px-2.5 py-1 text-[10px] font-medium rounded bg-text-muted/20 text-text-secondary hover:bg-text-muted/30 transition-colors"
            >
              <XCircle size={10} />
              Abort
            </button>
          </div>
        )}
      </div>

      {/* Activity Strip */}
      {showActivityStrip && (
        <div className="flex items-center gap-1.5 h-5 px-3 bg-bg-secondary border-t border-bg-border/50 text-[10px] text-text-muted">
          <ActivityIcon state={activityInfo.state} tool={activityInfo.tool} />
          <span className="truncate">
            {getActivityLabel(activityInfo.state, activityInfo.tool, activityInfo.file)}
          </span>
        </div>
      )}

      {/* Thinking indicator (when no specific tool) */}
      {alive && activityInfo.state === "thinking" && !activityInfo.tool && (
        <div className="flex items-center gap-1.5 h-5 px-3 bg-bg-secondary border-t border-bg-border/50 text-[10px] text-text-muted">
          <Brain size={10} className="text-accent-blue animate-pulse" />
          <span>Thinking...</span>
        </div>
      )}

      {/* Status Bars */}
      {cliCommand === "claude" && alive && (
        <ClaudeStatusBar projectPath={projectPath} />
      )}
      {cliCommand === "codex" && alive && (
        <CodexStatusBar projectPath={projectPath} />
      )}
    </div>
  );
}

function ActivityIcon({
  state,
  tool,
}: {
  state: string;
  tool: string | null;
}) {
  if (state === "thinking") {
    return <Brain size={10} className="text-accent-blue animate-pulse flex-shrink-0" />;
  }
  if (tool === "Edit" || tool === "Write") {
    return <FileEdit size={10} className="text-accent-amber flex-shrink-0" />;
  }
  if (tool === "Bash") {
    return <TerminalSquare size={10} className="text-accent-green flex-shrink-0" />;
  }
  return <FileEdit size={10} className="text-text-muted flex-shrink-0" />;
}

function getActivityLabel(
  state: string,
  tool: string | null,
  file: string | null
): string {
  if (state === "thinking") return "Thinking...";

  if (!tool) return "";

  const shortFile = file
    ? file.length > 50
      ? "..." + file.slice(-47)
      : file
    : "";

  switch (tool) {
    case "Edit":
      return `Editing ${shortFile}`;
    case "Write":
      return `Writing ${shortFile}`;
    case "Read":
      return `Reading ${shortFile}`;
    case "Bash":
      return `Running: ${shortFile}`;
    case "Glob":
      return `Searching: ${shortFile}`;
    case "Grep":
      return `Searching: ${shortFile}`;
    case "Task":
      return `Running task: ${shortFile}`;
    default:
      return `${tool} ${shortFile}`;
  }
}

import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { X, RotateCcw, Plus } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { useTabStore } from "@/stores/tabStore";
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
  initialPrompt,
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

  const projectPath = useLayoutStore((s) => s.projectPath);
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId);

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
        invoke("write_pty", { sessionId: sid, data }).catch((err) => {
          console.error("write_pty error:", err);
        });
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
        invoke("resize_pty", { sessionId: sid, cols, rows }).catch(() => {});
      }
    });

    return () => {
      resizeObserver.disconnect();
      term.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, []);

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

      const sessionId = await invoke<string>("create_pty_session", {
        projectPath,
        cols,
        rows,
        command: cliCommand,
        args: cliArgs || null,
      });

      sessionIdRef.current = sessionId;
      setAlive(true);

      // Update tab with PTY session ID and set to running
      useTabStore.getState().updateTabStatus(tabId, "running");
      // Store the ptySessionId on the tab — need a small helper
      const tabStore = useTabStore.getState();
      const currentTab = tabStore.tabs.find((t) => t.id === tabId);
      if (currentTab) {
        // Update ptySessionId directly
        useTabStore.setState((s) => ({
          tabs: s.tabs.map((t) =>
            t.id === tabId ? { ...t, ptySessionId: sessionId } : t
          ),
        }));
      }

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
          term.write("\r\n\x1b[90m[Session ended]\x1b[0m\r\n");
          useTabStore.getState().updateTabStatus(tabId, "done");
          stopDurationTimer();
        }
      });

      unlistenersRef.current = [outputUnlisten, exitUnlisten];

      // If there's an initial prompt, send it after the CLI starts
      if (initialPrompt) {
        setTimeout(() => {
          invoke("write_pty", { sessionId, data: initialPrompt + "\n" }).catch(() => {});
        }, 1200);
      }
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
    }
  }, [projectPath, cliCommand, cliArgs, initialPrompt, startDurationTimer, stopDurationTimer]);

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
      invoke("write_pty", { sessionId: sid, data: prompt + "\n" }).catch(() => {});

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
        invoke("kill_pty", { sessionId: sid }).catch(() => {});
      }
      // Remove tab
      const tid = tabIdRef.current;
      if (tid) {
        useTabStore.getState().removeTab(tid);
      }
    };
  }, [stopDurationTimer]);

  const handleKill = useCallback(async () => {
    const sid = sessionIdRef.current;
    if (sid) {
      await invoke("kill_pty", { sessionId: sid }).catch(() => {});
      setAlive(false);
    }
    stopDurationTimer();
    const tid = tabIdRef.current;
    if (tid) {
      useTabStore.getState().updateTabStatus(tid, "done");
    }
  }, [stopDurationTimer]);

  const handleRestart = useCallback(async () => {
    // Kill existing session
    const sid = sessionIdRef.current;
    if (sid) {
      await invoke("kill_pty", { sessionId: sid }).catch(() => {});
    }
    sessionIdRef.current = null;
    setAlive(false);
    stopDurationTimer();

    // Remove old tab
    const tid = tabIdRef.current;
    if (tid) {
      useTabStore.getState().removeTab(tid);
    }
    tabIdRef.current = null;

    // Start fresh
    await startSession();
  }, [startSession, stopDurationTimer]);

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
              alive
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
      <div
        ref={termRef}
        className="flex-1 overflow-hidden"
        style={{ padding: "4px" }}
      />

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

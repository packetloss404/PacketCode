import { useEffect, useRef, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import "@xterm/xterm/css/xterm.css";

interface PtyOutput {
  session_id: string;
  data: string;
}

interface DeployTerminalProps {
  sessionId: string | null;
  onExit?: (code: number) => void;
}

export function DeployTerminal({ sessionId, onExit }: DeployTerminalProps) {
  const termRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);

  const handleResize = useCallback(() => {
    fitAddonRef.current?.fit();
  }, []);

  useEffect(() => {
    if (!termRef.current) return;

    const xterm = new Terminal({
      theme: {
        background: "#0d1117",
        foreground: "#c9d1d9",
        cursor: "#58a6ff",
        selectionBackground: "#264f78",
        black: "#484f58",
        red: "#ff7b72",
        green: "#3fb950",
        yellow: "#d29922",
        blue: "#58a6ff",
        magenta: "#bc8cff",
        cyan: "#39d353",
        white: "#b1bac4",
      },
      fontSize: 12,
      fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace",
      cursorBlink: false,
      disableStdin: true,
      scrollback: 5000,
    });

    const fitAddon = new FitAddon();
    xterm.loadAddon(fitAddon);
    xterm.open(termRef.current);

    // Delay initial fit to ensure container is rendered
    requestAnimationFrame(() => fitAddon.fit());

    xtermRef.current = xterm;
    fitAddonRef.current = fitAddon;

    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      xterm.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, [handleResize]);

  // Listen for PTY output
  useEffect(() => {
    if (!sessionId) return;

    let unlisten: UnlistenFn | null = null;

    listen<PtyOutput>("pty:output", (event) => {
      if (event.payload.session_id === sessionId && xtermRef.current) {
        xtermRef.current.write(event.payload.data);
      }
    }).then((u) => {
      unlisten = u;
    });

    // Listen for exit
    let unlistenExit: UnlistenFn | null = null;
    listen<{ session_id: string; code: number }>("pty:exit", (event) => {
      if (event.payload.session_id === sessionId) {
        onExit?.(event.payload.code);
      }
    }).then((u) => {
      unlistenExit = u;
    });

    return () => {
      unlisten?.();
      unlistenExit?.();
    };
  }, [sessionId, onExit]);

  return (
    <div
      ref={termRef}
      className="flex-1 min-h-0 bg-[#0d1117] rounded-lg overflow-hidden"
    />
  );
}

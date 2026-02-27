/* eslint-disable no-control-regex */
import { useEffect, useRef, useCallback } from "react";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";

// --- ANSI strip ---
const ANSI_RE =
  /\x1B\[[0-9;]*[A-Za-z]|\x1B\].*?\x07|\x1B[()][A-Z0-9]|\x1B[>=<]|\x0F|\x0E/g;

function stripAnsi(str: string): string {
  return str.replace(ANSI_RE, "");
}

// --- Approval detection patterns ---
const APPROVAL_PATTERNS = [
  /Allow\s+\w+.*\?/i,
  /\(y\/n\)/i,
  /Do you want to (?:proceed|continue|allow)/i,
  /Press\s+y\s+to\s+(?:approve|allow|confirm)/i,
  /\[Y\/n\]/i,
  /\[y\/N\]/i,
  /Allow once|Allow always|Deny/i,
];

// --- Tool use detection patterns ---
const TOOL_PATTERNS: { pattern: RegExp; tool: string; fileGroup?: number }[] = [
  { pattern: /⏺\s*Read\(([^)]+)\)/i, tool: "Read", fileGroup: 1 },
  { pattern: /⏺\s*Edit\(([^)]+)\)/i, tool: "Edit", fileGroup: 1 },
  { pattern: /⏺\s*Write\(([^)]+)\)/i, tool: "Write", fileGroup: 1 },
  { pattern: /⏺\s*Bash\(([^)]*)\)/i, tool: "Bash", fileGroup: 1 },
  { pattern: /⏺\s*Glob\(([^)]*)\)/i, tool: "Glob", fileGroup: 1 },
  { pattern: /⏺\s*Grep\(([^)]*)\)/i, tool: "Grep", fileGroup: 1 },
  { pattern: /⏺\s*Task\(([^)]*)\)/i, tool: "Task", fileGroup: 1 },
  { pattern: /Reading\s+(.+)/i, tool: "Read", fileGroup: 1 },
  { pattern: /Editing\s+(.+)/i, tool: "Edit", fileGroup: 1 },
  { pattern: /Writing\s+(.+)/i, tool: "Write", fileGroup: 1 },
  { pattern: /Running\s+(.+)/i, tool: "Bash", fileGroup: 1 },
];

// --- Thinking / idle patterns ---
const THINKING_PATTERN = /⏺\s*Thinking|thinking\.\.\./i;
const IDLE_PATTERN = /^\s*[>❯]\s*$/m;

export interface PtyDetectorState {
  needsApproval: boolean;
  currentTool: string | null;
  currentFile: string | null;
  agentState: "idle" | "thinking" | "tool_use" | "responding";
  lastActivityAt: number;
}

const INITIAL_STATE: PtyDetectorState = {
  needsApproval: false,
  currentTool: null,
  currentFile: null,
  agentState: "idle",
  lastActivityAt: 0,
};

const MAX_BUFFER_SIZE = 4096;

interface UsePtyStateDetectorOpts {
  sessionId: string | null;
  onStateChange?: (prev: PtyDetectorState, next: PtyDetectorState) => void;
}

export function usePtyStateDetector({
  sessionId,
  onStateChange,
}: UsePtyStateDetectorOpts) {
  const stateRef = useRef<PtyDetectorState>({ ...INITIAL_STATE });
  const bufferRef = useRef("");
  const onStateChangeRef = useRef(onStateChange);
  onStateChangeRef.current = onStateChange;

  const updateState = useCallback((partial: Partial<PtyDetectorState>) => {
    const prev = { ...stateRef.current };
    const next = { ...stateRef.current, ...partial };

    // Only fire callback if something actually changed
    if (
      prev.needsApproval !== next.needsApproval ||
      prev.currentTool !== next.currentTool ||
      prev.currentFile !== next.currentFile ||
      prev.agentState !== next.agentState
    ) {
      stateRef.current = next;
      onStateChangeRef.current?.(prev, next);
    } else {
      stateRef.current = next;
    }
  }, []);

  // Process incoming PTY data
  const processData = useCallback(
    (data: string) => {
      const stripped = stripAnsi(data);
      // Append to rolling buffer
      bufferRef.current += stripped;
      if (bufferRef.current.length > MAX_BUFFER_SIZE) {
        bufferRef.current = bufferRef.current.slice(-MAX_BUFFER_SIZE);
      }

      const now = Date.now();
      // Check only the last ~1KB for patterns (recent output)
      const recent = bufferRef.current.slice(-1024);
      const lines = recent.split("\n");
      const lastLines = lines.slice(-8);
      const lastChunk = lastLines.join("\n");

      // 1. Approval detection — check last few lines
      let needsApproval = false;
      for (const pat of APPROVAL_PATTERNS) {
        if (pat.test(lastChunk)) {
          needsApproval = true;
          break;
        }
      }

      // 2. Tool use detection — scan recent lines in reverse for latest tool
      let currentTool: string | null = null;
      let currentFile: string | null = null;
      for (let i = lastLines.length - 1; i >= 0; i--) {
        const line = lastLines[i];
        for (const { pattern, tool, fileGroup } of TOOL_PATTERNS) {
          const m = line.match(pattern);
          if (m) {
            currentTool = tool;
            currentFile = fileGroup && m[fileGroup] ? m[fileGroup].trim() : null;
            break;
          }
        }
        if (currentTool) break;
      }

      // 3. Agent state
      let agentState: PtyDetectorState["agentState"] = "responding";
      if (THINKING_PATTERN.test(lastChunk)) {
        agentState = "thinking";
      } else if (currentTool) {
        agentState = "tool_use";
      } else if (IDLE_PATTERN.test(lastChunk)) {
        agentState = "idle";
      }

      updateState({
        needsApproval,
        currentTool,
        currentFile,
        agentState,
        lastActivityAt: now,
      });
    },
    [updateState]
  );

  // Clear approval state when user responds (data written to PTY)
  const clearApproval = useCallback(() => {
    if (stateRef.current.needsApproval) {
      updateState({ needsApproval: false });
    }
  }, [updateState]);

  // Reset on session change
  const reset = useCallback(() => {
    bufferRef.current = "";
    stateRef.current = { ...INITIAL_STATE };
  }, []);

  // Listen to PTY output
  useEffect(() => {
    if (!sessionId) return;

    let unlisten: UnlistenFn | null = null;
    let mounted = true;

    listen<{ session_id: string; data: string }>("pty:output", (event) => {
      if (!mounted) return;
      if (event.payload.session_id === sessionId) {
        processData(event.payload.data);
      }
    }).then((fn) => {
      if (mounted) {
        unlisten = fn;
      } else {
        fn();
      }
    });

    return () => {
      mounted = false;
      unlisten?.();
      reset();
    };
  }, [sessionId, processData, reset]);

  // Auto-clear idle after 10s of no activity
  useEffect(() => {
    const interval = setInterval(() => {
      const st = stateRef.current;
      if (
        st.lastActivityAt > 0 &&
        Date.now() - st.lastActivityAt > 10_000 &&
        st.agentState !== "idle" &&
        !st.needsApproval
      ) {
        updateState({
          currentTool: null,
          currentFile: null,
          agentState: "idle",
        });
      }
    }, 2000);
    return () => clearInterval(interval);
  }, [updateState]);

  return { stateRef, clearApproval, reset };
}

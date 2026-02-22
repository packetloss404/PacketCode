import { useEffect, useRef } from "react";
import { readCodexStatusLineStates } from "@/lib/tauri";
import { useCodexStatusLineStore, useCodexStatusLineForCwd } from "@/stores/codexStatusLineStore";

const POLL_INTERVAL_MS = 2000;

/**
 * Polls readCodexStatusLineStates every 2s and updates the store.
 * Call once at the app root level.
 */
export function useCodexStatusLinePoller() {
  const update = useCodexStatusLineStore((s) => s.update);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    async function poll() {
      try {
        const states = await readCodexStatusLineStates();
        if (states.length > 0) {
          update(states);
        }
      } catch {
        // Silently ignore polling errors
      }
    }

    // Initial poll
    poll();

    intervalRef.current = setInterval(poll, POLL_INTERVAL_MS);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [update]);
}

export { useCodexStatusLineForCwd };

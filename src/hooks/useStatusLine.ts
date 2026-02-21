import { useEffect, useRef } from "react";
import { readStatusLineStates } from "@/lib/tauri";
import { useStatusLineStore, useStatusLineForCwd } from "@/stores/statusLineStore";

const POLL_INTERVAL_MS = 2000;

/**
 * Polls readStatusLineStates every 2s and updates the store.
 * Call once at the app root level.
 */
export function useStatusLinePoller() {
  const update = useStatusLineStore((s) => s.update);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    async function poll() {
      try {
        const states = await readStatusLineStates();
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

export { useStatusLineForCwd };

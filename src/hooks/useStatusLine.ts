import { readStatusLineStates } from "@/lib/tauri";
import { useStatusLineStore, useStatusLineForCwd } from "@/stores/statusLineStore";
import { useStatusLinePollerBase } from "@/hooks/useStatusLinePollerBase";

const POLL_INTERVAL_MS = 5000;

/**
 * Polls readStatusLineStates every 5s and updates the store.
 * Call once at the app root level.
 */
export function useStatusLinePoller() {
  const update = useStatusLineStore((s) => s.update);
  useStatusLinePollerBase({
    read: readStatusLineStates,
    update,
    intervalMs: POLL_INTERVAL_MS,
  });
}

export { useStatusLineForCwd };

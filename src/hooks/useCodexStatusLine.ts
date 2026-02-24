import { readCodexStatusLineStates } from "@/lib/tauri";
import { useCodexStatusLineStore, useCodexStatusLineForCwd } from "@/stores/codexStatusLineStore";
import { useStatusLinePollerBase } from "@/hooks/useStatusLinePollerBase";

const POLL_INTERVAL_MS = 5000;

/**
 * Polls readCodexStatusLineStates every 5s and updates the store.
 * Call once at the app root level.
 */
export function useCodexStatusLinePoller() {
  const update = useCodexStatusLineStore((s) => s.update);
  useStatusLinePollerBase({
    read: readCodexStatusLineStates,
    update,
    intervalMs: POLL_INTERVAL_MS,
  });
}

export { useCodexStatusLineForCwd };

import { readStatusLineStates, readCodexStatusLineStates } from "@/lib/tauri";
import {
  useStatusLineStore,
  useStatusLineForCwd,
  useCodexStatusLineStore,
  useCodexStatusLineForCwd,
} from "@/stores/statusLineStore";
import { useStatusLinePollerBase } from "@/hooks/useStatusLinePollerBase";

const POLL_INTERVAL_MS = 5000;

export function useStatusLinePoller() {
  const update = useStatusLineStore((s) => s.update);
  useStatusLinePollerBase({ read: readStatusLineStates, update, intervalMs: POLL_INTERVAL_MS });
}

export function useCodexStatusLinePoller() {
  const update = useCodexStatusLineStore((s) => s.update);
  useStatusLinePollerBase({ read: readCodexStatusLineStates, update, intervalMs: POLL_INTERVAL_MS });
}

export { useStatusLineForCwd, useCodexStatusLineForCwd };

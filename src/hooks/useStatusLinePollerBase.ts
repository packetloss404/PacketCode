import { useEffect, useRef } from "react";

const DEFAULT_POLL_INTERVAL_MS = 5000;

interface UseStatusLinePollerBaseOptions<T> {
  read: () => Promise<T[]>;
  update: (entries: T[]) => void;
  intervalMs?: number;
}

export function useStatusLinePollerBase<T>({
  read,
  update,
  intervalMs = DEFAULT_POLL_INTERVAL_MS,
}: UseStatusLinePollerBaseOptions<T>) {
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    async function poll() {
      try {
        const states = await read();
        update(states);
      } catch {
        // Silently ignore polling errors.
      }
    }

    poll();
    intervalRef.current = setInterval(poll, intervalMs);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [intervalMs, read, update]);
}


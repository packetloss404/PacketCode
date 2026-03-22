import { devLog } from "./env";

/**
 * Report an error with context. Logs in dev, could forward to external
 * service in the future.
 */
export function reportError(error: unknown, context?: string): void {
  const message = error instanceof Error ? error.message : String(error);
  const stack = error instanceof Error ? error.stack : undefined;

  devLog(`[error] ${context ? context + ": " : ""}${message}`);
  if (stack) {
    devLog(stack);
  }

  // Future: forward to Sentry, PostHog, etc.
}

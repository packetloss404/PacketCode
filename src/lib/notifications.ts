import { useNotificationStore } from "@/stores/notificationStore";

// Per-session debounce tracking (5s window)
const lastNotificationTime: Record<string, number> = {};
const DEBOUNCE_MS = 5000;

function shouldNotify(sessionId: string): boolean {
  const prefs = useNotificationStore.getState();
  if (!prefs.enabled) return false;
  if (prefs.onlyWhenUnfocused && document.hasFocus()) return false;

  const now = Date.now();
  const last = lastNotificationTime[sessionId] ?? 0;
  if (now - last < DEBOUNCE_MS) return false;

  lastNotificationTime[sessionId] = now;
  return true;
}

async function ensurePermission(): Promise<boolean> {
  if (!("Notification" in window)) return false;
  if (Notification.permission === "granted") return true;
  if (Notification.permission === "denied") return false;
  const result = await Notification.requestPermission();
  return result === "granted";
}

export async function notifyApprovalNeeded(
  sessionId: string,
  sessionName: string
) {
  if (!useNotificationStore.getState().onApprovalNeeded) return;
  if (!shouldNotify(sessionId)) return;
  if (!(await ensurePermission())) return;

  new Notification("Approval Needed", {
    body: `${sessionName} is waiting for your approval`,
    tag: `approval-${sessionId}`,
  });
}

export async function notifySessionComplete(
  sessionId: string,
  sessionName: string
) {
  if (!useNotificationStore.getState().onSessionComplete) return;
  if (!shouldNotify(sessionId)) return;
  if (!(await ensurePermission())) return;

  new Notification("Session Complete", {
    body: `${sessionName} has finished`,
    tag: `complete-${sessionId}`,
  });
}

export async function notifySessionError(
  sessionId: string,
  sessionName: string
) {
  if (!useNotificationStore.getState().onSessionError) return;
  if (!shouldNotify(sessionId)) return;
  if (!(await ensurePermission())) return;

  new Notification("Session Error", {
    body: `${sessionName} encountered an error`,
    tag: `error-${sessionId}`,
  });
}

export async function requestNotificationPermission(): Promise<boolean> {
  return ensurePermission();
}

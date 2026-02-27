import { Bell } from "lucide-react";
import { useNotificationStore } from "@/stores/notificationStore";
import { requestNotificationPermission } from "@/lib/notifications";

export function NotificationSettingsCard() {
  const enabled = useNotificationStore((s) => s.enabled);
  const onlyWhenUnfocused = useNotificationStore((s) => s.onlyWhenUnfocused);
  const onApprovalNeeded = useNotificationStore((s) => s.onApprovalNeeded);
  const onSessionComplete = useNotificationStore((s) => s.onSessionComplete);
  const onSessionError = useNotificationStore((s) => s.onSessionError);

  const setEnabled = useNotificationStore((s) => s.setEnabled);
  const setOnlyWhenUnfocused = useNotificationStore((s) => s.setOnlyWhenUnfocused);
  const setOnApprovalNeeded = useNotificationStore((s) => s.setOnApprovalNeeded);
  const setOnSessionComplete = useNotificationStore((s) => s.setOnSessionComplete);
  const setOnSessionError = useNotificationStore((s) => s.setOnSessionError);

  const handleEnableToggle = async (checked: boolean) => {
    if (checked) {
      const granted = await requestNotificationPermission();
      if (!granted) return;
    }
    setEnabled(checked);
  };

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
      <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
        <Bell size={12} className="text-accent-amber" />
        Notifications
      </h3>

      <div className="space-y-2">
        <Toggle label="Enable notifications" checked={enabled} onChange={handleEnableToggle} />
        {enabled && (
          <>
            <Toggle
              label="Only when app unfocused"
              checked={onlyWhenUnfocused}
              onChange={setOnlyWhenUnfocused}
            />
            <div className="border-t border-bg-border/50 pt-2 mt-2">
              <p className="text-[9px] text-text-muted uppercase tracking-wider mb-1.5">
                Events
              </p>
              <Toggle label="Approval needed" checked={onApprovalNeeded} onChange={setOnApprovalNeeded} />
              <Toggle label="Session complete" checked={onSessionComplete} onChange={setOnSessionComplete} />
              <Toggle label="Session error" checked={onSessionError} onChange={setOnSessionError} />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <label className="flex items-center justify-between cursor-pointer group">
      <span className="text-[11px] text-text-secondary group-hover:text-text-primary transition-colors">
        {label}
      </span>
      <div
        className={`relative w-7 h-4 rounded-full transition-colors ${
          checked ? "bg-accent-green" : "bg-bg-elevated"
        }`}
        onClick={() => onChange(!checked)}
      >
        <div
          className={`absolute top-0.5 w-3 h-3 rounded-full bg-white transition-transform ${
            checked ? "translate-x-3.5" : "translate-x-0.5"
          }`}
        />
      </div>
    </label>
  );
}

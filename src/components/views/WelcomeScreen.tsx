import { Columns2 } from "lucide-react";

export function WelcomeScreen() {
  return (
    <div className="flex flex-col items-center justify-center h-full bg-bg-primary select-none">
      <Columns2 size={64} className="text-accent-green mb-4" strokeWidth={1.5} />
      <h1 className="text-2xl font-semibold text-text-primary mb-2">
        PacketCode
      </h1>
      <p className="text-sm text-text-secondary mb-8">
        Click <span className="text-text-primary font-medium">Claude</span> or{" "}
        <span className="text-text-primary font-medium">Codex</span> above to
        start a session
      </p>

      <div className="flex flex-col gap-1.5 text-[11px] text-text-muted">
        <div className="flex items-center gap-3">
          <kbd className="px-1.5 py-0.5 bg-bg-elevated border border-bg-border rounded text-[10px]">
            Ctrl+Shift+1
          </kbd>
          <span>Claude</span>
        </div>
        <div className="flex items-center gap-3">
          <kbd className="px-1.5 py-0.5 bg-bg-elevated border border-bg-border rounded text-[10px]">
            Ctrl+Shift+2
          </kbd>
          <span>Codex</span>
        </div>
        <div className="flex items-center gap-3">
          <kbd className="px-1.5 py-0.5 bg-bg-elevated border border-bg-border rounded text-[10px]">
            Ctrl+\
          </kbd>
          <span>Split pane</span>
        </div>
      </div>
    </div>
  );
}

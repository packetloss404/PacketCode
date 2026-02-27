import { FolderPlus } from "lucide-react";
import { useAppStore, moduleViewId } from "@/stores/appStore";
import { useModuleStore } from "@/stores/moduleStore";

export function WelcomeScreen() {
  const setActiveView = useAppStore((s) => s.setActiveView);
  const scaffoldEnabled = useModuleStore((s) => s.isEnabled("scaffold"));

  return (
    <div className="flex flex-col items-center justify-center h-full bg-bg-primary select-none">
      <img src="/favicon.png" alt="PacketCode" className="w-16 h-16 mb-4" />
      <h1 className="text-2xl font-semibold text-text-primary mb-2">
        PacketCode
      </h1>
      <p className="text-sm text-text-secondary mb-6">
        Click <span className="text-text-primary font-medium">Claude</span> or{" "}
        <span className="text-text-primary font-medium">Codex</span> above to
        start a session
      </p>

      {scaffoldEnabled && (
        <button
          onClick={() => setActiveView(moduleViewId("scaffold"))}
          className="flex items-center gap-2 px-4 py-2 mb-6 bg-accent-green/20 text-accent-green text-xs font-medium rounded-lg hover:bg-accent-green/30 transition-colors"
        >
          <FolderPlus size={14} />
          New Project
        </button>
      )}

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

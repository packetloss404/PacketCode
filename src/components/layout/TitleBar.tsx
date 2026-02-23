import { useCallback, useEffect } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { Minus, Square, Copy, X } from "lucide-react";
import { useAppStore } from "@/stores/appStore";

export function TitleBar() {
  const isMaximized = useAppStore((s) => s.isMaximized);
  const setIsMaximized = useAppStore((s) => s.setIsMaximized);

  // Sync maximized state on mount
  useEffect(() => {
    getCurrentWindow()
      .isMaximized()
      .then(setIsMaximized)
      .catch(() => {});
  }, [setIsMaximized]);

  const handleMinimize = useCallback(() => {
    getCurrentWindow().minimize();
  }, []);

  const handleMaximize = useCallback(async () => {
    const win = getCurrentWindow();
    const maximized = await win.isMaximized();
    if (maximized) {
      await win.unmaximize();
      setIsMaximized(false);
    } else {
      await win.maximize();
      setIsMaximized(true);
    }
  }, [setIsMaximized]);

  const handleClose = useCallback(() => {
    getCurrentWindow().close();
  }, []);

  return (
    <div className="flex items-center h-8 bg-bg-secondary border-b border-bg-border select-none">
      {/* Drag region — data-tauri-drag-region handles dragging natively */}
      <div
        className="flex-1 h-full flex items-center px-3"
        data-tauri-drag-region
      >
        {/* pointer-events-none so children don't swallow the drag */}
        <div className="flex items-center gap-2 pointer-events-none">
          <img src="/favicon.png" alt="PacketCode" className="w-4 h-4" />
          <span className="text-text-primary text-xs font-semibold tracking-wide">
            PacketCode
          </span>
        </div>
      </div>

      {/* Window controls */}
      <div className="flex h-full">
        <button
          onClick={handleMinimize}
          className="flex items-center justify-center w-12 h-full hover:bg-bg-hover transition-colors"
          title="Minimize"
        >
          <Minus size={14} className="text-text-secondary" />
        </button>
        <button
          onClick={handleMaximize}
          className="flex items-center justify-center w-12 h-full hover:bg-bg-hover transition-colors"
          title={isMaximized ? "Restore" : "Maximize"}
        >
          {isMaximized ? (
            <Copy size={11} className="text-text-secondary" />
          ) : (
            <Square size={11} className="text-text-secondary" />
          )}
        </button>
        <button
          onClick={handleClose}
          className="flex items-center justify-center w-12 h-full hover:bg-accent-red/80 transition-colors group"
          title="Close"
        >
          <X size={14} className="text-text-secondary group-hover:text-white" />
        </button>
      </div>
    </div>
  );
}

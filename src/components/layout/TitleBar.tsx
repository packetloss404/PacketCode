import { useCallback } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { Minus, Square, X, Columns2 } from "lucide-react";
import { useAppStore } from "@/stores/appStore";

const appWindow = getCurrentWindow();

export function TitleBar() {
  const isMaximized = useAppStore((s) => s.isMaximized);
  const setIsMaximized = useAppStore((s) => s.setIsMaximized);

  const handleMinimize = useCallback(() => {
    appWindow.minimize();
  }, []);

  const handleMaximize = useCallback(async () => {
    const maximized = await appWindow.isMaximized();
    if (maximized) {
      await appWindow.unmaximize();
      setIsMaximized(false);
    } else {
      await appWindow.maximize();
      setIsMaximized(true);
    }
  }, []);

  const handleClose = useCallback(() => {
    appWindow.close();
  }, []);

  return (
    <div className="flex items-center h-8 bg-bg-secondary border-b border-bg-border select-none">
      {/* Drag region */}
      <div
        className="flex-1 h-full flex items-center px-3"
        data-tauri-drag-region
        onMouseDown={() => appWindow.startDragging()}
      >
        <div className="flex items-center gap-2">
          <Columns2 size={14} className="text-accent-green" />
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
        >
          <Minus size={14} className="text-text-secondary" />
        </button>
        <button
          onClick={handleMaximize}
          className="flex items-center justify-center w-12 h-full hover:bg-bg-hover transition-colors"
        >
          <Square size={11} className="text-text-secondary" />
        </button>
        <button
          onClick={handleClose}
          className="flex items-center justify-center w-12 h-full hover:bg-accent-red/80 transition-colors group"
        >
          <X size={14} className="text-text-secondary group-hover:text-white" />
        </button>
      </div>
    </div>
  );
}

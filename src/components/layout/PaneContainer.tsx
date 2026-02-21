import { useMemo } from "react";
import { TerminalPane } from "@/components/session/TerminalPane";
import { SessionTabBar } from "@/components/layout/SessionTabBar";
import { useLayoutStore } from "@/stores/layoutStore";
import type { PaneConfig } from "@/types/layout";

interface PaneContainerProps {
  cliCommand?: string;
  paneSource?: "claude" | "codex";
}

/**
 * Flexible grid layout for terminal panes.
 * Sessions auto-arrange: 1 = full, 2 = side-by-side, 3 = row,
 * 4+ = wrapping grid. Closing a pane makes others expand to fill space.
 */
export function PaneContainer({
  cliCommand = "claude",
  paneSource = "claude",
}: PaneContainerProps) {
  const claudePanes = useLayoutStore((s) => s.panes);
  const codexPanes = useLayoutStore((s) => s.codexPanes);
  const removePane = useLayoutStore((s) => s.removePane);
  const removeCodexPane = useLayoutStore((s) => s.removeCodexPane);

  const panes: PaneConfig[] = paneSource === "codex" ? codexPanes : claudePanes;
  const removeFn = paneSource === "codex" ? removeCodexPane : removePane;

  // Calculate grid columns based on pane count
  const gridStyle = useMemo(() => {
    const count = panes.length;
    if (count <= 1) {
      return { gridTemplateColumns: "1fr" };
    }
    if (count === 2) {
      return { gridTemplateColumns: "1fr 1fr" };
    }
    if (count === 3) {
      return { gridTemplateColumns: "1fr 1fr 1fr" };
    }
    if (count <= 6) {
      // 2 rows: e.g. 4 → 2x2, 5 → 3+2, 6 → 3x2
      const cols = Math.ceil(count / 2);
      return { gridTemplateColumns: `repeat(${cols}, 1fr)` };
    }
    if (count <= 9) {
      // 3 rows
      const cols = Math.ceil(count / 3);
      return { gridTemplateColumns: `repeat(${cols}, 1fr)` };
    }
    // 10+: 4+ columns auto-fill
    const cols = Math.ceil(count / Math.ceil(count / 4));
    return { gridTemplateColumns: `repeat(${cols}, 1fr)` };
  }, [panes.length]);

  if (panes.length === 0) {
    return (
      <div className="flex flex-col flex-1 overflow-hidden">
        <SessionTabBar cliType={paneSource} />
        <div className="flex-1 flex items-center justify-center bg-bg-primary">
          <p className="text-sm text-text-muted">
            Click <span className="text-text-secondary font-medium">+</span> above to start a {paneSource === "codex" ? "Codex" : "Claude"} session
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      {/* Session tab bar */}
      <SessionTabBar cliType={paneSource} />

      {/* Grid of panes */}
      <div
        className="flex-1 grid gap-[2px] bg-bg-border overflow-hidden"
        style={gridStyle}
      >
        {panes.map((pane) => (
          <div key={pane.id} className="min-h-0 min-w-0 overflow-hidden">
            <TerminalPane
              paneId={pane.id}
              onClose={() => removeFn(pane.id)}
              showCloseButton={panes.length > 1}
              cliCommand={cliCommand}
              cliArgs={pane.cliArgs}
              initialPrompt={pane.initialPrompt}
            />
          </div>
        ))}
      </div>
    </div>
  );
}

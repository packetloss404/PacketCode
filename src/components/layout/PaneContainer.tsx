import { Fragment, useCallback, useRef, useState, useMemo, useEffect } from "react";
import { TerminalPane } from "@/components/session/TerminalPane";
import { useLayoutStore } from "@/stores/layoutStore";
import type { PaneConfig } from "@/types/layout";

/**
 * Unified resizable pane container for all CLI sessions (Claude + Codex).
 * Panes arranged in a grid: 1-2 = single row, 3+ = rows of 2.
 * Drag handles between columns (col-resize) and rows (row-resize).
 */
export function PaneContainer() {
  const panes = useLayoutStore((s) => s.panes);
  const removePane = useLayoutStore((s) => s.removePane);
  const updatePaneSize = useLayoutStore((s) => s.updatePaneSize);

  const containerRef = useRef<HTMLDivElement>(null);

  // Group panes into rows (max 2 per row when 3+ panes)
  const rows = useMemo((): PaneConfig[][] => {
    if (panes.length <= 2) return [panes];
    const result: PaneConfig[][] = [];
    for (let i = 0; i < panes.length; i += 2) {
      result.push(panes.slice(i, i + 2));
    }
    return result;
  }, [panes]);

  // Row flex heights (local state)
  const [rowHeights, setRowHeights] = useState<number[]>(() => rows.map(() => 1));
  const rowHeightsRef = useRef(rowHeights);
  rowHeightsRef.current = rowHeights;

  // Sync row heights with row count
  useEffect(() => {
    setRowHeights((prev) => {
      if (prev.length === rows.length) return prev;
      const next: number[] = [];
      for (let i = 0; i < rows.length; i++) {
        next.push(prev[i] ?? 1);
      }
      return next;
    });
  }, [rows.length]);

  // Column resize (horizontal width)
  const handleColResizeStart = useCallback(
    (e: React.MouseEvent, row: PaneConfig[], leftIdx: number) => {
      e.preventDefault();
      const container = containerRef.current;
      if (!container) return;

      const leftPane = row[leftIdx];
      const rightPane = row[leftIdx + 1];
      if (!leftPane || !rightPane) return;

      const leftFlex = leftPane.flexSize ?? 1;
      const rightFlex = rightPane.flexSize ?? 1;
      const rowTotalFlex = row.reduce((sum, p) => sum + (p.flexSize ?? 1), 0);
      const handleWidth = 4 * (row.length - 1);
      const containerWidth = container.getBoundingClientRect().width - handleWidth;
      const pxPerFlex = containerWidth / rowTotalFlex;
      const startX = e.clientX;

      function onMouseMove(ev: MouseEvent) {
        const dx = ev.clientX - startX;
        const dFlex = dx / pxPerFlex;
        const newLeft = Math.max(0.1, leftFlex + dFlex);
        const newRight = Math.max(0.1, rightFlex - dFlex);
        updatePaneSize(leftPane.id, newLeft);
        updatePaneSize(rightPane.id, newRight);
      }

      function onMouseUp() {
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      }

      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },
    [updatePaneSize]
  );

  // Row resize (vertical height)
  const handleRowResizeStart = useCallback(
    (e: React.MouseEvent, topRowIndex: number) => {
      e.preventDefault();
      const container = containerRef.current;
      if (!container) return;

      const heights = rowHeightsRef.current;
      const handleHeight = 4 * (heights.length - 1);
      const containerHeight = container.getBoundingClientRect().height - handleHeight;
      const totalFlex = heights.reduce((s, h) => s + h, 0);
      const pxPerFlex = containerHeight / totalFlex;
      const startY = e.clientY;
      const topFlex = heights[topRowIndex];
      const bottomFlex = heights[topRowIndex + 1];

      function onMouseMove(ev: MouseEvent) {
        const dy = ev.clientY - startY;
        const dFlex = dy / pxPerFlex;
        const newTop = Math.max(0.1, topFlex + dFlex);
        const newBottom = Math.max(0.1, bottomFlex - dFlex);
        setRowHeights((prev) => {
          const next = [...prev];
          next[topRowIndex] = newTop;
          next[topRowIndex + 1] = newBottom;
          return next;
        });
      }

      function onMouseUp() {
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      }

      document.body.style.cursor = "row-resize";
      document.body.style.userSelect = "none";
      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },
    []
  );

  if (panes.length === 0) {
    return (
      <div className="flex flex-col flex-1 overflow-hidden">
        <div className="flex-1 flex items-center justify-center bg-bg-primary">
          <p className="text-sm text-text-muted">
            Click <span className="text-text-secondary font-medium">Sessions</span> to view active sessions, or{" "}
            <span className="text-text-secondary font-medium">Claude</span> /{" "}
            <span className="text-text-secondary font-medium">Codex</span> to start a new one
          </p>
        </div>
      </div>
    );
  }

  return (
    <div ref={containerRef} className="flex flex-col flex-1 overflow-hidden bg-bg-border">
      {rows.map((row, rowIdx) => (
        <Fragment key={rowIdx}>
          {/* Row of panes */}
          <div
            className="flex min-h-0 overflow-hidden"
            style={{ flex: rowHeights[rowIdx] ?? 1 }}
          >
            {row.map((pane, colIdx) => (
              <Fragment key={pane.id}>
                {/* Pane */}
                <div
                  className="min-w-0 min-h-0 overflow-hidden"
                  style={{ flex: pane.flexSize ?? 1 }}
                >
                  <TerminalPane
                    paneId={pane.id}
                    onClose={() => removePane(pane.id)}
                    showCloseButton={panes.length > 1}
                    cliCommand={pane.cliCommand}
                    cliArgs={pane.cliArgs}
                    initialPrompt={pane.initialPrompt}
                  />
                </div>
                {/* Column resize handle */}
                {colIdx < row.length - 1 && (
                  <div
                    className="w-[4px] bg-bg-border hover:bg-accent-green/40 cursor-col-resize flex-shrink-0 transition-colors"
                    onMouseDown={(e) => handleColResizeStart(e, row, colIdx)}
                  />
                )}
              </Fragment>
            ))}
          </div>
          {/* Row resize handle */}
          {rowIdx < rows.length - 1 && (
            <div
              className="h-[4px] bg-bg-border hover:bg-accent-green/40 cursor-row-resize flex-shrink-0 transition-colors"
              onMouseDown={(e) => handleRowResizeStart(e, rowIdx)}
            />
          )}
        </Fragment>
      ))}
    </div>
  );
}

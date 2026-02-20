import { PanelGroup, Panel, PanelResizeHandle } from "react-resizable-panels";
import { SessionPane } from "@/components/session/SessionPane";
import { useLayoutStore } from "@/stores/layoutStore";

export function PaneContainer() {
  const panes = useLayoutStore((s) => s.panes);
  const removePane = useLayoutStore((s) => s.removePane);

  return (
    <PanelGroup direction="horizontal" className="flex-1">
      {panes.map((pane, index) => (
        <div key={pane.id} className="contents">
          {index > 0 && (
            <PanelResizeHandle className="w-[3px] bg-bg-elevated hover:bg-accent-green transition-colors" />
          )}
          <Panel minSize={20} defaultSize={100 / panes.length}>
            <SessionPane
              paneId={pane.id}
              onClose={() => removePane(pane.id)}
              showCloseButton={panes.length > 1}
            />
          </Panel>
        </div>
      ))}
    </PanelGroup>
  );
}

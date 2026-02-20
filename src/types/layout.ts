export interface PaneConfig {
  id: string;
  sessionId: string | null;
}

export interface LayoutState {
  panes: PaneConfig[];
  activePaneId: string;
}

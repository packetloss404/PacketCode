export interface PaneConfig {
  id: string;
  sessionId: string | null;
  cliArgs?: string[];
  initialPrompt?: string;
}

export interface LayoutState {
  panes: PaneConfig[];
  activePaneId: string;
}

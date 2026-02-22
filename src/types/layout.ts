export interface PaneConfig {
  id: string;
  sessionId: string | null;
  cliCommand: "claude" | "codex";
  cliArgs?: string[];
  initialPrompt?: string;
  flexSize?: number; // relative size for resizing (default 1)
}

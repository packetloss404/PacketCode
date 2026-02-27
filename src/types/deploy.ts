export interface DeployConfig {
  name: string;
  command: string;
  env?: Record<string, string>;
}

export type DeployStatus = "idle" | "running" | "success" | "failed";

export interface DeployRun {
  id: string;
  configName: string;
  command: string;
  status: DeployStatus;
  startedAt: number;
  finishedAt: number | null;
  sessionId: string | null;
}

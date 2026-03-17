export type MissionStatus = "draft" | "active" | "blocked" | "needs_human" | "done" | "failed";
export type MissionPriority = "low" | "medium" | "high" | "critical";

export interface Mission {
  id: string;
  title: string;
  objective: string;
  status: MissionStatus;
  priority: MissionPriority;
  issueIds: string[];
  linkedSessionIds: string[];
  createdAt: number;
  updatedAt: number;
}

export type FlightStatus = "draft" | "active" | "blocked" | "needs_human" | "done" | "failed";
export type FlightPriority = "low" | "medium" | "high" | "critical";

export interface Flight {
  id: string;
  title: string;
  objective: string;
  status: FlightStatus;
  priority: FlightPriority;
  issueIds: string[];
  linkedSessionIds: string[];
  createdAt: number;
  updatedAt: number;
}

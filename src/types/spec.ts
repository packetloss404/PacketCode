export interface TicketCandidate {
  title: string;
  description: string;
  priority: "low" | "medium" | "high" | "critical";
  labels: string[];
  acceptanceCriteria: string[];
  selected: boolean;
}

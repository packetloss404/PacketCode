export interface InsightsMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  timestamp: number;
}

export interface InsightsSession {
  id: string;
  title: string;
  messages: InsightsMessage[];
  createdAt: number;
  updatedAt: number;
}

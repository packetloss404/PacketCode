export interface CostEntry {
  sessionId: string;
  timestamp: number;
  cost: number;
  model: string;
}

export interface CostSummary {
  totalCost: number;
  sessionCount: number;
  costByDay: Record<string, number>;
  costByModel: Record<string, number>;
}

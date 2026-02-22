export type IdeationType =
  | "code_improvements"
  | "security"
  | "performance"
  | "code_quality"
  | "documentation"
  | "ui_ux";

export type IdeaStatus = "active" | "converted" | "dismissed";

export interface Idea {
  id: string;
  type: IdeationType;
  title: string;
  description: string;
  severity: "low" | "medium" | "high" | "critical";
  affectedFiles: string[];
  suggestion: string;
  effort: "trivial" | "small" | "medium" | "large";
  status: IdeaStatus;
  issueId?: string;
}

export interface IdeationSession {
  id: string;
  ideas: Idea[];
  config: { enabledTypes: IdeationType[] };
  generatedAt: number;
}

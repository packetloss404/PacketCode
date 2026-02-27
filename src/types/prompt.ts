export interface PromptTemplate {
  id: string;
  name: string;
  content: string;
  category: "general" | "debugging" | "review" | "feature" | "custom";
  createdAt: number;
  updatedAt: number;
}

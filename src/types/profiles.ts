export interface AgentProfile {
  id: string;
  name: string;
  description: string;
  icon: string;
  color: string;
  systemPrompt: string;
  defaultModel: string;
  isBuiltin: boolean;
}

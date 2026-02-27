export interface McpServerConfig {
  command: string;
  args?: string[];
  env?: Record<string, string>;
}

export interface McpServerEntry {
  name: string;
  config: McpServerConfig;
  scope: "global" | "project";
  disabled: boolean;
}

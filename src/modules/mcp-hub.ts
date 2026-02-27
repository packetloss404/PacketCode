import { Plug } from "lucide-react";
import { McpHubView } from "@/components/views/McpHubView";
import type { ModuleManifest } from "@/types/modules";

export const mcpHubModule: ModuleManifest = {
  id: "mcp-hub",
  name: "MCP Servers",
  description: "Manage Claude Code MCP server configurations",
  icon: Plug,
  iconColor: "text-accent-blue",
  component: McpHubView,
  category: "integration",
  order: 10,
  enabledByDefault: true,
};

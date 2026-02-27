import type { ModuleManifest } from "@/types/modules";
import { vibeArchitectModule } from "./vibe-architect";
import { ideationModule } from "./ideation";
import { mcpHubModule } from "./mcp-hub";
import { scaffoldModule } from "./scaffold";

export const moduleRegistry: ModuleManifest[] = [
  vibeArchitectModule,
  ideationModule,
  mcpHubModule,
  scaffoldModule,
];

export function getModule(id: string): ModuleManifest | undefined {
  return moduleRegistry.find((mod) => mod.id === id);
}

const categoryOrder: Record<string, number> = {
  ai: 0,
  analysis: 1,
  integration: 2,
  utility: 3,
};

export function getModulesSorted(): ModuleManifest[] {
  return [...moduleRegistry].sort((a, b) => {
    const catDiff = (categoryOrder[a.category] ?? 99) - (categoryOrder[b.category] ?? 99);
    if (catDiff !== 0) return catDiff;
    return (a.order ?? 100) - (b.order ?? 100);
  });
}

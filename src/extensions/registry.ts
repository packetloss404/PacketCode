import type { ExtensionManifest } from "@/types/extensions";
import { vibeArchitectExtension } from "./vibe-architect";
import { ideationExtension } from "./ideation";

export const extensionRegistry: ExtensionManifest[] = [
  vibeArchitectExtension,
  ideationExtension,
];

export function getExtension(id: string): ExtensionManifest | undefined {
  return extensionRegistry.find((ext) => ext.id === id);
}

const categoryOrder: Record<string, number> = {
  ai: 0,
  analysis: 1,
  integration: 2,
  utility: 3,
};

export function getExtensionsSorted(): ExtensionManifest[] {
  return [...extensionRegistry].sort((a, b) => {
    const catDiff = (categoryOrder[a.category] ?? 99) - (categoryOrder[b.category] ?? 99);
    if (catDiff !== 0) return catDiff;
    return (a.order ?? 100) - (b.order ?? 100);
  });
}

import { Sparkles } from "lucide-react";
import { VibeArchitectView } from "@/components/views/VibeArchitectView";
import type { ExtensionManifest } from "@/types/extensions";

export const vibeArchitectExtension: ExtensionManifest = {
  id: "vibe-architect",
  name: "Vibe Architect",
  description: "AI project spec generator — design specs and import as issues",
  icon: Sparkles,
  iconColor: "text-accent-purple",
  component: VibeArchitectView,
  category: "ai",
  order: 10,
  enabledByDefault: true,
  shortcutHint: "Ctrl+Shift+6",
};

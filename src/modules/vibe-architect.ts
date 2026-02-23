import { Sparkles } from "lucide-react";
import { VibeArchitectView } from "@/components/views/VibeArchitectView";
import type { ModuleManifest } from "@/types/modules";

export const vibeArchitectModule: ModuleManifest = {
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

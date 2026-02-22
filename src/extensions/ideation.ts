import { Lightbulb } from "lucide-react";
import { IdeationView } from "@/components/views/IdeationView";
import type { ExtensionManifest } from "@/types/extensions";

export const ideationExtension: ExtensionManifest = {
  id: "ideation",
  name: "Ideation Scanner",
  description: "Scan your codebase for improvement ideas and feature suggestions",
  icon: Lightbulb,
  iconColor: "text-accent-amber",
  component: IdeationView,
  category: "analysis",
  order: 10,
  enabledByDefault: true,
};

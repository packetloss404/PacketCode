import { FolderPlus } from "lucide-react";
import { ScaffoldView } from "@/components/views/ScaffoldView";
import type { ModuleManifest } from "@/types/modules";

export const scaffoldModule: ModuleManifest = {
  id: "scaffold",
  name: "New Project",
  description: "Create new projects from built-in templates",
  icon: FolderPlus,
  iconColor: "text-accent-green",
  component: ScaffoldView,
  category: "utility",
  order: 10,
  enabledByDefault: true,
};

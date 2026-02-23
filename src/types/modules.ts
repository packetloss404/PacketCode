import type { ComponentType } from "react";
import type { LucideIcon } from "lucide-react";

export type ModuleCategory = "ai" | "integration" | "utility" | "analysis";

export interface ModuleManifest {
  id: string;
  name: string;
  description: string;
  icon: LucideIcon;
  iconColor: string;
  component: ComponentType;
  category: ModuleCategory;
  order?: number;
  enabledByDefault: boolean;
  shortcutHint?: string;
}

export interface ModuleState {
  enabled: boolean;
}

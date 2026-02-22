import type { ComponentType } from "react";
import type { LucideIcon } from "lucide-react";

export type ExtensionCategory = "ai" | "integration" | "utility" | "analysis";

export interface ExtensionManifest {
  id: string;
  name: string;
  description: string;
  icon: LucideIcon;
  iconColor: string;
  component: ComponentType;
  category: ExtensionCategory;
  order?: number;
  enabledByDefault: boolean;
  shortcutHint?: string;
}

export interface ExtensionState {
  enabled: boolean;
}

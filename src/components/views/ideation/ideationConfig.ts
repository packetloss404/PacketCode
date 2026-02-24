import { Shield, Zap, Code2, FileText, Palette, Wrench, Lightbulb } from "lucide-react";
import type { IdeationType } from "@/types/ideation";

export const TYPE_CONFIG: Record<
  IdeationType,
  { label: string; icon: typeof Shield; color: string }
> = {
  security: { label: "Security", icon: Shield, color: "text-accent-red" },
  performance: { label: "Performance", icon: Zap, color: "text-accent-amber" },
  code_quality: { label: "Code Quality", icon: Code2, color: "text-accent-blue" },
  code_improvements: { label: "Improvements", icon: Wrench, color: "text-accent-green" },
  documentation: { label: "Documentation", icon: FileText, color: "text-accent-purple" },
  ui_ux: { label: "UI/UX", icon: Palette, color: "text-accent-cyan" },
};

export const ALL_TYPES: IdeationType[] = [
  "security", "performance", "code_quality", "code_improvements", "documentation", "ui_ux",
];

export const SEVERITY_COLORS: Record<string, string> = {
  critical: "bg-red-500/20 text-red-400",
  high: "bg-orange-500/20 text-orange-400",
  medium: "bg-yellow-500/20 text-yellow-400",
  low: "bg-blue-500/20 text-blue-400",
};

export const EFFORT_COLORS: Record<string, string> = {
  trivial: "bg-green-500/20 text-green-400",
  small: "bg-blue-500/20 text-blue-400",
  medium: "bg-yellow-500/20 text-yellow-400",
  large: "bg-red-500/20 text-red-400",
};

export function getTypeConfig(type: IdeationType) {
  const cfg = TYPE_CONFIG[type];
  return {
    icon: cfg?.icon || Lightbulb,
    color: cfg?.color || "text-text-muted",
    label: cfg?.label || type,
  };
}

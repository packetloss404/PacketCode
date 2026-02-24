export const LABEL_COLORS: Record<string, { bg: string; text: string }> = {
  api: { bg: "bg-accent-green/20", text: "text-accent-green" },
  frontend: { bg: "bg-accent-amber/20", text: "text-accent-amber" },
  working: { bg: "bg-accent-green/20", text: "text-accent-green" },
  bug: { bg: "bg-accent-red/20", text: "text-accent-red" },
  feature: { bg: "bg-accent-blue/20", text: "text-accent-blue" },
  enhancement: { bg: "bg-accent-blue/20", text: "text-accent-blue" },
  refactor: { bg: "bg-accent-purple/20", text: "text-accent-purple" },
  docs: { bg: "bg-text-muted/20", text: "text-text-secondary" },
  devops: { bg: "bg-accent-amber/20", text: "text-accent-amber" },
  mvp: { bg: "bg-accent-green/20", text: "text-accent-green" },
};

const DEFAULT_LABEL_COLOR = { bg: "bg-bg-elevated", text: "text-text-muted" };

export function getLabelColor(label: string): { bg: string; text: string } {
  return LABEL_COLORS[label.toLowerCase()] || DEFAULT_LABEL_COLOR;
}

export function getPriorityColor(priority: string): { text: string; cls: string } {
  switch (priority) {
    case "critical": return { text: "Critical", cls: "text-accent-red" };
    case "high": return { text: "High", cls: "text-accent-amber" };
    case "medium": return { text: "Medium", cls: "text-accent-blue" };
    case "low": return { text: "Low", cls: "text-text-muted" };
    default: return { text: priority, cls: "text-text-muted" };
  }
}

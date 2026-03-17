import type { MissionStatus, MissionPriority } from "@/types/mission";
import type { IssueStatus } from "@/stores/issueStore";

export const MISSION_STATUS_CONFIG: Record<MissionStatus, { dot: string; bg: string; text: string; label: string }> = {
  draft: { dot: "bg-text-muted", bg: "bg-text-muted/10", text: "text-text-muted", label: "Draft" },
  active: { dot: "bg-accent-blue", bg: "bg-accent-blue/10", text: "text-accent-blue", label: "Active" },
  blocked: { dot: "bg-accent-red", bg: "bg-accent-red/10", text: "text-accent-red", label: "Blocked" },
  needs_human: { dot: "bg-accent-amber", bg: "bg-accent-amber/10", text: "text-accent-amber", label: "Needs Human" },
  done: { dot: "bg-accent-green", bg: "bg-accent-green/10", text: "text-accent-green", label: "Done" },
  failed: { dot: "bg-accent-red", bg: "bg-accent-red/10", text: "text-accent-red", label: "Failed" },
};

export const MISSION_PRIORITY_COLORS: Record<MissionPriority, string> = {
  critical: "text-accent-red",
  high: "text-accent-amber",
  medium: "text-accent-blue",
  low: "text-text-muted",
};

export const ISSUE_STATUS_COLORS: Record<IssueStatus, string> = {
  todo: "bg-text-muted",
  in_progress: "bg-accent-blue",
  qa: "bg-accent-purple",
  done: "bg-accent-green",
  blocked: "bg-accent-red",
  needs_human: "bg-accent-amber",
};

export const ISSUE_STATUS_LABELS: Record<IssueStatus, string> = {
  todo: "To Do",
  in_progress: "In Progress",
  qa: "QA",
  done: "Done",
  blocked: "Blocked",
  needs_human: "Needs Human",
};

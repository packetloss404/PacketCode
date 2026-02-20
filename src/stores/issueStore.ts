import { create } from "zustand";

export type IssueStatus = "todo" | "in_progress" | "qa" | "done" | "blocked" | "needs_human";
export type IssuePriority = "low" | "medium" | "high" | "critical";

export interface Issue {
  id: string;
  ticketId: string;
  title: string;
  description: string;
  status: IssueStatus;
  priority: IssuePriority;
  labels: string[];
  epic: string | null;
  sessionId: string | null;
  createdAt: number;
  updatedAt: number;
}

const STATUS_COLUMNS: { key: IssueStatus; label: string }[] = [
  { key: "todo", label: "To Do" },
  { key: "in_progress", label: "In Progress" },
  { key: "qa", label: "QA" },
  { key: "done", label: "Done" },
  { key: "blocked", label: "Blocked" },
  { key: "needs_human", label: "Needs Human" },
];

interface IssueStore {
  issues: Issue[];
  nextTicketNum: number;
  ticketPrefix: string;
  epics: string[];
  labels: string[];

  addIssue: (issue: Omit<Issue, "id" | "ticketId" | "createdAt" | "updatedAt">) => Issue;
  updateIssue: (id: string, updates: Partial<Issue>) => void;
  deleteIssue: (id: string) => void;
  moveIssue: (id: string, status: IssueStatus) => void;
  linkSession: (issueId: string, sessionId: string | null) => void;
  addEpic: (epic: string) => void;
  addLabel: (label: string) => void;
  setTicketPrefix: (prefix: string) => void;
  getIssuesByStatus: (status: IssueStatus) => Issue[];
  getColumns: () => typeof STATUS_COLUMNS;
}

function generateId(): string {
  return `issue_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
}

// Load from localStorage
function loadState(): { issues: Issue[]; nextTicketNum: number; ticketPrefix: string; epics: string[]; labels: string[] } {
  try {
    const saved = localStorage.getItem("packetcode:issues");
    if (saved) {
      return JSON.parse(saved);
    }
  } catch {}
  return {
    issues: [],
    nextTicketNum: 1,
    ticketPrefix: "PKT",
    epics: [],
    labels: ["bug", "feature", "enhancement", "refactor", "docs", "api", "frontend", "working", "devops"],
  };
}

function saveState(state: { issues: Issue[]; nextTicketNum: number; ticketPrefix: string; epics: string[]; labels: string[] }) {
  try {
    localStorage.setItem("packetcode:issues", JSON.stringify(state));
  } catch {}
}

const initial = loadState();

export const useIssueStore = create<IssueStore>((set, get) => ({
  ...initial,

  addIssue: (issue) => {
    const state = get();
    const ticketId = `${state.ticketPrefix}-${String(state.nextTicketNum).padStart(3, "0")}`;
    const newIssue: Issue = {
      ...issue,
      id: generateId(),
      ticketId,
      createdAt: Date.now(),
      updatedAt: Date.now(),
    };
    const newState = {
      issues: [...state.issues, newIssue],
      nextTicketNum: state.nextTicketNum + 1,
      ticketPrefix: state.ticketPrefix,
      epics: state.epics,
      labels: state.labels,
    };
    set(newState);
    saveState(newState);
    return newIssue;
  },

  updateIssue: (id, updates) => {
    set((s) => {
      const issues = s.issues.map((i) =>
        i.id === id ? { ...i, ...updates, updatedAt: Date.now() } : i
      );
      saveState({ issues, nextTicketNum: s.nextTicketNum, ticketPrefix: s.ticketPrefix, epics: s.epics, labels: s.labels });
      return { issues };
    });
  },

  deleteIssue: (id) => {
    set((s) => {
      const issues = s.issues.filter((i) => i.id !== id);
      saveState({ issues, nextTicketNum: s.nextTicketNum, ticketPrefix: s.ticketPrefix, epics: s.epics, labels: s.labels });
      return { issues };
    });
  },

  moveIssue: (id, status) => {
    get().updateIssue(id, { status });
  },

  linkSession: (issueId, sessionId) => {
    get().updateIssue(issueId, { sessionId });
  },

  addEpic: (epic) => {
    set((s) => {
      const epics = s.epics.includes(epic) ? s.epics : [...s.epics, epic];
      saveState({ issues: s.issues, nextTicketNum: s.nextTicketNum, ticketPrefix: s.ticketPrefix, epics, labels: s.labels });
      return { epics };
    });
  },

  addLabel: (label) => {
    set((s) => {
      const labels = s.labels.includes(label) ? s.labels : [...s.labels, label];
      saveState({ issues: s.issues, nextTicketNum: s.nextTicketNum, ticketPrefix: s.ticketPrefix, epics: s.epics, labels });
      return { labels };
    });
  },

  setTicketPrefix: (prefix) => {
    set((s) => {
      saveState({ issues: s.issues, nextTicketNum: s.nextTicketNum, ticketPrefix: prefix, epics: s.epics, labels: s.labels });
      return { ticketPrefix: prefix };
    });
  },

  getIssuesByStatus: (status) => get().issues.filter((i) => i.status === status),

  getColumns: () => STATUS_COLUMNS,
}));

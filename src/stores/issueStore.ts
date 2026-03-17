import { create } from "zustand";
import { loadFromStorage, saveToStorage, generateId as genId } from "@/lib/storage";

export type IssueStatus = "todo" | "in_progress" | "qa" | "done" | "blocked" | "needs_human";
export type IssuePriority = "low" | "medium" | "high" | "critical";

export interface AcceptanceCriterion {
  id: string;
  text: string;
  checked: boolean;
}

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
  missionId: string | null;
  acceptanceCriteria: AcceptanceCriterion[];
  blockedBy: string[]; // issue IDs
  blocks: string[];    // issue IDs
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

  addIssue: (issue: Omit<Issue, "id" | "ticketId" | "createdAt" | "updatedAt" | "missionId"> & { missionId?: string | null }) => Issue;
  updateIssue: (id: string, updates: Partial<Issue>) => void;
  deleteIssue: (id: string) => void;
  moveIssue: (id: string, status: IssueStatus) => void;
  linkSession: (issueId: string, sessionId: string | null) => void;
  assignToMission: (issueId: string, missionId: string | null) => void;
  addEpic: (epic: string) => void;
  addLabel: (label: string) => void;
  setTicketPrefix: (prefix: string) => void;
  getIssuesByStatus: (status: IssueStatus) => Issue[];
  getColumns: () => typeof STATUS_COLUMNS;

  // Acceptance criteria
  toggleCriterion: (issueId: string, criterionId: string) => void;
  addCriterion: (issueId: string, text: string) => void;
  removeCriterion: (issueId: string, criterionId: string) => void;

  // Dependencies
  addBlockedBy: (issueId: string, blockerIssueId: string) => void;
  removeBlockedBy: (issueId: string, blockerIssueId: string) => void;
  addBlocks: (issueId: string, blockedIssueId: string) => void;
  removeBlocks: (issueId: string, blockedIssueId: string) => void;
}

const generateIssueId = () => genId("issue");
const generateCriterionId = () => genId("ac", 6);

// Migrate old issues that lack new fields
function migrateIssue(issue: Issue): Issue {
  return {
    ...issue,
    missionId: issue.missionId ?? null,
    acceptanceCriteria: issue.acceptanceCriteria || [],
    blockedBy: issue.blockedBy || [],
    blocks: issue.blocks || [],
  };
}

type IssueState = { issues: Issue[]; nextTicketNum: number; ticketPrefix: string; epics: string[]; labels: string[] };

const DEFAULT_ISSUE_STATE: IssueState = {
  issues: [],
  nextTicketNum: 1,
  ticketPrefix: "PKT",
  epics: [],
  labels: ["bug", "feature", "enhancement", "refactor", "docs", "api", "frontend", "working", "devops"],
};

function loadState(): IssueState {
  const parsed = loadFromStorage<IssueState>("packetcode:issues", DEFAULT_ISSUE_STATE);
  return { ...parsed, issues: (parsed.issues || []).map(migrateIssue) };
}

function saveState(state: IssueState) {
  saveToStorage("packetcode:issues", state);
}

const initial = loadState();

export const useIssueStore = create<IssueStore>((set, get) => ({
  ...initial,

  addIssue: (issue) => {
    const state = get();
    const ticketId = `${state.ticketPrefix}-${String(state.nextTicketNum).padStart(3, "0")}`;
    const newIssue: Issue = {
      ...issue,
      missionId: issue.missionId ?? null,
      id: generateIssueId(),
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
      // Also remove this issue from any blockedBy/blocks arrays
      const issues = s.issues
        .filter((i) => i.id !== id)
        .map((i) => ({
          ...i,
          blockedBy: i.blockedBy.filter((bid) => bid !== id),
          blocks: i.blocks.filter((bid) => bid !== id),
        }));
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

  assignToMission: (issueId, missionId) => {
    get().updateIssue(issueId, { missionId });
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

  // Acceptance criteria
  toggleCriterion: (issueId, criterionId) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue) return;
    const acceptanceCriteria = issue.acceptanceCriteria.map((c) =>
      c.id === criterionId ? { ...c, checked: !c.checked } : c
    );
    get().updateIssue(issueId, { acceptanceCriteria });
  },

  addCriterion: (issueId, text) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue) return;
    const newCriterion: AcceptanceCriterion = {
      id: generateCriterionId(),
      text,
      checked: false,
    };
    get().updateIssue(issueId, {
      acceptanceCriteria: [...issue.acceptanceCriteria, newCriterion],
    });
  },

  removeCriterion: (issueId, criterionId) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue) return;
    get().updateIssue(issueId, {
      acceptanceCriteria: issue.acceptanceCriteria.filter((c) => c.id !== criterionId),
    });
  },

  // Dependencies
  addBlockedBy: (issueId, blockerIssueId) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue || issue.blockedBy.includes(blockerIssueId)) return;
    get().updateIssue(issueId, { blockedBy: [...issue.blockedBy, blockerIssueId] });
    // Also add the reverse relationship
    const blocker = get().issues.find((i) => i.id === blockerIssueId);
    if (blocker && !blocker.blocks.includes(issueId)) {
      get().updateIssue(blockerIssueId, { blocks: [...blocker.blocks, issueId] });
    }
  },

  removeBlockedBy: (issueId, blockerIssueId) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue) return;
    get().updateIssue(issueId, { blockedBy: issue.blockedBy.filter((id) => id !== blockerIssueId) });
    // Also remove the reverse relationship
    const blocker = get().issues.find((i) => i.id === blockerIssueId);
    if (blocker) {
      get().updateIssue(blockerIssueId, { blocks: blocker.blocks.filter((id) => id !== issueId) });
    }
  },

  addBlocks: (issueId, blockedIssueId) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue || issue.blocks.includes(blockedIssueId)) return;
    get().updateIssue(issueId, { blocks: [...issue.blocks, blockedIssueId] });
    // Also add the reverse relationship
    const blocked = get().issues.find((i) => i.id === blockedIssueId);
    if (blocked && !blocked.blockedBy.includes(issueId)) {
      get().updateIssue(blockedIssueId, { blockedBy: [...blocked.blockedBy, issueId] });
    }
  },

  removeBlocks: (issueId, blockedIssueId) => {
    const issue = get().issues.find((i) => i.id === issueId);
    if (!issue) return;
    get().updateIssue(issueId, { blocks: issue.blocks.filter((id) => id !== blockedIssueId) });
    // Also remove the reverse relationship
    const blocked = get().issues.find((i) => i.id === blockedIssueId);
    if (blocked) {
      get().updateIssue(blockedIssueId, { blockedBy: blocked.blockedBy.filter((id) => id !== issueId) });
    }
  },
}));

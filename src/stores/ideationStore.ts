import { create } from "zustand";
import type { Idea, IdeationType, IdeationSession } from "@/types/ideation";
import { generateIdeas as generateIdeasApi } from "@/lib/tauri";
import { useLayoutStore } from "@/stores/layoutStore";
import { useIssueStore } from "@/stores/issueStore";
import { loadFromStorage, saveToStorage, removeFromStorage, parseJsonFromResponse, generateId } from "@/lib/storage";

const STORAGE_KEY = "packetcode:ideation-session";

interface IdeationStore {
  session: IdeationSession | null;
  isGenerating: boolean;
  selectedIdeaId: string | null;

  generate: (types: IdeationType[]) => Promise<void>;
  dismiss: (id: string) => void;
  convertToIssue: (id: string) => void;
  clearAll: () => void;
  selectIdea: (id: string | null) => void;
}

function loadSession(): IdeationSession | null {
  return loadFromStorage<IdeationSession | null>(STORAGE_KEY, null);
}

function saveSession(session: IdeationSession | null) {
  if (session) {
    saveToStorage(STORAGE_KEY, session);
  } else {
    removeFromStorage(STORAGE_KEY);
  }
}

export const useIdeationStore = create<IdeationStore>((set, get) => ({
  session: loadSession(),
  isGenerating: false,
  selectedIdeaId: null,

  generate: async (types) => {
    set({ isGenerating: true });

    try {
      const projectPath = useLayoutStore.getState().projectPath;
      const raw = await generateIdeasApi(projectPath, types);

      const parsed = parseJsonFromResponse(raw) as Array<{
        type: string;
        title: string;
        description: string;
        severity: string;
        affectedFiles: string[];
        suggestion: string;
        effort: string;
      }>;

      const ideas: Idea[] = parsed.map((item) => ({
        id: generateId("idea"),
        type: item.type as IdeationType,
        title: item.title,
        description: item.description,
        severity: item.severity as Idea["severity"],
        affectedFiles: item.affectedFiles || [],
        suggestion: item.suggestion,
        effort: item.effort as Idea["effort"],
        status: "active",
      }));

      const session: IdeationSession = {
        id: generateId("idea"),
        ideas,
        config: { enabledTypes: types },
        generatedAt: Date.now(),
      };

      set({ session, isGenerating: false, selectedIdeaId: null });
      saveSession(session);
    } catch (err) {
      set({ isGenerating: false });
      throw err;
    }
  },

  dismiss: (id) => {
    const session = get().session;
    if (!session) return;
    const updated = {
      ...session,
      ideas: session.ideas.map((i) =>
        i.id === id ? { ...i, status: "dismissed" as const } : i
      ),
    };
    set({ session: updated, selectedIdeaId: get().selectedIdeaId === id ? null : get().selectedIdeaId });
    saveSession(updated);
  },

  convertToIssue: (id) => {
    const session = get().session;
    if (!session) return;
    const idea = session.ideas.find((i) => i.id === id);
    if (!idea || idea.status !== "active") return;

    const issue = useIssueStore.getState().addIssue({
      title: idea.title,
      description: `${idea.description}\n\n**Suggestion:** ${idea.suggestion}\n\n**Affected files:** ${idea.affectedFiles.join(", ")}`,
      status: "todo",
      priority: idea.severity === "critical" ? "critical" : idea.severity === "high" ? "high" : idea.severity === "medium" ? "medium" : "low",
      labels: [idea.type, `effort:${idea.effort}`],
      epic: null,
      sessionId: null,
      acceptanceCriteria: [],
      blockedBy: [],
      blocks: [],
    });

    const updated = {
      ...session,
      ideas: session.ideas.map((i) =>
        i.id === id ? { ...i, status: "converted" as const, issueId: issue.id } : i
      ),
    };
    set({ session: updated });
    saveSession(updated);
  },

  clearAll: () => {
    set({ session: null, selectedIdeaId: null });
    saveSession(null);
  },

  selectIdea: (id) => {
    set({ selectedIdeaId: id });
  },
}));

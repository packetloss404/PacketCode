import { create } from "zustand";
import { invoke } from "@tauri-apps/api/core";
import type {
  MemoryState,
  FileMapEntry,
  SessionSummary,
  LearnedPattern,
} from "@/types/memory";

function loadMemory(): MemoryState {
  try {
    const saved = localStorage.getItem("packetcode:memory");
    if (saved) return JSON.parse(saved);
  } catch {
    // ignore
  }
  return {
    fileMap: [],
    sessionSummaries: [],
    patterns: [],
    lastScanAt: null,
  };
}

function saveMemory(memory: MemoryState) {
  localStorage.setItem("packetcode:memory", JSON.stringify(memory));
}

interface MemoryStore {
  memory: MemoryState;
  isScanning: boolean;
  scanError: string | null;

  scanCodebase: (projectPath: string) => Promise<void>;
  addSessionSummary: (
    projectPath: string,
    sessionTitle: string,
    sessionLog: string
  ) => Promise<void>;
  refreshPatterns: (projectPath: string) => Promise<void>;
  clearMemory: () => void;
  deletePattern: (id: string) => void;
  deleteSummary: (id: string) => void;
  getContextForSession: () => string;
}

function parseJsonFromResponse(text: string): unknown {
  // Try direct parse first
  try {
    return JSON.parse(text);
  } catch {
    // Try extracting JSON from markdown code blocks
    const match = text.match(/```(?:json)?\s*([\s\S]*?)```/);
    if (match) {
      return JSON.parse(match[1].trim());
    }
    // Try finding array or object
    const arrMatch = text.match(/\[[\s\S]*\]/);
    if (arrMatch) return JSON.parse(arrMatch[0]);
    const objMatch = text.match(/\{[\s\S]*\}/);
    if (objMatch) return JSON.parse(objMatch[0]);
    throw new Error("No valid JSON found in response");
  }
}

export const useMemoryStore = create<MemoryStore>((set, get) => ({
  memory: loadMemory(),
  isScanning: false,
  scanError: null,

  scanCodebase: async (projectPath) => {
    set({ isScanning: true, scanError: null });
    try {
      const result = await invoke<string>("scan_codebase_memory", {
        projectPath,
      });
      const parsed = parseJsonFromResponse(result) as {
        path: string;
        summary: string;
      }[];
      const fileMap: FileMapEntry[] = parsed.map((e) => ({
        path: e.path,
        summary: e.summary,
        lastAnalyzed: Date.now(),
      }));
      const memory = { ...get().memory, fileMap, lastScanAt: Date.now() };
      saveMemory(memory);
      set({ memory, isScanning: false });
    } catch (e) {
      set({ scanError: String(e), isScanning: false });
    }
  },

  addSessionSummary: async (projectPath, sessionTitle, sessionLog) => {
    set({ isScanning: true, scanError: null });
    try {
      const result = await invoke<string>("summarize_session", {
        projectPath,
        sessionLog,
      });
      const parsed = parseJsonFromResponse(result) as {
        summary: string;
        keyDecisions: string[];
        filesModified: string[];
      };
      const summary: SessionSummary = {
        id: `session-${Date.now()}`,
        sessionTitle,
        summary: parsed.summary,
        keyDecisions: parsed.keyDecisions || [],
        filesModified: parsed.filesModified || [],
        createdAt: Date.now(),
      };
      const memory = {
        ...get().memory,
        sessionSummaries: [...get().memory.sessionSummaries, summary],
      };
      saveMemory(memory);
      set({ memory, isScanning: false });
    } catch (e) {
      set({ scanError: String(e), isScanning: false });
    }
  },

  refreshPatterns: async (projectPath) => {
    const { sessionSummaries } = get().memory;
    if (sessionSummaries.length === 0) return;
    set({ isScanning: true, scanError: null });
    try {
      const summariesText = sessionSummaries
        .map((s) => `[${s.sessionTitle}]: ${s.summary}`)
        .join("\n\n");
      const result = await invoke<string>("extract_patterns", {
        projectPath,
        summaries: summariesText,
      });
      const parsed = parseJsonFromResponse(result) as {
        pattern: string;
        category: string;
        confidence: number;
      }[];
      const patterns: LearnedPattern[] = parsed.map((p, i) => ({
        id: `pattern-${Date.now()}-${i}`,
        pattern: p.pattern,
        category: p.category as LearnedPattern["category"],
        confidence: p.confidence,
        sources: sessionSummaries.map((s) => s.id),
        createdAt: Date.now(),
        updatedAt: Date.now(),
      }));
      const memory = { ...get().memory, patterns };
      saveMemory(memory);
      set({ memory, isScanning: false });
    } catch (e) {
      set({ scanError: String(e), isScanning: false });
    }
  },

  clearMemory: () => {
    const memory: MemoryState = {
      fileMap: [],
      sessionSummaries: [],
      patterns: [],
      lastScanAt: null,
    };
    saveMemory(memory);
    set({ memory });
  },

  deletePattern: (id) => {
    const memory = {
      ...get().memory,
      patterns: get().memory.patterns.filter((p) => p.id !== id),
    };
    saveMemory(memory);
    set({ memory });
  },

  deleteSummary: (id) => {
    const memory = {
      ...get().memory,
      sessionSummaries: get().memory.sessionSummaries.filter(
        (s) => s.id !== id
      ),
    };
    saveMemory(memory);
    set({ memory });
  },

  getContextForSession: () => {
    const { patterns, fileMap } = get().memory;
    const lines: string[] = [];

    if (patterns.length > 0) {
      lines.push("## Codebase Patterns");
      patterns
        .filter((p) => p.confidence >= 0.5)
        .forEach((p) => {
          lines.push(`- [${p.category}] ${p.pattern}`);
        });
      lines.push("");
    }

    if (fileMap.length > 0) {
      lines.push("## Key Files");
      fileMap.slice(0, 20).forEach((f) => {
        lines.push(`- ${f.path}: ${f.summary}`);
      });
      lines.push("");
    }

    return lines.join("\n");
  },
}));

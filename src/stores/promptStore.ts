import { create } from "zustand";
import { loadFromStorage, saveToStorage, generateId } from "@/lib/storage";
import type { PromptTemplate } from "@/types/prompt";

const STORAGE_KEY = "packetcode:prompt-templates";

interface PromptStore {
  templates: PromptTemplate[];
  addTemplate: (name: string, content: string, category: PromptTemplate["category"]) => void;
  updateTemplate: (id: string, updates: Partial<Pick<PromptTemplate, "name" | "content" | "category">>) => void;
  deleteTemplate: (id: string) => void;
}

export const usePromptStore = create<PromptStore>((set, get) => ({
  templates: loadFromStorage<PromptTemplate[]>(STORAGE_KEY, []),

  addTemplate: (name, content, category) => {
    const now = Date.now();
    const template: PromptTemplate = {
      id: generateId("tpl"),
      name,
      content,
      category,
      createdAt: now,
      updatedAt: now,
    };
    const updated = [...get().templates, template];
    set({ templates: updated });
    saveToStorage(STORAGE_KEY, updated);
  },

  updateTemplate: (id, updates) => {
    const updated = get().templates.map((t) =>
      t.id === id ? { ...t, ...updates, updatedAt: Date.now() } : t
    );
    set({ templates: updated });
    saveToStorage(STORAGE_KEY, updated);
  },

  deleteTemplate: (id) => {
    const updated = get().templates.filter((t) => t.id !== id);
    set({ templates: updated });
    saveToStorage(STORAGE_KEY, updated);
  },
}));

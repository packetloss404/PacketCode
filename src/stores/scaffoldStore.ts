import { create } from "zustand";
import type { ScaffoldResult, ToolAvailability } from "@/types/scaffold";
import { scaffoldProject, checkScaffoldTools } from "@/lib/tauri";

type WizardStep = "template" | "config" | "result";

interface ScaffoldStore {
  step: WizardStep;
  selectedTemplate: string | null;
  projectName: string;
  parentDir: string;
  scaffolding: boolean;
  result: ScaffoldResult | null;
  tools: ToolAvailability;
  toolsChecked: boolean;

  setStep: (step: WizardStep) => void;
  setSelectedTemplate: (id: string) => void;
  setProjectName: (name: string) => void;
  setParentDir: (dir: string) => void;
  checkTools: () => Promise<void>;
  runScaffold: () => Promise<ScaffoldResult>;
  reset: () => void;
}

export const useScaffoldStore = create<ScaffoldStore>((set, get) => ({
  step: "template",
  selectedTemplate: null,
  projectName: "",
  parentDir: "",
  scaffolding: false,
  result: null,
  tools: {},
  toolsChecked: false,

  setStep: (step) => set({ step }),
  setSelectedTemplate: (id) => set({ selectedTemplate: id, step: "config" }),
  setProjectName: (name) => set({ projectName: name }),
  setParentDir: (dir) => set({ parentDir: dir }),

  checkTools: async () => {
    try {
      const tools = await checkScaffoldTools();
      set({ tools, toolsChecked: true });
    } catch {
      set({ toolsChecked: true });
    }
  },

  runScaffold: async () => {
    const { parentDir, projectName, selectedTemplate } = get();
    if (!parentDir || !projectName || !selectedTemplate) {
      throw new Error("Missing scaffold parameters");
    }
    set({ scaffolding: true });
    try {
      const result = await scaffoldProject(parentDir, projectName, selectedTemplate);
      set({ result, scaffolding: false, step: "result" });
      return result;
    } catch (e) {
      const result: ScaffoldResult = {
        success: false,
        project_path: "",
        message: String(e),
      };
      set({ result, scaffolding: false, step: "result" });
      return result;
    }
  },

  reset: () =>
    set({
      step: "template",
      selectedTemplate: null,
      projectName: "",
      parentDir: "",
      scaffolding: false,
      result: null,
    }),
}));

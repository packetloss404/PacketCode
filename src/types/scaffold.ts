export interface ProjectTemplate {
  id: string;
  name: string;
  description: string;
  requires: string | null;
  icon: string;
}

export interface ScaffoldResult {
  success: boolean;
  project_path: string;
  message: string;
}

export type ToolAvailability = Record<string, boolean>;

import { useEffect } from "react";
import {
  FolderPlus,
  Globe,
  Zap,
  Server,
  Terminal,
  Box,
  FileText,
  ChevronRight,
  CheckCircle2,
  XCircle,
  ArrowLeft,
  FolderOpen,
  Loader2,
} from "lucide-react";
import { useScaffoldStore } from "@/stores/scaffoldStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { open } from "@tauri-apps/plugin-dialog";

const TEMPLATES = [
  {
    id: "nextjs",
    name: "Next.js",
    description: "Full-stack React framework with SSR",
    icon: Globe,
    iconColor: "text-text-primary",
    requires: "node",
  },
  {
    id: "react-vite",
    name: "React + Vite",
    description: "Fast React SPA with TypeScript",
    icon: Zap,
    iconColor: "text-accent-blue",
    requires: "node",
  },
  {
    id: "python-fastapi",
    name: "Python FastAPI",
    description: "Modern Python REST API",
    icon: Server,
    iconColor: "text-accent-green",
    requires: "python",
  },
  {
    id: "rust-cli",
    name: "Rust CLI",
    description: "Rust command-line application",
    icon: Terminal,
    iconColor: "text-accent-amber",
    requires: "cargo",
  },
  {
    id: "node-express",
    name: "Node Express",
    description: "Node.js REST API with Express",
    icon: Box,
    iconColor: "text-accent-purple",
    requires: "node",
  },
  {
    id: "blank",
    name: "Blank Project",
    description: "README + .gitignore only",
    icon: FileText,
    iconColor: "text-text-muted",
    requires: null,
  },
];

export function ScaffoldView() {
  const store = useScaffoldStore();

  useEffect(() => {
    if (!store.toolsChecked) {
      store.checkTools();
    }
  }, [store, store.toolsChecked]);

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-y-auto">
      {/* Header */}
      <div className="flex items-center gap-2 px-4 py-3 border-b border-bg-border bg-bg-secondary">
        {store.step !== "template" && (
          <button
            onClick={() => {
              if (store.step === "config") store.setStep("template");
              else store.reset();
            }}
            className="p-1 text-text-muted hover:text-text-primary transition-colors"
          >
            <ArrowLeft size={14} />
          </button>
        )}
        <FolderPlus size={14} className="text-accent-green" />
        <h2 className="text-sm font-medium text-text-primary">New Project</h2>
        <div className="flex items-center gap-1 ml-2 text-[10px] text-text-muted">
          <StepIndicator active={store.step === "template"} done={store.step !== "template"} label="Template" />
          <ChevronRight size={10} />
          <StepIndicator active={store.step === "config"} done={store.step === "result"} label="Configure" />
          <ChevronRight size={10} />
          <StepIndicator active={store.step === "result"} done={false} label="Done" />
        </div>
      </div>

      <div className="flex-1 p-4">
        {store.step === "template" && <TemplateGrid />}
        {store.step === "config" && <ConfigStep />}
        {store.step === "result" && <ResultStep />}
      </div>
    </div>
  );
}

function StepIndicator({ active, done, label }: { active: boolean; done: boolean; label: string }) {
  return (
    <span
      className={`px-1.5 py-0.5 rounded ${
        active
          ? "bg-accent-green/20 text-accent-green"
          : done
          ? "text-accent-green"
          : "text-text-muted"
      }`}
    >
      {label}
    </span>
  );
}

function TemplateGrid() {
  const { setSelectedTemplate, tools } = useScaffoldStore();

  return (
    <div>
      <p className="text-xs text-text-secondary mb-4">Choose a project template to get started</p>
      <div className="grid grid-cols-2 lg:grid-cols-3 gap-3">
        {TEMPLATES.map((t) => {
          const Icon = t.icon;
          const available = !t.requires || tools[t.requires];
          return (
            <button
              key={t.id}
              onClick={() => setSelectedTemplate(t.id)}
              disabled={!available}
              className={`flex flex-col items-start gap-2 p-4 bg-bg-secondary border border-bg-border rounded-lg text-left transition-colors ${
                available
                  ? "hover:border-accent-green/50 hover:bg-bg-elevated cursor-pointer"
                  : "opacity-40 cursor-not-allowed"
              }`}
            >
              <Icon size={20} className={t.iconColor} />
              <div>
                <div className="text-xs font-medium text-text-primary">{t.name}</div>
                <div className="text-[11px] text-text-muted mt-0.5">{t.description}</div>
              </div>
              {t.requires && !available && (
                <span className="text-[9px] text-red-400 mt-1">
                  Requires {t.requires}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function ConfigStep() {
  const { selectedTemplate, projectName, parentDir, scaffolding, setProjectName, setParentDir, runScaffold } =
    useScaffoldStore();
  const setProjectPath = useLayoutStore((s) => s.setProjectPath);

  const template = TEMPLATES.find((t) => t.id === selectedTemplate);

  async function handlePickDir() {
    const selected = await open({
      directory: true,
      multiple: false,
      title: "Select Parent Directory",
    });
    if (selected) setParentDir(selected as string);
  }

  async function handleCreate() {
    const result = await runScaffold();
    if (result.success && result.project_path) {
      setProjectPath(result.project_path);
    }
  }

  return (
    <div className="max-w-md">
      <div className="flex items-center gap-2 mb-4">
        {template && <template.icon size={16} className={template.iconColor} />}
        <span className="text-xs font-medium text-text-primary">{template?.name}</span>
      </div>

      <div className="space-y-3">
        <div>
          <label className="block text-[11px] text-text-secondary mb-1">Project Name</label>
          <input
            type="text"
            value={projectName}
            onChange={(e) => setProjectName(e.target.value)}
            placeholder="my-awesome-project"
            className="w-full px-3 py-1.5 bg-bg-secondary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
            autoFocus
          />
        </div>

        <div>
          <label className="block text-[11px] text-text-secondary mb-1">Parent Directory</label>
          <div className="flex gap-2">
            <input
              type="text"
              value={parentDir}
              onChange={(e) => setParentDir(e.target.value)}
              placeholder="/home/user/projects"
              className="flex-1 px-3 py-1.5 bg-bg-secondary border border-bg-border rounded text-xs text-text-primary focus:outline-none focus:border-accent-green"
            />
            <button
              onClick={handlePickDir}
              className="px-2.5 py-1.5 bg-bg-secondary border border-bg-border rounded text-text-secondary hover:text-text-primary transition-colors"
            >
              <FolderOpen size={12} />
            </button>
          </div>
        </div>

        <button
          onClick={handleCreate}
          disabled={scaffolding || !projectName.trim() || !parentDir.trim()}
          className="flex items-center gap-2 px-4 py-2 bg-accent-green text-bg-primary text-xs font-medium rounded hover:bg-accent-green/80 transition-colors disabled:opacity-50 mt-4"
        >
          {scaffolding ? (
            <>
              <Loader2 size={12} className="animate-spin" />
              Creating...
            </>
          ) : (
            <>
              <FolderPlus size={12} />
              Create Project
            </>
          )}
        </button>
      </div>
    </div>
  );
}

function ResultStep() {
  const { result, reset } = useScaffoldStore();

  if (!result) return null;

  return (
    <div className="max-w-md text-center py-8">
      {result.success ? (
        <>
          <CheckCircle2 size={32} className="mx-auto mb-3 text-accent-green" />
          <h3 className="text-sm font-medium text-text-primary mb-1">Project Created</h3>
          <p className="text-[11px] text-text-muted mb-2">{result.message}</p>
          <p className="text-[11px] text-text-secondary bg-bg-secondary px-3 py-1.5 rounded inline-block">
            {result.project_path}
          </p>
        </>
      ) : (
        <>
          <XCircle size={32} className="mx-auto mb-3 text-red-400" />
          <h3 className="text-sm font-medium text-text-primary mb-1">Scaffold Failed</h3>
          <p className="text-[11px] text-red-400">{result.message}</p>
        </>
      )}
      <button
        onClick={reset}
        className="mt-6 px-4 py-1.5 text-xs text-text-secondary bg-bg-secondary rounded hover:text-text-primary transition-colors"
      >
        Create Another
      </button>
    </div>
  );
}

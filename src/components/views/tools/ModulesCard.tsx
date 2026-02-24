import { Puzzle } from "lucide-react";
import { useModuleStore } from "@/stores/moduleStore";
import { useAppStore, moduleViewId } from "@/stores/appStore";
import { getModulesSorted } from "@/modules/registry";

const CATEGORY_LABELS: Record<string, string> = {
  ai: "AI",
  analysis: "Analysis",
  integration: "Integration",
  utility: "Utility",
};

const CATEGORY_COLORS: Record<string, string> = {
  ai: "bg-accent-purple/15 text-accent-purple",
  analysis: "bg-accent-amber/15 text-accent-amber",
  integration: "bg-accent-blue/15 text-accent-blue",
  utility: "bg-bg-elevated text-text-muted",
};

export function ModulesCard() {
  const modules = getModulesSorted();
  const { states, toggleModule } = useModuleStore();
  const activeView = useAppStore((s) => s.activeView);
  const setActiveView = useAppStore((s) => s.setActiveView);

  const enabledCount = modules.filter((mod) => states[mod.id]?.enabled).length;

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 col-span-2">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
          <Puzzle size={12} className="text-accent-blue" />
          Modules
        </h3>
        <span className="text-[10px] text-text-muted px-1.5 py-0.5 bg-bg-elevated rounded">
          {enabledCount} of {modules.length} enabled
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {modules.map((mod) => {
          const enabled = states[mod.id]?.enabled ?? false;
          const Icon = mod.icon;
          return (
            <div key={mod.id} className="flex items-center gap-3 px-3 py-2 bg-bg-primary border border-bg-border rounded">
              <span className={enabled ? mod.iconColor : "text-text-muted"}>
                <Icon size={12} />
              </span>
              <div className="flex-1 min-w-0">
                <div className="text-[11px] text-text-primary font-medium flex items-center gap-2">
                  {mod.name}
                  <span className={`text-[9px] px-1.5 py-0.5 rounded ${CATEGORY_COLORS[mod.category] ?? "bg-bg-elevated text-text-muted"}`}>
                    {CATEGORY_LABELS[mod.category] ?? mod.category}
                  </span>
                </div>
                <div className="text-[10px] text-text-muted truncate">
                  {mod.description}
                  {mod.shortcutHint && (
                    <span className="ml-2 text-text-muted opacity-60">({mod.shortcutHint})</span>
                  )}
                </div>
              </div>
              <button
                onClick={() => {
                  toggleModule(mod.id);
                  if (enabled && activeView === moduleViewId(mod.id)) {
                    setActiveView("tools");
                  }
                }}
                className={`relative w-8 h-4 rounded-full transition-colors ${enabled ? "bg-accent-green" : "bg-bg-elevated"}`}
                title={enabled ? "Disable" : "Enable"}
              >
                <span className={`absolute top-0.5 w-3 h-3 rounded-full bg-white transition-transform ${enabled ? "left-4" : "left-0.5"}`} />
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}

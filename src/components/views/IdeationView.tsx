import { useState } from "react";
import {
  Lightbulb,
  RefreshCw,
  Loader2,
  Shield,
  Zap,
  Code2,
  FileText,
  Palette,
  Wrench,
  ArrowRight,
  X,
  ExternalLink,
  ChevronDown,
  ChevronRight,
  Eye,
  EyeOff,
} from "lucide-react";
import { useIdeationStore } from "@/stores/ideationStore";
import type { Idea, IdeationType } from "@/types/ideation";

const TYPE_CONFIG: Record<
  IdeationType,
  { label: string; icon: typeof Shield; color: string }
> = {
  security: { label: "Security", icon: Shield, color: "text-accent-red" },
  performance: { label: "Performance", icon: Zap, color: "text-accent-amber" },
  code_quality: { label: "Code Quality", icon: Code2, color: "text-accent-blue" },
  code_improvements: { label: "Improvements", icon: Wrench, color: "text-accent-green" },
  documentation: { label: "Documentation", icon: FileText, color: "text-accent-purple" },
  ui_ux: { label: "UI/UX", icon: Palette, color: "text-accent-cyan" },
};

const ALL_TYPES: IdeationType[] = [
  "security",
  "performance",
  "code_quality",
  "code_improvements",
  "documentation",
  "ui_ux",
];

const SEVERITY_COLORS: Record<string, string> = {
  critical: "bg-red-500/20 text-red-400",
  high: "bg-orange-500/20 text-orange-400",
  medium: "bg-yellow-500/20 text-yellow-400",
  low: "bg-blue-500/20 text-blue-400",
};

const EFFORT_COLORS: Record<string, string> = {
  trivial: "bg-green-500/20 text-green-400",
  small: "bg-blue-500/20 text-blue-400",
  medium: "bg-yellow-500/20 text-yellow-400",
  large: "bg-red-500/20 text-red-400",
};

function IdeaCard({
  idea,
  isSelected,
  onSelect,
}: {
  idea: Idea;
  isSelected: boolean;
  onSelect: () => void;
}) {
  const TypeIcon = TYPE_CONFIG[idea.type]?.icon || Lightbulb;
  const typeColor = TYPE_CONFIG[idea.type]?.color || "text-text-muted";

  return (
    <div
      onClick={onSelect}
      className={`p-3 rounded-lg border cursor-pointer transition-all ${
        isSelected
          ? "bg-bg-elevated border-accent-green/30"
          : idea.status === "converted"
          ? "bg-bg-secondary/50 border-bg-border opacity-60"
          : "bg-bg-secondary border-bg-border hover:border-bg-hover"
      }`}
    >
      <div className="flex items-start gap-2">
        <TypeIcon size={14} className={`${typeColor} mt-0.5 flex-shrink-0`} />
        <div className="flex-1 min-w-0">
          <p className="text-xs font-medium text-text-primary truncate">
            {idea.title}
          </p>
          <p className="text-[11px] text-text-muted mt-0.5 line-clamp-2">
            {idea.description}
          </p>
          <div className="flex items-center gap-1.5 mt-2">
            <span
              className={`px-1.5 py-0.5 rounded text-[9px] font-medium ${
                SEVERITY_COLORS[idea.severity] || ""
              }`}
            >
              {idea.severity}
            </span>
            <span
              className={`px-1.5 py-0.5 rounded text-[9px] font-medium ${
                EFFORT_COLORS[idea.effort] || ""
              }`}
            >
              {idea.effort}
            </span>
            {idea.status === "converted" && (
              <span className="px-1.5 py-0.5 rounded text-[9px] font-medium bg-accent-green/20 text-accent-green">
                converted
              </span>
            )}
            {idea.affectedFiles.length > 0 && (
              <span className="text-[9px] text-text-muted ml-auto">
                {idea.affectedFiles.length} file
                {idea.affectedFiles.length !== 1 ? "s" : ""}
              </span>
            )}
          </div>
        </div>
        <ChevronRight
          size={12}
          className={`text-text-muted flex-shrink-0 transition-transform ${
            isSelected ? "rotate-90" : ""
          }`}
        />
      </div>
    </div>
  );
}

function IdeaDetail({ idea }: { idea: Idea }) {
  const dismiss = useIdeationStore((s) => s.dismiss);
  const convertToIssue = useIdeationStore((s) => s.convertToIssue);
  const selectIdea = useIdeationStore((s) => s.selectIdea);

  const TypeIcon = TYPE_CONFIG[idea.type]?.icon || Lightbulb;
  const typeColor = TYPE_CONFIG[idea.type]?.color || "text-text-muted";
  const typeLabel = TYPE_CONFIG[idea.type]?.label || idea.type;

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 space-y-4">
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2">
          <TypeIcon size={16} className={typeColor} />
          <span className={`text-xs font-medium ${typeColor}`}>
            {typeLabel}
          </span>
        </div>
        <button
          onClick={() => selectIdea(null)}
          className="text-text-muted hover:text-text-primary"
        >
          <X size={14} />
        </button>
      </div>

      <h3 className="text-sm font-medium text-text-primary">{idea.title}</h3>

      <div className="flex gap-2">
        <span
          className={`px-2 py-0.5 rounded text-[10px] font-medium ${
            SEVERITY_COLORS[idea.severity]
          }`}
        >
          {idea.severity} severity
        </span>
        <span
          className={`px-2 py-0.5 rounded text-[10px] font-medium ${
            EFFORT_COLORS[idea.effort]
          }`}
        >
          {idea.effort} effort
        </span>
      </div>

      <div>
        <h4 className="text-[11px] text-text-muted mb-1 uppercase tracking-wider">
          Description
        </h4>
        <p className="text-xs text-text-secondary leading-relaxed">
          {idea.description}
        </p>
      </div>

      <div>
        <h4 className="text-[11px] text-text-muted mb-1 uppercase tracking-wider">
          Suggestion
        </h4>
        <p className="text-xs text-text-secondary leading-relaxed">
          {idea.suggestion}
        </p>
      </div>

      {idea.affectedFiles.length > 0 && (
        <div>
          <h4 className="text-[11px] text-text-muted mb-1 uppercase tracking-wider">
            Affected Files
          </h4>
          <div className="space-y-0.5">
            {idea.affectedFiles.map((f, i) => (
              <p key={i} className="text-[11px] text-accent-amber font-mono">
                {f}
              </p>
            ))}
          </div>
        </div>
      )}

      {idea.status === "active" && (
        <div className="flex gap-2 pt-2 border-t border-bg-border">
          <button
            onClick={() => convertToIssue(idea.id)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-accent-green/10 text-accent-green border border-accent-green/20 rounded-lg hover:bg-accent-green/20 transition-colors"
          >
            <ArrowRight size={12} />
            Convert to Issue
          </button>
          <button
            onClick={() => dismiss(idea.id)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-bg-elevated text-text-muted rounded-lg hover:text-text-secondary transition-colors"
          >
            <X size={12} />
            Dismiss
          </button>
        </div>
      )}

      {idea.status === "converted" && (
        <div className="flex items-center gap-2 pt-2 border-t border-bg-border">
          <ExternalLink size={12} className="text-accent-green" />
          <span className="text-xs text-accent-green">
            Converted to issue
          </span>
        </div>
      )}
    </div>
  );
}

export function IdeationView() {
  const session = useIdeationStore((s) => s.session);
  const isGenerating = useIdeationStore((s) => s.isGenerating);
  const selectedIdeaId = useIdeationStore((s) => s.selectedIdeaId);
  const generate = useIdeationStore((s) => s.generate);
  const selectIdea = useIdeationStore((s) => s.selectIdea);
  const clearAll = useIdeationStore((s) => s.clearAll);

  const [enabledTypes, setEnabledTypes] = useState<IdeationType[]>(ALL_TYPES);
  const [showDismissed, setShowDismissed] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(
    new Set()
  );

  function toggleType(t: IdeationType) {
    setEnabledTypes((prev) =>
      prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]
    );
  }

  function toggleGroup(type: string) {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(type)) next.delete(type);
      else next.add(type);
      return next;
    });
  }

  async function handleGenerate() {
    if (enabledTypes.length === 0) return;
    setError(null);
    try {
      await generate(enabledTypes);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  const ideas = session?.ideas || [];
  const visibleIdeas = ideas.filter(
    (i) =>
      enabledTypes.includes(i.type) &&
      (showDismissed || i.status !== "dismissed")
  );

  // Group by type
  const grouped = new Map<IdeationType, Idea[]>();
  for (const idea of visibleIdeas) {
    const list = grouped.get(idea.type) || [];
    list.push(idea);
    grouped.set(idea.type, list);
  }

  const activeCount = ideas.filter((i) => i.status === "active").length;
  const convertedCount = ideas.filter((i) => i.status === "converted").length;
  const dismissedCount = ideas.filter((i) => i.status === "dismissed").length;

  const selectedIdea = ideas.find((i) => i.id === selectedIdeaId);

  return (
    <div className="flex flex-col flex-1 overflow-hidden bg-bg-primary">
      {/* Header */}
      <div className="px-4 py-3 border-b border-bg-border bg-bg-secondary">
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <Lightbulb size={16} className="text-accent-amber" />
            <h2 className="text-sm font-medium text-text-primary">
              Ideation Scanner
            </h2>
            {ideas.length > 0 && (
              <span className="text-[11px] text-text-muted">
                {ideas.length} ideas ({activeCount} active
                {convertedCount > 0 ? `, ${convertedCount} converted` : ""}
                {dismissedCount > 0 ? `, ${dismissedCount} dismissed` : ""})
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            {session && (
              <button
                onClick={clearAll}
                className="px-2.5 py-1 text-[11px] text-text-muted hover:text-text-secondary transition-colors"
              >
                Clear
              </button>
            )}
            <button
              onClick={handleGenerate}
              disabled={isGenerating || enabledTypes.length === 0}
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-accent-green/10 text-accent-green border border-accent-green/20 rounded-lg hover:bg-accent-green/20 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {isGenerating ? (
                <Loader2 size={12} className="animate-spin" />
              ) : (
                <RefreshCw size={12} />
              )}
              {session ? "Refresh" : "Generate Ideas"}
            </button>
          </div>
        </div>

        {/* Type filters */}
        <div className="flex items-center gap-2 flex-wrap">
          {ALL_TYPES.map((t) => {
            const cfg = TYPE_CONFIG[t];
            const Icon = cfg.icon;
            const active = enabledTypes.includes(t);
            return (
              <button
                key={t}
                onClick={() => toggleType(t)}
                className={`flex items-center gap-1 px-2 py-1 text-[10px] rounded-lg border transition-colors ${
                  active
                    ? "bg-bg-elevated border-bg-border text-text-primary"
                    : "border-transparent text-text-muted hover:text-text-secondary"
                }`}
              >
                <Icon size={10} className={active ? cfg.color : ""} />
                {cfg.label}
              </button>
            );
          })}
          <div className="w-px h-4 bg-bg-border mx-1" />
          <button
            onClick={() => setShowDismissed(!showDismissed)}
            className="flex items-center gap-1 px-2 py-1 text-[10px] text-text-muted hover:text-text-secondary transition-colors"
          >
            {showDismissed ? <EyeOff size={10} /> : <Eye size={10} />}
            {showDismissed ? "Hide" : "Show"} dismissed
          </button>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="px-4 py-2 bg-red-500/10 border-b border-red-500/20">
          <p className="text-xs text-red-400">{error}</p>
        </div>
      )}

      {/* Content */}
      <div className="flex flex-1 overflow-hidden">
        {/* Ideas list */}
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {isGenerating && (
            <div className="flex flex-col items-center justify-center py-16">
              <Loader2
                size={24}
                className="animate-spin text-accent-green mb-3"
              />
              <p className="text-sm text-text-secondary">
                Analyzing your codebase...
              </p>
              <p className="text-[11px] text-text-muted mt-1">
                This may take a minute
              </p>
            </div>
          )}

          {!isGenerating && ideas.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16">
              <Lightbulb
                size={32}
                className="text-text-muted mb-3"
              />
              <p className="text-sm text-text-secondary mb-1">
                No ideas generated yet
              </p>
              <p className="text-[11px] text-text-muted mb-4">
                Select categories and click Generate to scan your codebase
              </p>
            </div>
          )}

          {!isGenerating &&
            Array.from(grouped.entries()).map(([type, typeIdeas]) => {
              const cfg = TYPE_CONFIG[type];
              const Icon = cfg?.icon || Lightbulb;
              const collapsed = collapsedGroups.has(type);

              return (
                <div key={type}>
                  <button
                    onClick={() => toggleGroup(type)}
                    className="flex items-center gap-2 mb-2 text-xs text-text-secondary hover:text-text-primary transition-colors"
                  >
                    {collapsed ? (
                      <ChevronRight size={12} />
                    ) : (
                      <ChevronDown size={12} />
                    )}
                    <Icon
                      size={12}
                      className={cfg?.color || "text-text-muted"}
                    />
                    <span className="font-medium">
                      {cfg?.label || type}
                    </span>
                    <span className="text-text-muted">
                      ({typeIdeas.length})
                    </span>
                  </button>
                  {!collapsed && (
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-2 ml-5">
                      {typeIdeas.map((idea) => (
                        <IdeaCard
                          key={idea.id}
                          idea={idea}
                          isSelected={idea.id === selectedIdeaId}
                          onSelect={() =>
                            selectIdea(
                              idea.id === selectedIdeaId ? null : idea.id
                            )
                          }
                        />
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
        </div>

        {/* Detail panel */}
        {selectedIdea && (
          <div className="w-80 flex-shrink-0 border-l border-bg-border overflow-y-auto p-4">
            <IdeaDetail idea={selectedIdea} />
          </div>
        )}
      </div>
    </div>
  );
}

import { X, ArrowRight, ExternalLink } from "lucide-react";
import { useIdeationStore } from "@/stores/ideationStore";
import type { Idea } from "@/types/ideation";
import { getTypeConfig, SEVERITY_COLORS, EFFORT_COLORS } from "./ideationConfig";

export function IdeaDetail({ idea }: { idea: Idea }) {
  const dismiss = useIdeationStore((s) => s.dismiss);
  const convertToIssue = useIdeationStore((s) => s.convertToIssue);
  const selectIdea = useIdeationStore((s) => s.selectIdea);

  const { icon: TypeIcon, color: typeColor, label: typeLabel } = getTypeConfig(idea.type);

  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 space-y-4">
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2">
          <TypeIcon size={16} className={typeColor} />
          <span className={`text-xs font-medium ${typeColor}`}>{typeLabel}</span>
        </div>
        <button onClick={() => selectIdea(null)} className="text-text-muted hover:text-text-primary">
          <X size={14} />
        </button>
      </div>

      <h3 className="text-sm font-medium text-text-primary">{idea.title}</h3>

      <div className="flex gap-2">
        <span className={`px-2 py-0.5 rounded text-[10px] font-medium ${SEVERITY_COLORS[idea.severity]}`}>
          {idea.severity} severity
        </span>
        <span className={`px-2 py-0.5 rounded text-[10px] font-medium ${EFFORT_COLORS[idea.effort]}`}>
          {idea.effort} effort
        </span>
      </div>

      <div>
        <h4 className="text-[11px] text-text-muted mb-1 uppercase tracking-wider">Description</h4>
        <p className="text-xs text-text-secondary leading-relaxed">{idea.description}</p>
      </div>

      <div>
        <h4 className="text-[11px] text-text-muted mb-1 uppercase tracking-wider">Suggestion</h4>
        <p className="text-xs text-text-secondary leading-relaxed">{idea.suggestion}</p>
      </div>

      {idea.affectedFiles.length > 0 && (
        <div>
          <h4 className="text-[11px] text-text-muted mb-1 uppercase tracking-wider">Affected Files</h4>
          <div className="space-y-0.5">
            {idea.affectedFiles.map((f, i) => (
              <p key={i} className="text-[11px] text-accent-amber font-mono">{f}</p>
            ))}
          </div>
        </div>
      )}

      {idea.status === "active" && (
        <div className="flex gap-2 pt-2 border-t border-bg-border">
          <button onClick={() => convertToIssue(idea.id)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-accent-green/10 text-accent-green border border-accent-green/20 rounded-lg hover:bg-accent-green/20 transition-colors">
            <ArrowRight size={12} />
            Convert to Issue
          </button>
          <button onClick={() => dismiss(idea.id)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-bg-elevated text-text-muted rounded-lg hover:text-text-secondary transition-colors">
            <X size={12} />
            Dismiss
          </button>
        </div>
      )}

      {idea.status === "converted" && (
        <div className="flex items-center gap-2 pt-2 border-t border-bg-border">
          <ExternalLink size={12} className="text-accent-green" />
          <span className="text-xs text-accent-green">Converted to issue</span>
        </div>
      )}
    </div>
  );
}

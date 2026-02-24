import { ChevronRight } from "lucide-react";
import type { Idea } from "@/types/ideation";
import { getTypeConfig, SEVERITY_COLORS, EFFORT_COLORS } from "./ideationConfig";

interface IdeaCardProps {
  idea: Idea;
  isSelected: boolean;
  onSelect: () => void;
}

export function IdeaCard({ idea, isSelected, onSelect }: IdeaCardProps) {
  const { icon: TypeIcon, color: typeColor } = getTypeConfig(idea.type);

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
          <p className="text-xs font-medium text-text-primary truncate">{idea.title}</p>
          <p className="text-[11px] text-text-muted mt-0.5 line-clamp-2">{idea.description}</p>
          <div className="flex items-center gap-1.5 mt-2">
            <span className={`px-1.5 py-0.5 rounded text-[9px] font-medium ${SEVERITY_COLORS[idea.severity] || ""}`}>
              {idea.severity}
            </span>
            <span className={`px-1.5 py-0.5 rounded text-[9px] font-medium ${EFFORT_COLORS[idea.effort] || ""}`}>
              {idea.effort}
            </span>
            {idea.status === "converted" && (
              <span className="px-1.5 py-0.5 rounded text-[9px] font-medium bg-accent-green/20 text-accent-green">converted</span>
            )}
            {idea.affectedFiles.length > 0 && (
              <span className="text-[9px] text-text-muted ml-auto">
                {idea.affectedFiles.length} file{idea.affectedFiles.length !== 1 ? "s" : ""}
              </span>
            )}
          </div>
        </div>
        <ChevronRight size={12} className={`text-text-muted flex-shrink-0 transition-transform ${isSelected ? "rotate-90" : ""}`} />
      </div>
    </div>
  );
}

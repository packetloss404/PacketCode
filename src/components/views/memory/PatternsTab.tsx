import { Sparkles, Loader2, Trash2 } from "lucide-react";
import { useMemoryStore } from "@/stores/memoryStore";

const CATEGORY_COLORS: Record<string, string> = {
  architecture: "text-accent-purple bg-accent-purple/15",
  convention: "text-accent-blue bg-accent-blue/15",
  preference: "text-accent-green bg-accent-green/15",
  pitfall: "text-accent-red bg-accent-red/15",
};

export function PatternsTab({ projectPath }: { projectPath: string }) {
  const { memory, isScanning, refreshPatterns, deletePattern } = useMemoryStore();

  return (
    <div>
      <div className="flex items-center gap-3 mb-4">
        <button
          onClick={() => refreshPatterns(projectPath)}
          disabled={isScanning || memory.sessionSummaries.length === 0}
          className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded hover:bg-accent-purple/25 transition-colors disabled:opacity-50"
        >
          {isScanning ? <Loader2 size={11} className="animate-spin" /> : <Sparkles size={11} />}
          Refresh Patterns
        </button>
        <span className="text-[10px] text-text-muted">
          {memory.patterns.length} patterns
          {memory.sessionSummaries.length === 0 && " (add session summaries first)"}
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {memory.patterns.map((pattern) => (
          <div key={pattern.id} className="flex items-start gap-3 px-3 py-2.5 bg-bg-secondary border border-bg-border rounded-lg">
            <span className={`text-[9px] px-1.5 py-0.5 rounded font-medium shrink-0 mt-0.5 ${CATEGORY_COLORS[pattern.category] || "text-text-muted bg-bg-elevated"}`}>
              {pattern.category}
            </span>
            <div className="flex-1 min-w-0">
              <div className="text-[11px] text-text-primary">{pattern.pattern}</div>
              <div className="flex items-center gap-3 mt-1.5">
                <div className="flex items-center gap-1.5">
                  <span className="text-[9px] text-text-muted">confidence</span>
                  <div className="w-16 h-1 bg-bg-elevated rounded-full overflow-hidden">
                    <div className="h-full bg-accent-green rounded-full" style={{ width: `${pattern.confidence * 100}%` }} />
                  </div>
                  <span className="text-[9px] text-text-muted">{Math.round(pattern.confidence * 100)}%</span>
                </div>
                <span className="text-[9px] text-text-muted">
                  {pattern.sources.length} source{pattern.sources.length !== 1 ? "s" : ""}
                </span>
              </div>
            </div>
            <button onClick={() => deletePattern(pattern.id)} className="p-1 text-text-muted hover:text-accent-red transition-colors shrink-0">
              <Trash2 size={11} />
            </button>
          </div>
        ))}
        {memory.patterns.length === 0 && (
          <div className="text-center py-12 text-[11px] text-text-muted">
            No patterns extracted yet. Add session summaries then click "Refresh Patterns".
          </div>
        )}
      </div>
    </div>
  );
}

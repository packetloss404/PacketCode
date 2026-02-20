import { useState } from "react";
import { ChevronDown, ChevronRight, FileDiff } from "lucide-react";

interface DiffBlockProps {
  filePath: string;
  oldContent?: string;
  newContent?: string;
  diff?: string;
}

function parseDiffLines(diff: string) {
  return diff.split("\n").map((line) => {
    if (line.startsWith("+") && !line.startsWith("+++")) {
      return { type: "added" as const, content: line };
    } else if (line.startsWith("-") && !line.startsWith("---")) {
      return { type: "removed" as const, content: line };
    } else if (line.startsWith("@@")) {
      return { type: "header" as const, content: line };
    }
    return { type: "context" as const, content: line };
  });
}

export function DiffBlock({ filePath, diff }: DiffBlockProps) {
  const [expanded, setExpanded] = useState(true);
  const lines = diff ? parseDiffLines(diff) : [];

  return (
    <div className="border border-bg-border rounded-md overflow-hidden my-1">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-1.5 bg-bg-elevated hover:bg-bg-hover text-left transition-colors"
      >
        {expanded ? (
          <ChevronDown size={12} className="text-text-muted" />
        ) : (
          <ChevronRight size={12} className="text-text-muted" />
        )}
        <FileDiff size={14} className="text-accent-amber" />
        <span className="text-text-primary text-xs font-medium truncate">
          {filePath}
        </span>
        <span className="text-text-muted text-[10px] ml-auto">
          {lines.filter((l) => l.type === "added").length} additions,{" "}
          {lines.filter((l) => l.type === "removed").length} removals
        </span>
      </button>

      {expanded && lines.length > 0 && (
        <div className="overflow-x-auto bg-bg-primary">
          <pre className="text-xs leading-5 font-mono">
            {lines.map((line, i) => (
              <div
                key={i}
                className={`px-3 ${
                  line.type === "added"
                    ? "bg-accent-green/10 text-accent-green"
                    : line.type === "removed"
                      ? "bg-accent-red/10 text-accent-red"
                      : line.type === "header"
                        ? "bg-accent-blue/10 text-accent-blue"
                        : "text-text-secondary"
                }`}
              >
                {line.content}
              </div>
            ))}
          </pre>
        </div>
      )}
    </div>
  );
}

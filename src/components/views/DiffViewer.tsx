import { useMemo } from "react";

interface DiffViewerProps {
  diff: string;
  className?: string;
}

interface DiffLine {
  type: "add" | "remove" | "context" | "header";
  content: string;
}

function parseDiff(diff: string): DiffLine[] {
  return diff.split("\n").map((line) => {
    if (line.startsWith("+++") || line.startsWith("---") || line.startsWith("diff ") || line.startsWith("index ")) {
      return { type: "header", content: line };
    }
    if (line.startsWith("@@")) {
      return { type: "header", content: line };
    }
    if (line.startsWith("+")) {
      return { type: "add", content: line };
    }
    if (line.startsWith("-")) {
      return { type: "remove", content: line };
    }
    return { type: "context", content: line };
  });
}

const lineStyles: Record<DiffLine["type"], string> = {
  add: "bg-accent-green/10 text-accent-green",
  remove: "bg-accent-red/10 text-accent-red",
  context: "text-text-secondary",
  header: "text-accent-blue bg-accent-blue/5 font-semibold",
};

export function DiffViewer({ diff, className = "" }: DiffViewerProps) {
  const lines = useMemo(() => parseDiff(diff), [diff]);

  return (
    <div className={`font-mono text-[11px] overflow-x-auto ${className}`}>
      {lines.map((line, i) => (
        <div
          key={i}
          className={`px-3 py-0.5 whitespace-pre ${lineStyles[line.type]}`}
        >
          {line.content}
        </div>
      ))}
    </div>
  );
}

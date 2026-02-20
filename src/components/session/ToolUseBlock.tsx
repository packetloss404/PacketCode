import { useState } from "react";
import {
  Terminal,
  FileText,
  Search,
  Edit3,
  FolderOpen,
  Globe,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import type { ParsedMessage } from "@/types/messages";

interface ToolUseBlockProps {
  message: ParsedMessage;
  result?: ParsedMessage;
}

const TOOL_ICONS: Record<string, React.ReactNode> = {
  Bash: <Terminal size={14} />,
  Read: <FileText size={14} />,
  Write: <Edit3 size={14} />,
  Edit: <Edit3 size={14} />,
  Grep: <Search size={14} />,
  Glob: <FolderOpen size={14} />,
  WebSearch: <Globe size={14} />,
  WebFetch: <Globe size={14} />,
};

function getToolIcon(toolName: string) {
  return TOOL_ICONS[toolName] || <Terminal size={14} />;
}

function getToolSummary(toolName: string, input?: Record<string, unknown>): string {
  if (!input) return toolName;

  switch (toolName) {
    case "Bash":
      return (input.command as string) || toolName;
    case "Read":
      return (input.file_path as string) || toolName;
    case "Write":
      return `Write → ${(input.file_path as string) || "file"}`;
    case "Edit":
      return `Edit → ${(input.file_path as string) || "file"}`;
    case "Grep":
      return `Grep: ${(input.pattern as string) || ""}`;
    case "Glob":
      return `Glob: ${(input.pattern as string) || ""}`;
    default:
      return toolName;
  }
}

export function ToolUseBlock({ message, result }: ToolUseBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const toolName = message.toolName || "Unknown";

  return (
    <div className="border border-bg-border rounded-md overflow-hidden my-1">
      {/* Header */}
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-1.5 bg-bg-elevated hover:bg-bg-hover text-left transition-colors"
      >
        {expanded ? (
          <ChevronDown size={12} className="text-text-muted flex-shrink-0" />
        ) : (
          <ChevronRight size={12} className="text-text-muted flex-shrink-0" />
        )}
        <span className="text-accent-blue flex-shrink-0">
          {getToolIcon(toolName)}
        </span>
        <span className="text-text-primary text-xs font-medium">
          {toolName}
        </span>
        <span className="text-text-muted text-xs truncate flex-1">
          {getToolSummary(toolName, message.toolInput)}
        </span>
        {message.isStreaming && (
          <span className="text-accent-amber text-[10px] animate-pulse">
            running...
          </span>
        )}
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="border-t border-bg-border">
          {/* Input */}
          {message.toolInput && (
            <div className="p-3 bg-bg-primary">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-1">
                Input
              </div>
              <pre className="selectable text-xs text-text-primary overflow-x-auto whitespace-pre-wrap break-all">
                {JSON.stringify(message.toolInput, null, 2)}
              </pre>
            </div>
          )}

          {/* Result */}
          {result && (
            <div className="p-3 bg-bg-secondary border-t border-bg-border">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-1">
                Output
              </div>
              <pre className="selectable text-xs text-text-primary overflow-x-auto whitespace-pre-wrap break-all max-h-64 overflow-y-auto">
                {result.content}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

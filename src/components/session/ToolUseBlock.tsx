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
  CheckCircle2,
  Loader2,
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
  Task: <Terminal size={14} />,
};

function getToolIcon(toolName: string) {
  return TOOL_ICONS[toolName] || <Terminal size={14} />;
}

function getToolSummary(
  toolName: string,
  input?: Record<string, unknown>
): string {
  if (!input || Object.keys(input).length === 0) return "";

  switch (toolName) {
    case "Bash": {
      const cmd = input.command as string;
      return cmd
        ? cmd.length > 80
          ? cmd.slice(0, 80) + "..."
          : cmd
        : "";
    }
    case "Read":
      return (input.file_path as string) || "";
    case "Write":
      return (input.file_path as string) || "";
    case "Edit":
      return (input.file_path as string) || "";
    case "Grep":
      return (input.pattern as string) || "";
    case "Glob":
      return (input.pattern as string) || "";
    default:
      return "";
  }
}

export function ToolUseBlock({ message, result }: ToolUseBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const toolName = message.toolName || "Unknown";
  const isStreaming = message.isStreaming && !result;
  const summary = getToolSummary(toolName, message.toolInput);

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
        <span className="text-text-primary text-xs font-medium flex-shrink-0">
          {toolName}
        </span>
        {summary && (
          <span className="text-text-muted text-xs truncate flex-1 font-mono">
            {summary}
          </span>
        )}
        <span className="flex-shrink-0 ml-auto">
          {isStreaming ? (
            <Loader2
              size={12}
              className="text-accent-amber animate-spin"
            />
          ) : result ? (
            <CheckCircle2 size={12} className="text-accent-green" />
          ) : null}
        </span>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="border-t border-bg-border">
          {/* Input */}
          {message.toolInput &&
            Object.keys(message.toolInput).length > 0 && (
              <div className="p-3 bg-bg-primary">
                <div className="text-[10px] uppercase tracking-wider text-text-muted mb-1">
                  Input
                </div>
                <pre className="selectable text-xs text-text-primary overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto">
                  {formatToolInput(toolName, message.toolInput)}
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

function formatToolInput(
  toolName: string,
  input: Record<string, unknown>
): string {
  // For Bash, show just the command prominently
  if (toolName === "Bash" && input.command) {
    const parts: string[] = [input.command as string];
    if (input.description) {
      parts.push(`\n# ${input.description}`);
    }
    return parts.join("");
  }

  // For Read/Write/Edit, show file path + relevant content
  if (
    (toolName === "Read" || toolName === "Write" || toolName === "Edit") &&
    input.file_path
  ) {
    const parts: string[] = [input.file_path as string];
    if (input.old_string) {
      parts.push(`\n--- old ---\n${input.old_string}`);
    }
    if (input.new_string) {
      parts.push(`\n+++ new +++\n${input.new_string}`);
    }
    if (input.content && typeof input.content === "string") {
      const content = input.content as string;
      parts.push(
        `\n${content.length > 500 ? content.slice(0, 500) + "\n...(truncated)" : content}`
      );
    }
    return parts.join("");
  }

  return JSON.stringify(input, null, 2);
}

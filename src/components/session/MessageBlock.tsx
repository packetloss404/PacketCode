import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { ParsedMessage } from "@/types/messages";

interface MessageBlockProps {
  message: ParsedMessage;
}

export function MessageBlock({ message }: MessageBlockProps) {
  if (message.type === "text") {
    return (
      <div className="selectable markdown-content">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {message.content}
        </ReactMarkdown>
      </div>
    );
  }

  if (message.type === "thinking") {
    return (
      <div className="selectable border-l-2 border-accent-purple pl-3 text-text-secondary text-xs italic opacity-60">
        <span className="text-accent-purple text-[10px] uppercase tracking-wider font-semibold">
          thinking
        </span>
        <p className="mt-1">{message.content}</p>
      </div>
    );
  }

  if (message.type === "system") {
    return (
      <div className="text-text-muted text-xs italic py-1">
        {message.content}
      </div>
    );
  }

  if (message.type === "result") {
    return (
      <div className="border-t border-bg-border pt-2 mt-2 text-xs text-text-secondary">
        <span className="text-accent-green">Session complete</span>
        {message.costUsd != null && (
          <span className="ml-3 text-accent-amber">
            ${message.costUsd.toFixed(4)}
          </span>
        )}
        {message.sessionId && (
          <span className="ml-3 text-text-muted">
            ID: {message.sessionId.slice(0, 8)}...
          </span>
        )}
      </div>
    );
  }

  if (message.type === "error") {
    return (
      <div className="text-accent-red text-xs font-mono bg-bg-elevated rounded px-2 py-1">
        {message.content}
      </div>
    );
  }

  return null;
}

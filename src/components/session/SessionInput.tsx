import { useState, useRef, useCallback } from "react";
import { Send, Square } from "lucide-react";
import type { SessionStatus } from "@/types/session";

interface SessionInputProps {
  onSubmit: (prompt: string) => void;
  onKill?: () => void;
  status: SessionStatus | null;
  placeholder?: string;
}

export function SessionInput({
  onSubmit,
  onKill,
  status,
  placeholder = "Enter a prompt... (Ctrl+Enter to send)",
}: SessionInputProps) {
  const [input, setInput] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const isRunning = status === "running";
  const canSubmit = input.trim().length > 0 && !isRunning;

  const handleSubmit = useCallback(() => {
    if (!canSubmit) return;
    onSubmit(input.trim());
    setInput("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  }, [input, canSubmit, onSubmit]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && e.ctrlKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit]
  );

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      setInput(e.target.value);
      // Auto-resize
      const el = e.target;
      el.style.height = "auto";
      el.style.height = Math.min(el.scrollHeight, 200) + "px";
    },
    []
  );

  return (
    <div className="border-t border-bg-border bg-bg-secondary p-3">
      <div className="flex items-end gap-2">
        <textarea
          ref={textareaRef}
          value={input}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          rows={1}
          className="flex-1 bg-bg-primary border border-bg-border rounded-md px-3 py-2 text-text-primary text-sm font-mono resize-none outline-none focus:border-accent-green transition-colors placeholder:text-text-muted"
        />
        {isRunning ? (
          <button
            onClick={onKill}
            className="flex items-center justify-center w-9 h-9 rounded-md bg-accent-red/20 text-accent-red hover:bg-accent-red/30 transition-colors"
            title="Stop session"
          >
            <Square size={16} />
          </button>
        ) : (
          <button
            onClick={handleSubmit}
            disabled={!canSubmit}
            className="flex items-center justify-center w-9 h-9 rounded-md bg-accent-green/20 text-accent-green hover:bg-accent-green/30 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            title="Send (Ctrl+Enter)"
          >
            <Send size={16} />
          </button>
        )}
      </div>
    </div>
  );
}

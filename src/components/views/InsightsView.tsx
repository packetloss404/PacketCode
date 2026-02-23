import { useState, useRef, useEffect } from "react";
import {
  MessageSquare,
  Plus,
  Trash2,
  Send,
  Loader2,
  Bot,
  User,
} from "lucide-react";
import { useInsightsStore } from "@/stores/insightsStore";
import type { InsightsMessage } from "@/types/insights";

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatDate(ts: number): string {
  return new Date(ts).toLocaleDateString([], {
    month: "short",
    day: "numeric",
  });
}

/** Simple markdown-ish renderer: handles code fences, inline code, bold, and paragraphs */
function MarkdownContent({ content }: { content: string }) {
  const parts = content.split(/(```[\s\S]*?```)/g);

  return (
    <div className="text-sm leading-relaxed space-y-2">
      {parts.map((part, i) => {
        if (part.startsWith("```")) {
          const match = part.match(/```(\w*)\n?([\s\S]*?)```/);
          const code = match ? match[2] : part.slice(3, -3);
          return (
            <pre
              key={i}
              className="bg-bg-primary rounded p-3 text-xs overflow-x-auto border border-bg-border"
            >
              <code>{code.trim()}</code>
            </pre>
          );
        }
        // Split into paragraphs and handle inline formatting
        return part.split("\n\n").map((para, j) => (
          <p key={`${i}-${j}`} className="whitespace-pre-wrap">
            {para.split(/(`[^`]+`)/).map((seg, k) =>
              seg.startsWith("`") && seg.endsWith("`") ? (
                <code
                  key={k}
                  className="bg-bg-primary px-1 py-0.5 rounded text-xs text-accent-amber"
                >
                  {seg.slice(1, -1)}
                </code>
              ) : (
                <span key={k}>
                  {seg
                    .split(/(\*\*.*?\*\*)/g)
                    .flatMap((fragment, fi) => {
                      const boldMatch = fragment.match(/^\*\*(.+?)\*\*$/);
                      const content = boldMatch ? boldMatch[1] : fragment;
                      const lines = content.split("\n");
                      const elements: React.ReactNode[] = [];
                      lines.forEach((line, li) => {
                        if (li > 0) elements.push(<br key={`${fi}-br-${li}`} />);
                        if (boldMatch) {
                          elements.push(<strong key={`${fi}-${li}`}>{line}</strong>);
                        } else {
                          elements.push(line);
                        }
                      });
                      return elements;
                    })}
                </span>
              )
            )}
          </p>
        ));
      })}
    </div>
  );
}

function MessageBubble({ message }: { message: InsightsMessage }) {
  const isUser = message.role === "user";

  return (
    <div className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""}`}>
      <div
        className={`w-7 h-7 rounded-full flex items-center justify-center flex-shrink-0 ${
          isUser
            ? "bg-accent-green/20 text-accent-green"
            : "bg-accent-purple/20 text-accent-purple"
        }`}
      >
        {isUser ? <User size={14} /> : <Bot size={14} />}
      </div>
      <div
        className={`max-w-[80%] rounded-lg px-4 py-3 ${
          isUser
            ? "bg-accent-green/10 border border-accent-green/20"
            : "bg-bg-secondary border border-bg-border"
        }`}
      >
        {isUser ? (
          <p className="text-sm whitespace-pre-wrap">{message.content}</p>
        ) : (
          <MarkdownContent content={message.content} />
        )}
        <span className="text-[10px] text-text-muted mt-1 block">
          {formatTime(message.timestamp)}
        </span>
      </div>
    </div>
  );
}

export function InsightsView() {
  const sessions = useInsightsStore((s) => s.sessions);
  const activeSessionId = useInsightsStore((s) => s.activeSessionId);
  const isLoading = useInsightsStore((s) => s.isLoading);
  const createSession = useInsightsStore((s) => s.createSession);
  const switchSession = useInsightsStore((s) => s.switchSession);
  const deleteSession = useInsightsStore((s) => s.deleteSession);
  const sendMessage = useInsightsStore((s) => s.sendMessage);

  const [input, setInput] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const activeSession = sessions.find((s) => s.id === activeSessionId);

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [activeSession?.messages.length, isLoading]);

  async function handleSend() {
    const text = input.trim();
    if (!text || isLoading) return;
    setInput("");
    await sendMessage(text);
    inputRef.current?.focus();
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      handleSend();
    }
  }

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* Sidebar */}
      <div className="w-56 flex-shrink-0 bg-bg-secondary border-r border-bg-border flex flex-col">
        <div className="p-3 border-b border-bg-border">
          <button
            onClick={createSession}
            className="flex items-center gap-2 w-full px-3 py-2 text-xs bg-accent-green/10 text-accent-green border border-accent-green/20 rounded-lg hover:bg-accent-green/20 transition-colors"
          >
            <Plus size={12} />
            New Chat
          </button>
        </div>
        <div className="flex-1 overflow-y-auto p-2 space-y-1">
          {sessions.length === 0 && (
            <p className="text-[11px] text-text-muted px-2 py-4 text-center">
              No conversations yet
            </p>
          )}
          {sessions.map((session) => (
            <div
              key={session.id}
              className={`group flex items-center gap-2 px-2.5 py-2 rounded-lg cursor-pointer transition-colors ${
                session.id === activeSessionId
                  ? "bg-bg-elevated text-text-primary"
                  : "text-text-secondary hover:bg-bg-hover hover:text-text-primary"
              }`}
              onClick={() => switchSession(session.id)}
            >
              <MessageSquare size={12} className="flex-shrink-0 text-text-muted" />
              <div className="flex-1 min-w-0">
                <p className="text-[11px] truncate">{session.title}</p>
                <p className="text-[9px] text-text-muted">
                  {formatDate(session.updatedAt)}
                </p>
              </div>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  deleteSession(session.id);
                }}
                className="opacity-0 group-hover:opacity-100 p-0.5 text-text-muted hover:text-accent-red transition-all"
              >
                <Trash2 size={10} />
              </button>
            </div>
          ))}
        </div>
      </div>

      {/* Chat area */}
      <div className="flex-1 flex flex-col bg-bg-primary">
        {activeSession ? (
          <>
            {/* Messages */}
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              {activeSession.messages.length === 0 && (
                <div className="flex items-center justify-center h-full">
                  <div className="text-center">
                    <MessageSquare
                      size={32}
                      className="text-text-muted mx-auto mb-3"
                    />
                    <p className="text-sm text-text-secondary">
                      Ask anything about your codebase
                    </p>
                    <p className="text-[11px] text-text-muted mt-1">
                      Claude will analyze your project files to answer
                    </p>
                  </div>
                </div>
              )}
              {activeSession.messages.map((msg) => (
                <MessageBubble key={msg.id} message={msg} />
              ))}
              {isLoading && (
                <div className="flex gap-3">
                  <div className="w-7 h-7 rounded-full flex items-center justify-center bg-accent-purple/20 text-accent-purple">
                    <Bot size={14} />
                  </div>
                  <div className="bg-bg-secondary border border-bg-border rounded-lg px-4 py-3">
                    <Loader2
                      size={16}
                      className="animate-spin text-accent-purple"
                    />
                  </div>
                </div>
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* Input */}
            <div className="p-3 border-t border-bg-border">
              <div className="flex gap-2">
                <textarea
                  ref={inputRef}
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder="Ask about your codebase... (Ctrl+Enter to send)"
                  className="flex-1 bg-bg-secondary border border-bg-border rounded-lg px-3 py-2 text-sm text-text-primary placeholder-text-muted resize-none focus:outline-none focus:border-accent-green/50"
                  rows={2}
                  disabled={isLoading}
                />
                <button
                  onClick={handleSend}
                  disabled={!input.trim() || isLoading}
                  className="px-3 bg-accent-green/20 text-accent-green rounded-lg hover:bg-accent-green/30 transition-colors disabled:opacity-40 disabled:cursor-not-allowed self-end"
                >
                  <Send size={16} />
                </button>
              </div>
            </div>
          </>
        ) : (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <MessageSquare
                size={40}
                className="text-text-muted mx-auto mb-4"
              />
              <h2 className="text-lg text-text-secondary mb-2">
                Insights Chat
              </h2>
              <p className="text-sm text-text-muted mb-4">
                Ask questions about your codebase and get AI-powered answers
              </p>
              <button
                onClick={createSession}
                className="px-4 py-2 text-sm bg-accent-green/10 text-accent-green border border-accent-green/20 rounded-lg hover:bg-accent-green/20 transition-colors"
              >
                Start a conversation
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

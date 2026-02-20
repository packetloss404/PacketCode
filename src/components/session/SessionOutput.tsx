import { useEffect, useRef } from "react";
import { MessageBlock } from "./MessageBlock";
import { ToolUseBlock } from "./ToolUseBlock";
import type { ParsedMessage } from "@/types/messages";

interface SessionOutputProps {
  messages: ParsedMessage[];
}

export function SessionOutput({ messages }: SessionOutputProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const isAutoScrolling = useRef(true);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el || !isAutoScrolling.current) return;
    el.scrollTop = el.scrollHeight;
  }, [messages]);

  function handleScroll() {
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
    isAutoScrolling.current = atBottom;
  }

  // Build grouped messages: pair tool_use with following tool_result
  const rendered: React.ReactNode[] = [];
  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i];

    if (msg.type === "tool_use" && !msg.isStreaming) {
      // Look ahead for matching tool result
      let result: ParsedMessage | undefined;
      for (let j = i + 1; j < messages.length && j < i + 5; j++) {
        if (messages[j].type === "tool_result") {
          result = messages[j];
          break;
        }
      }
      rendered.push(
        <ToolUseBlock key={msg.id} message={msg} result={result} />
      );
    } else if (msg.type === "tool_result") {
      // Skip standalone tool results — they're paired above
      continue;
    } else if (msg.type === "text" && msg.isStreaming) {
      // Accumulate streaming text deltas into a single block
      let accumulated = msg.content;
      while (
        i + 1 < messages.length &&
        messages[i + 1].type === "text" &&
        messages[i + 1].isStreaming &&
        messages[i + 1].role === "assistant"
      ) {
        i++;
        accumulated += messages[i].content;
      }
      rendered.push(
        <MessageBlock
          key={msg.id}
          message={{ ...msg, content: accumulated }}
        />
      );
    } else {
      rendered.push(<MessageBlock key={msg.id} message={msg} />);
    }
  }

  return (
    <div
      ref={scrollRef}
      onScroll={handleScroll}
      className="flex-1 overflow-y-auto px-4 py-3 space-y-2"
    >
      {rendered.length === 0 ? (
        <div className="flex items-center justify-center h-full text-text-muted text-sm">
          Enter a prompt to start a Claude session
        </div>
      ) : (
        rendered
      )}
    </div>
  );
}

import { useEffect, useRef } from "react";
import { MessageBlock } from "./MessageBlock";
import { ToolUseBlock } from "./ToolUseBlock";
import type { ParsedMessage } from "@/types/messages";

interface SessionOutputProps {
  messages: ParsedMessage[];
}

interface GroupedBlock {
  key: string;
  type: "text" | "tool" | "other";
  messages: ParsedMessage[];
}

/**
 * Group raw streaming messages into renderable blocks.
 * - Consecutive text deltas from assistant → single text block
 * - tool_use start + input deltas → single tool block, paired with following tool_result
 * - thinking deltas → single thinking block
 * - everything else → standalone
 */
function groupMessages(messages: ParsedMessage[]): GroupedBlock[] {
  const blocks: GroupedBlock[] = [];
  let i = 0;

  while (i < messages.length) {
    const msg = messages[i];

    // Accumulate streaming text deltas
    if (msg.type === "text" && msg.role === "assistant") {
      const group: ParsedMessage[] = [msg];
      while (
        i + 1 < messages.length &&
        messages[i + 1].type === "text" &&
        messages[i + 1].role === "assistant"
      ) {
        i++;
        group.push(messages[i]);
      }
      blocks.push({ key: group[0].id, type: "text", messages: group });
      i++;
      continue;
    }

    // Accumulate thinking deltas
    if (msg.type === "thinking") {
      const group: ParsedMessage[] = [msg];
      while (
        i + 1 < messages.length &&
        messages[i + 1].type === "thinking"
      ) {
        i++;
        group.push(messages[i]);
      }
      blocks.push({ key: group[0].id, type: "other", messages: group });
      i++;
      continue;
    }

    // Tool use: collect the start + all input_json deltas into one block,
    // then look for the tool_result that follows
    if (msg.type === "tool_use") {
      const group: ParsedMessage[] = [msg];
      // Collect streaming input deltas
      while (
        i + 1 < messages.length &&
        messages[i + 1].type === "tool_use" &&
        messages[i + 1].isStreaming
      ) {
        i++;
        group.push(messages[i]);
      }
      // Look ahead for tool_result
      let resultMsg: ParsedMessage | undefined;
      if (
        i + 1 < messages.length &&
        messages[i + 1].type === "tool_result"
      ) {
        i++;
        resultMsg = messages[i];
      }
      if (resultMsg) {
        group.push(resultMsg);
      }
      blocks.push({ key: group[0].id, type: "tool", messages: group });
      i++;
      continue;
    }

    // Skip standalone tool_results (they should be consumed above)
    if (msg.type === "tool_result") {
      i++;
      continue;
    }

    // Everything else: system, result, error
    blocks.push({ key: msg.id, type: "other", messages: [msg] });
    i++;
  }

  return blocks;
}

function renderBlock(block: GroupedBlock) {
  if (block.type === "text") {
    // Merge all text content
    const merged = block.messages.map((m) => m.content).join("");
    const first = block.messages[0];
    return (
      <MessageBlock
        key={block.key}
        message={{ ...first, content: merged }}
      />
    );
  }

  if (block.type === "tool") {
    // First message has toolName, rest are input deltas, last might be result
    const msgs = block.messages;
    const startMsg = msgs[0];
    const resultMsg = msgs.find((m) => m.type === "tool_result");

    // Reconstruct tool input from partial JSON deltas
    let toolInput = startMsg.toolInput || {};
    const inputDeltas = msgs
      .filter(
        (m) => m.type === "tool_use" && m.isStreaming && m !== startMsg
      )
      .map((m) => m.content)
      .join("");

    if (inputDeltas) {
      try {
        toolInput = JSON.parse(inputDeltas);
      } catch {
        // Partial JSON — try the accumulated string as-is
        // It might not be complete yet if still streaming
      }
    }

    const toolMsg: ParsedMessage = {
      ...startMsg,
      toolInput,
      isStreaming:
        !resultMsg &&
        msgs[msgs.length - 1].isStreaming === true,
    };

    return (
      <ToolUseBlock key={block.key} message={toolMsg} result={resultMsg} />
    );
  }

  // "other" — render each message individually
  return block.messages.map((msg) => (
    <MessageBlock key={msg.id} message={msg} />
  ));
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
    const atBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight < 50;
    isAutoScrolling.current = atBottom;
  }

  const blocks = groupMessages(messages);

  return (
    <div
      ref={scrollRef}
      onScroll={handleScroll}
      className="flex-1 overflow-y-auto px-4 py-3 space-y-2"
    >
      {blocks.length === 0 ? (
        <div className="flex items-center justify-center h-full text-text-muted text-sm">
          Enter a prompt to start a Claude session
        </div>
      ) : (
        blocks.map(renderBlock)
      )}
    </div>
  );
}

import { useState } from "react";
import { X, Bot } from "lucide-react";
import { useAppStore } from "@/stores/appStore";
import { useLayoutStore } from "@/stores/layoutStore";

type CliChoice = "claude" | "codex";

interface ModelOption {
  label: string;
  value: string | null; // null = system default (no --model flag)
}

const MODELS: ModelOption[] = [
  { label: "System Default", value: null },
  { label: "Opus 4.6", value: "claude-opus-4-6-20250610" },
  { label: "Opus 4.5", value: "claude-opus-4-5-20250514" },
  { label: "Sonnet 4.5", value: "claude-sonnet-4-5-20250514" },
  { label: "Haiku 4.5", value: "claude-haiku-4-5-20250514" },
];

interface NewSessionModalProps {
  defaultCli?: CliChoice;
  onClose: () => void;
}

export function NewSessionModal({ defaultCli = "claude", onClose }: NewSessionModalProps) {
  const [cli, setCli] = useState<CliChoice>(defaultCli);
  const [model, setModel] = useState<string | null>(null);
  const [prompt, setPrompt] = useState("");

  const cliLabel = cli === "claude" ? "Claude" : "Codex";

  function handleStart() {
    const args: string[] = [];
    if (model) {
      args.push("--model", model);
    }

    // Switch to the right view
    useAppStore.getState().setActiveView(cli);

    // Create pane with args and prompt
    if (cli === "codex") {
      useLayoutStore.getState().addCodexPane({
        cliArgs: args.length > 0 ? args : undefined,
        initialPrompt: prompt.trim() || undefined,
      });
    } else {
      useLayoutStore.getState().addPane({
        cliArgs: args.length > 0 ? args : undefined,
        initialPrompt: prompt.trim() || undefined,
      });
    }

    onClose();
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.ctrlKey && e.key === "Enter") {
      e.preventDefault();
      handleStart();
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
      <div className="bg-bg-secondary border border-bg-border rounded-lg w-[420px] overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-bg-border">
          <div className="flex items-center gap-2">
            <Bot size={14} className="text-accent-green" />
            <h2 className="text-sm font-semibold text-text-primary">New Agent Session</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-text-muted hover:text-text-primary transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        <div className="px-5 py-4 flex flex-col gap-4" onKeyDown={handleKeyDown}>
          {/* CLI toggle */}
          <div className="flex rounded-lg border border-bg-border overflow-hidden">
            <button
              onClick={() => setCli("claude")}
              className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                cli === "claude"
                  ? "bg-accent-green/15 text-accent-green border-r border-bg-border"
                  : "bg-bg-primary text-text-muted hover:text-text-secondary border-r border-bg-border"
              }`}
            >
              Claude
            </button>
            <button
              onClick={() => setCli("codex")}
              className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                cli === "codex"
                  ? "bg-accent-green/15 text-accent-green"
                  : "bg-bg-primary text-text-muted hover:text-text-secondary"
              }`}
            >
              Codex
            </button>
          </div>

          {/* Model selection */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1.5 uppercase tracking-wider">
              Model
            </label>
            <div className="flex flex-wrap gap-1.5">
              {MODELS.map((m) => (
                <button
                  key={m.label}
                  onClick={() => setModel(m.value)}
                  className={`px-2.5 py-1 text-[11px] rounded border transition-colors ${
                    model === m.value
                      ? "bg-accent-amber/15 border-accent-amber/40 text-accent-amber font-medium"
                      : "bg-bg-primary border-bg-border text-text-muted hover:text-text-secondary hover:border-text-muted/30"
                  }`}
                >
                  {m.label}
                </button>
              ))}
            </div>
          </div>

          {/* Prompt */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1.5 uppercase tracking-wider">
              What should {cliLabel} work on?
            </label>
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={4}
              placeholder="Describe the task, feature, or question..."
              className="w-full bg-bg-primary border border-bg-border rounded px-3 py-2 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-amber resize-none"
              autoFocus
            />
            <p className="text-[10px] text-text-muted mt-1">
              Press Ctrl+Enter to start
            </p>
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-bg-border">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleStart}
            className="px-4 py-1.5 text-xs bg-accent-green/15 text-accent-green border border-accent-green/30 rounded font-medium hover:bg-accent-green/25 transition-colors"
          >
            Start {cliLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

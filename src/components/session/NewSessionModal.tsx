import { useState } from "react";
import { X, Bot, User } from "lucide-react";
import { useAppStore } from "@/stores/appStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { useProfileStore } from "@/stores/profileStore";
import { useMemoryStore } from "@/stores/memoryStore";

type CliChoice = "claude" | "codex";

interface ModelOption {
  label: string;
  value: string | null; // null = system default (no --model flag)
}

const CLAUDE_MODELS: ModelOption[] = [
  { label: "System Default", value: null },
  { label: "Opus 4.6", value: "claude-opus-4-6-20250610" },
  { label: "Opus 4.5", value: "claude-opus-4-5-20250514" },
  { label: "Sonnet 4.5", value: "claude-sonnet-4-5-20250514" },
  { label: "Haiku 4.5", value: "claude-haiku-4-5-20250514" },
];

const CODEX_MODELS: ModelOption[] = [
  { label: "System Default", value: null },
  { label: "GPT-5.3 Codex", value: "gpt-5.3-codex" },
  { label: "GPT-5.2 Codex", value: "gpt-5.2-codex" },
  { label: "GPT-5.1 Codex", value: "gpt-5.1-codex" },
  { label: "Codex Mini", value: "codex-mini-latest" },
];

interface NewSessionModalProps {
  defaultCli?: CliChoice;
  onClose: () => void;
}

export function NewSessionModal({ defaultCli = "claude", onClose }: NewSessionModalProps) {
  const [cli, setCli] = useState<CliChoice>(defaultCli);
  const [model, setModel] = useState<string | null>(null);
  const [prompt, setPrompt] = useState("");
  const [selectedProfileId, setSelectedProfileId] = useState<string | null>(
    useProfileStore.getState().activeProfileId
  );
  const [includeMemory, setIncludeMemory] = useState(true);

  const profiles = useProfileStore((s) => s.profiles);
  const getContextForSession = useMemoryStore((s) => s.getContextForSession);

  const cliLabel = cli === "claude" ? "Claude" : "Codex";
  const models = cli === "claude" ? CLAUDE_MODELS : CODEX_MODELS;

  function handleCliChange(newCli: CliChoice) {
    setCli(newCli);
    setModel(null); // Reset to system default when switching CLIs
  }

  function handleProfileChange(profileId: string | null) {
    setSelectedProfileId(profileId);
    if (profileId) {
      const profile = profiles.find((p) => p.id === profileId);
      if (profile?.defaultModel) {
        const matchingModel = models.find((m) => m.value && profile.defaultModel.includes(m.value));
        if (matchingModel) setModel(matchingModel.value);
      }
    }
  }

  function handleStart() {
    const args: string[] = [];
    const selectedProfile = selectedProfileId
      ? profiles.find((p) => p.id === selectedProfileId)
      : null;

    // Use profile's default model if set and no manual override
    const effectiveModel = model || (selectedProfile?.defaultModel || null);
    if (effectiveModel) {
      args.push("--model", effectiveModel);
    }

    // Build final prompt with profile system prompt and memory context
    let finalPrompt = "";

    if (selectedProfile?.systemPrompt) {
      finalPrompt += selectedProfile.systemPrompt + "\n\n";
    }

    if (includeMemory) {
      const memoryContext = getContextForSession();
      if (memoryContext.trim()) {
        finalPrompt += memoryContext + "\n\n";
      }
    }

    if (prompt.trim()) {
      finalPrompt += prompt.trim();
    }

    // Set active profile globally
    if (selectedProfileId) {
      useProfileStore.getState().setActiveProfile(selectedProfileId);
    }

    // Switch to the right view
    useAppStore.getState().setActiveView(cli);

    // Create pane with args and prompt
    useLayoutStore.getState().addPane({
      cliCommand: cli,
      cliArgs: args.length > 0 ? args : undefined,
      initialPrompt: finalPrompt.trim() || undefined,
    });

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
              onClick={() => handleCliChange("claude")}
              className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                cli === "claude"
                  ? "bg-accent-green/15 text-accent-green border-r border-bg-border"
                  : "bg-bg-primary text-text-muted hover:text-text-secondary border-r border-bg-border"
              }`}
            >
              Claude
            </button>
            <button
              onClick={() => handleCliChange("codex")}
              className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                cli === "codex"
                  ? "bg-accent-green/15 text-accent-green"
                  : "bg-bg-primary text-text-muted hover:text-text-secondary"
              }`}
            >
              Codex
            </button>
          </div>

          {/* Agent Profile */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1.5 uppercase tracking-wider">
              Agent Profile
            </label>
            <div className="flex flex-wrap gap-1.5">
              <button
                onClick={() => handleProfileChange(null)}
                className={`flex items-center gap-1 px-2.5 py-1 text-[11px] rounded border transition-colors ${
                  !selectedProfileId
                    ? "bg-accent-green/15 border-accent-green/40 text-accent-green font-medium"
                    : "bg-bg-primary border-bg-border text-text-muted hover:text-text-secondary hover:border-text-muted/30"
                }`}
              >
                None
              </button>
              {profiles.map((p) => (
                <button
                  key={p.id}
                  onClick={() => handleProfileChange(p.id)}
                  className={`flex items-center gap-1 px-2.5 py-1 text-[11px] rounded border transition-colors ${
                    selectedProfileId === p.id
                      ? "bg-accent-green/15 border-accent-green/40 text-accent-green font-medium"
                      : "bg-bg-primary border-bg-border text-text-muted hover:text-text-secondary hover:border-text-muted/30"
                  }`}
                  title={p.description}
                >
                  <User size={10} className={p.color} />
                  {p.name}
                </button>
              ))}
            </div>
          </div>

          {/* Memory context toggle */}
          <div className="flex items-center gap-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={includeMemory}
                onChange={(e) => setIncludeMemory(e.target.checked)}
                className="w-3 h-3 rounded border-bg-border accent-accent-green"
              />
              <span className="text-[11px] text-text-secondary">
                Include memory context
              </span>
            </label>
          </div>

          {/* Model selection */}
          <div>
            <label className="block text-[10px] text-text-muted mb-1.5 uppercase tracking-wider">
              Model
            </label>
            <div className="flex flex-wrap gap-1.5">
              {models.map((m) => (
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

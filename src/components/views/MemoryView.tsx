import { useState } from "react";
import { Brain, FileText, History, Sparkles } from "lucide-react";
import { useLayoutStore } from "@/stores/layoutStore";
import { FileMapTab } from "./memory/FileMapTab";
import { SessionHistoryTab } from "./memory/SessionHistoryTab";
import { PatternsTab } from "./memory/PatternsTab";

type MemoryTab = "filemap" | "sessions" | "patterns";

const tabs: { key: MemoryTab; label: string; icon: typeof FileText }[] = [
  { key: "filemap", label: "File Map", icon: FileText },
  { key: "sessions", label: "Session History", icon: History },
  { key: "patterns", label: "Patterns", icon: Sparkles },
];

export function MemoryView() {
  const [activeTab, setActiveTab] = useState<MemoryTab>("filemap");
  const projectPath = useLayoutStore((s) => s.projectPath);

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-bg-border">
        <Brain size={14} className="text-accent-purple" />
        <h2 className="text-xs font-semibold text-text-primary">Memory Layer</h2>
        <div className="flex-1" />
        <div className="flex gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-[11px] rounded transition-colors ${
                activeTab === tab.key
                  ? "bg-bg-elevated text-accent-purple"
                  : "text-text-muted hover:text-text-primary hover:bg-bg-hover"
              }`}
            >
              <tab.icon size={11} />
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {activeTab === "filemap" && <FileMapTab projectPath={projectPath} />}
        {activeTab === "sessions" && <SessionHistoryTab projectPath={projectPath} />}
        {activeTab === "patterns" && <PatternsTab projectPath={projectPath} />}
      </div>
    </div>
  );
}

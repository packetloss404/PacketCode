import { useState } from "react";
import {
  Brain,
  FileText,
  History,
  Sparkles,
  RefreshCw,
  Trash2,
  Search,
  Loader2,
  ChevronDown,
  ChevronRight,
  AlertCircle,
} from "lucide-react";
import { useMemoryStore } from "@/stores/memoryStore";
import { useLayoutStore } from "@/stores/layoutStore";

type MemoryTab = "filemap" | "sessions" | "patterns";

const CATEGORY_COLORS: Record<string, string> = {
  architecture: "text-accent-purple bg-accent-purple/15",
  convention: "text-accent-blue bg-accent-blue/15",
  preference: "text-accent-green bg-accent-green/15",
  pitfall: "text-accent-red bg-accent-red/15",
};

export function MemoryView() {
  const [activeTab, setActiveTab] = useState<MemoryTab>("filemap");
  const projectPath = useLayoutStore((s) => s.projectPath);

  const tabs: { key: MemoryTab; label: string; icon: typeof FileText }[] = [
    { key: "filemap", label: "File Map", icon: FileText },
    { key: "sessions", label: "Session History", icon: History },
    { key: "patterns", label: "Patterns", icon: Sparkles },
  ];

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-bg-border">
        <Brain size={14} className="text-accent-purple" />
        <h2 className="text-xs font-semibold text-text-primary">
          Memory Layer
        </h2>
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

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto p-4">
        {activeTab === "filemap" && <FileMapTab projectPath={projectPath} />}
        {activeTab === "sessions" && (
          <SessionHistoryTab projectPath={projectPath} />
        )}
        {activeTab === "patterns" && (
          <PatternsTab projectPath={projectPath} />
        )}
      </div>
    </div>
  );
}

function FileMapTab({ projectPath }: { projectPath: string }) {
  const { memory, isScanning, scanError, scanCodebase } = useMemoryStore();
  const [searchQuery, setSearchQuery] = useState("");

  const filtered = memory.fileMap.filter(
    (f) =>
      f.path.toLowerCase().includes(searchQuery.toLowerCase()) ||
      f.summary.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div>
      <div className="flex items-center gap-3 mb-4">
        <button
          onClick={() => scanCodebase(projectPath)}
          disabled={isScanning}
          className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded hover:bg-accent-purple/25 transition-colors disabled:opacity-50"
        >
          {isScanning ? (
            <Loader2 size={11} className="animate-spin" />
          ) : (
            <RefreshCw size={11} />
          )}
          {isScanning ? "Scanning..." : "Scan Codebase"}
        </button>
        {memory.lastScanAt && (
          <span className="text-[10px] text-text-muted">
            Last scan: {new Date(memory.lastScanAt).toLocaleString()}
          </span>
        )}
      </div>

      {scanError && (
        <div className="flex items-center gap-2 px-3 py-2 mb-3 bg-accent-red/10 rounded text-[11px] text-accent-red">
          <AlertCircle size={12} />
          {scanError}
        </div>
      )}

      {memory.fileMap.length > 0 && (
        <div className="mb-3">
          <div className="flex items-center gap-2 bg-bg-secondary border border-bg-border rounded px-2.5 py-1.5">
            <Search size={11} className="text-text-muted" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search files..."
              className="flex-1 bg-transparent text-[11px] text-text-primary placeholder:text-text-muted focus:outline-none"
            />
            <span className="text-[10px] text-text-muted">
              {filtered.length} files
            </span>
          </div>
        </div>
      )}

      <div className="flex flex-col gap-1">
        {filtered.map((file) => (
          <div
            key={file.path}
            className="flex items-start gap-3 px-3 py-2 bg-bg-secondary border border-bg-border rounded hover:bg-bg-hover transition-colors"
          >
            <FileText size={12} className="text-text-muted mt-0.5 shrink-0" />
            <div className="min-w-0">
              <div className="text-[11px] text-text-primary font-mono truncate">
                {file.path}
              </div>
              <div className="text-[10px] text-text-muted mt-0.5">
                {file.summary}
              </div>
            </div>
          </div>
        ))}
        {memory.fileMap.length === 0 && !isScanning && (
          <div className="text-center py-12 text-[11px] text-text-muted">
            No file map yet. Click "Scan Codebase" to generate one.
          </div>
        )}
      </div>
    </div>
  );
}

function SessionHistoryTab({ projectPath }: { projectPath: string }) {
  const { memory, isScanning, addSessionSummary, deleteSummary } =
    useMemoryStore();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [showManualForm, setShowManualForm] = useState(false);
  const [manualTitle, setManualTitle] = useState("");
  const [manualLog, setManualLog] = useState("");

  async function handleManualSummarize() {
    if (!manualTitle.trim() || !manualLog.trim()) return;
    await addSessionSummary(projectPath, manualTitle, manualLog);
    setManualTitle("");
    setManualLog("");
    setShowManualForm(false);
  }

  return (
    <div>
      <div className="flex items-center gap-3 mb-4">
        <button
          onClick={() => setShowManualForm(!showManualForm)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-blue/15 text-accent-blue border border-accent-blue/30 rounded hover:bg-accent-blue/25 transition-colors"
        >
          <History size={11} />
          Summarize Session
        </button>
        <span className="text-[10px] text-text-muted">
          {memory.sessionSummaries.length} summaries
        </span>
      </div>

      {showManualForm && (
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 mb-4">
          <input
            type="text"
            value={manualTitle}
            onChange={(e) => setManualTitle(e.target.value)}
            placeholder="Session title..."
            className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-blue mb-2"
          />
          <textarea
            value={manualLog}
            onChange={(e) => setManualLog(e.target.value)}
            placeholder="Paste session log or notes here..."
            rows={4}
            className="w-full bg-bg-primary border border-bg-border rounded px-3 py-2 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-blue resize-none mb-2"
          />
          <div className="flex gap-2">
            <button
              onClick={handleManualSummarize}
              disabled={isScanning}
              className="px-3 py-1.5 text-[11px] bg-accent-blue/15 text-accent-blue rounded hover:bg-accent-blue/25 transition-colors disabled:opacity-50"
            >
              {isScanning ? "Summarizing..." : "Generate Summary"}
            </button>
            <button
              onClick={() => setShowManualForm(false)}
              className="px-3 py-1.5 text-[11px] text-text-muted hover:text-text-primary transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="flex flex-col gap-2">
        {memory.sessionSummaries.map((session) => (
          <div
            key={session.id}
            className="bg-bg-secondary border border-bg-border rounded-lg overflow-hidden"
          >
            <button
              onClick={() =>
                setExpandedId(expandedId === session.id ? null : session.id)
              }
              className="flex items-center gap-2 w-full px-3 py-2.5 text-left hover:bg-bg-hover transition-colors"
            >
              {expandedId === session.id ? (
                <ChevronDown size={11} className="text-text-muted" />
              ) : (
                <ChevronRight size={11} className="text-text-muted" />
              )}
              <span className="text-[11px] text-text-primary font-medium flex-1 truncate">
                {session.sessionTitle}
              </span>
              <span className="text-[9px] text-text-muted">
                {new Date(session.createdAt).toLocaleDateString()}
              </span>
            </button>

            {expandedId === session.id && (
              <div className="px-3 pb-3 border-t border-bg-border pt-2">
                <p className="text-[11px] text-text-secondary mb-2">
                  {session.summary}
                </p>
                {session.keyDecisions.length > 0 && (
                  <div className="mb-2">
                    <span className="text-[10px] text-text-muted uppercase tracking-wider">
                      Key Decisions
                    </span>
                    <ul className="mt-1 space-y-0.5">
                      {session.keyDecisions.map((d, i) => (
                        <li
                          key={i}
                          className="text-[10px] text-text-secondary pl-2 border-l border-accent-amber/30"
                        >
                          {d}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                {session.filesModified.length > 0 && (
                  <div className="mb-2">
                    <span className="text-[10px] text-text-muted uppercase tracking-wider">
                      Files Modified
                    </span>
                    <div className="flex flex-wrap gap-1 mt-1">
                      {session.filesModified.map((f) => (
                        <span
                          key={f}
                          className="text-[9px] px-1.5 py-0.5 bg-bg-elevated text-text-muted rounded font-mono"
                        >
                          {f}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
                <button
                  onClick={() => deleteSummary(session.id)}
                  className="flex items-center gap-1 text-[10px] text-accent-red/60 hover:text-accent-red transition-colors mt-1"
                >
                  <Trash2 size={10} />
                  Delete
                </button>
              </div>
            )}
          </div>
        ))}
        {memory.sessionSummaries.length === 0 && (
          <div className="text-center py-12 text-[11px] text-text-muted">
            No session summaries yet. Use "Summarize Session" to add one.
          </div>
        )}
      </div>
    </div>
  );
}

function PatternsTab({ projectPath }: { projectPath: string }) {
  const { memory, isScanning, refreshPatterns, deletePattern } =
    useMemoryStore();

  return (
    <div>
      <div className="flex items-center gap-3 mb-4">
        <button
          onClick={() => refreshPatterns(projectPath)}
          disabled={isScanning || memory.sessionSummaries.length === 0}
          className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded hover:bg-accent-purple/25 transition-colors disabled:opacity-50"
        >
          {isScanning ? (
            <Loader2 size={11} className="animate-spin" />
          ) : (
            <Sparkles size={11} />
          )}
          Refresh Patterns
        </button>
        <span className="text-[10px] text-text-muted">
          {memory.patterns.length} patterns
          {memory.sessionSummaries.length === 0 &&
            " (add session summaries first)"}
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {memory.patterns.map((pattern) => (
          <div
            key={pattern.id}
            className="flex items-start gap-3 px-3 py-2.5 bg-bg-secondary border border-bg-border rounded-lg"
          >
            <span
              className={`text-[9px] px-1.5 py-0.5 rounded font-medium shrink-0 mt-0.5 ${
                CATEGORY_COLORS[pattern.category] || "text-text-muted bg-bg-elevated"
              }`}
            >
              {pattern.category}
            </span>
            <div className="flex-1 min-w-0">
              <div className="text-[11px] text-text-primary">
                {pattern.pattern}
              </div>
              <div className="flex items-center gap-3 mt-1.5">
                {/* Confidence bar */}
                <div className="flex items-center gap-1.5">
                  <span className="text-[9px] text-text-muted">
                    confidence
                  </span>
                  <div className="w-16 h-1 bg-bg-elevated rounded-full overflow-hidden">
                    <div
                      className="h-full bg-accent-green rounded-full"
                      style={{ width: `${pattern.confidence * 100}%` }}
                    />
                  </div>
                  <span className="text-[9px] text-text-muted">
                    {Math.round(pattern.confidence * 100)}%
                  </span>
                </div>
                <span className="text-[9px] text-text-muted">
                  {pattern.sources.length} source
                  {pattern.sources.length !== 1 ? "s" : ""}
                </span>
              </div>
            </div>
            <button
              onClick={() => deletePattern(pattern.id)}
              className="p-1 text-text-muted hover:text-accent-red transition-colors shrink-0"
            >
              <Trash2 size={11} />
            </button>
          </div>
        ))}
        {memory.patterns.length === 0 && (
          <div className="text-center py-12 text-[11px] text-text-muted">
            No patterns extracted yet. Add session summaries then click "Refresh
            Patterns".
          </div>
        )}
      </div>
    </div>
  );
}

import { useState } from "react";
import { History, Trash2, ChevronDown, ChevronRight } from "lucide-react";
import { useMemoryStore } from "@/stores/memoryStore";

export function SessionHistoryTab({ projectPath }: { projectPath: string }) {
  const { memory, isScanning, addSessionSummary, deleteSummary } = useMemoryStore();
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
            <button onClick={handleManualSummarize} disabled={isScanning}
              className="px-3 py-1.5 text-[11px] bg-accent-blue/15 text-accent-blue rounded hover:bg-accent-blue/25 transition-colors disabled:opacity-50">
              {isScanning ? "Summarizing..." : "Generate Summary"}
            </button>
            <button onClick={() => setShowManualForm(false)}
              className="px-3 py-1.5 text-[11px] text-text-muted hover:text-text-primary transition-colors">
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="flex flex-col gap-2">
        {memory.sessionSummaries.map((session) => (
          <div key={session.id} className="bg-bg-secondary border border-bg-border rounded-lg overflow-hidden">
            <button
              onClick={() => setExpandedId(expandedId === session.id ? null : session.id)}
              className="flex items-center gap-2 w-full px-3 py-2.5 text-left hover:bg-bg-hover transition-colors"
            >
              {expandedId === session.id ? <ChevronDown size={11} className="text-text-muted" /> : <ChevronRight size={11} className="text-text-muted" />}
              <span className="text-[11px] text-text-primary font-medium flex-1 truncate">{session.sessionTitle}</span>
              <span className="text-[9px] text-text-muted">{new Date(session.createdAt).toLocaleDateString()}</span>
            </button>

            {expandedId === session.id && (
              <div className="px-3 pb-3 border-t border-bg-border pt-2">
                <p className="text-[11px] text-text-secondary mb-2">{session.summary}</p>
                {session.keyDecisions.length > 0 && (
                  <div className="mb-2">
                    <span className="text-[10px] text-text-muted uppercase tracking-wider">Key Decisions</span>
                    <ul className="mt-1 space-y-0.5">
                      {session.keyDecisions.map((d, i) => (
                        <li key={i} className="text-[10px] text-text-secondary pl-2 border-l border-accent-amber/30">{d}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {session.filesModified.length > 0 && (
                  <div className="mb-2">
                    <span className="text-[10px] text-text-muted uppercase tracking-wider">Files Modified</span>
                    <div className="flex flex-wrap gap-1 mt-1">
                      {session.filesModified.map((f) => (
                        <span key={f} className="text-[9px] px-1.5 py-0.5 bg-bg-elevated text-text-muted rounded font-mono">{f}</span>
                      ))}
                    </div>
                  </div>
                )}
                <button onClick={() => deleteSummary(session.id)}
                  className="flex items-center gap-1 text-[10px] text-accent-red/60 hover:text-accent-red transition-colors mt-1">
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

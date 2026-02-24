import { useState } from "react";
import { Loader2, Check, ChevronLeft, AlertCircle } from "lucide-react";
import { parseSpecToTickets } from "@/lib/tauri";
import { useIssueStore } from "@/stores/issueStore";
import { useAppStore } from "@/stores/appStore";
import { Modal } from "@/components/ui/Modal";
import { parseJsonFromResponse, generateId } from "@/lib/storage";
import { getPriorityColor } from "@/lib/colors";
import type { TicketCandidate } from "@/types/spec";

type Phase = "paste" | "loading" | "preview";

function parseTickets(raw: string): TicketCandidate[] {
  const parsed = parseJsonFromResponse(raw);
  if (!Array.isArray(parsed)) {
    throw new Error("Expected a JSON array of tickets");
  }
  return parsed.map((t: Record<string, unknown>) => ({
    title: String(t.title || "Untitled"),
    description: String(t.description || ""),
    priority: (["low", "medium", "high", "critical"].includes(t.priority as string)
      ? t.priority
      : "medium") as TicketCandidate["priority"],
    labels: Array.isArray(t.labels) ? t.labels.map(String) : [],
    acceptanceCriteria: Array.isArray(t.acceptanceCriteria)
      ? t.acceptanceCriteria.map(String)
      : [],
    selected: true,
  }));
}

interface SpecImportModalProps {
  onClose: () => void;
}

export function SpecImportModal({ onClose }: SpecImportModalProps) {
  const [phase, setPhase] = useState<Phase>("paste");
  const [specText, setSpecText] = useState("");
  const [tickets, setTickets] = useState<TicketCandidate[]>([]);
  const [error, setError] = useState<string | null>(null);

  const addIssue = useIssueStore((s) => s.addIssue);
  const setActiveView = useAppStore((s) => s.setActiveView);

  const selectedCount = tickets.filter((t) => t.selected).length;

  async function handleGenerate() {
    if (!specText.trim()) return;
    setPhase("loading");
    setError(null);
    try {
      const raw = await parseSpecToTickets(specText);
      const parsed = parseTickets(raw);
      if (parsed.length === 0) {
        throw new Error("No tickets were generated from the spec");
      }
      setTickets(parsed);
      setPhase("preview");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setPhase("paste");
    }
  }

  function toggleTicket(idx: number) {
    setTickets((prev) =>
      prev.map((t, i) => (i === idx ? { ...t, selected: !t.selected } : t))
    );
  }

  function toggleAll() {
    const allSelected = tickets.every((t) => t.selected);
    setTickets((prev) => prev.map((t) => ({ ...t, selected: !allSelected })));
  }

  function handleCreate() {
    const selected = tickets.filter((t) => t.selected);
    for (const ticket of selected) {
      addIssue({
        title: ticket.title,
        description: ticket.description,
        status: "todo",
        priority: ticket.priority,
        labels: ticket.labels,
        epic: null,
        sessionId: null,
        acceptanceCriteria: ticket.acceptanceCriteria.map((text) => ({
          id: generateId("ac", 6),
          text,
          checked: false,
        })),
        blockedBy: [],
        blocks: [],
      });
    }
    onClose();
    setActiveView("issues");
  }

  const footerContent = (
    <div className="flex items-center justify-between">
      {phase === "preview" ? (
        <>
          <button
            onClick={() => { setPhase("paste"); setTickets([]); }}
            className="flex items-center gap-1 text-[11px] text-text-muted hover:text-text-primary transition-colors"
          >
            <ChevronLeft size={12} />
            Back
          </button>
          <button
            onClick={handleCreate}
            disabled={selectedCount === 0}
            className="px-3 py-1.5 text-[11px] font-medium rounded bg-accent-green text-bg-primary hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Create {selectedCount} Ticket{selectedCount !== 1 ? "s" : ""}
          </button>
        </>
      ) : phase === "paste" ? (
        <>
          <div />
          <button
            onClick={handleGenerate}
            disabled={!specText.trim()}
            className="px-3 py-1.5 text-[11px] font-medium rounded bg-accent-green text-bg-primary hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Generate Tickets
          </button>
        </>
      ) : (
        <div />
      )}
    </div>
  );

  return (
    <Modal onClose={onClose} title="Import Spec to Issues" width="w-[720px]" footer={footerContent}>
      <div className="p-4">
          {/* Phase: Paste */}
          {phase === "paste" && (
            <div className="flex flex-col gap-3">
              <p className="text-[11px] text-text-muted">
                Paste your project spec from Vibe Architect below. Claude will
                parse it into structured tickets for the Issues board.
              </p>
              {error && (
                <div className="flex items-center gap-2 px-3 py-2 rounded bg-red-500/10 border border-red-500/30">
                  <AlertCircle size={12} className="text-red-400 shrink-0" />
                  <span className="text-[11px] text-red-400">{error}</span>
                </div>
              )}
              <textarea
                value={specText}
                onChange={(e) => setSpecText(e.target.value)}
                placeholder="Paste your project spec here..."
                className="w-full h-64 px-3 py-2 text-[11px] font-mono bg-bg-primary border border-bg-border rounded text-text-primary placeholder-text-muted focus:outline-none focus:border-accent-green resize-none"
              />
            </div>
          )}

          {/* Phase: Loading */}
          {phase === "loading" && (
            <div className="flex flex-col items-center justify-center py-16 gap-3">
              <Loader2 size={24} className="text-accent-green animate-spin" />
              <span className="text-xs text-text-muted">
                Claude is analyzing your spec...
              </span>
            </div>
          )}

          {/* Phase: Preview */}
          {phase === "preview" && (
            <div className="flex flex-col gap-3">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-text-muted">
                  {selectedCount} of {tickets.length} tickets selected
                </span>
                <button
                  onClick={toggleAll}
                  className="text-[10px] text-accent-green hover:underline"
                >
                  {tickets.every((t) => t.selected)
                    ? "Deselect all"
                    : "Select all"}
                </button>
              </div>

              <div className="flex flex-col gap-2">
                {tickets.map((ticket, idx) => (
                  <div
                    key={idx}
                    className={`flex gap-3 p-3 rounded border transition-colors cursor-pointer ${
                      ticket.selected
                        ? "border-accent-green/40 bg-accent-green/5"
                        : "border-bg-border bg-bg-primary opacity-60"
                    }`}
                    onClick={() => toggleTicket(idx)}
                  >
                    {/* Checkbox */}
                    <div
                      className={`w-4 h-4 mt-0.5 rounded border flex items-center justify-center shrink-0 ${
                        ticket.selected
                          ? "bg-accent-green border-accent-green"
                          : "border-bg-border"
                      }`}
                    >
                      {ticket.selected && (
                        <Check size={10} className="text-bg-primary" />
                      )}
                    </div>

                    {/* Content */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-[11px] font-semibold text-text-primary truncate">
                          {ticket.title}
                        </span>
                        <span
                          className={`text-[9px] px-1.5 py-0.5 rounded font-medium ${getPriorityColor(ticket.priority).cls}`}
                        >
                          {ticket.priority}
                        </span>
                      </div>
                      <p className="text-[10px] text-text-muted line-clamp-2 mb-1.5">
                        {ticket.description}
                      </p>
                      <div className="flex items-center gap-1.5 flex-wrap">
                        {ticket.labels.map((label, i) => (
                          <span
                            key={i}
                            className="text-[9px] px-1.5 py-0.5 rounded bg-bg-border text-text-muted"
                          >
                            {label}
                          </span>
                        ))}
                        {ticket.acceptanceCriteria.length > 0 && (
                          <span className="text-[9px] text-text-muted ml-1">
                            {ticket.acceptanceCriteria.length} AC
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
      </div>
    </Modal>
  );
}

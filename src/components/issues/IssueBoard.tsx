import { useState, useMemo } from "react";
import { Plus, Search, Eye, EyeOff } from "lucide-react";
import {
  useIssueStore,
  type Issue,
  type IssueStatus,
} from "@/stores/issueStore";
import { IssueCard } from "./IssueCard";
import { NewIssueForm } from "./NewIssueForm";
import { IssueDetailView } from "./IssueDetailView";

export function IssueBoard() {
  const issues = useIssueStore((s) => s.issues);
  const epics = useIssueStore((s) => s.epics);
  const labels = useIssueStore((s) => s.labels);
  const getColumns = useIssueStore((s) => s.getColumns);
  const moveIssue = useIssueStore((s) => s.moveIssue);

  const [showNewIssue, setShowNewIssue] = useState(false);
  const [newIssueColumn, setNewIssueColumn] = useState<IssueStatus>("todo");
  const [dragOverColumn, setDragOverColumn] = useState<IssueStatus | null>(null);
  const [selectedIssueId, setSelectedIssueId] = useState<string | null>(null);

  // Filters
  const [searchQuery, setSearchQuery] = useState("");
  const [filterEpic, setFilterEpic] = useState<string>("all");
  const [filterPriority, setFilterPriority] = useState<string>("all");
  const [filterLabel, setFilterLabel] = useState<string>("all");
  const [showCompleted, setShowCompleted] = useState(false);

  const columns = getColumns();

  // Filtered issues
  const filteredIssues = useMemo(() => {
    return issues.filter((issue) => {
      // Search
      if (searchQuery) {
        const q = searchQuery.toLowerCase();
        if (
          !issue.title.toLowerCase().includes(q) &&
          !issue.description.toLowerCase().includes(q) &&
          !issue.ticketId.toLowerCase().includes(q)
        ) {
          return false;
        }
      }
      // Epic filter
      if (filterEpic !== "all" && issue.epic !== filterEpic) return false;
      // Priority filter
      if (filterPriority !== "all" && issue.priority !== filterPriority) return false;
      // Label filter
      if (filterLabel !== "all" && !issue.labels.includes(filterLabel)) return false;
      // Show completed
      if (!showCompleted && issue.status === "done") return false;
      return true;
    });
  }, [issues, searchQuery, filterEpic, filterPriority, filterLabel, showCompleted]);

  // Stats
  const activeCount = issues.filter((i) => i.status !== "done").length;
  const inProgressCount = issues.filter((i) => i.status === "in_progress").length;

  function handleDragStart(e: React.DragEvent, issueId: string) {
    e.dataTransfer.setData("text/plain", issueId);
    e.dataTransfer.effectAllowed = "move";
  }

  function handleDragOver(e: React.DragEvent, columnKey: IssueStatus) {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setDragOverColumn(columnKey);
  }

  function handleDragLeave() {
    setDragOverColumn(null);
  }

  function handleDrop(e: React.DragEvent, columnKey: IssueStatus) {
    e.preventDefault();
    const issueId = e.dataTransfer.getData("text/plain");
    if (issueId) {
      moveIssue(issueId, columnKey);
    }
    setDragOverColumn(null);
  }

  function getIssuesForColumn(status: IssueStatus): Issue[] {
    return filteredIssues.filter((i) => i.status === status);
  }

  return (
    <div className="flex flex-col h-full bg-bg-primary">
      {/* Board header */}
      <div className="flex flex-col border-b border-bg-border">
        {/* Top row: title + epics */}
        <div className="flex items-center gap-3 px-4 py-2">
          <span className="text-xs font-semibold text-accent-green bg-accent-green/10 px-2 py-0.5 rounded">
            Issues
          </span>
          {epics.length > 0 && (
            <span className="text-[10px] text-text-muted">Epics</span>
          )}
          <div className="flex-1" />
          <span className="text-[10px] text-text-muted">
            {activeCount} active &middot; {inProgressCount} in progress
          </span>
        </div>

        {/* Filter row */}
        <div className="flex items-center gap-2 px-4 py-1.5 bg-bg-secondary">
          {/* Epic filter */}
          <select
            value={filterEpic}
            onChange={(e) => setFilterEpic(e.target.value)}
            className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
          >
            <option value="all">Epics</option>
            {epics.map((e) => (
              <option key={e} value={e}>{e}</option>
            ))}
          </select>

          {/* Priority filter */}
          <select
            value={filterPriority}
            onChange={(e) => setFilterPriority(e.target.value)}
            className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
          >
            <option value="all">All priorities</option>
            <option value="critical">Critical</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </select>

          {/* Label filter */}
          <select
            value={filterLabel}
            onChange={(e) => setFilterLabel(e.target.value)}
            className="bg-bg-primary border border-bg-border rounded px-2 py-1 text-[11px] text-text-secondary focus:outline-none focus:border-accent-green"
          >
            <option value="all">Add label filter...</option>
            {labels.map((l) => (
              <option key={l} value={l}>{l}</option>
            ))}
          </select>

          {/* Search */}
          <div className="flex items-center gap-1.5 bg-bg-primary border border-bg-border rounded px-2 py-1 flex-1 max-w-[300px]">
            <Search size={11} className="text-text-muted flex-shrink-0" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search issues..."
              className="bg-transparent text-[11px] text-text-primary placeholder:text-text-muted focus:outline-none w-full"
            />
          </div>

          <div className="flex-1" />

          {/* Show completed toggle */}
          <button
            onClick={() => setShowCompleted(!showCompleted)}
            className={`flex items-center gap-1 px-2 py-1 text-[11px] rounded transition-colors ${
              showCompleted
                ? "text-accent-green bg-accent-green/10"
                : "text-text-muted hover:text-text-secondary"
            }`}
          >
            {showCompleted ? <Eye size={11} /> : <EyeOff size={11} />}
            Show completed
          </button>
        </div>
      </div>

      {/* Kanban columns */}
      <div className="flex flex-1 overflow-x-auto p-3 gap-3">
        {columns.map((col) => {
          // Hide Done column if showCompleted is off
          if (col.key === "done" && !showCompleted) return null;

          const columnIssues = getIssuesForColumn(col.key);
          const isDragOver = dragOverColumn === col.key;

          return (
            <div
              key={col.key}
              className={`flex flex-col min-w-[220px] flex-1 rounded-lg bg-bg-secondary border transition-colors ${
                isDragOver
                  ? "border-accent-green/50"
                  : "border-bg-border"
              }`}
              onDragOver={(e) => handleDragOver(e, col.key)}
              onDragLeave={handleDragLeave}
              onDrop={(e) => handleDrop(e, col.key)}
            >
              {/* Column header */}
              <div className="flex items-center justify-between px-3 py-2 border-b border-bg-border">
                <div className="flex items-center gap-2">
                  <div
                    className={`w-2 h-2 rounded-full ${getColumnColor(col.key)}`}
                  />
                  <span className="text-xs font-medium text-text-primary">
                    {col.label}
                  </span>
                  <span className="text-[10px] text-text-muted bg-bg-elevated px-1.5 rounded-full">
                    {columnIssues.length}
                  </span>
                </div>
                <button
                  onClick={() => {
                    setNewIssueColumn(col.key);
                    setShowNewIssue(true);
                  }}
                  className="p-0.5 text-text-muted hover:text-accent-green transition-colors"
                >
                  <Plus size={12} />
                </button>
              </div>

              {/* Cards */}
              <div className="flex flex-col gap-2 p-2 overflow-y-auto flex-1">
                {columnIssues.length === 0 ? (
                  <div className="text-[10px] text-text-muted text-center py-4">
                    No Issues
                  </div>
                ) : (
                  columnIssues.map((issue) => (
                    <IssueCard
                      key={issue.id}
                      issue={issue}
                      onDragStart={(e) => handleDragStart(e, issue.id)}
                      onClick={() => setSelectedIssueId(issue.id)}
                    />
                  ))
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Full-width new issue button at bottom */}
      <button
        onClick={() => {
          setNewIssueColumn("todo");
          setShowNewIssue(true);
        }}
        className="flex items-center justify-center gap-2 mx-3 mb-3 py-2.5 bg-accent-green/10 border border-accent-green/20 rounded-lg text-accent-green text-xs font-medium hover:bg-accent-green/20 transition-colors"
      >
        <Plus size={14} />
        New Issue
      </button>

      {/* New issue modal */}
      {showNewIssue && (
        <NewIssueForm
          defaultStatus={newIssueColumn}
          onClose={() => setShowNewIssue(false)}
        />
      )}

      {/* Issue detail view modal */}
      {selectedIssueId && (
        <IssueDetailView
          issueId={selectedIssueId}
          onClose={() => setSelectedIssueId(null)}
        />
      )}
    </div>
  );
}

function getColumnColor(status: IssueStatus): string {
  switch (status) {
    case "todo":
      return "bg-text-muted";
    case "in_progress":
      return "bg-accent-blue";
    case "qa":
      return "bg-accent-amber";
    case "done":
      return "bg-accent-green";
    case "blocked":
      return "bg-accent-red";
    case "needs_human":
      return "bg-accent-purple";
    default:
      return "bg-text-muted";
  }
}

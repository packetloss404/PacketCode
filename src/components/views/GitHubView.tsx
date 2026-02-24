import { useState, useEffect } from "react";
import {
  Github,
  RefreshCw,
  Search,
  ExternalLink,
  Download,
  Brain,
  GitPullRequest,
  Loader2,
  AlertCircle,
  X,
} from "lucide-react";
import { useGitHubStore } from "@/stores/githubStore";
import { useIssueStore } from "@/stores/issueStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { MarkdownRenderer } from "@/components/common/MarkdownRenderer";
import type { GitHubIssue } from "@/types/github";

export function GitHubView() {
  const {
    config,
    isConnected,
    isInitializing,
    repos,
    issues,
    isLoading,
    error,
    investigation,
    isInvestigating,
    initializeAuth,
    connect,
    disconnect,
    fetchRepos,
    selectRepo,
    fetchIssues,
    investigateIssue,
    createPR,
    clearError,
    clearInvestigation,
  } = useGitHubStore();

  const addIssue = useIssueStore((s) => s.addIssue);
  const projectPath = useLayoutStore((s) => s.projectPath);

  const [tokenInput, setTokenInput] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedIssue, setSelectedIssue] = useState<GitHubIssue | null>(null);
  const [showPRModal, setShowPRModal] = useState(false);

  useEffect(() => {
    initializeAuth();
  }, [initializeAuth]);

  useEffect(() => {
    if (isConnected && repos.length === 0) {
      fetchRepos();
    }
  }, [isConnected, repos.length, fetchRepos]);

  useEffect(() => {
    if (isConnected && config.selectedRepo) {
      fetchIssues();
    }
  }, [isConnected, config.selectedRepo, fetchIssues]);

  async function handleConnect() {
    if (tokenInput.trim()) {
      await connect(tokenInput.trim());
      setTokenInput("");
    }
  }

  function handleImportIssue(issue: GitHubIssue) {
    addIssue({
      title: `[GH-${issue.number}] ${issue.title}`,
      description: issue.body || "",
      status: "todo",
      priority: "medium",
      labels: issue.labels.map((l) => l.name),
      epic: null,
      sessionId: null,
      acceptanceCriteria: [],
      blockedBy: [],
      blocks: [],
    });
  }

  const filteredIssues = issues.filter(
    (i) =>
      i.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
      String(i.number).includes(searchQuery)
  );

  // Not connected — show setup
  if (!isConnected) {
    return (
      <div className="flex flex-col h-full bg-bg-primary p-4 overflow-y-auto">
        <div className="flex items-center gap-2 mb-6">
          <Github size={16} className="text-text-primary" />
          <h2 className="text-sm font-semibold text-text-primary">
            GitHub Integration
          </h2>
        </div>

        <div className="max-w-md mx-auto mt-16">
          <div className="bg-bg-secondary border border-bg-border rounded-lg p-6 text-center">
            <Github size={32} className="mx-auto mb-4 text-text-muted" />
            <h3 className="text-sm font-semibold text-text-primary mb-2">
              Connect to GitHub
            </h3>
            <p className="text-[11px] text-text-muted mb-4">
              Enter a personal access token with repo scope to browse
              repositories and issues.
            </p>
            {isInitializing && (
              <p className="text-[11px] text-text-muted mb-3">Checking auth state...</p>
            )}
            <div className="flex gap-2">
              <input
                type="password"
                value={tokenInput}
                onChange={(e) => setTokenInput(e.target.value)}
                placeholder="ghp_xxxxxxxxxxxx"
                className="flex-1 bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-green"
                onKeyDown={(e) => e.key === "Enter" && handleConnect()}
              />
              <button
                onClick={handleConnect}
                disabled={isLoading || isInitializing}
                className="px-4 py-1.5 text-xs bg-accent-green/15 text-accent-green border border-accent-green/30 rounded font-medium hover:bg-accent-green/25 transition-colors"
              >
                Connect
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-bg-border">
        <Github size={14} className="text-text-primary" />
        <h2 className="text-xs font-semibold text-text-primary">GitHub</h2>

        {/* Repo selector */}
        <select
          value={
            config.selectedRepo
              ? `${config.selectedRepo.owner}/${config.selectedRepo.repo}`
              : ""
          }
          onChange={(e) => {
            const [owner, repo] = e.target.value.split("/");
            if (owner && repo) selectRepo(owner, repo);
          }}
          className="bg-bg-secondary border border-bg-border rounded px-2 py-1 text-[11px] text-text-primary focus:outline-none focus:border-accent-green min-w-[200px]"
        >
          <option value="">Select repository...</option>
          {repos.map((r) => (
            <option key={r.id} value={r.full_name}>
              {r.private ? "🔒 " : ""}
              {r.full_name}
            </option>
          ))}
        </select>

        <button
          onClick={() => {
            fetchRepos();
            if (config.selectedRepo) fetchIssues();
          }}
          disabled={isLoading}
          className="p-1 text-text-muted hover:text-text-primary transition-colors"
          title="Refresh"
        >
          <RefreshCw size={12} className={isLoading ? "animate-spin" : ""} />
        </button>

        <div className="flex-1" />

        <button
          onClick={() => setShowPRModal(true)}
          className="flex items-center gap-1.5 px-2.5 py-1 text-[11px] bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded hover:bg-accent-purple/25 transition-colors"
        >
          <GitPullRequest size={11} />
          New PR
        </button>

        <button
          onClick={disconnect}
          className="text-[10px] text-text-muted hover:text-accent-red transition-colors"
        >
          Disconnect
        </button>
      </div>

      {/* Error banner */}
      {error && (
        <div className="flex items-center gap-2 px-4 py-2 bg-accent-red/10 border-b border-accent-red/20">
          <AlertCircle size={12} className="text-accent-red" />
          <span className="text-[11px] text-accent-red flex-1">{error}</span>
          <button onClick={clearError} className="text-accent-red/60 hover:text-accent-red">
            <X size={12} />
          </button>
        </div>
      )}

      {/* Main content — 2 columns */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left: Issues list */}
        <div className="w-[360px] border-r border-bg-border flex flex-col">
          <div className="px-3 py-2 border-b border-bg-border">
            <div className="flex items-center gap-2 bg-bg-secondary border border-bg-border rounded px-2 py-1">
              <Search size={11} className="text-text-muted" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search issues..."
                className="flex-1 bg-transparent text-[11px] text-text-primary placeholder:text-text-muted focus:outline-none"
              />
            </div>
          </div>

          <div className="flex-1 overflow-y-auto">
            {isLoading && issues.length === 0 ? (
              <div className="flex items-center justify-center py-12 text-text-muted">
                <Loader2 size={16} className="animate-spin" />
              </div>
            ) : filteredIssues.length === 0 ? (
              <div className="text-center py-12 text-[11px] text-text-muted">
                {config.selectedRepo
                  ? "No open issues found"
                  : "Select a repository to view issues"}
              </div>
            ) : (
              filteredIssues.map((issue) => (
                <button
                  key={issue.number}
                  onClick={() => {
                    setSelectedIssue(issue);
                    clearInvestigation();
                  }}
                  className={`w-full text-left px-3 py-2.5 border-b border-bg-border hover:bg-bg-hover transition-colors ${
                    selectedIssue?.number === issue.number
                      ? "bg-bg-elevated"
                      : ""
                  }`}
                >
                  <div className="flex items-start gap-2">
                    <span className="text-[10px] text-text-muted mt-0.5">
                      #{issue.number}
                    </span>
                    <div className="flex-1 min-w-0">
                      <div className="text-[11px] text-text-primary truncate">
                        {issue.title}
                      </div>
                      <div className="flex items-center gap-1.5 mt-1">
                        {issue.labels.slice(0, 3).map((l) => (
                          <span
                            key={l.name}
                            className="text-[9px] px-1.5 py-0.5 rounded"
                            style={{
                              backgroundColor: `#${l.color}22`,
                              color: `#${l.color}`,
                            }}
                          >
                            {l.name}
                          </span>
                        ))}
                        <span className="text-[9px] text-text-muted ml-auto">
                          {issue.user.login}
                        </span>
                      </div>
                    </div>
                  </div>
                </button>
              ))
            )}
          </div>
        </div>

        {/* Right: Issue detail */}
        <div className="flex-1 overflow-y-auto">
          {selectedIssue ? (
            <div className="p-4">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <h3 className="text-sm font-semibold text-text-primary">
                    #{selectedIssue.number} {selectedIssue.title}
                  </h3>
                  <div className="flex items-center gap-2 mt-1">
                    <span className="text-[10px] text-text-muted">
                      by {selectedIssue.user.login}
                    </span>
                    <span className="text-[10px] text-text-muted">
                      {new Date(selectedIssue.created_at).toLocaleDateString()}
                    </span>
                  </div>
                </div>
                <a
                  href={selectedIssue.html_url}
                  target="_blank"
                  rel="noreferrer"
                  className="p-1 text-text-muted hover:text-text-primary"
                >
                  <ExternalLink size={12} />
                </a>
              </div>

              {/* Labels */}
              {selectedIssue.labels.length > 0 && (
                <div className="flex flex-wrap gap-1.5 mb-3">
                  {selectedIssue.labels.map((l) => (
                    <span
                      key={l.name}
                      className="text-[10px] px-2 py-0.5 rounded"
                      style={{
                        backgroundColor: `#${l.color}22`,
                        color: `#${l.color}`,
                      }}
                    >
                      {l.name}
                    </span>
                  ))}
                </div>
              )}

              {/* Body */}
              <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 mb-4">
                <MarkdownRenderer
                  content={selectedIssue.body || "No description provided."}
                  className="text-[11px] text-text-secondary leading-relaxed"
                />
              </div>

              {/* Action buttons */}
              <div className="flex gap-2 mb-4">
                <button
                  onClick={() => handleImportIssue(selectedIssue)}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-green/15 text-accent-green border border-accent-green/30 rounded hover:bg-accent-green/25 transition-colors"
                >
                  <Download size={11} />
                  Import to Board
                </button>
                <button
                  onClick={() =>
                    investigateIssue(projectPath, selectedIssue.number)
                  }
                  disabled={isInvestigating}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] bg-accent-blue/15 text-accent-blue border border-accent-blue/30 rounded hover:bg-accent-blue/25 transition-colors disabled:opacity-50"
                >
                  {isInvestigating ? (
                    <Loader2 size={11} className="animate-spin" />
                  ) : (
                    <Brain size={11} />
                  )}
                  Investigate with AI
                </button>
              </div>

              {/* AI Investigation result */}
              {(investigation || isInvestigating) && (
                <div className="bg-bg-secondary border border-accent-blue/30 rounded-lg p-4">
                  <h4 className="text-[11px] font-semibold text-accent-blue mb-2 flex items-center gap-1.5">
                    <Brain size={12} />
                    AI Investigation
                  </h4>
                  {isInvestigating ? (
                    <div className="flex items-center gap-2 text-[11px] text-text-muted py-4">
                      <Loader2 size={12} className="animate-spin" />
                      Analyzing codebase...
                    </div>
                  ) : (
                    <MarkdownRenderer
                      content={investigation || ""}
                      className="text-[11px] text-text-secondary leading-relaxed"
                    />
                  )}
                </div>
              )}
            </div>
          ) : (
            <div className="flex items-center justify-center h-full text-[11px] text-text-muted">
              Select an issue to view details
            </div>
          )}
        </div>
      </div>

      {/* PR Modal */}
      {showPRModal && (
        <PRModal
          onClose={() => setShowPRModal(false)}
          onSubmit={createPR}
          isLoading={isLoading}
        />
      )}
    </div>
  );
}

function PRModal({
  onClose,
  onSubmit,
  isLoading,
}: {
  onClose: () => void;
  onSubmit: (
    title: string,
    body: string,
    head: string,
    base: string
  ) => Promise<string>;
  isLoading: boolean;
}) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [head, setHead] = useState("");
  const [base, setBase] = useState("main");
  const [result, setResult] = useState<string | null>(null);

  async function handleSubmit() {
    try {
      const json = await onSubmit(title, body, head, base);
      const pr = JSON.parse(json);
      setResult(pr.html_url || "PR created successfully");
    } catch {
      // error handled by store
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
      <div className="bg-bg-secondary border border-bg-border rounded-lg w-[480px] overflow-hidden">
        <div className="flex items-center justify-between px-5 py-3 border-b border-bg-border">
          <div className="flex items-center gap-2">
            <GitPullRequest size={14} className="text-accent-purple" />
            <h2 className="text-sm font-semibold text-text-primary">
              Create Pull Request
            </h2>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-text-muted hover:text-text-primary"
          >
            <X size={16} />
          </button>
        </div>

        <div className="px-5 py-4 flex flex-col gap-3">
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="PR title"
            className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple"
          />
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Description..."
            rows={4}
            className="w-full bg-bg-primary border border-bg-border rounded px-3 py-2 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple resize-none"
          />
          <div className="flex gap-2">
            <div className="flex-1">
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Head branch
              </label>
              <input
                type="text"
                value={head}
                onChange={(e) => setHead(e.target.value)}
                placeholder="feature-branch"
                className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple"
              />
            </div>
            <div className="flex-1">
              <label className="block text-[10px] text-text-muted mb-1 uppercase tracking-wider">
                Base branch
              </label>
              <input
                type="text"
                value={base}
                onChange={(e) => setBase(e.target.value)}
                placeholder="main"
                className="w-full bg-bg-primary border border-bg-border rounded px-3 py-1.5 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent-purple"
              />
            </div>
          </div>

          {result && (
            <div className="text-[11px] text-accent-green bg-accent-green/10 rounded px-3 py-2">
              {result}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-bg-border">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!title.trim() || !head.trim() || isLoading}
            className="px-4 py-1.5 text-xs bg-accent-purple/15 text-accent-purple border border-accent-purple/30 rounded font-medium hover:bg-accent-purple/25 transition-colors disabled:opacity-50"
          >
            {isLoading ? "Creating..." : "Create PR"}
          </button>
        </div>
      </div>
    </div>
  );
}

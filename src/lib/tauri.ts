import { invoke } from "@tauri-apps/api/core";
import type { SessionInfo } from "@/types/session";
import type { StatusLineData, CodexStatusLineData } from "@/types/statusline";

export async function createSession(
  projectPath: string,
  prompt: string,
  model?: string,
  resumeSessionId?: string
): Promise<string> {
  return invoke<string>("create_session", {
    projectPath,
    prompt,
    model,
    resumeSessionId,
  });
}

export async function sendInput(
  sessionId: string,
  input: string
): Promise<void> {
  return invoke("send_input", { sessionId, input });
}

export async function killSession(sessionId: string): Promise<void> {
  return invoke("kill_session", { sessionId });
}

export async function listSessions(): Promise<SessionInfo[]> {
  return invoke<SessionInfo[]>("list_sessions");
}

export async function getSessionInfo(
  sessionId: string
): Promise<SessionInfo> {
  return invoke<SessionInfo>("get_session_info", { sessionId });
}

export async function getGitBranch(projectPath: string): Promise<string> {
  return invoke<string>("get_git_branch", { projectPath });
}

export async function getGitStatus(projectPath: string): Promise<string> {
  return invoke<string>("get_git_status", { projectPath });
}

export async function readStatusLineStates(): Promise<StatusLineData[]> {
  return invoke<StatusLineData[]>("read_statusline_states");
}

export async function readCodexStatusLineStates(): Promise<CodexStatusLineData[]> {
  return invoke<CodexStatusLineData[]>("read_codex_statusline_states");
}

export async function parseSpecToTickets(specText: string): Promise<string> {
  return invoke<string>("parse_spec_to_tickets", { specText });
}

export async function askInsights(
  projectPath: string,
  messages: { role: string; content: string }[]
): Promise<string> {
  return invoke<string>("ask_insights", { projectPath, messages });
}

export async function generateIdeas(
  projectPath: string,
  ideaTypes: string[]
): Promise<string> {
  return invoke<string>("generate_ideas", { projectPath, ideaTypes });
}

// GitHub integration
export async function githubListRepos(token: string): Promise<string> {
  return invoke<string>("github_list_repos", { token });
}

export async function githubListIssues(
  token: string,
  owner: string,
  repo: string
): Promise<string> {
  return invoke<string>("github_list_issues", { token, owner, repo });
}

export async function githubGetIssue(
  token: string,
  owner: string,
  repo: string,
  issueNumber: number
): Promise<string> {
  return invoke<string>("github_get_issue", { token, owner, repo, issueNumber });
}

export async function githubCreatePr(
  token: string,
  owner: string,
  repo: string,
  title: string,
  body: string,
  head: string,
  base: string
): Promise<string> {
  return invoke<string>("github_create_pr", { token, owner, repo, title, body, head, base });
}

export async function githubInvestigateIssue(
  projectPath: string,
  token: string,
  owner: string,
  repo: string,
  issueNumber: number
): Promise<string> {
  return invoke<string>("github_investigate_issue", { projectPath, token, owner, repo, issueNumber });
}

// Memory layer
export async function scanCodebaseMemory(projectPath: string): Promise<string> {
  return invoke<string>("scan_codebase_memory", { projectPath });
}

export async function summarizeSession(
  projectPath: string,
  sessionLog: string
): Promise<string> {
  return invoke<string>("summarize_session", { projectPath, sessionLog });
}

export async function extractPatterns(
  projectPath: string,
  summaries: string
): Promise<string> {
  return invoke<string>("extract_patterns", { projectPath, summaries });
}

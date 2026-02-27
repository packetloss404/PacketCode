import { invoke } from "@tauri-apps/api/core";
import type { StatusLineData, CodexStatusLineData } from "@/types/statusline";

export async function getGitBranch(projectPath: string): Promise<string> {
  return invoke<string>("get_git_branch", { projectPath });
}

export async function getGitStatus(projectPath: string): Promise<string> {
  return invoke<string>("get_git_status", { projectPath });
}

export async function gitCommit(
  projectPath: string,
  message: string,
  stageAll: boolean
): Promise<string> {
  return invoke<string>("git_commit", { projectPath, message, stageAll });
}

export async function gitPush(projectPath: string): Promise<string> {
  return invoke<string>("git_push", { projectPath });
}

export async function gitPull(projectPath: string): Promise<string> {
  return invoke<string>("git_pull", { projectPath });
}

export async function gitCreateBranch(
  projectPath: string,
  branchName: string,
  checkout: boolean
): Promise<string> {
  return invoke<string>("git_create_branch", { projectPath, branchName, checkout });
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

export async function askInsightsStream(
  projectPath: string,
  messages: { role: string; content: string }[],
  sessionContext?: string
): Promise<void> {
  return invoke("ask_insights_stream", {
    projectPath,
    messages,
    sessionContext: sessionContext || null,
  });
}

export async function generateIdeas(
  projectPath: string,
  ideaTypes: string[]
): Promise<string> {
  return invoke<string>("generate_ideas", { projectPath, ideaTypes });
}

// GitHub integration
export async function githubSetToken(token: string): Promise<void> {
  return invoke("github_set_token", { token });
}

export async function githubClearToken(): Promise<void> {
  return invoke("github_clear_token");
}

export async function githubHasToken(): Promise<boolean> {
  return invoke<boolean>("github_has_token");
}

export async function githubListRepos(): Promise<string> {
  return invoke<string>("github_list_repos");
}

export async function githubListIssues(
  owner: string,
  repo: string
): Promise<string> {
  return invoke<string>("github_list_issues", { owner, repo });
}

export async function githubCreatePr(
  owner: string,
  repo: string,
  title: string,
  body: string,
  head: string,
  base: string
): Promise<string> {
  return invoke<string>("github_create_pr", { owner, repo, title, body, head, base });
}

export async function githubListPrs(
  owner: string,
  repo: string
): Promise<string> {
  return invoke<string>("github_list_prs", { owner, repo });
}

export async function githubGetPrDiff(
  owner: string,
  repo: string,
  prNumber: number
): Promise<string> {
  return invoke<string>("github_get_pr_diff", { owner, repo, prNumber });
}

export async function githubInvestigateIssue(
  projectPath: string,
  owner: string,
  repo: string,
  issueNumber: number
): Promise<string> {
  return invoke<string>("github_investigate_issue", {
    projectPath,
    owner,
    repo,
    issueNumber,
  });
}

// Prompt history
export async function readPromptHistory(): Promise<string> {
  return invoke<string>("read_prompt_history");
}

// Usage analytics
export async function readUsageAnalytics(): Promise<string> {
  return invoke<string>("read_usage_analytics");
}

// MCP server management
import type { McpServerEntry } from "@/types/mcp";
import type { ScaffoldResult, ToolAvailability } from "@/types/scaffold";

export async function readMcpServers(projectPath: string): Promise<McpServerEntry[]> {
  return invoke<McpServerEntry[]>("read_mcp_servers", { projectPath });
}

export async function writeMcpServer(
  projectPath: string,
  name: string,
  command: string,
  args: string[],
  env: Record<string, string>,
  scope: string
): Promise<void> {
  return invoke("write_mcp_server", { projectPath, name, command, args, env, scope });
}

export async function deleteMcpServer(
  projectPath: string,
  name: string,
  scope: string
): Promise<void> {
  return invoke("delete_mcp_server", { projectPath, name, scope });
}

// Project scaffolding
export async function scaffoldProject(
  parentDir: string,
  projectName: string,
  template: string
): Promise<ScaffoldResult> {
  return invoke<ScaffoldResult>("scaffold_project", { parentDir, projectName, template });
}

export async function checkScaffoldTools(): Promise<ToolAvailability> {
  return invoke<ToolAvailability>("check_scaffold_tools");
}

// Deploy pipeline
import type { DeployConfig } from "@/types/deploy";

interface DeployConfigFile {
  configs: DeployConfig[];
  source: string;
}

export async function readDeployConfig(projectPath: string): Promise<DeployConfigFile> {
  return invoke<DeployConfigFile>("read_deploy_config", { projectPath });
}

export async function createDeployConfig(
  projectPath: string,
  configs: DeployConfig[]
): Promise<void> {
  return invoke("create_deploy_config", { projectPath, configs });
}


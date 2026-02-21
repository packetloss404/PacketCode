import { invoke } from "@tauri-apps/api/core";
import type { SessionInfo } from "@/types/session";
import type { StatusLineData } from "@/types/statusline";

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

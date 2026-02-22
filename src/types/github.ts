export interface GitHubRepo {
  id: number;
  full_name: string;
  name: string;
  owner: { login: string };
  description: string | null;
  private: boolean;
  html_url: string;
  updated_at: string;
}

export interface GitHubIssue {
  number: number;
  title: string;
  body: string | null;
  state: string;
  labels: { name: string; color: string }[];
  user: { login: string };
  html_url: string;
  created_at: string;
}

export interface GitHubConfig {
  token: string;
  selectedRepo: { owner: string; repo: string } | null;
}

export const LANG_COLORS: Record<string, string> = {
  typescript: "#3178c6",
  javascript: "#f7df1e",
  rust: "#dea584",
  python: "#3572A5",
  go: "#00ADD8",
  json: "#a0a0a0",
  markdown: "#083fa1",
  html: "#e34c26",
  css: "#563d7c",
  sql: "#e38c00",
  yaml: "#cb171e",
  toml: "#9c4221",
  shell: "#89e051",
  ruby: "#cc342d",
  java: "#b07219",
  cpp: "#f34b7d",
  c: "#555555",
  vue: "#41b883",
  svelte: "#ff3e00",
  dart: "#00B4AB",
  kotlin: "#A97BFF",
  swift: "#F05138",
  php: "#4F5D95",
  csharp: "#178600",
  elixir: "#6e4a7e",
  r: "#198CE7",
  lua: "#000080",
  zig: "#ec915c",
  xml: "#0060ac",
  powershell: "#012456",
};

export function getLangColor(lang: string): string {
  return LANG_COLORS[lang] || "#6e7681";
}

export function formatNumber(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "K";
  return String(n);
}

export function calcCommentScore(ratio: number): number {
  if (ratio >= 0.10 && ratio <= 0.20) return 100;
  if (ratio >= 0.05 && ratio < 0.10) return 60 + (ratio - 0.05) / 0.05 * 40;
  if (ratio > 0.20 && ratio <= 0.30) return 80;
  if (ratio > 0.30) return 60;
  return Math.max(10, Math.round(ratio / 0.05 * 60));
}

export function calcComplexityScore(avg: number): number {
  if (avg <= 3) return 95;
  if (avg <= 5) return 85;
  if (avg <= 8) return 75;
  if (avg <= 12) return 65;
  if (avg <= 18) return 50;
  if (avg <= 25) return 35;
  return 20;
}

export function calcTestScore(ratio: number): number {
  if (ratio >= 0.25) return 100;
  if (ratio >= 0.15) return 80;
  if (ratio >= 0.10) return 60;
  if (ratio >= 0.05) return 40;
  if (ratio > 0) return 20;
  return 0;
}

export function getLetterGrade(score: number): { letter: string; color: string } {
  if (score >= 90) return { letter: "A", color: "#00ff41" };
  if (score >= 80) return { letter: "B", color: "#56d364" };
  if (score >= 65) return { letter: "C", color: "#f0b400" };
  if (score >= 50) return { letter: "D", color: "#f85149" };
  return { letter: "F", color: "#f85149" };
}

export function getComplexityLabel(avg: number): string {
  if (avg <= 3) return "Simple";
  if (avg <= 8) return "Moderate";
  if (avg <= 15) return "Complex";
  return "Very Complex";
}

export interface LanguageStats {
  name: string;
  extension: string;
  files: number;
  code_lines: number;
  comment_lines: number;
  blank_lines: number;
  total_lines: number;
}

export interface FileComplexity {
  path: string;
  language: string;
  lines: number;
  complexity: number;
}

export interface CodeQualityReport {
  total_files: number;
  total_code_lines: number;
  total_lines: number;
  total_comment_lines: number;
  total_blank_lines: number;
  language_count: number;
  languages: LanguageStats[];
  avg_complexity: number;
  test_files: number;
  test_lines: number;
  top_complex_files: FileComplexity[];
  comment_ratio: number;
  test_ratio: number;
  org_score: number;
}

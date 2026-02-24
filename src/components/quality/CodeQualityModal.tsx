import { useState, useEffect } from "react";
import { Diamond, Code2, FileText, Gauge, TestTube2 } from "lucide-react";
import { invoke } from "@tauri-apps/api/core";
import { Modal } from "@/components/ui/Modal";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore } from "@/stores/appStore";

interface LanguageStats {
  name: string;
  extension: string;
  files: number;
  code_lines: number;
  comment_lines: number;
  blank_lines: number;
  total_lines: number;
}

interface FileComplexity {
  path: string;
  language: string;
  lines: number;
  complexity: number;
}

interface CodeQualityReport {
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

type TabKey = "overview" | "languages" | "complexity" | "tests";

const TABS: { key: TabKey; label: string }[] = [
  { key: "overview", label: "Overview" },
  { key: "languages", label: "Languages" },
  { key: "complexity", label: "Complexity" },
  { key: "tests", label: "Tests" },
];

// Color palette for languages
const LANG_COLORS: Record<string, string> = {
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

function getLangColor(lang: string): string {
  return LANG_COLORS[lang] || "#6e7681";
}

function formatNumber(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "K";
  return String(n);
}

function calcCommentScore(ratio: number): number {
  // Ideal comment ratio is 10-20%. Score peaks there.
  if (ratio >= 0.10 && ratio <= 0.20) return 100;
  if (ratio >= 0.05 && ratio < 0.10) return 60 + (ratio - 0.05) / 0.05 * 40;
  if (ratio > 0.20 && ratio <= 0.30) return 80;
  if (ratio > 0.30) return 60;
  // Less than 5%
  return Math.max(10, Math.round(ratio / 0.05 * 60));
}

function calcComplexityScore(avg: number): number {
  // Lower complexity = better. avg < 5 is great, > 20 is bad.
  if (avg <= 3) return 95;
  if (avg <= 5) return 85;
  if (avg <= 8) return 75;
  if (avg <= 12) return 65;
  if (avg <= 18) return 50;
  if (avg <= 25) return 35;
  return 20;
}

function calcTestScore(ratio: number): number {
  // test file ratio: 20%+ is great
  if (ratio >= 0.25) return 100;
  if (ratio >= 0.15) return 80;
  if (ratio >= 0.10) return 60;
  if (ratio >= 0.05) return 40;
  if (ratio > 0) return 20;
  return 0;
}

function getLetterGrade(score: number): { letter: string; color: string } {
  if (score >= 90) return { letter: "A", color: "#00ff41" };
  if (score >= 80) return { letter: "B", color: "#56d364" };
  if (score >= 65) return { letter: "C", color: "#f0b400" };
  if (score >= 50) return { letter: "D", color: "#f85149" };
  return { letter: "F", color: "#f85149" };
}

function getComplexityLabel(avg: number): string {
  if (avg <= 3) return "Simple";
  if (avg <= 8) return "Moderate";
  if (avg <= 15) return "Complex";
  return "Very Complex";
}

interface DonutChartProps {
  score: number;
  size?: number;
}

function DonutChart({ score, size = 110 }: DonutChartProps) {
  const { letter, color } = getLetterGrade(score);
  const radius = (size - 16) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (score / 100) * circumference;
  const center = size / 2;

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        {/* Background ring */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke="#30363d"
          strokeWidth="8"
        />
        {/* Score ring */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth="8"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          transform={`rotate(-90 ${center} ${center})`}
          style={{ transition: "stroke-dashoffset 0.8s ease" }}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-bold" style={{ color }}>{letter}</span>
        <span className="text-[10px] text-text-muted">{score}/100</span>
      </div>
    </div>
  );
}

interface ScoreBarProps {
  label: string;
  weight: string;
  score: number;
  detail: string;
  color: string;
}

function ScoreBar({ label, weight, score, detail, color }: ScoreBarProps) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between">
        <span className="text-[11px] text-text-secondary">
          {label} <span className="text-text-muted">({weight})</span>
        </span>
        <span className="text-[11px] text-text-secondary">
          {score} <span className="text-text-muted">({detail})</span>
        </span>
      </div>
      <div className="h-2 bg-bg-primary rounded-full overflow-hidden">
        <div
          className="h-full rounded-full transition-all duration-700"
          style={{ width: `${score}%`, backgroundColor: color }}
        />
      </div>
    </div>
  );
}

interface CodeQualityModalProps {
  onClose: () => void;
}

export function CodeQualityModal({ onClose }: CodeQualityModalProps) {
  const [activeTab, setActiveTab] = useState<TabKey>("overview");
  const [report, setReport] = useState<CodeQualityReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const projectPath = useLayoutStore((s) => s.projectPath);

  useEffect(() => {
    setLoading(true);
    setError(null);
    invoke<CodeQualityReport>("analyze_code_quality", { projectPath })
      .then((r) => {
        setReport(r);
        setLoading(false);
      })
      .catch((err) => {
        setError(String(err));
        setLoading(false);
      });
  }, [projectPath]);

  // Compute scores
  const commentScore = report ? calcCommentScore(report.comment_ratio) : 0;
  const testScore = report ? calcTestScore(report.test_ratio) : 0;
  const complexityScore = report ? calcComplexityScore(report.avg_complexity) : 0;
  const orgScore = report ? report.org_score : 50;

  // Weighted total: comment 20%, test 30%, complexity 30%, org 20%
  const totalScore = Math.round(
    commentScore * 0.2 + testScore * 0.3 + complexityScore * 0.3 + orgScore * 0.2
  );

  function handleGetAIInsight() {
    if (!report) return;

    const lines: string[] = [];
    lines.push("Analyze this codebase's code quality and give specific, actionable recommendations:");
    lines.push("");
    lines.push(`## Code Quality Report`);
    lines.push(`- **Score**: ${totalScore}/100 (${getLetterGrade(totalScore).letter})`);
    lines.push(`- **Files**: ${report.total_files} across ${report.language_count} languages`);
    lines.push(`- **Lines of Code**: ${report.total_code_lines} (${report.total_lines} total)`);
    lines.push(`- **Comment Ratio**: ${(report.comment_ratio * 100).toFixed(1)}%`);
    lines.push(`- **Avg Complexity**: ${report.avg_complexity.toFixed(1)} (${getComplexityLabel(report.avg_complexity)})`);
    lines.push(`- **Test Files**: ${report.test_files} (${(report.test_ratio * 100).toFixed(1)}% of files)`);
    lines.push(`- **Organization Score**: ${orgScore}/100`);
    lines.push("");
    lines.push("### Scores");
    lines.push(`- Comment Ratio: ${commentScore}/100`);
    lines.push(`- Test Coverage: ${testScore}/100`);
    lines.push(`- Complexity: ${complexityScore}/100`);
    lines.push(`- Organization: ${orgScore}/100`);
    if (report.top_complex_files.length > 0) {
      lines.push("");
      lines.push("### Most Complex Files");
      for (const f of report.top_complex_files.slice(0, 5)) {
        lines.push(`- ${f.path} (complexity: ${f.complexity}, ${f.lines} lines)`);
      }
    }
    lines.push("");
    lines.push("### Languages");
    for (const lang of report.languages.slice(0, 5)) {
      lines.push(`- ${lang.name}: ${lang.files} files, ${lang.code_lines} code lines`);
    }
    lines.push("");
    lines.push("Please provide 5-7 specific, prioritized recommendations to improve this codebase's quality.");

    const prompt = lines.join("\n");

    useAppStore.getState().setActiveView("claude");
    useLayoutStore.getState().addPane();

    setTimeout(() => {
      window.dispatchEvent(
        new CustomEvent("packetcode:issue-prompt", { detail: { prompt } })
      );
    }, 1500);

    onClose();
  }

  const footerContent = report && !loading ? (
    <button
      onClick={handleGetAIInsight}
      className="flex items-center justify-center gap-2 w-full py-2.5 bg-accent-green/10 border border-accent-green/20 rounded-lg text-accent-green text-xs font-medium hover:bg-accent-green/20 transition-colors"
    >
      <Diamond size={12} />
      Get AI Insight
    </button>
  ) : undefined;

  return (
    <Modal
      onClose={onClose}
      title="Code Quality"
      icon={<Diamond size={14} className="text-accent-amber" />}
      footer={footerContent}
    >
      {/* Tabs */}
      <div className="flex items-center gap-1 px-5 pt-3">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-3 py-1.5 text-[11px] rounded-t transition-colors ${
              activeTab === tab.key
                ? "text-accent-green bg-bg-primary border border-bg-border border-b-0"
                : "text-text-muted hover:text-text-secondary"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="px-5 py-4 bg-bg-primary mx-0">
        {loading && (
          <div className="flex items-center justify-center py-12">
            <span className="text-xs text-text-muted animate-pulse">Analyzing codebase...</span>
          </div>
        )}
        {error && (
          <div className="text-xs text-accent-red py-4">{error}</div>
        )}
        {report && !loading && (
          <>
            {activeTab === "overview" && (
              <OverviewTab report={report} totalScore={totalScore}
                commentScore={commentScore} testScore={testScore}
                complexityScore={complexityScore} orgScore={orgScore} />
            )}
            {activeTab === "languages" && <LanguagesTab report={report} />}
            {activeTab === "complexity" && <ComplexityTab report={report} />}
            {activeTab === "tests" && <TestsTab report={report} />}
          </>
        )}
      </div>
    </Modal>
  );
}

// ─── Overview Tab ────────────────────────────────────────────

function OverviewTab({
  report, totalScore, commentScore, testScore, complexityScore, orgScore,
}: {
  report: CodeQualityReport;
  totalScore: number;
  commentScore: number;
  testScore: number;
  complexityScore: number;
  orgScore: number;
}) {
  const commentPct = (report.comment_ratio * 100).toFixed(0);
  const testPct = (report.test_ratio * 100).toFixed(0);

  // Language composition
  const totalLines = report.total_lines || 1;
  const langSlices = report.languages.map((l) => ({
    name: l.name,
    pct: (l.total_lines / totalLines) * 100,
    color: getLangColor(l.name),
  }));

  return (
    <div className="flex flex-col gap-5">
      {/* Top stats row */}
      <div className="flex items-center gap-5">
        {/* Donut chart */}
        <DonutChart score={totalScore} />

        {/* Stats */}
        <div className="flex flex-col gap-3 flex-1">
          <div className="flex items-center gap-2">
            <Code2 size={12} className="text-accent-blue" />
            <div>
              <div className="text-sm font-semibold text-text-primary">
                {formatNumber(report.total_code_lines)}
              </div>
              <div className="text-[10px] text-text-muted">
                Lines of Code
                <span className="ml-1 text-text-muted">{formatNumber(report.total_lines)} total</span>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <FileText size={12} className="text-text-secondary" />
            <div>
              <div className="text-sm font-semibold text-text-primary">{report.total_files}</div>
              <div className="text-[10px] text-text-muted">Files &middot; {report.language_count} languages</div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Gauge size={12} className="text-accent-amber" />
            <div>
              <div className="text-sm font-semibold text-text-primary">
                {report.avg_complexity.toFixed(1)}
              </div>
              <div className="text-[10px] text-text-muted">
                Avg Complexity &middot; {getComplexityLabel(report.avg_complexity)}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Score Breakdown */}
      <div>
        <h3 className="text-xs font-semibold text-text-primary mb-3">Score Breakdown</h3>
        <div className="flex flex-col gap-3">
          <ScoreBar
            label="Comment Ratio"
            weight="20%"
            score={commentScore}
            detail={`${commentPct}%`}
            color="#58a6ff"
          />
          <ScoreBar
            label="Test Coverage"
            weight="30%"
            score={testScore}
            detail={`${testPct}% ratio`}
            color="#00ff41"
          />
          <ScoreBar
            label="Complexity"
            weight="30%"
            score={complexityScore}
            detail={`avg ${report.avg_complexity.toFixed(1)}`}
            color="#f0b400"
          />
          <ScoreBar
            label="Organization"
            weight="20%"
            score={orgScore}
            detail={`${orgScore}`}
            color="#56d364"
          />
        </div>
      </div>

      {/* Language Composition */}
      <div>
        <h3 className="text-xs font-semibold text-text-primary mb-2">Language Composition</h3>
        {/* Stacked bar */}
        <div className="flex h-3 rounded-full overflow-hidden bg-bg-secondary">
          {langSlices.map((s) => (
            <div
              key={s.name}
              style={{ width: `${s.pct}%`, backgroundColor: s.color }}
              title={`${s.name}: ${s.pct.toFixed(1)}%`}
            />
          ))}
        </div>
        {/* Legend */}
        <div className="flex flex-wrap gap-3 mt-2">
          {langSlices.filter((s) => s.pct >= 1).map((s) => (
            <div key={s.name} className="flex items-center gap-1.5">
              <div
                className="w-2 h-2 rounded-full"
                style={{ backgroundColor: s.color }}
              />
              <span className="text-[10px] text-text-muted">
                {s.name} {s.pct.toFixed(1)}%
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ─── Languages Tab ───────────────────────────────────────────

function LanguagesTab({ report }: { report: CodeQualityReport }) {
  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-xs font-semibold text-text-primary">
        {report.language_count} Languages Detected
      </h3>
      <div className="flex flex-col gap-1">
        {/* Header */}
        <div className="flex items-center text-[10px] text-text-muted uppercase tracking-wider px-2 py-1">
          <span className="flex-1">Language</span>
          <span className="w-14 text-right">Files</span>
          <span className="w-16 text-right">Code</span>
          <span className="w-16 text-right">Comments</span>
          <span className="w-16 text-right">Total</span>
        </div>
        {report.languages.map((lang) => {
          const pct = report.total_lines > 0
            ? ((lang.total_lines / report.total_lines) * 100).toFixed(1)
            : "0";
          return (
            <div
              key={lang.name}
              className="flex items-center text-[11px] px-2 py-1.5 rounded hover:bg-bg-secondary transition-colors"
            >
              <div className="flex items-center gap-2 flex-1 min-w-0">
                <div
                  className="w-2.5 h-2.5 rounded-full flex-shrink-0"
                  style={{ backgroundColor: getLangColor(lang.name) }}
                />
                <span className="text-text-primary truncate">{lang.name}</span>
                <span className="text-text-muted text-[10px]">{pct}%</span>
              </div>
              <span className="w-14 text-right text-text-secondary">{lang.files}</span>
              <span className="w-16 text-right text-text-secondary">{formatNumber(lang.code_lines)}</span>
              <span className="w-16 text-right text-text-muted">{formatNumber(lang.comment_lines)}</span>
              <span className="w-16 text-right text-text-muted">{formatNumber(lang.total_lines)}</span>
            </div>
          );
        })}
      </div>
      {/* Totals */}
      <div className="flex items-center text-[11px] px-2 py-1.5 border-t border-bg-border font-medium">
        <span className="flex-1 text-text-primary">Total</span>
        <span className="w-14 text-right text-text-primary">{report.total_files}</span>
        <span className="w-16 text-right text-text-primary">{formatNumber(report.total_code_lines)}</span>
        <span className="w-16 text-right text-text-secondary">{formatNumber(report.total_comment_lines)}</span>
        <span className="w-16 text-right text-text-secondary">{formatNumber(report.total_lines)}</span>
      </div>
    </div>
  );
}

// ─── Complexity Tab ──────────────────────────────────────────

function ComplexityTab({ report }: { report: CodeQualityReport }) {
  const maxComplexity = report.top_complex_files.length > 0
    ? report.top_complex_files[0].complexity
    : 1;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h3 className="text-xs font-semibold text-text-primary">
          Average Complexity: {report.avg_complexity.toFixed(1)}
        </h3>
        <p className="text-[10px] text-text-muted mt-0.5">
          {getComplexityLabel(report.avg_complexity)} — based on branches, loops, and conditional logic per file
        </p>
      </div>

      {report.top_complex_files.length > 0 && (
        <div>
          <h4 className="text-[11px] text-text-secondary font-medium mb-2">
            Most Complex Files
          </h4>
          <div className="flex flex-col gap-1.5">
            {report.top_complex_files.map((f) => (
              <div key={f.path} className="flex flex-col gap-0.5">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] text-text-secondary truncate flex-1 mr-2 font-mono">
                    {f.path}
                  </span>
                  <span className="text-[10px] text-text-muted flex-shrink-0">
                    {f.complexity} &middot; {f.lines} lines
                  </span>
                </div>
                <div className="h-1.5 bg-bg-secondary rounded-full overflow-hidden">
                  <div
                    className="h-full rounded-full"
                    style={{
                      width: `${(f.complexity / maxComplexity) * 100}%`,
                      backgroundColor:
                        f.complexity > 30
                          ? "#f85149"
                          : f.complexity > 15
                            ? "#f0b400"
                            : "#58a6ff",
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {report.top_complex_files.length === 0 && (
        <p className="text-[10px] text-text-muted py-4 text-center">
          No complexity data available
        </p>
      )}
    </div>
  );
}

// ─── Tests Tab ───────────────────────────────────────────────

function TestsTab({ report }: { report: CodeQualityReport }) {
  const testPct = report.total_files > 0
    ? ((report.test_files / report.total_files) * 100).toFixed(1)
    : "0";
  const testLinePct = report.total_lines > 0
    ? ((report.test_lines / report.total_lines) * 100).toFixed(1)
    : "0";

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-6">
        <div>
          <div className="text-lg font-semibold text-text-primary">{report.test_files}</div>
          <div className="text-[10px] text-text-muted">Test files ({testPct}% of files)</div>
        </div>
        <div>
          <div className="text-lg font-semibold text-text-primary">{formatNumber(report.test_lines)}</div>
          <div className="text-[10px] text-text-muted">Test lines ({testLinePct}% of lines)</div>
        </div>
      </div>

      <div>
        <div className="flex items-center justify-between mb-1">
          <span className="text-[11px] text-text-secondary">File coverage ratio</span>
          <span className="text-[11px] text-text-secondary">{testPct}%</span>
        </div>
        <div className="h-2.5 bg-bg-secondary rounded-full overflow-hidden">
          <div
            className="h-full rounded-full bg-accent-green transition-all duration-700"
            style={{ width: `${Math.min(parseFloat(testPct), 100)}%` }}
          />
        </div>
      </div>

      <div>
        <h4 className="text-[11px] text-text-secondary font-medium mb-1">Detection</h4>
        <p className="text-[10px] text-text-muted leading-relaxed">
          Files are detected as tests when the path includes{" "}
          <code className="text-accent-blue">/tests/</code> or{" "}
          <code className="text-accent-blue">/__tests__/</code>, or when filenames match{" "}
          <code className="text-accent-blue">*.test.*</code> or{" "}
          <code className="text-accent-blue">*.spec.*</code>. For more accurate coverage
          data, integrate a coverage tool and import results.
        </p>
      </div>

      {report.test_files === 0 && (
        <div className="py-4 text-center">
          <TestTube2 size={24} className="text-text-muted mx-auto mb-2" />
          <p className="text-xs text-text-muted">No test files detected</p>
          <p className="text-[10px] text-text-muted mt-1">
            Consider adding tests to improve code quality
          </p>
        </div>
      )}
    </div>
  );
}

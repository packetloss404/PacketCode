import { useState, useEffect } from "react";
import { Diamond } from "lucide-react";
import { invoke } from "@tauri-apps/api/core";
import { Modal } from "@/components/ui/Modal";
import { useLayoutStore } from "@/stores/layoutStore";
import { useAppStore } from "@/stores/appStore";
import type { CodeQualityReport } from "./codeQualityUtils";
import { calcCommentScore, calcTestScore, calcComplexityScore, getLetterGrade, getComplexityLabel } from "./codeQualityUtils";
import { OverviewTab } from "./OverviewTab";
import { LanguagesTab } from "./LanguagesTab";
import { ComplexityTab } from "./ComplexityTab";
import { TestsTab } from "./TestsTab";

type TabKey = "overview" | "languages" | "complexity" | "tests";

const TABS: { key: TabKey; label: string }[] = [
  { key: "overview", label: "Overview" },
  { key: "languages", label: "Languages" },
  { key: "complexity", label: "Complexity" },
  { key: "tests", label: "Tests" },
];

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
      .then((r) => { setReport(r); setLoading(false); })
      .catch((err) => { setError(String(err)); setLoading(false); });
  }, [projectPath]);

  const commentScore = report ? calcCommentScore(report.comment_ratio) : 0;
  const testScore = report ? calcTestScore(report.test_ratio) : 0;
  const complexityScore = report ? calcComplexityScore(report.avg_complexity) : 0;
  const orgScore = report ? report.org_score : 50;
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
      window.dispatchEvent(new CustomEvent("packetcode:issue-prompt", { detail: { prompt } }));
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

      <div className="px-5 py-4 bg-bg-primary mx-0">
        {loading && (
          <div className="flex items-center justify-center py-12">
            <span className="text-xs text-text-muted animate-pulse">Analyzing codebase...</span>
          </div>
        )}
        {error && <div className="text-xs text-accent-red py-4">{error}</div>}
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

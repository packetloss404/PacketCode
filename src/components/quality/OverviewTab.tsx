import { Code2, FileText, Gauge } from "lucide-react";
import type { CodeQualityReport } from "./codeQualityUtils";
import { formatNumber, getLangColor, getComplexityLabel } from "./codeQualityUtils";
import { DonutChart } from "./DonutChart";
import { ScoreBar } from "./ScoreBar";

interface OverviewTabProps {
  report: CodeQualityReport;
  totalScore: number;
  commentScore: number;
  testScore: number;
  complexityScore: number;
  orgScore: number;
}

export function OverviewTab({ report, totalScore, commentScore, testScore, complexityScore, orgScore }: OverviewTabProps) {
  const commentPct = (report.comment_ratio * 100).toFixed(0);
  const testPct = (report.test_ratio * 100).toFixed(0);

  const totalLines = report.total_lines || 1;
  const langSlices = report.languages.map((l) => ({
    name: l.name,
    pct: (l.total_lines / totalLines) * 100,
    color: getLangColor(l.name),
  }));

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-5">
        <DonutChart score={totalScore} />
        <div className="flex flex-col gap-3 flex-1">
          <div className="flex items-center gap-2">
            <Code2 size={12} className="text-accent-blue" />
            <div>
              <div className="text-sm font-semibold text-text-primary">{formatNumber(report.total_code_lines)}</div>
              <div className="text-[10px] text-text-muted">
                Lines of Code<span className="ml-1 text-text-muted">{formatNumber(report.total_lines)} total</span>
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
              <div className="text-sm font-semibold text-text-primary">{report.avg_complexity.toFixed(1)}</div>
              <div className="text-[10px] text-text-muted">Avg Complexity &middot; {getComplexityLabel(report.avg_complexity)}</div>
            </div>
          </div>
        </div>
      </div>

      <div>
        <h3 className="text-xs font-semibold text-text-primary mb-3">Score Breakdown</h3>
        <div className="flex flex-col gap-3">
          <ScoreBar label="Comment Ratio" weight="20%" score={commentScore} detail={`${commentPct}%`} color="#58a6ff" />
          <ScoreBar label="Test Coverage" weight="30%" score={testScore} detail={`${testPct}% ratio`} color="#00ff41" />
          <ScoreBar label="Complexity" weight="30%" score={complexityScore} detail={`avg ${report.avg_complexity.toFixed(1)}`} color="#f0b400" />
          <ScoreBar label="Organization" weight="20%" score={orgScore} detail={`${orgScore}`} color="#56d364" />
        </div>
      </div>

      <div>
        <h3 className="text-xs font-semibold text-text-primary mb-2">Language Composition</h3>
        <div className="flex h-3 rounded-full overflow-hidden bg-bg-secondary">
          {langSlices.map((s) => (
            <div key={s.name} style={{ width: `${s.pct}%`, backgroundColor: s.color }} title={`${s.name}: ${s.pct.toFixed(1)}%`} />
          ))}
        </div>
        <div className="flex flex-wrap gap-3 mt-2">
          {langSlices.filter((s) => s.pct >= 1).map((s) => (
            <div key={s.name} className="flex items-center gap-1.5">
              <div className="w-2 h-2 rounded-full" style={{ backgroundColor: s.color }} />
              <span className="text-[10px] text-text-muted">{s.name} {s.pct.toFixed(1)}%</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

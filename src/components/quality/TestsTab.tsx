import { TestTube2 } from "lucide-react";
import type { CodeQualityReport } from "./codeQualityUtils";
import { formatNumber } from "./codeQualityUtils";

export function TestsTab({ report }: { report: CodeQualityReport }) {
  const testPct = report.total_files > 0 ? ((report.test_files / report.total_files) * 100).toFixed(1) : "0";
  const testLinePct = report.total_lines > 0 ? ((report.test_lines / report.total_lines) * 100).toFixed(1) : "0";

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
          <p className="text-[10px] text-text-muted mt-1">Consider adding tests to improve code quality</p>
        </div>
      )}
    </div>
  );
}

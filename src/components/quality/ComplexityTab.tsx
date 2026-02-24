import type { CodeQualityReport } from "./codeQualityUtils";
import { getComplexityLabel } from "./codeQualityUtils";

export function ComplexityTab({ report }: { report: CodeQualityReport }) {
  const maxComplexity = report.top_complex_files.length > 0 ? report.top_complex_files[0].complexity : 1;

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
          <h4 className="text-[11px] text-text-secondary font-medium mb-2">Most Complex Files</h4>
          <div className="flex flex-col gap-1.5">
            {report.top_complex_files.map((f) => (
              <div key={f.path} className="flex flex-col gap-0.5">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] text-text-secondary truncate flex-1 mr-2 font-mono">{f.path}</span>
                  <span className="text-[10px] text-text-muted flex-shrink-0">{f.complexity} &middot; {f.lines} lines</span>
                </div>
                <div className="h-1.5 bg-bg-secondary rounded-full overflow-hidden">
                  <div
                    className="h-full rounded-full"
                    style={{
                      width: `${(f.complexity / maxComplexity) * 100}%`,
                      backgroundColor: f.complexity > 30 ? "#f85149" : f.complexity > 15 ? "#f0b400" : "#58a6ff",
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {report.top_complex_files.length === 0 && (
        <p className="text-[10px] text-text-muted py-4 text-center">No complexity data available</p>
      )}
    </div>
  );
}

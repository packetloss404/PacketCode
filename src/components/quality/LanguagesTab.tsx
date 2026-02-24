import type { CodeQualityReport } from "./codeQualityUtils";
import { formatNumber, getLangColor } from "./codeQualityUtils";

export function LanguagesTab({ report }: { report: CodeQualityReport }) {
  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-xs font-semibold text-text-primary">
        {report.language_count} Languages Detected
      </h3>
      <div className="flex flex-col gap-1">
        <div className="flex items-center text-[10px] text-text-muted uppercase tracking-wider px-2 py-1">
          <span className="flex-1">Language</span>
          <span className="w-14 text-right">Files</span>
          <span className="w-16 text-right">Code</span>
          <span className="w-16 text-right">Comments</span>
          <span className="w-16 text-right">Total</span>
        </div>
        {report.languages.map((lang) => {
          const pct = report.total_lines > 0 ? ((lang.total_lines / report.total_lines) * 100).toFixed(1) : "0";
          return (
            <div key={lang.name} className="flex items-center text-[11px] px-2 py-1.5 rounded hover:bg-bg-secondary transition-colors">
              <div className="flex items-center gap-2 flex-1 min-w-0">
                <div className="w-2.5 h-2.5 rounded-full flex-shrink-0" style={{ backgroundColor: getLangColor(lang.name) }} />
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

import { useEffect } from "react";
import { BarChart3, DollarSign, Hash, Cpu, ArrowUpRight, ArrowDownLeft, RefreshCw } from "lucide-react";
import { useAnalyticsStore, type ModelUsage, type DailyCost } from "@/stores/analyticsStore";

export function AnalyticsView() {
  const data = useAnalyticsStore((s) => s.data);
  const loading = useAnalyticsStore((s) => s.loading);
  const error = useAnalyticsStore((s) => s.error);
  const load = useAnalyticsStore((s) => s.load);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="flex flex-col h-full bg-bg-primary p-4 overflow-y-auto">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <BarChart3 size={16} className="text-accent-blue" />
          <h2 className="text-sm font-semibold text-text-primary">
            Usage Analytics
          </h2>
        </div>
        <button
          onClick={load}
          disabled={loading}
          className="p-1.5 text-text-muted hover:text-text-primary transition-colors disabled:opacity-50"
          title="Refresh"
        >
          <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
        </button>
      </div>

      {error && (
        <div className="text-[10px] text-accent-amber bg-accent-amber/10 rounded px-2 py-1 mb-3">
          {error}
        </div>
      )}

      {loading && !data && (
        <p className="text-xs text-text-muted">Loading analytics...</p>
      )}

      {data && (
        <div className="space-y-4 max-w-3xl">
          {/* Summary cards */}
          <div className="grid grid-cols-4 gap-3">
            <SummaryCard
              icon={<DollarSign size={12} />}
              label="Total Cost"
              value={`$${data.totalCostUsd.toFixed(2)}`}
              color="text-accent-green"
            />
            <SummaryCard
              icon={<Hash size={12} />}
              label="Sessions"
              value={String(data.totalSessions)}
              color="text-accent-blue"
            />
            <SummaryCard
              icon={<ArrowUpRight size={12} />}
              label="Input Tokens"
              value={formatTokens(data.totalInputTokens)}
              color="text-accent-purple"
            />
            <SummaryCard
              icon={<ArrowDownLeft size={12} />}
              label="Output Tokens"
              value={formatTokens(data.totalOutputTokens)}
              color="text-accent-amber"
            />
          </div>

          {/* Daily cost chart */}
          {data.dailyCosts.length > 0 && (
            <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
              <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
                <BarChart3 size={12} className="text-accent-green" />
                Daily Cost (Last 30 Days)
              </h3>
              <DailyCostChart data={data.dailyCosts} />
            </div>
          )}

          {/* Model usage table */}
          {data.modelUsage.length > 0 && (
            <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
              <h3 className="text-xs font-semibold text-text-primary mb-3 flex items-center gap-2">
                <Cpu size={12} className="text-accent-blue" />
                Model Breakdown
              </h3>
              <div className="overflow-x-auto">
                <table className="w-full text-[11px]">
                  <thead>
                    <tr className="text-text-muted border-b border-bg-border">
                      <th className="text-left py-1.5 pr-4 font-medium">Model</th>
                      <th className="text-right py-1.5 px-3 font-medium">Sessions</th>
                      <th className="text-right py-1.5 px-3 font-medium">Input</th>
                      <th className="text-right py-1.5 px-3 font-medium">Output</th>
                      <th className="text-right py-1.5 pl-3 font-medium">Cost</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.modelUsage.map((m) => (
                      <ModelRow key={m.model} model={m} />
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {data.modelUsage.length === 0 && data.dailyCosts.length === 0 && (
            <p className="text-xs text-text-muted">
              No usage data found. Analytics are sourced from ~/.claude/cost-tally.json.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function SummaryCard({
  icon,
  label,
  value,
  color,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  color: string;
}) {
  return (
    <div className="bg-bg-secondary border border-bg-border rounded-lg p-3">
      <div className={`flex items-center gap-1.5 mb-1 ${color}`}>
        {icon}
        <span className="text-[10px] text-text-muted">{label}</span>
      </div>
      <p className="text-sm font-semibold text-text-primary">{value}</p>
    </div>
  );
}

function ModelRow({ model }: { model: ModelUsage }) {
  return (
    <tr className="border-b border-bg-border/50 text-text-secondary hover:bg-bg-hover/30">
      <td className="py-1.5 pr-4 text-text-primary font-medium truncate max-w-[200px]">
        {model.model}
      </td>
      <td className="text-right py-1.5 px-3">{model.sessions}</td>
      <td className="text-right py-1.5 px-3">{formatTokens(model.inputTokens)}</td>
      <td className="text-right py-1.5 px-3">{formatTokens(model.outputTokens)}</td>
      <td className="text-right py-1.5 pl-3 text-accent-green">
        ${model.costUsd.toFixed(2)}
      </td>
    </tr>
  );
}

function DailyCostChart({ data }: { data: DailyCost[] }) {
  const maxCost = Math.max(...data.map((d) => d.costUsd), 0.01);
  const chartHeight = 120;
  const barWidth = Math.max(4, Math.min(16, (600 - data.length * 2) / data.length));

  return (
    <div className="flex items-end gap-[2px] h-[120px] overflow-x-auto">
      {data.map((d, i) => {
        const height = Math.max(2, (d.costUsd / maxCost) * chartHeight);
        return (
          <div
            key={i}
            className="flex flex-col items-center flex-shrink-0 group"
          >
            <div className="relative">
              {/* Tooltip */}
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 hidden group-hover:block z-10">
                <div className="bg-bg-elevated text-text-primary text-[9px] px-1.5 py-0.5 rounded shadow-lg whitespace-nowrap border border-bg-border">
                  {d.date}: ${d.costUsd.toFixed(2)}
                </div>
              </div>
              <div
                className="bg-accent-green/60 hover:bg-accent-green/80 rounded-t-sm transition-colors"
                style={{ width: barWidth, height }}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

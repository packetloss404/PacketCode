import { useMemo } from "react";
import { DollarSign, Trash2, BarChart3 } from "lucide-react";
import { useCostStore } from "@/stores/costStore";

export function CostDashboardView() {
  const entries = useCostStore((s) => s.entries);
  const clearEntries = useCostStore((s) => s.clearEntries);
  const getSummary = useCostStore((s) => s.getSummary);

  const summary = useMemo(() => getSummary(), [entries]);

  // Last 7 days for the chart
  const days = useMemo(() => {
    const result: { day: string; cost: number }[] = [];
    for (let i = 6; i >= 0; i--) {
      const d = new Date();
      d.setDate(d.getDate() - i);
      const key = d.toISOString().slice(0, 10);
      const label = d.toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" });
      result.push({ day: label, cost: summary.costByDay[key] || 0 });
    }
    return result;
  }, [summary.costByDay]);

  const maxDayCost = Math.max(...days.map((d) => d.cost), 0.01);

  return (
    <div className="flex flex-col h-full bg-bg-primary overflow-y-auto p-4">
      <div className="flex items-center gap-2 mb-6">
        <DollarSign size={16} className="text-accent-amber" />
        <h2 className="text-sm font-semibold text-text-primary">Cost Dashboard</h2>
        <div className="flex-1" />
        <button
          onClick={clearEntries}
          className="flex items-center gap-1 px-2 py-1 text-[11px] text-text-muted hover:text-accent-red transition-colors"
        >
          <Trash2 size={11} />
          Clear
        </button>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-3 mb-6 max-w-xl">
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <p className="text-[10px] text-text-muted uppercase tracking-wider mb-1">Total Cost</p>
          <p className="text-lg font-semibold text-accent-amber">
            ${summary.totalCost.toFixed(2)}
          </p>
        </div>
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <p className="text-[10px] text-text-muted uppercase tracking-wider mb-1">Sessions</p>
          <p className="text-lg font-semibold text-text-primary">
            {summary.sessionCount}
          </p>
        </div>
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4">
          <p className="text-[10px] text-text-muted uppercase tracking-wider mb-1">Avg / Session</p>
          <p className="text-lg font-semibold text-text-primary">
            ${summary.sessionCount > 0 ? (summary.totalCost / summary.sessionCount).toFixed(2) : "0.00"}
          </p>
        </div>
      </div>

      {/* Bar chart — last 7 days */}
      <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 mb-6 max-w-xl">
        <h3 className="text-xs font-semibold text-text-primary mb-4 flex items-center gap-2">
          <BarChart3 size={12} className="text-accent-green" />
          Last 7 Days
        </h3>
        <div className="flex items-end gap-2 h-32">
          {days.map((d, i) => (
            <div key={i} className="flex-1 flex flex-col items-center gap-1">
              <span className="text-[9px] text-text-muted">
                {d.cost > 0 ? `$${d.cost.toFixed(2)}` : ""}
              </span>
              <div
                className="w-full bg-accent-green/30 rounded-t transition-all"
                style={{
                  height: `${Math.max(2, (d.cost / maxDayCost) * 100)}%`,
                  minHeight: d.cost > 0 ? 4 : 2,
                }}
              />
              <span className="text-[8px] text-text-muted truncate max-w-full">
                {d.day.split(",")[0]}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Cost by model */}
      {Object.keys(summary.costByModel).length > 0 && (
        <div className="bg-bg-secondary border border-bg-border rounded-lg p-4 max-w-xl">
          <h3 className="text-xs font-semibold text-text-primary mb-3">Cost by Model</h3>
          <div className="flex flex-col gap-2">
            {Object.entries(summary.costByModel)
              .sort(([, a], [, b]) => b - a)
              .map(([model, cost]) => (
                <div key={model} className="flex items-center gap-2">
                  <span className="text-[11px] text-text-secondary flex-1 truncate">
                    {model}
                  </span>
                  <span className="text-[11px] text-accent-amber font-medium">
                    ${cost.toFixed(2)}
                  </span>
                </div>
              ))}
          </div>
        </div>
      )}

      {entries.length === 0 && (
        <div className="text-center text-text-muted text-sm mt-8">
          No cost data yet. Start a session to begin tracking.
        </div>
      )}
    </div>
  );
}

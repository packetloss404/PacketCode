interface ScoreBarProps {
  label: string;
  weight: string;
  score: number;
  detail: string;
  color: string;
}

export function ScoreBar({ label, weight, score, detail, color }: ScoreBarProps) {
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

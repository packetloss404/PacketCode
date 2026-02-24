import { getLetterGrade } from "./codeQualityUtils";

interface DonutChartProps {
  score: number;
  size?: number;
}

export function DonutChart({ score, size = 110 }: DonutChartProps) {
  const { letter, color } = getLetterGrade(score);
  const radius = (size - 16) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (score / 100) * circumference;
  const center = size / 2;

  return (
    <div className="relative" style={{ width: size, height: size }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        <circle cx={center} cy={center} r={radius} fill="none" stroke="#30363d" strokeWidth="8" />
        <circle
          cx={center} cy={center} r={radius} fill="none" stroke={color} strokeWidth="8"
          strokeDasharray={circumference} strokeDashoffset={offset} strokeLinecap="round"
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

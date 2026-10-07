// src/components/dashboard/charts/Sparkline.tsx
import React from "react";
import { ANALYTICS_TEAL } from "./analyticsTheme";

interface SparklineProps {
  data: number[];
  width?: number;
  height?: number;
  color?: string;
  showEndDot?: boolean;
  ariaLabel: string;
}

// Dependency-free, word-sized line. Tufte's sparkline: no axes, no chartjunk.
export function Sparkline({
  data,
  width = 80,
  height = 24,
  color = ANALYTICS_TEAL,
  showEndDot = true,
  ariaLabel,
}: SparklineProps) {
  if (!data || data.length < 2) {
    return <span role="img" aria-label={ariaLabel} className="inline-block" style={{ width, height }} />;
  }
  const min = Math.min(...data);
  const max = Math.max(...data);
  const span = max - min || 1;
  const pad = 2;
  const stepX = (width - pad * 2) / (data.length - 1);
  const toY = (v: number) => height - pad - ((v - min) / span) * (height - pad * 2);
  const points = data.map((v, i) => `${pad + i * stepX},${toY(v)}`).join(" ");
  const lastX = pad + (data.length - 1) * stepX;
  const lastY = toY(data[data.length - 1]);

  return (
    <svg role="img" aria-label={ariaLabel} width={width} height={height} className="overflow-visible">
      <polyline points={points} fill="none" stroke={color} strokeWidth={1.25}
        strokeLinejoin="round" strokeLinecap="round" />
      {showEndDot && <circle cx={lastX} cy={lastY} r={1.75} fill={color} />}
    </svg>
  );
}

import React from "react";

interface SkeletonChartProps {
  height?: string;
  className?: string;
}

export function SkeletonChart({
  height = "16rem",
  className = "",
}: SkeletonChartProps) {
  return (
    <div
      className={`border border-ink-100 rounded-2xl bg-ink-50 ${className}`}
      style={{ height }}
    />
  );
}

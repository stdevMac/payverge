import React from "react";

interface SkeletonLineProps {
  width?: string;
  height?: string;
  className?: string;
  "data-skeleton-line"?: boolean;
}

export function SkeletonLine({
  width = "100%",
  height = "1rem",
  className = "",
  "data-skeleton-line": dataSkeletonLine,
}: SkeletonLineProps) {
  return (
    <div
      className={`bg-ink-100 rounded ${className}`}
      style={{ width, height }}
      data-skeleton-line={dataSkeletonLine || undefined}
    />
  );
}

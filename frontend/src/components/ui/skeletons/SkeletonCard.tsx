import React from "react";
import { SkeletonLine } from "./SkeletonLine";

interface SkeletonCardProps {
  lines?: number;
  hasHeader?: boolean;
  className?: string;
}

export function SkeletonCard({
  lines = 3,
  hasHeader = false,
  className = "",
}: SkeletonCardProps) {
  return (
    <div
      className={`border border-ink-100 rounded-2xl p-4 space-y-3 ${className}`}
    >
      {hasHeader && <SkeletonLine width="60%" height="1.25rem" />}
      {Array.from({ length: lines }, (_, i) => (
        <SkeletonLine
          key={i}
          width={i === lines - 1 ? "75%" : "100%"}
          height="0.875rem"
        />
      ))}
    </div>
  );
}

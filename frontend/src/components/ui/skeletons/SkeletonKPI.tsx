import React from "react";
import { SkeletonLine } from "./SkeletonLine";

interface SkeletonKPIProps {
  className?: string;
}

export function SkeletonKPI({ className = "" }: SkeletonKPIProps) {
  return (
    <div
      className={`border border-ink-100 rounded-2xl p-4 space-y-2 ${className}`}
    >
      <SkeletonLine width="60%" height="0.75rem" />
      <SkeletonLine width="40%" height="1.75rem" />
      <SkeletonLine width="80%" height="0.625rem" />
    </div>
  );
}

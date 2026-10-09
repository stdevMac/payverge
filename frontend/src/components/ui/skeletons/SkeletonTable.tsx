import React from "react";
import { SkeletonLine } from "./SkeletonLine";

interface SkeletonTableProps {
  rows?: number;
  columns?: number;
  className?: string;
}

export function SkeletonTable({
  rows = 5,
  columns = 4,
  className = "",
}: SkeletonTableProps) {
  return (
    <div className={`border border-ink-100 rounded-2xl overflow-hidden ${className}`}>
      {/* Header row */}
      <div className="flex gap-4 px-4 py-3 border-b border-ink-100 bg-warm-50">
        {Array.from({ length: columns }, (_, i) => (
          <SkeletonLine
            key={i}
            width={i === 0 ? "30%" : "18%"}
            height="0.75rem"
            className="flex-shrink-0"
          />
        ))}
      </div>
      {/* Body rows */}
      {Array.from({ length: rows }, (_, row) => (
        <div
          key={row}
          className="flex gap-4 px-4 py-3 border-b border-ink-50 last:border-b-0"
        >
          {Array.from({ length: columns }, (_, col) => (
            <SkeletonLine
              key={col}
              width={col === 0 ? "30%" : "18%"}
              height="0.875rem"
              className="flex-shrink-0"
            />
          ))}
        </div>
      ))}
    </div>
  );
}

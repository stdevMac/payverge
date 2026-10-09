"use client";

import React from "react";
import type { ScanStatus } from "@/api/spaces";
import { scanStatusTone } from "./scanStatus";

interface SpaceScanStatusBadgeProps {
  status: ScanStatus | string;
  label: string;
  progressPct?: number;
  className?: string;
  "data-testid"?: string;
}

function SpaceScanStatusBadge({
  status,
  label,
  progressPct,
  className = "",
  "data-testid": testId = "space-scan-status",
}: SpaceScanStatusBadgeProps) {
  return (
    <div
      className={`inline-flex flex-wrap items-center gap-2 ${className}`}
      data-testid={testId}
      data-status={status}
    >
      <span
        className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${scanStatusTone(status)}`}
        data-testid="space-scan-status-badge"
      >
        {label}
      </span>
      {typeof progressPct === "number" && progressPct > 0 ? (
        <span
          className="text-xs tabular-nums text-ink-500"
          data-testid="space-scan-progress"
        >
          {progressPct}%
        </span>
      ) : null}
    </div>
  );
}

export default SpaceScanStatusBadge;

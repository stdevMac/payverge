"use client";

import React, { useMemo } from "react";
import { parseLayoutDocument } from "@/api/spaces";

interface LayoutPreviewSvgProps {
  /** Raw draft or published layout JSON from the space row. */
  layoutRaw?: unknown;
  publishedRaw?: unknown;
  widthMm?: number | null;
  heightMm?: number | null;
  className?: string;
  "aria-label"?: string;
}

/**
 * Tiny SVG floor-plan thumbnail for space cards.
 * Prefers published layout, falls back to draft.
 */
export function LayoutPreviewSvg({
  layoutRaw,
  publishedRaw,
  widthMm,
  heightMm,
  className = "",
  "aria-label": ariaLabel,
}: LayoutPreviewSvgProps) {
  const layout = useMemo(() => {
    const published = parseLayoutDocument(publishedRaw);
    if (published && ((published.tables?.length ?? 0) > 0 || published.boundary)) {
      return published;
    }
    return parseLayoutDocument(layoutRaw);
  }, [layoutRaw, publishedRaw]);

  const { viewW, viewH, tables, boundaryPath } = useMemo(() => {
    const w =
      layout?.width_mm ||
      widthMm ||
      10000;
    const h =
      layout?.height_mm ||
      heightMm ||
      8000;
    const safeW = Math.max(w, 1);
    const safeH = Math.max(h, 1);
    const tables = layout?.tables ?? [];
    let boundaryPath = "";
    const pts = layout?.boundary?.points_mm;
    if (pts && pts.length >= 3) {
      boundaryPath =
        pts
          .map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`)
          .join(" ") + " Z";
    }
    return { viewW: safeW, viewH: safeH, tables, boundaryPath };
  }, [layout, widthMm, heightMm]);

  const hasContent =
    tables.length > 0 || Boolean(boundaryPath) || Boolean(layout?.width_mm);

  if (!hasContent) {
    return (
      <div
        className={`flex h-full w-full items-center justify-center bg-warm-50 ${className}`}
        aria-hidden={ariaLabel ? undefined : true}
        aria-label={ariaLabel}
      >
        <svg
          viewBox="0 0 80 56"
          className="h-10 w-14 text-ink-500"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        >
          <rect x="8" y="8" width="64" height="40" rx="3" />
          <rect x="18" y="18" width="14" height="14" rx="2" />
          <rect x="40" y="18" width="18" height="12" rx="2" />
          <circle cx="28" cy="38" r="4" />
        </svg>
      </div>
    );
  }

  return (
    <svg
      viewBox={`0 0 ${viewW} ${viewH}`}
      className={`h-full w-full ${className}`}
      preserveAspectRatio="xMidYMid meet"
      aria-label={ariaLabel}
      role="img"
    >
      <rect
        x={0}
        y={0}
        width={viewW}
        height={viewH}
        className="fill-warm-50"
      />
      {boundaryPath ? (
        <path
          d={boundaryPath}
          className="fill-brand/5 stroke-brand/40"
          strokeWidth={Math.max(viewW, viewH) * 0.004}
        />
      ) : (
        <rect
          x={viewW * 0.04}
          y={viewH * 0.04}
          width={viewW * 0.92}
          height={viewH * 0.92}
          rx={Math.min(viewW, viewH) * 0.02}
          className="fill-none stroke-warm-200"
          strokeWidth={Math.max(viewW, viewH) * 0.003}
        />
      )}
      {tables.map((t) => {
        const isRound = t.shape === "round" || t.shape === "oval";
        const cx = t.x_mm + t.width_mm / 2;
        const cy = t.y_mm + t.height_mm / 2;
        if (isRound) {
          return (
            <ellipse
              key={t.table_id}
              cx={cx}
              cy={cy}
              rx={t.width_mm / 2}
              ry={t.height_mm / 2}
              transform={
                t.rotation_deg
                  ? `rotate(${t.rotation_deg} ${cx} ${cy})`
                  : undefined
              }
              className="fill-brand/25 stroke-brand/60"
              strokeWidth={Math.max(viewW, viewH) * 0.002}
            />
          );
        }
        return (
          <rect
            key={t.table_id}
            x={t.x_mm}
            y={t.y_mm}
            width={t.width_mm}
            height={t.height_mm}
            rx={Math.min(t.width_mm, t.height_mm) * 0.12}
            transform={
              t.rotation_deg
                ? `rotate(${t.rotation_deg} ${cx} ${cy})`
                : undefined
            }
            className="fill-brand/25 stroke-brand/60"
            strokeWidth={Math.max(viewW, viewH) * 0.002}
          />
        );
      })}
    </svg>
  );
}

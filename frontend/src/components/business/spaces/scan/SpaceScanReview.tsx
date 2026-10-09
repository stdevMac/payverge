"use client";

import React, { useCallback, useMemo, useState } from "react";
import { Button, Checkbox } from "@nextui-org/react";
import { AlertTriangle, Check, X } from "lucide-react";
import {
  parseLayoutDocument,
  type LayoutDocument,
  type LayoutTable,
} from "@/api/spaces";
import { LayoutPreviewSvg } from "../layoutPreviewSvg";

interface SpaceScanReviewProps {
  layout: LayoutDocument | Record<string, unknown> | null | unknown;
  /** Confidence threshold below which tables are pre-unchecked. Default 0.55. */
  lowConfidenceThreshold?: number;
  t: (key: string, params?: Record<string, string | number>) => string;
  onApply: (acceptedTables: LayoutTable[], layout: LayoutDocument) => void;
  onOpenEditor: () => void;
  onCancel?: () => void;
  isApplying?: boolean;
}

function layoutConfidence(layout: LayoutDocument | null): number {
  if (!layout?.meta || typeof layout.meta !== "object") return 0.5;
  const c = (layout.meta as Record<string, unknown>).confidence;
  return typeof c === "number" ? c : 0.5;
}

function isApproximate(layout: LayoutDocument | null): boolean {
  if (!layout?.meta || typeof layout.meta !== "object") return true;
  return Boolean((layout.meta as Record<string, unknown>).approximate);
}

function SpaceScanReview({
  layout: rawLayout,
  lowConfidenceThreshold = 0.55,
  t,
  onApply,
  onOpenEditor,
  onCancel,
  isApplying = false,
}: SpaceScanReviewProps) {
  const layout = useMemo(
    () => parseLayoutDocument(rawLayout) ?? { schema_version: 1, tables: [] },
    [rawLayout],
  );
  const conf = layoutConfidence(layout);
  const approx = isApproximate(layout) || conf < lowConfidenceThreshold;
  const tables = useMemo(() => layout.tables ?? [], [layout.tables]);

  // Pre-accept high-confidence overall; still let user reject individual tables.
  const [accepted, setAccepted] = useState<Record<number, boolean>>(() => {
    const init: Record<number, boolean> = {};
    (layout.tables ?? []).forEach((_, i) => {
      // Web keyframe path is approximate — default-accept all so merge is one-tap,
      // but surface low-confidence warning and allow uncheck.
      init[i] = true;
    });
    return init;
  });

  const toggle = useCallback((index: number) => {
    setAccepted((prev) => ({ ...prev, [index]: !prev[index] }));
  }, []);

  const acceptedTables = useMemo(
    () => tables.filter((_, i) => accepted[i]),
    [tables, accepted],
  );

  const previewLayout = useMemo((): LayoutDocument => {
    return {
      ...layout,
      tables: acceptedTables,
    };
  }, [layout, acceptedTables]);

  const handleApply = () => {
    onApply(acceptedTables, {
      ...layout,
      tables: acceptedTables,
    });
  };

  return (
    <div className="flex flex-col gap-4" data-testid="space-scan-review">
      <div className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2.5">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" />
        <div className="min-w-0 text-sm text-amber-950">
          <p className="font-medium">{t("scan.review.warningTitle")}</p>
          <p className="mt-0.5 text-amber-900/90">
            {approx
              ? t("scan.review.approximateBody")
              : t("scan.review.readyBody")}
          </p>
          <p className="mt-1 text-xs text-amber-800/80">
            {t("scan.review.confidence", {
              pct: Math.round(conf * 100),
            })}
          </p>
        </div>
      </div>

      <div
        className="overflow-hidden rounded-xl border border-warm-200 bg-warm-50"
        data-testid="space-scan-review-preview"
      >
        <LayoutPreviewSvg
          layoutRaw={previewLayout}
          className="h-48 w-full"
          aria-label={t("scan.review.previewAria")}
        />
      </div>

      {tables.length === 0 ? (
        <p className="text-sm text-ink-500" data-testid="space-scan-review-empty">
          {t("scan.review.noTables")}
        </p>
      ) : (
        <ul
          className="max-h-48 space-y-2 overflow-y-auto"
          data-testid="space-scan-review-tables"
        >
          {tables.map((table, i) => {
            const low = approx;
            return (
              <li
                key={`${table.table_id}-${i}`}
                className="flex items-center justify-between gap-3 rounded-lg border border-warm-200 bg-white px-3 py-2"
                data-testid={`space-scan-review-table-${i}`}
                data-low-confidence={low ? "true" : "false"}
              >
                <Checkbox
                  isSelected={accepted[i]}
                  onValueChange={() => toggle(i)}
                  size="sm"
                  classNames={{ label: "text-sm text-ink-800" }}
                >
                  <span className="font-medium">
                    {table.name || t("scan.review.unnamedTable", { n: i + 1 })}
                  </span>
                  {low ? (
                    <span className="ml-2 text-xs text-amber-700">
                      {t("scan.review.lowConfidence")}
                    </span>
                  ) : null}
                </Checkbox>
                <span className="shrink-0 text-xs tabular-nums text-ink-400">
                  {table.width_mm}×{table.height_mm} mm
                </span>
              </li>
            );
          })}
        </ul>
      )}

      <div className="flex flex-wrap items-center justify-end gap-2">
        {onCancel ? (
          <Button
            variant="light"
            onPress={onCancel}
            startContent={<X className="h-4 w-4" />}
            data-testid="space-scan-review-cancel"
          >
            {t("scan.review.cancel")}
          </Button>
        ) : null}
        <Button
          variant="bordered"
          className="border-warm-300"
          onPress={onOpenEditor}
          data-testid="space-scan-review-editor"
        >
          {t("scan.review.openEditor")}
        </Button>
        <Button
          className="bg-brand text-white font-medium"
          isLoading={isApplying}
          onPress={handleApply}
          startContent={<Check className="h-4 w-4" />}
          data-testid="space-scan-review-apply"
        >
          {t("scan.review.apply", { count: acceptedTables.length })}
        </Button>
      </div>
    </div>
  );
}

export default SpaceScanReview;

"use client";

import React from "react";
import { Checkbox, Chip } from "@nextui-org/react";
import { ChevronLeft, ChevronRight, ClipboardList } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonList } from "@/components/ui/skeletons";
import type { ChecklistRun, ChecklistRunDetail, ChecklistRunStatus } from "@/api/engagement";

// Presentational checklist surface (Slice 9). Labels-prop contract: every string
// comes from `labels`. House style: rounded-xl/border cards, font-title heading,
// teal accent, shared EmptyState, role="status" loading. NO money here.
//
// Two views off one component. The runs overview (entry point) lists the caller's
// assigned runs with status; selecting a run opens the run-detail ticker — its
// items as toggleable checkboxes (required items marked). Toggling drives the tick
// endpoint upstream; the run status recomputes server-side.

export interface ChecklistRunnerLabels {
  title: string;
  empty: string;
  emptyHint: string;
  loading: string;
  statusPending: string;
  statusInProgress: string;
  statusComplete: string;
  back: string;
  required: string;
  itemsEmpty: string;
  openRun: string; // aria-label for opening a run
  tickError: string; // toast when a tick fails (used by the container)
}

export interface ChecklistRunnerProps {
  runs: ChecklistRun[];
  labels: ChecklistRunnerLabels;
  loading?: boolean;
  formatDate: (iso: string) => string;
  selectedRunId: number | null;
  onSelectRun: (runId: number) => void;
  onBack: () => void;
  detail: ChecklistRunDetail | null;
  detailLoading?: boolean;
  togglingItemId: number | null;
  onToggleItem: (itemId: number, done: boolean) => void;
}

const STATUS_COLOR: Record<ChecklistRunStatus, "default" | "warning" | "success"> = {
  pending: "default",
  in_progress: "warning",
  complete: "success",
};

export default function ChecklistRunner({
  runs,
  labels,
  loading,
  formatDate,
  selectedRunId,
  onSelectRun,
  onBack,
  detail,
  detailLoading,
  togglingItemId,
  onToggleItem,
}: ChecklistRunnerProps) {
  const statusLabel = (s: ChecklistRunStatus): string =>
    s === "complete" ? labels.statusComplete : s === "in_progress" ? labels.statusInProgress : labels.statusPending;

  if (loading) {
    return <SkeletonList rows={3} ariaLabel={labels.loading} />;
  }

  // ---- Run-detail ticker ----
  if (selectedRunId != null) {
    const items = detail?.items ?? [];
    return (
      <section aria-label={labels.title} className="space-y-3">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1 text-sm font-medium text-brand transition hover:text-brand-800"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          {labels.back}
        </button>

        {detail ? (
          <div className="flex items-center justify-between gap-3">
            <h2 className="font-title text-base text-gray-900">{formatDate(detail.run.for_date)}</h2>
            <Chip size="sm" variant="flat" color={STATUS_COLOR[detail.run.status]}>
              {statusLabel(detail.run.status)}
            </Chip>
          </div>
        ) : null}

        {detailLoading ? (
          <SkeletonList rows={4} ariaLabel={labels.loading} />
        ) : items.length === 0 ? (
          <EmptyState icon={ClipboardList} title={labels.itemsEmpty} subtitle={labels.emptyHint} />
        ) : (
          <ul className="space-y-2">
            {items.map((item) => {
              const busy = togglingItemId === item.item_id;
              return (
                <li
                  key={item.item_id}
                  className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 p-3"
                >
                  <Checkbox
                    isSelected={item.done}
                    isDisabled={busy}
                    onValueChange={(next) => onToggleItem(item.item_id, next)}
                    classNames={{ label: "text-sm text-gray-800" }}
                  >
                    {item.label}
                  </Checkbox>
                  {item.is_required ? (
                    <Chip size="sm" variant="flat" color="warning">
                      {labels.required}
                    </Chip>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </section>
    );
  }

  // ---- Runs overview ----
  if (runs.length === 0) {
    return <EmptyState icon={ClipboardList} title={labels.empty} subtitle={labels.emptyHint} />;
  }

  return (
    <section aria-label={labels.title} className="space-y-3">
      <h2 className="font-title text-base text-gray-900">{labels.title}</h2>
      <ul className="space-y-2">
        {runs.map((run) => (
          <li key={run.id}>
            <button
              type="button"
              aria-label={labels.openRun}
              onClick={() => onSelectRun(run.id)}
              className="flex w-full items-center justify-between gap-3 rounded-xl border border-gray-200 p-4 text-left transition hover:border-brand-300"
            >
              <span className="flex items-center gap-2 text-sm text-gray-800">
                <ClipboardList className="h-4 w-4 text-brand" aria-hidden="true" />
                {formatDate(run.for_date)}
              </span>
              <span className="flex items-center gap-2">
                <Chip size="sm" variant="flat" color={STATUS_COLOR[run.status]}>
                  {statusLabel(run.status)}
                </Chip>
                <ChevronRight className="h-4 w-4 text-gray-400" aria-hidden="true" />
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

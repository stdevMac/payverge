"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { checklistsApi } from "@/api/engagement";
import { queryKeys } from "@/api/queryKeys";
import { useToast } from "@/contexts/ToastContext";
import { intlLocaleFor } from "@/utils/intlLocale";
import ChecklistRunner, { type ChecklistRunnerLabels } from "./ChecklistRunner";

// Data container for the staff "More → Checklists" / "Onboarding" surfaces.
// Mounts only when the staff member opens the section (StaffMore render-prop), so
// the runs fetch stays dormant until then. The shipped read path returns the
// caller's ASSIGNED runs only; `onboardingOnly` further narrows to non-shift
// (onboarding) runs. Selecting a run opens the run-detail ticker (its items); a
// toggle drives the tick endpoint and refetches the detail + the runs list (the
// run status recomputes server-side). Hands a fully-resolved `labels` object + a
// locale date formatter to the presentational ChecklistRunner. NO money on this wire.

export interface ChecklistRunnerContainerProps {
  businessId: string;
  locale: string;
  labels: ChecklistRunnerLabels;
  onboardingOnly?: boolean;
}

export default function ChecklistRunnerContainer({
  businessId,
  locale,
  labels,
  onboardingOnly,
}: ChecklistRunnerContainerProps) {
  const qc = useQueryClient();
  const toast = useToast();
  const [selectedRunId, setSelectedRunId] = useState<number | null>(null);

  const runsKey = queryKeys.engagement.checklistRuns(businessId);
  const { data, isLoading } = useQuery({
    queryKey: runsKey,
    queryFn: () => checklistsApi.listRuns(businessId),
  });

  const runs = useMemo(() => {
    const all = data ?? [];
    return onboardingOnly ? all.filter((r) => r.shift_id == null) : all;
  }, [data, onboardingOnly]);

  const detailQuery = useQuery({
    queryKey: selectedRunId == null
      ? queryKeys.engagement.checklistRunDetail(businessId, 0)
      : queryKeys.engagement.checklistRunDetail(businessId, selectedRunId),
    queryFn: () => checklistsApi.getRunDetail(businessId, selectedRunId as number),
    enabled: selectedRunId != null,
  });

  const tickMutation = useMutation({
    mutationFn: (vars: { itemId: number; done: boolean }) =>
      checklistsApi.tickItem(businessId, selectedRunId as number, vars.itemId, vars.done),
    onSuccess: () => {
      if (selectedRunId != null) {
        void qc.invalidateQueries({
          queryKey: queryKeys.engagement.checklistRunDetail(businessId, selectedRunId),
        });
      }
      void qc.invalidateQueries({ queryKey: runsKey });
    },
    onError: () => toast.showError(labels.tickError),
  });

  const formatDate = useCallback(
    (iso: string): string => {
      const d = new Date(iso);
      if (Number.isNaN(d.getTime())) return "";
      return new Intl.DateTimeFormat(intlLocaleFor(locale), {
        weekday: "short",
        month: "short",
        day: "numeric",
      }).format(d);
    },
    [locale],
  );

  return (
    <ChecklistRunner
      runs={runs}
      labels={labels}
      loading={isLoading}
      formatDate={formatDate}
      selectedRunId={selectedRunId}
      onSelectRun={setSelectedRunId}
      onBack={() => setSelectedRunId(null)}
      detail={detailQuery.data ?? null}
      detailLoading={detailQuery.isLoading}
      togglingItemId={tickMutation.isPending ? (tickMutation.variables?.itemId ?? null) : null}
      onToggleItem={(itemId, done) => tickMutation.mutate({ itemId, done })}
    />
  );
}

"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Chip, Input, Textarea } from "@nextui-org/react";
import { CalendarClock, CalendarOff } from "lucide-react";
import {
  availabilityApi,
  type TimeOffRequest,
  type TimeOffStatus,
} from "@/api/availability";
import { queryKeys } from "@/api/queryKeys";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonLine } from "@/components/ui/skeletons";
import { useToast } from "@/contexts/ToastContext";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import type { StaffData } from "@/utils/staffAuth";

export interface TimeOffRequestsLabels {
  title: string;
  subtitle: string;
  loading: string;
  error: string;
  emptyTitle: string;
  emptySubtitle: string;
  formTitle: string;
  startLabel: string;
  endLabel: string;
  reasonLabel: string;
  reasonPlaceholder: string;
  submit: string;
  submitting: string;
  submitError: string;
  invalidRange: string;
  statusPending: string;
  statusApproved: string;
  statusDenied: string;
  statusCancelled: string;
}

export interface TimeOffRequestsProps {
  staff: StaffData;
  labels: TimeOffRequestsLabels;
  /** Display locale for Intl date/time formatting (not a translated string). */
  locale: string;
}

type ChipColor = "warning" | "success" | "danger" | "default";

function statusMeta(
  status: TimeOffStatus,
  labels: TimeOffRequestsLabels,
): { label: string; color: ChipColor } {
  switch (status) {
    case "approved":
      return { label: labels.statusApproved, color: "success" };
    case "denied":
      return { label: labels.statusDenied, color: "danger" };
    case "cancelled":
      return { label: labels.statusCancelled, color: "default" };
    default:
      return { label: labels.statusPending, color: "warning" };
  }
}

function formatRange(startIso: string, endIso: string, locale: string): string {
  const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
  return `${fmt.format(new Date(startIso))} – ${fmt.format(new Date(endIso))}`;
}

export default function TimeOffRequests({ staff, labels, locale }: TimeOffRequestsProps) {
  const businessId = String(staff.business_id);
  const toast = useToast();
  const queryClient = useQueryClient();
  const listKey = queryKeys.timeOff.list(businessId);

  const query = useQuery({
    queryKey: listKey,
    queryFn: () => availabilityApi.listTimeOff(businessId),
  });

  // A manager's decision flips the requester's own row; refresh on that event.
  useStaffRealtime({
    businessId: staff.business_id,
    onTimeOffDecided: () => queryClient.invalidateQueries({ queryKey: listKey }),
    // Decisions made during an SSE gap were never delivered — resync.
    onReconnect: () => queryClient.invalidateQueries({ queryKey: listKey }),
  });

  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [reason, setReason] = useState("");

  const invalidRange = useMemo(() => {
    if (!start || !end) return false;
    return new Date(end).getTime() <= new Date(start).getTime();
  }, [start, end]);

  const canSubmit = Boolean(start) && Boolean(end) && !invalidRange;

  const createMutation = useMutation({
    mutationFn: () =>
      availabilityApi.createTimeOff(businessId, {
        starts_at: new Date(start).toISOString(),
        ends_at: new Date(end).toISOString(),
        reason: reason.trim(),
      }),
    onSuccess: () => {
      setStart("");
      setEnd("");
      setReason("");
      void queryClient.invalidateQueries({ queryKey: listKey });
    },
    onError: () => toast.showError(labels.submitError),
  });

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (!canSubmit) return;
      createMutation.mutate();
    },
    [canSubmit, createMutation],
  );

  const requests: TimeOffRequest[] = query.data ?? [];

  return (
    <section className="space-y-4" aria-label={labels.title}>
      <header>
        <h2 className="font-title text-base text-ink-900">{labels.title}</h2>
        <p className="text-sm text-ink-500">{labels.subtitle}</p>
      </header>

      {/* Request form */}
      <form onSubmit={handleSubmit} className="rounded-xl border border-warm-200 p-4">
        <h3 className="text-sm font-semibold text-ink-900">{labels.formTitle}</h3>
        <div className="mt-3 grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
          <Input
            type="datetime-local"
            label={labels.startLabel}
            aria-label={labels.startLabel}
            value={start}
            onValueChange={setStart}
            labelPlacement="outside"
            variant="bordered"
            radius="lg"
            startContent={<CalendarClock className="h-4 w-4 shrink-0 text-ink-400" aria-hidden />}
            classNames={{ label: "text-sm font-medium text-ink-700" }}
          />
          <Input
            type="datetime-local"
            label={labels.endLabel}
            aria-label={labels.endLabel}
            value={end}
            onValueChange={setEnd}
            labelPlacement="outside"
            variant="bordered"
            radius="lg"
            startContent={<CalendarClock className="h-4 w-4 shrink-0 text-ink-400" aria-hidden />}
            classNames={{ label: "text-sm font-medium text-ink-700" }}
          />
        </div>
        <Textarea
          label={labels.reasonLabel}
          aria-label={labels.reasonLabel}
          value={reason}
          onValueChange={setReason}
          placeholder={labels.reasonPlaceholder}
          minRows={2}
          labelPlacement="outside"
          variant="bordered"
          radius="lg"
          className="mt-3"
          classNames={{ label: "text-sm font-medium text-ink-700" }}
        />
        {invalidRange ? (
          <p className="mt-2 text-xs text-red-600">{labels.invalidRange}</p>
        ) : null}
        <Button
          type="submit"
          color="primary"
          className="mt-3 w-full"
          isLoading={createMutation.isPending}
          isDisabled={!canSubmit}
        >
          {createMutation.isPending ? labels.submitting : labels.submit}
        </Button>
      </form>

      {query.isLoading ? (
        <div
          role="status"
          aria-live="polite"
          aria-label={labels.loading}
          className="space-y-3"
        >
          {[0, 1].map((i) => (
            <div
              key={i}
              className="flex items-center justify-between gap-3 rounded-xl border border-warm-200 p-4 motion-safe:animate-pulse"
            >
              <div className="min-w-0 flex-1 space-y-1.5">
                <SkeletonLine width="45%" height="0.875rem" />
                <SkeletonLine width="65%" height="0.75rem" />
              </div>
              <SkeletonLine width="4.5rem" height="1.5rem" className="shrink-0 rounded-full" />
            </div>
          ))}
        </div>
      ) : query.isError ? (
        <div className="rounded-xl border border-warm-200 p-4">
          <p className="text-sm text-ink-500">{labels.error}</p>
        </div>
      ) : requests.length === 0 ? (
        <div className="rounded-xl border border-warm-200">
          <EmptyState
            icon={CalendarOff}
            title={labels.emptyTitle}
            subtitle={labels.emptySubtitle}
          />
        </div>
      ) : (
        <ul className="space-y-3">
          {requests.map((req) => {
            const meta = statusMeta(req.status, labels);
            return (
              <li
                key={req.id}
                className="flex items-start justify-between gap-3 rounded-xl border border-warm-200 p-4 motion-safe:animate-[cmdk-fade_140ms_ease-out]"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium text-ink-900">
                    {formatRange(req.starts_at, req.ends_at, locale)}
                  </p>
                  {req.reason ? (
                    <p className="mt-1 truncate text-sm text-ink-500">{req.reason}</p>
                  ) : null}
                </div>
                <Chip size="sm" variant="flat" color={meta.color}>
                  {meta.label}
                </Chip>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

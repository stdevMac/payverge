"use client";

import React from "react";
import { Button, Chip } from "@nextui-org/react";
import { CalendarClock, ArrowLeftRight, Inbox } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonCard } from "@/components/ui/skeletons";
import type {
  OpenCoverage,
  MyCoverage,
  SwapStatus,
  OpenClaimStatus,
  CancelKind,
} from "@/api/coverage";

// Terminal swap states can't be cancelled; a claim can only be withdrawn while
// pending. These mirror the backend transition guards (coverage_service.go).
const TERMINAL_SWAP_STATUS = new Set<SwapStatus>(["approved", "denied", "cancelled"]);

// Presentational coverage board for the staff "More → Coverage" surface. Fully
// labels-driven (the shell resolves every string) and entirely money-free —
// hours/time ranges + position names only, never pay. The container that feeds
// it (StaffDashboardShell → CoverageSection) owns the data fetch + mutations.

export interface CoverageBoardLabels {
  title: string;
  openShifts: string;
  swapInbox: string;
  myRequests: string;
  claim: string;
  accept: string;
  cancel: string;
  rowSwap: string;
  rowGiveup: string;
  rowClaim: string;
  statusPending: string;
  statusAccepted: string;
  statusPendingApproval: string;
  statusApproved: string;
  statusDenied: string;
  statusCancelled: string;
  statusWithdrawn: string;
  emptyOpenTitle: string;
  emptyOpenSubtitle: string;
  emptyInboxTitle: string;
  emptyInboxSubtitle: string;
  emptyMineTitle: string;
  emptyMineSubtitle: string;
  loading: string;
}

export interface CoverageBoardProps {
  open: OpenCoverage | null;
  mine: MyCoverage | null;
  loading: boolean;
  /** Disables the single row whose claim/accept mutation is in flight. */
  busyId: number | null;
  /** Row key ("sw-{id}"/"cl-{id}") whose cancel mutation is in flight, or null. */
  cancelBusyKey: string | null;
  onClaim: (shiftId: number) => void;
  onAccept: (swapId: number) => void;
  /** Retract one of the caller's own requests (swap/giveup id or claim id). */
  onCancel: (requestId: number, kind: CancelKind) => void;
  positionName: (positionId: number) => string;
  /** Locale-formatted time range — hours/time only, never money. */
  formatRange: (startsAt: string, endsAt: string) => string;
  labels: CoverageBoardLabels;
}

type ChipColor = "warning" | "success" | "danger" | "default";

function statusLabel(
  labels: CoverageBoardLabels,
  s: SwapStatus | OpenClaimStatus,
): string {
  const map: Record<string, string> = {
    pending: labels.statusPending,
    open: labels.statusPending,
    accepted: labels.statusAccepted,
    pending_approval: labels.statusPendingApproval,
    approved: labels.statusApproved,
    denied: labels.statusDenied,
    cancelled: labels.statusCancelled,
    withdrawn: labels.statusWithdrawn,
  };
  return map[s] ?? s;
}

function statusColor(s: SwapStatus | OpenClaimStatus): ChipColor {
  if (s === "approved") return "success";
  if (s === "denied" || s === "cancelled" || s === "withdrawn") return "danger";
  return "warning";
}

export default function CoverageBoard({
  open,
  mine,
  loading,
  busyId,
  cancelBusyKey,
  onClaim,
  onAccept,
  onCancel,
  positionName,
  formatRange,
  labels,
}: CoverageBoardProps) {
  if (loading || !open || !mine) {
    return (
      <div
        role="status"
        aria-live="polite"
        aria-label={labels.loading}
        className="space-y-3"
      >
        <SkeletonCard />
        <SkeletonCard />
      </div>
    );
  }

  const hasMine = mine.claims.length > 0 || mine.swaps.length > 0;

  return (
    <section className="space-y-6" aria-label={labels.title}>
      <h2 className="font-title text-base text-gray-900">{labels.title}</h2>

      {/* Open shifts I can claim */}
      <div className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
          <CalendarClock className="h-4 w-4 text-brand" aria-hidden="true" />
          {labels.openShifts}
        </h3>
        {open.open_shifts.length === 0 ? (
          <div className="rounded-xl border border-gray-200">
            <EmptyState
              icon={CalendarClock}
              title={labels.emptyOpenTitle}
              subtitle={labels.emptyOpenSubtitle}
            />
          </div>
        ) : (
          <ul className="space-y-2">
            {open.open_shifts.map((s) => (
              <li
                key={s.shift_id}
                className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 p-4"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {positionName(s.position_id)}
                  </p>
                  <p className="text-xs text-gray-500">
                    {formatRange(s.starts_at, s.ends_at)}
                  </p>
                </div>
                <Button
                  size="sm"
                  color="primary"
                  variant="flat"
                  isDisabled={busyId === s.shift_id}
                  isLoading={busyId === s.shift_id}
                  onPress={() => onClaim(s.shift_id)}
                >
                  {labels.claim}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Swap inbox — swaps I'm eligible to cover */}
      <div className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
          <ArrowLeftRight className="h-4 w-4 text-brand" aria-hidden="true" />
          {labels.swapInbox}
        </h3>
        {open.swap_inbox.length === 0 ? (
          <div className="rounded-xl border border-gray-200">
            <EmptyState
              icon={ArrowLeftRight}
              title={labels.emptyInboxTitle}
              subtitle={labels.emptyInboxSubtitle}
            />
          </div>
        ) : (
          <ul className="space-y-2">
            {open.swap_inbox.map((s) => (
              <li
                key={s.swap_id}
                className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 p-4"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {positionName(s.position_id)}
                  </p>
                  <p className="text-xs text-gray-500">
                    {formatRange(s.starts_at, s.ends_at)}
                  </p>
                </div>
                <Button
                  size="sm"
                  color="primary"
                  variant="flat"
                  isDisabled={busyId === s.swap_id}
                  isLoading={busyId === s.swap_id}
                  onPress={() => onAccept(s.swap_id)}
                >
                  {labels.accept}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* My requests — claims I filed + swaps I requested, with status */}
      <div className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-900">
          <Inbox className="h-4 w-4 text-brand" aria-hidden="true" />
          {labels.myRequests}
        </h3>
        {!hasMine ? (
          <div className="rounded-xl border border-gray-200">
            <EmptyState
              icon={Inbox}
              title={labels.emptyMineTitle}
              subtitle={labels.emptyMineSubtitle}
            />
          </div>
        ) : (
          <ul className="space-y-2">
            {mine.swaps.map((s) => {
              const rowKey = `sw-${s.id}`;
              const cancellable = !TERMINAL_SWAP_STATUS.has(s.status);
              return (
                <li
                  key={rowKey}
                  className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 p-4"
                >
                  <p className="text-sm text-gray-900">
                    {s.kind === "giveup" ? labels.rowGiveup : labels.rowSwap}
                  </p>
                  <div className="flex items-center gap-2">
                    <Chip size="sm" variant="flat" color={statusColor(s.status)}>
                      {statusLabel(labels, s.status)}
                    </Chip>
                    {cancellable ? (
                      <Button
                        size="sm"
                        variant="light"
                        className="h-auto min-w-0 px-2 py-0.5 text-xs text-gray-500"
                        isDisabled={cancelBusyKey === rowKey}
                        isLoading={cancelBusyKey === rowKey}
                        onPress={() => onCancel(s.id, s.kind)}
                      >
                        {labels.cancel}
                      </Button>
                    ) : null}
                  </div>
                </li>
              );
            })}
            {mine.claims.map((c) => {
              const rowKey = `cl-${c.id}`;
              const cancellable = c.status === "pending";
              return (
                <li
                  key={rowKey}
                  className="flex items-center justify-between gap-3 rounded-xl border border-gray-200 p-4"
                >
                  <p className="text-sm text-gray-900">{labels.rowClaim}</p>
                  <div className="flex items-center gap-2">
                    <Chip size="sm" variant="flat" color={statusColor(c.status)}>
                      {statusLabel(labels, c.status)}
                    </Chip>
                    {cancellable ? (
                      <Button
                        size="sm"
                        variant="light"
                        className="h-auto min-w-0 px-2 py-0.5 text-xs text-gray-500"
                        isDisabled={cancelBusyKey === rowKey}
                        isLoading={cancelBusyKey === rowKey}
                        onPress={() => onCancel(c.id, "open_claim")}
                      >
                        {labels.cancel}
                      </Button>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}

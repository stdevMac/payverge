"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Chip } from "@nextui-org/react";
import {
  ArrowLeftRight,
  CalendarCheck2,
  Check,
  ChevronDown,
  History,
  X,
} from "lucide-react";
import { availabilityApi, type TimeOffRequest } from "@/api/availability";
import {
  coverageApi,
  type CoverageHistoryItem,
  type PendingApprovalItem,
} from "@/api/coverage";
import { positionsApi, type Position } from "@/api/positions";
import { translatePositionName } from "./positionLabel";
import { getBusinessStaff, type StaffMember } from "@/api/staff";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { EmptyState } from "@/components/ui/EmptyState";
import { btnTonalSuccess, btnQuietDanger } from "@/components/ui/buttonStyles";
import { SkeletonLine } from "@/components/ui/skeletons";
import { intlLocaleFor } from "@/utils/intlLocale";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";

interface ApprovalsPanelProps {
  businessId: string;
  /**
   * When embedded inside the Schedule "Needs your approval" inbox, the panel
   * drops its own card chrome + header (the inbox supplies one) and its empties
   * render compact. Standalone (default) keeps the full section.
   */
  /** Venue IANA timezone for request time ranges (never device-local). */
  businessTimezone?: string | null;
  embedded?: boolean;
  /** Reports pending request count (time-off + coverage) for the inbox badge. */
  onPendingCountChange?: (count: number) => void;
}

/** Instant comparison in UTC: ends_at already on the wire is an absolute instant. */
function isElapsedIso(iso: string, nowMs: number = Date.now()): boolean {
  const endsAt = new Date(iso).getTime();
  return Number.isFinite(endsAt) && endsAt < nowMs;
}

function formatRange(
  startIso: string,
  endIso: string,
  locale: string,
  timeZone: string | null,
): string {
  const fmt = new Intl.DateTimeFormat(intlLocaleFor(locale), {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: timeZone || "UTC",
  });
  return `${fmt.format(new Date(startIso))} – ${fmt.format(new Date(endIso))}`;
}

export default function ApprovalsPanel({
  businessId,
  businessTimezone = null,
  embedded = false,
  onPendingCountChange,
}: ApprovalsPanelProps) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const queryClient = useQueryClient();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardApprovals.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const pendingKey = queryKeys.timeOff.list(businessId, "pending");
  const coverageKey = queryKeys.coverage.open(businessId);

  const query = useQuery({
    queryKey: pendingKey,
    queryFn: () => availabilityApi.listTimeOff(businessId, "pending"),
  });

  // Approver-scoped: GET /coverage/open returns the manager queue (pending swaps
  // + open-shift claims) only for approvers; empty otherwise. Same route, role
  // branched server-side.
  const coverageQuery = useQuery({
    queryKey: coverageKey,
    queryFn: () => coverageApi.listOpen(businessId),
  });

  const staffQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => getBusinessStaff(businessId),
    staleTime: 5 * 60 * 1000,
  });

  const positionsQuery = useQuery({
    queryKey: ["positions", businessId],
    queryFn: () => positionsApi.list(businessId),
    staleTime: 5 * 60 * 1000,
  });

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: pendingKey }),
    [queryClient, pendingKey],
  );
  // Approving/denying time-off doesn't just empty the pending queue — it moves
  // the request into the "approved" list that the schedule grid's time-off
  // overlay and the shift-editor conflict check read (both cached 5min). Refresh
  // every time-off list (all statuses) plus the team-availability overlay so the
  // grid stops showing a decided request as still pending.
  const invalidateTimeOff = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.timeOff.all });
    void queryClient.invalidateQueries({
      queryKey: queryKeys.availability.team(businessId),
    });
  }, [queryClient, businessId]);
  const invalidateCoverage = useCallback(
    () => queryClient.invalidateQueries({ queryKey: coverageKey }),
    [queryClient, coverageKey],
  );
  const invalidateHistory = useCallback(
    () =>
      queryClient.invalidateQueries({
        queryKey: queryKeys.coverage.history(businessId),
      }),
    [queryClient, businessId],
  );
  // A decided coverage request leaves the queue AND enters history — refresh both.
  const invalidateCoverageAndHistory = useCallback(() => {
    void invalidateCoverage();
    void invalidateHistory();
  }, [invalidateCoverage, invalidateHistory]);

  // A new request or a decision elsewhere should refresh the queue live —
  // time-off AND coverage (a coworker claims/accepts, a manager decides).
  // A decision elsewhere leaves the pending queue AND updates the approved list
  // + availability overlay — refresh all three.
  const invalidateTimeOffDecided = useCallback(() => {
    void invalidate();
    invalidateTimeOff();
  }, [invalidate, invalidateTimeOff]);

  useStaffRealtime({
    businessId: Number(businessId),
    onTimeOffRequested: invalidate,
    onTimeOffDecided: invalidateTimeOffDecided,
    onOpenShiftClaimed: invalidateCoverage,
    onOpenShiftDecided: invalidateCoverageAndHistory,
    onSwapRequested: invalidateCoverage,
    onSwapAccepted: invalidateCoverage,
    onSwapDecided: invalidateCoverageAndHistory,
    // Events emitted during an SSE gap were never delivered — resync every
    // queue this panel feeds (pending time-off + overlays, coverage, history).
    onReconnect: () => {
      invalidateTimeOffDecided();
      invalidateCoverageAndHistory();
    },
  });

  const staffById = useMemo(() => {
    const map = new Map<number, StaffMember>();
    (staffQuery.data?.staff ?? []).forEach((s) => map.set(s.id, s));
    return map;
  }, [staffQuery.data]);

  const positionsById = useMemo(() => {
    const map = new Map<number, Position>();
    (positionsQuery.data ?? []).forEach((p) => map.set(p.id, p));
    return map;
  }, [positionsQuery.data]);

  const decideMutation = useMutation({
    mutationFn: ({ reqId, approve }: { reqId: number; approve: boolean }) =>
      availabilityApi.decide(businessId, reqId, approve, ""),
    onSuccess: (_data, vars) => {
      void invalidate();
      invalidateTimeOff();
      toast.showSuccess(vars.approve ? t("approveSuccess") : t("denySuccess"));
    },
    onError: () => toast.showError(t("decisionError")),
  });

  // The single locked decision route is polymorphic — pass the `kind`
  // discriminator so the backend routes to the swap vs. open-claim machine.
  const coverageDecideMutation = useMutation({
    mutationFn: ({
      item,
      approve,
    }: {
      item: PendingApprovalItem;
      approve: boolean;
    }) =>
      coverageApi.decide(businessId, item.id, {
        kind: item.kind === "open_claim" ? "open_claim" : "swap",
        decision: approve ? "approve" : "deny",
      }),
    onSuccess: (_data, vars) => {
      void invalidateCoverage();
      void invalidateHistory(); // a decided request now belongs to history
      toast.showSuccess(vars.approve ? t("approveSuccess") : t("denySuccess"));
    },
    onError: () => toast.showError(t("decisionError")),
  });

  const requests = useMemo<TimeOffRequest[]>(
    () => query.data ?? [],
    [query.data],
  );
  const liveRequests = useMemo(
    () => requests.filter((req) => !isElapsedIso(req.ends_at)),
    [requests],
  );
  const staleRequests = useMemo(
    () => requests.filter((req) => isElapsedIso(req.ends_at)),
    [requests],
  );
  const pendingCount = liveRequests.length;
  const deciding = decideMutation.isPending ? decideMutation.variables : null;

  const coverageItems = useMemo<PendingApprovalItem[]>(() => {
    const pa = coverageQuery.data?.pending_approvals;
    return pa ? [...pa.swaps, ...pa.claims] : [];
  }, [coverageQuery.data]);
  const liveCoverage = useMemo(
    () => coverageItems.filter((item) => !isElapsedIso(item.ends_at)),
    [coverageItems],
  );
  const staleCoverage = useMemo(
    () => coverageItems.filter((item) => isElapsedIso(item.ends_at)),
    [coverageItems],
  );
  const coverageCount = liveCoverage.length;
  const coverageDeciding = coverageDecideMutation.isPending
    ? coverageDecideMutation.variables
    : null;

  const totalPending = pendingCount + coverageCount;
  const hasStale = staleRequests.length + staleCoverage.length > 0;

  // Surface the pending count to the inbox so its tab badge stays in sync.
  useEffect(() => {
    onPendingCountChange?.(totalPending);
  }, [totalPending, onPendingCountChange]);

  // Seeded default position names localize via the dashboardSchedule
  // namespace (this panel's own `t` is dashboardApprovals-scoped, and pointing
  // it at positions.* would fire missing-translation telemetry on every row).
  const scheduleT = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardSchedule.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const positionName = useCallback(
    (positionId: number) => {
      const raw = positionsById.get(positionId)?.name;
      return raw
        ? translatePositionName(raw, scheduleT)
        : t("coveragePositionFallback");
    },
    [positionsById, scheduleT, t],
  );

  const coverageActorId = useCallback((item: PendingApprovalItem): number => {
    if (item.kind === "open_claim") {
      return item.claiming_staff_id ?? 0;
    }
    return item.requesting_staff_id;
  }, []);

  const rowLabel = useCallback(
    (
      kind: PendingApprovalItem["kind"],
      positionId: number,
      actorName: string,
    ): string => {
      const tpl =
        kind === "open_claim"
          ? t("claimRow")
          : kind === "giveup"
            ? t("giveupRow")
            : t("swapRow");
      return tpl
        .replace("{name}", actorName)
        .replace("{position}", positionName(positionId));
    },
    [t, positionName],
  );

  const staffName = useCallback(
    (id: number) =>
      staffById.get(id)?.name || t("staffFallback").replace("{id}", String(id)),
    [staffById, t],
  );

  // Resolved coverage events (Slice 5c). Approver-only server-side; a non-approver
  // 403s → we render nothing. retry:false so a 403 doesn't thrash. Collapsed by
  // default (progressive disclosure) — the manager opens it on demand.
  const [historyOpen, setHistoryOpen] = useState(false);
  const historyQuery = useQuery({
    queryKey: queryKeys.coverage.history(businessId),
    queryFn: () => coverageApi.history(businessId),
    retry: false,
  });
  const historyItems = useMemo<CoverageHistoryItem[]>(
    () => historyQuery.data ?? [],
    [historyQuery.data],
  );
  const historyStatusColor = (
    status: CoverageHistoryItem["status"],
  ): "success" | "danger" | "default" =>
    status === "approved"
      ? "success"
      : status === "denied"
        ? "danger"
        : "default";

  return (
    <section
      aria-label={t("title")}
      className={embedded ? "" : "rounded-2xl border border-warm-200 bg-white"}
    >
      {!embedded ? (
        <div className="flex items-center justify-between gap-3 border-b border-warm-200 p-4">
          <div>
            <h2 className="font-title text-base text-ink-900">{t("title")}</h2>
            <p className="text-sm text-ink-500">{t("subtitle")}</p>
          </div>
          {totalPending > 0 ? (
            <Chip size="sm" variant="flat" color="warning" aria-live="polite">
              {/* L5-35: cap matches header chip / SegmentedTabs. */}
              {t("pendingBadge").replace(
                "{count}",
                totalPending > 9 ? "9+" : String(totalPending),
              )}
            </Chip>
          ) : null}
        </div>
      ) : null}

      {query.isLoading || coverageQuery.isLoading ? (
        <div
          role="status"
          aria-live="polite"
          aria-label={t("loading")}
          className="space-y-4 p-4"
        >
          {[0, 1].map((i) => (
            <div
              key={i}
              className="flex items-center justify-between gap-3 motion-safe:animate-pulse"
            >
              <div className="min-w-0 flex-1 space-y-1.5">
                <SkeletonLine width="40%" height="0.875rem" />
                <SkeletonLine width="60%" height="0.75rem" />
              </div>
              <SkeletonLine
                width="5rem"
                height="2rem"
                className="shrink-0 rounded-lg"
              />
            </div>
          ))}
        </div>
      ) : query.isError || coverageQuery.isError ? (
        // A coverage-queue error must NOT read as "All clear": swaps/claims could
        // be pending but unfetched, so surface the error instead of the empty.
        <p className="p-4 text-sm text-ink-500">{t("error")}</p>
      ) : totalPending === 0 && !hasStale ? (
        // One consolidated empty for the whole approval inbox — no more a styled
        // time-off empty stacked over a bare coverage line.
        <EmptyState
          panel
          compact
          icon={CalendarCheck2}
          title={t("emptyTitle")}
          subtitle={t("emptySubtitle")}
        />
      ) : (
        <>
          {pendingCount > 0 ? (
            <div className="p-4">
              <ul className="space-y-3">
                {liveRequests.map((req) => {
                  const name = staffName(req.staff_id);
                  const busy = deciding?.reqId === req.id;
                  return (
                    <li
                      key={req.id}
                      className="flex flex-wrap items-start justify-between gap-3 border-t border-warm-100 pt-3 first:border-0 first:pt-0 motion-safe:animate-[cmdk-fade_140ms_ease-out]"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-sm font-medium text-ink-900">
                            {name}
                          </p>
                        </div>
                        <p className="text-sm text-ink-500">
                          {formatRange(req.starts_at, req.ends_at, locale, businessTimezone)}
                        </p>
                        <p className="mt-0.5 text-sm text-ink-500">
                          {req.reason || t("reasonNone")}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          radius="full"
                          className={btnTonalSuccess}
                          aria-label={`${t("approve")} — ${name}`}
                          startContent={<Check className="h-4 w-4" />}
                          isLoading={busy && deciding?.approve === true}
                          isDisabled={decideMutation.isPending}
                          onPress={() =>
                            decideMutation.mutate({
                              reqId: req.id,
                              approve: true,
                            })
                          }
                        >
                          {t("approve")}
                        </Button>
                        <Button
                          size="sm"
                          radius="full"
                          className={btnQuietDanger}
                          aria-label={`${t("deny")} — ${name}`}
                          startContent={<X className="h-4 w-4" />}
                          isLoading={busy && deciding?.approve === false}
                          isDisabled={decideMutation.isPending}
                          onPress={() =>
                            decideMutation.mutate({
                              reqId: req.id,
                              approve: false,
                            })
                          }
                        >
                          {t("deny")}
                        </Button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          ) : null}

          {/* Coverage approvals — swaps, give-ups, and open-shift claims (Slice 5).
              Same Approve/Deny verbs; routed by the `kind` discriminator. Only
              rendered when there's something to decide, so an empty queue no
              longer leaves a dangling "no coverage" line. */}
          {coverageCount > 0 ? (
            <div
              className={
                pendingCount > 0 ? "border-t border-warm-200 p-4" : "p-4"
              }
            >
              <div className="mb-3">
                <div className="flex items-center gap-2">
                  <ArrowLeftRight
                    className="h-4 w-4 text-brand"
                    aria-hidden="true"
                  />
                  <h3 className="text-sm font-semibold text-ink-900">
                    {t("coverageTitle")}
                  </h3>
                </div>
                <p className="mt-0.5 text-xs text-ink-500">
                  {t("coverageHint")}
                </p>
              </div>
              <ul className="space-y-3">
                {liveCoverage.map((item) => {
                  const busy =
                    coverageDeciding?.item.id === item.id &&
                    coverageDeciding?.item.kind === item.kind;
                  const actor = staffName(coverageActorId(item));
                  const label = rowLabel(
                    item.kind,
                    item.position_id,
                    actor,
                  );
                  return (
                    <li
                      key={`cov-${item.kind}-${item.id}`}
                      className="flex flex-wrap items-start justify-between gap-3 border-t border-warm-100 pt-3 first:border-0 first:pt-0 motion-safe:animate-[cmdk-fade_140ms_ease-out]"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-ink-900">
                          {label}
                        </p>
                        <p className="text-sm text-ink-500">
                          {formatRange(item.starts_at, item.ends_at, locale, businessTimezone)}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          radius="full"
                          className={btnTonalSuccess}
                          aria-label={`${t("approve")} — ${label}`}
                          startContent={<Check className="h-4 w-4" />}
                          isLoading={busy && coverageDeciding?.approve === true}
                          isDisabled={coverageDecideMutation.isPending}
                          onPress={() =>
                            coverageDecideMutation.mutate({
                              item,
                              approve: true,
                            })
                          }
                        >
                          {t("approve")}
                        </Button>
                        <Button
                          size="sm"
                          radius="full"
                          className={btnQuietDanger}
                          aria-label={`${t("deny")} — ${label}`}
                          startContent={<X className="h-4 w-4" />}
                          isLoading={
                            busy && coverageDeciding?.approve === false
                          }
                          isDisabled={coverageDecideMutation.isPending}
                          onPress={() =>
                            coverageDecideMutation.mutate({
                              item,
                              approve: false,
                            })
                          }
                        >
                          {t("deny")}
                        </Button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          ) : null}

          {hasStale ? (
            <div
              className={
                totalPending > 0 ? "border-t border-warm-200 p-4" : "p-4"
              }
            >
              <h3 className="text-sm font-semibold text-ink-900">
                {t("expiredTitle")}
              </h3>
              <p className="mt-0.5 text-xs text-ink-500">{t("expiredHint")}</p>
              <ul className="mt-3 space-y-3">
                {staleRequests.map((req) => {
                  const name = staffName(req.staff_id);
                  return (
                    <li
                      key={`stale-to-${req.id}`}
                      data-stale="true"
                      className="border-t border-warm-100 pt-3 first:border-0 first:pt-0"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-medium text-ink-900">
                          {name}
                        </p>
                        <Chip size="sm" variant="flat" color="warning">
                          {t("staleBadge")}
                        </Chip>
                      </div>
                      <p className="text-sm text-ink-500">
                        {formatRange(
                          req.starts_at,
                          req.ends_at,
                          locale,
                          businessTimezone,
                        )}
                      </p>
                      <p className="mt-0.5 text-sm text-ink-500">
                        {req.reason || t("reasonNone")}
                      </p>
                      <p className="mt-1 text-xs text-ink-500">
                        {t("expiredActionHidden")}
                      </p>
                    </li>
                  );
                })}
                {staleCoverage.map((item) => {
                  const actor = staffName(coverageActorId(item));
                  const label = rowLabel(item.kind, item.position_id, actor);
                  return (
                    <li
                      key={`stale-cov-${item.kind}-${item.id}`}
                      data-stale="true"
                      className="border-t border-warm-100 pt-3 first:border-0 first:pt-0"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-medium text-ink-900">
                          {label}
                        </p>
                        <Chip size="sm" variant="flat" color="warning">
                          {t("staleBadge")}
                        </Chip>
                      </div>
                      <p className="text-sm text-ink-500">
                        {formatRange(
                          item.starts_at,
                          item.ends_at,
                          locale,
                          businessTimezone,
                        )}
                      </p>
                      <p className="mt-1 text-xs text-ink-500">
                        {t("expiredActionHidden")}
                      </p>
                    </li>
                  );
                })}
              </ul>
            </div>
          ) : null}
        </>
      )}

      {/* Coverage history (Slice 5c) — resolved swaps/give-ups/claims, newest
          first. Collapsed by default (progressive disclosure); self-hides when
          there's nothing resolved yet or the read 403s for a non-approver. */}
      {historyItems.length > 0 ? (
        <div className="border-t border-warm-200 p-4">
          <button
            type="button"
            onClick={() => setHistoryOpen((v) => !v)}
            aria-expanded={historyOpen}
            className="flex w-full items-center justify-between gap-2 text-left"
          >
            <span className="flex items-center gap-2">
              <History className="h-4 w-4 text-ink-500" aria-hidden="true" />
              <span className="text-sm font-semibold text-ink-900">
                {t("historyTitle")}
              </span>
              <Chip size="sm" variant="flat">
                {historyItems.length}
              </Chip>
            </span>
            <ChevronDown
              className={`h-4 w-4 text-ink-400 transition-transform ${historyOpen ? "rotate-180" : ""}`}
              aria-hidden="true"
            />
          </button>

          {historyOpen ? (
            <ul className="mt-3 space-y-3">
              {historyItems.map((item) => (
                <li
                  key={`hist-${item.kind}-${item.request_id}`}
                  className="flex flex-wrap items-start justify-between gap-3 border-t border-warm-100 pt-3 first:border-0 first:pt-0"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-ink-900">
                      {staffName(item.requester_staff_id)}
                    </p>
                    <p className="text-sm text-ink-500">
                      {rowLabel(
                        item.kind,
                        item.position_id,
                        staffName(item.requester_staff_id),
                      )}
                    </p>
                    <p className="text-sm text-ink-500">
                      {formatRange(item.starts_at, item.ends_at, locale, businessTimezone)}
                    </p>
                    {item.decider_staff_id ? (
                      <p className="mt-0.5 text-xs text-ink-500">
                        {t("historyDecidedBy").replace(
                          "{name}",
                          staffName(item.decider_staff_id),
                        )}
                      </p>
                    ) : null}
                  </div>
                  <Chip
                    size="sm"
                    variant="flat"
                    color={historyStatusColor(item.status)}
                  >
                    {t(`historyStatus.${item.status}`)}
                  </Chip>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

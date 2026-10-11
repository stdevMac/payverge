"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  useDisclosure,
} from "@nextui-org/react";
import {
  AlertTriangle,
  CalendarClock,
  Check,
  ClipboardCheck,
  Pencil,
  Plus,
  X,
} from "lucide-react";
import { timeclockApi, type TimeEntry } from "@/api/timeclock";
import { laborCostApi } from "@/api/laborCost";
import { getBusinessStaff, type StaffMember } from "@/api/staff";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useToast } from "@/contexts/ToastContext";
import { useStaffRealtime } from "@/hooks/useStaffRealtime";
import { EmptyState } from "@/components/ui/EmptyState";
import { SkeletonLine } from "@/components/ui/skeletons";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  formatBusinessDateTime,
  resolveBusinessTimeZone,
} from "@/utils/businessTime";
import { instantToWallTime, wallTimeToInstant } from "@/utils/zonedDateTime";

interface TimesheetReviewProps {
  businessId: string;
  businessTimezone?: string | null;
  /**
   * Whether the caller may see labor dollars. Owners and managers with
   * financial:read pass true; everyone else gets hours-only. Even when true the
   * server independently strips $ from the labor-actuals response, so this is a
   * UI affordance, not the security boundary.
   */
  canViewFinancials: boolean;
  /** Business reporting currency (ISO code) for any labor-$ formatting. */
  currency: string;
  /**
   * When embedded inside the Schedule "Needs your approval" inbox, the panel
   * drops its own card chrome + header (the inbox supplies one), its empty
   * renders compact, and the manual-entry form moves into a modal triggered by
   * a button. Standalone (default) keeps the full section with an inline form.
   */
  embedded?: boolean;
  /** Reports the pending-review count for the inbox badge. */
  onPendingCountChange?: (count: number) => void;
}

function formatRange(
  startIso: string,
  endIso: string | null,
  locale: string,
  timeZone: string | null,
): string {
  const opts = {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  } as const;
  const start = formatBusinessDateTime(startIso, locale, timeZone, opts);
  const end = endIso
    ? formatBusinessDateTime(endIso, locale, timeZone, opts)
    : "—";
  return `${start} – ${end}`;
}

export default function TimesheetReview({
  businessId,
  businessTimezone = null,
  canViewFinancials,
  currency,
  embedded = false,
  onPendingCountChange,
}: TimesheetReviewProps) {
  const { locale } = useSimpleLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const manualModal = useDisclosure();

  const t = useCallback(
    (key: string): string => {
      const v = getTranslation(`dashboardTimesheets.${key}`, locale);
      return Array.isArray(v) ? v[0] || key : (v as string);
    },
    [locale],
  );

  const fmtMoney = useCallback(
    (value: number): string =>
      new Intl.NumberFormat(intlLocaleFor(locale), {
        style: "currency",
        currency: (currency || "USD").toUpperCase(),
      }).format(value),
    [locale, currency],
  );

  const fmtWorked = useCallback(
    (minutes: number): string => {
      const safe = Math.max(0, minutes);
      const h = Math.floor(safe / 60);
      const m = safe % 60;
      const parts: string[] = [];
      if (h > 0) parts.push(`${h}${t("hoursUnit")}`);
      if (m > 0 || h === 0) parts.push(`${m}${t("minutesUnit")}`);
      return parts.join(" ");
    },
    [t],
  );

  // Server pagination with an honest total (the queue used to fetch everything up
  // to the silent 500 cap with a frozen badge).
  const PAGE_SIZE = 20;
  const [page, setPage] = useState(0);
  const reviewKey = [
    ...queryKeys.timesheet.review(businessId, "pending_review"),
    "page",
    page,
  ] as const;
  const query = useQuery({
    queryKey: reviewKey,
    queryFn: () =>
      timeclockApi.listForReviewPaged(businessId, {
        status: "pending_review",
        offset: page * PAGE_SIZE,
        limit: PAGE_SIZE,
      }),
  });

  const staffQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => getBusinessStaff(businessId),
    staleTime: 5 * 60 * 1000,
  });

  // Actual-basis labor is fetched only for financial callers; the server gates
  // every $ field regardless, so non-financial managers never receive amounts.
  const laborQuery = useQuery({
    queryKey: ["labor-actual", businessId, "week"],
    queryFn: () => laborCostApi.getActual(businessId, "week"),
    enabled: canViewFinancials,
    staleTime: 60 * 1000,
    retry: false,
  });

  const invalidate = useCallback(() => {
    // Prefix-invalidate every page of the review query (any page may have changed).
    void queryClient.invalidateQueries({
      queryKey: queryKeys.timesheet.review(businessId, "pending_review"),
    });
    void queryClient.invalidateQueries({
      queryKey: ["labor-actual", businessId, "week"],
    });
  }, [queryClient, businessId]);

  const staffById = useMemo(() => {
    const map = new Map<number, StaffMember>();
    (staffQuery.data?.staff ?? []).forEach((s) => map.set(s.id, s));
    return map;
  }, [staffQuery.data]);

  const staffName = useCallback(
    (id: number) =>
      staffById.get(id)?.name || t("staffFallback").replace("{id}", String(id)),
    [staffById, t],
  );

  // Live freshness: a clock-out or a correction elsewhere refetches the queue so
  // the timesheet inbox matches the SSE-live time-off half (§3.6 one-inbox fix).
  useStaffRealtime({
    businessId: Number(businessId),
    onTimeclockEntry: () => invalidate(),
    onReconnect: () => invalidate(),
  });

  const approveMutation = useMutation({
    mutationFn: (entryId: number) => timeclockApi.approve(businessId, entryId),
    onSuccess: () => {
      invalidate();
      toast.showSuccess(t("approveSuccess"));
    },
    onError: () => toast.showError(t("approveError")),
  });

  // --- Reject (with confirm) ---
  const [pendingReject, setPendingReject] = useState<TimeEntry | null>(null);
  const [rejectReason, setRejectReason] = useState("");
  const rejectMutation = useMutation({
    mutationFn: (vars: { entryId: number; reason: string }) =>
      timeclockApi.reject(businessId, vars.entryId, vars.reason || undefined),
    onSuccess: () => {
      invalidate();
      toast.showSuccess(t("rejectSuccess"));
    },
    onError: () => toast.showError(t("rejectError")),
  });

  // --- Edit (manager correction of a mispunch) ---
  const editModal = useDisclosure();
  const [editEntry, setEditEntry] = useState<TimeEntry | null>(null);
  const emptyEdit = { clockIn: "", clockOut: "", breakMinutes: "0", note: "" };
  const [editForm, setEditForm] = useState(emptyEdit);
  const [editError, setEditError] = useState<string | null>(null);
  const editMutation = useMutation({
    mutationFn: (vars: {
      entryId: number;
      clock_in_at: string;
      clock_out_at: string;
      break_minutes: number;
      note?: string;
    }) =>
      timeclockApi.edit(businessId, vars.entryId, {
        clock_in_at: vars.clock_in_at,
        clock_out_at: vars.clock_out_at,
        break_minutes: vars.break_minutes,
        note: vars.note,
      }),
    onSuccess: () => {
      invalidate();
      setEditEntry(null);
      setEditError(null);
      editModal.onClose();
      toast.showSuccess(t("editSuccess"));
    },
    onError: () => setEditError(t("editError")),
  });

  // datetime-local holds a "YYYY-MM-DDTHH:mm" wall time. The list renders
  // punches in the venue zone, so the editor and the manual form read and
  // write that same zone; using the device clock let an owner abroad see
  // 17:00 in the row but 02:00 the next day in the editor.
  const venueTimeZone = resolveBusinessTimeZone(businessTimezone);
  const toLocalInput = useCallback(
    (iso: string | null): string => {
      if (!iso) return "";
      try {
        return instantToWallTime(iso, venueTimeZone);
      } catch {
        return "";
      }
    },
    [venueTimeZone],
  );
  // null when the wall time is malformed or falls in a DST gap.
  const fromLocalInput = useCallback(
    (wall: string): Date | null => {
      try {
        return wallTimeToInstant(wall.slice(0, 16), venueTimeZone);
      } catch {
        return null;
      }
    },
    [venueTimeZone],
  );

  const beginEdit = useCallback(
    (entry: TimeEntry) => {
      setEditEntry(entry);
      setEditForm({
        clockIn: toLocalInput(entry.clock_in_at),
        clockOut: toLocalInput(entry.clock_out_at),
        breakMinutes: String(entry.break_minutes),
        note: entry.note || "",
      });
      setEditError(null);
      editModal.onOpen();
    },
    [toLocalInput, editModal],
  );

  const submitEdit = useCallback(() => {
    if (!editEntry || !editForm.clockIn || !editForm.clockOut) return;
    const clockIn = fromLocalInput(editForm.clockIn);
    const clockOut = fromLocalInput(editForm.clockOut);
    if (!clockIn || !clockOut) {
      setEditError(t("editError"));
      return;
    }
    if (clockOut.getTime() <= clockIn.getTime()) {
      setEditError(t("manualInvalidRange"));
      return;
    }
    setEditError(null);
    editMutation.mutate({
      entryId: editEntry.id,
      clock_in_at: clockIn.toISOString(),
      clock_out_at: clockOut.toISOString(),
      break_minutes: Math.max(0, Number(editForm.breakMinutes) || 0),
      note: editForm.note.trim() || undefined,
    });
  }, [editEntry, editForm, editMutation, fromLocalInput, t]);

  // --- Manual entry ---
  const emptyForm = {
    staffId: "",
    clockIn: "",
    clockOut: "",
    breakMinutes: "0",
    note: "",
  };
  const [form, setForm] = useState(emptyForm);
  const [formError, setFormError] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: (input: {
      staff_id: number;
      clock_in_at: string;
      clock_out_at: string;
      break_minutes?: number;
      note?: string;
    }) => timeclockApi.createManual(businessId, input),
    onSuccess: () => {
      invalidate();
      setForm(emptyForm);
      setFormError(null);
      manualModal.onClose();
      toast.showSuccess(t("manualSuccess"));
    },
    onError: () => setFormError(t("manualError")),
  });

  const submitManual = useCallback(() => {
    const staffId = Number(form.staffId);
    if (!staffId || !form.clockIn || !form.clockOut) return;
    const clockIn = fromLocalInput(form.clockIn);
    const clockOut = fromLocalInput(form.clockOut);
    if (!clockIn || !clockOut) {
      setFormError(t("manualError"));
      return;
    }
    if (clockOut.getTime() <= clockIn.getTime()) {
      setFormError(t("manualInvalidRange"));
      return;
    }
    setFormError(null);
    createMutation.mutate({
      staff_id: staffId,
      clock_in_at: clockIn.toISOString(),
      clock_out_at: clockOut.toISOString(),
      break_minutes: Math.max(0, Number(form.breakMinutes) || 0),
      note: form.note.trim() || undefined,
    });
  }, [form, createMutation, fromLocalInput, t]);

  const entries: TimeEntry[] = query.data?.data ?? [];
  const total = query.data?.total ?? entries.length;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const staffOptions = staffQuery.data?.staff ?? [];

  // Surface the TRUE total (not the page length) to the inbox badge.
  useEffect(() => {
    onPendingCountChange?.(total);
  }, [total, onPendingCountChange]);

  const manualDisabled =
    !form.staffId ||
    !form.clockIn ||
    !form.clockOut ||
    createMutation.isPending;

  // The same set of inputs renders inline (standalone) or inside the modal
  // (embedded inbox), so it lives in one place.
  const manualFields = (
    <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <Select
          label={t("manualStaff")}
          placeholder={t("manualStaffPlaceholder")}
          selectedKeys={form.staffId ? [form.staffId] : []}
          onChange={(e) => setForm((f) => ({ ...f, staffId: e.target.value }))}
          labelPlacement="outside"
          variant="bordered"
          radius="lg"
          classNames={{ label: "text-sm font-medium text-ink-700" }}
        >
          {staffOptions.map((s) => (
            <SelectItem key={String(s.id)} textValue={s.name}>
              {s.name}
            </SelectItem>
          ))}
        </Select>
      </div>

      <Input
        type="datetime-local"
        label={t("manualClockIn")}
        aria-label={t("manualClockIn")}
        value={form.clockIn}
        onValueChange={(v) => setForm((f) => ({ ...f, clockIn: v }))}
        labelPlacement="outside"
        variant="bordered"
        radius="lg"
        startContent={
          <CalendarClock
            className="h-4 w-4 shrink-0 text-ink-400"
            aria-hidden
          />
        }
        classNames={{ label: "text-sm font-medium text-ink-700" }}
      />

      <Input
        type="datetime-local"
        label={t("manualClockOut")}
        aria-label={t("manualClockOut")}
        value={form.clockOut}
        onValueChange={(v) => setForm((f) => ({ ...f, clockOut: v }))}
        labelPlacement="outside"
        variant="bordered"
        radius="lg"
        startContent={
          <CalendarClock
            className="h-4 w-4 shrink-0 text-ink-400"
            aria-hidden
          />
        }
        classNames={{ label: "text-sm font-medium text-ink-700" }}
      />

      <Input
        type="number"
        min={0}
        label={t("manualBreak")}
        value={form.breakMinutes}
        onValueChange={(v) => setForm((f) => ({ ...f, breakMinutes: v }))}
        labelPlacement="outside"
        variant="bordered"
        radius="lg"
        classNames={{ label: "text-sm font-medium text-ink-700" }}
      />

      <Input
        label={t("manualNote")}
        value={form.note}
        onValueChange={(v) => setForm((f) => ({ ...f, note: v }))}
        labelPlacement="outside"
        variant="bordered"
        radius="lg"
        classNames={{ label: "text-sm font-medium text-ink-700" }}
      />
    </div>
  );

  return (
    <section
      aria-label={t("title")}
      className={embedded ? "" : "rounded-xl border border-warm-200"}
    >
      {!embedded ? (
        <div className="flex items-center justify-between gap-3 border-b border-warm-200 p-4">
          <div>
            <h2 className="font-title text-base text-ink-900">{t("title")}</h2>
            <p className="text-sm text-ink-500">{t("subtitle")}</p>
          </div>
          {total > 0 ? (
            <Chip size="sm" variant="flat" color="warning">
              {t("pendingBadge").replace("{count}", String(total))}
            </Chip>
          ) : null}
        </div>
      ) : null}

      {/* Labor-$ is shown only to financial callers, as a period summary (the
          honest granularity the actuals endpoint provides). Hours stay visible
          to everyone on each row below. */}
      {canViewFinancials && laborQuery.data?.has_data ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-warm-100 bg-warm-50 px-4 py-2 text-sm">
          {/* The actuals strip is pinned to the CURRENT week (the endpoint's
              period). Label it explicitly so it never reads as the whole queue. */}
          <span className="text-ink-500">{t("laborLabelThisWeek")}</span>
          <span className="font-medium text-ink-800">
            {Math.round((laborQuery.data.labor_cost_pct || 0) * 1000) / 10}%
          </span>
          {typeof laborQuery.data.labor_cost === "number" ? (
            <span className="font-medium text-ink-800">
              {fmtMoney(laborQuery.data.labor_cost)}
            </span>
          ) : null}
          {/* A bare "158.5%" reads as an alarm with no referent — say what it's
              a percentage OF. */}
          <span className="basis-full text-xs text-ink-500">
            {t("laborHint")}
          </span>
        </div>
      ) : null}

      <div className="p-4">
        {query.isLoading ? (
          <div
            role="status"
            aria-live="polite"
            aria-label={t("loading")}
            className="space-y-4"
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
        ) : query.isError ? (
          <p className="text-sm text-ink-500">{t("error")}</p>
        ) : total === 0 ? (
          <EmptyState
            icon={ClipboardCheck}
            title={t("emptyTitle")}
            subtitle={t("emptySubtitle")}
            compact={embedded}
          />
        ) : (
          <ul className="space-y-3">
            {entries.map((entry) => {
              const busy =
                approveMutation.isPending &&
                approveMutation.variables === entry.id;
              return (
                <li
                  key={entry.id}
                  className="flex flex-wrap items-start justify-between gap-3 border-t border-warm-100 pt-3 first:border-0 first:pt-0"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-ink-900">
                      {staffName(entry.staff_id)}
                    </p>
                    <p className="text-sm text-ink-500">
                      {formatRange(
                        entry.clock_in_at,
                        entry.clock_out_at,
                        locale,
                        businessTimezone,
                      )}
                    </p>
                    <p className="mt-0.5 text-xs text-ink-500">
                      {entry.shift_id
                        ? `${t("shiftLabel")} #${entry.shift_id}`
                        : t("noShift")}
                      {" · "}
                      <span className="font-medium text-ink-700">
                        {t("workedLabel")} {fmtWorked(entry.worked_minutes)}
                      </span>
                    </p>
                    {/* Anomaly flag: zero minutes or no matching shift usually
                        means a mispunch — worth a look before approving.
                        Informative only; approval stays one tap. */}
                    {entry.worked_minutes === 0 || entry.shift_id == null ? (
                      <span className="mt-1 inline-flex items-center gap-1 rounded-md bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-700">
                        <AlertTriangle
                          className="h-3 w-3"
                          aria-hidden="true"
                        />
                        {entry.worked_minutes === 0
                          ? t("flagZeroWorked")
                          : t("flagNoShift")}
                      </span>
                    ) : null}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <Button
                      size="sm"
                      variant="light"
                      startContent={<Pencil className="h-4 w-4" />}
                      isDisabled={
                        approveMutation.isPending || rejectMutation.isPending
                      }
                      onPress={() => beginEdit(entry)}
                    >
                      {t("edit")}
                    </Button>
                    <Button
                      size="sm"
                      color="danger"
                      variant="light"
                      startContent={<X className="h-4 w-4" />}
                      isDisabled={
                        approveMutation.isPending || rejectMutation.isPending
                      }
                      onPress={() => {
                        setRejectReason("");
                        setPendingReject(entry);
                      }}
                    >
                      {t("reject")}
                    </Button>
                    <Button
                      size="sm"
                      color="success"
                      variant="flat"
                      startContent={<Check className="h-4 w-4" />}
                      isLoading={busy}
                      isDisabled={
                        approveMutation.isPending || rejectMutation.isPending
                      }
                      onPress={() => approveMutation.mutate(entry.id)}
                    >
                      {t("approve")}
                    </Button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}

        {/* Honest pagination: "Showing X of N" + prev/next when the queue
            exceeds one page. */}
        {total > PAGE_SIZE ? (
          <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-warm-100 pt-3 text-sm">
            <span className="text-ink-500">
              {t("pageStatus")
                .replace("{from}", String(page * PAGE_SIZE + 1))
                .replace(
                  "{to}",
                  String(Math.min((page + 1) * PAGE_SIZE, total)),
                )
                .replace("{total}", String(total))}
            </span>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="flat"
                isDisabled={page === 0 || query.isFetching}
                onPress={() => setPage((p) => Math.max(0, p - 1))}
              >
                {t("pagePrev")}
              </Button>
              <Button
                size="sm"
                variant="flat"
                isDisabled={page >= totalPages - 1 || query.isFetching}
                onPress={() => setPage((p) => p + 1)}
              >
                {t("pageNext")}
              </Button>
            </div>
          </div>
        ) : null}
      </div>

      {/* Manual entry — for teammates who forgot to punch. Inline below the list
          when standalone; a button + modal when embedded in the inbox so the
          form doesn't sit always-open under the approvals queue. */}
      {!embedded ? (
        <div className="border-t border-warm-200 p-4">
          <h3 className="font-title text-sm text-ink-900">
            {t("manualTitle")}
          </h3>
          <p className="mt-0.5 text-xs text-ink-500">{t("manualHint")}</p>

          <div className="mt-3">{manualFields}</div>

          {formError ? (
            <p className="mt-2 text-sm text-red-600">{formError}</p>
          ) : null}

          <Button
            className="mt-3"
            color="primary"
            startContent={<Plus className="h-4 w-4" />}
            isLoading={createMutation.isPending}
            isDisabled={manualDisabled}
            onPress={submitManual}
          >
            {createMutation.isPending
              ? t("manualSubmitting")
              : t("manualSubmit")}
          </Button>
        </div>
      ) : (
        <div className="border-t border-warm-100 px-4 py-3">
          <Button
            size="sm"
            variant="flat"
            startContent={<Plus className="h-4 w-4" />}
            onPress={manualModal.onOpen}
          >
            {t("manualTitle")}
          </Button>
        </div>
      )}

      {/* Reject with an optional reason, behind a confirm (returns the entry to
          the staffer). */}
      <Modal
        isOpen={pendingReject != null}
        onOpenChange={() => setPendingReject(null)}
        size="md"
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5 text-danger" />
                {t("rejectConfirmTitle")}
              </ModalHeader>
              <ModalBody>
                <p className="text-sm text-ink-600">{t("rejectConfirmBody")}</p>
                <Input
                  label={t("rejectReasonLabel")}
                  placeholder={t("rejectReasonPlaceholder")}
                  value={rejectReason}
                  onValueChange={setRejectReason}
                  labelPlacement="outside"
                  variant="bordered"
                  radius="lg"
                  classNames={{ label: "text-sm font-medium text-ink-700" }}
                />
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("manualCancel")}
                </Button>
                <Button
                  color="danger"
                  isLoading={rejectMutation.isPending}
                  onPress={() => {
                    if (pendingReject) {
                      rejectMutation.mutate({
                        entryId: pendingReject.id,
                        reason: rejectReason.trim(),
                      });
                    }
                    setPendingReject(null);
                  }}
                >
                  {t("reject")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Manager correction: adjust clock-in/out/break/note on a non-approved
          entry. Original values are kept in the server audit trail. */}
      <Modal isOpen={editModal.isOpen} onOpenChange={editModal.onOpenChange} size="lg">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <span className="font-title text-base text-ink-900">
                  {t("editTitle")}
                </span>
                <span className="text-xs font-normal text-ink-500">
                  {t("editHint")}
                </span>
              </ModalHeader>
              <ModalBody>
                <div className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2">
                  <Input
                    type="datetime-local"
                    label={t("manualClockIn")}
                    aria-label={t("manualClockIn")}
                    value={editForm.clockIn}
                    onValueChange={(v) => setEditForm((f) => ({ ...f, clockIn: v }))}
                    labelPlacement="outside"
                    variant="bordered"
                    radius="lg"
                    classNames={{ label: "text-sm font-medium text-ink-700" }}
                  />
                  <Input
                    type="datetime-local"
                    label={t("manualClockOut")}
                    aria-label={t("manualClockOut")}
                    value={editForm.clockOut}
                    onValueChange={(v) => setEditForm((f) => ({ ...f, clockOut: v }))}
                    labelPlacement="outside"
                    variant="bordered"
                    radius="lg"
                    classNames={{ label: "text-sm font-medium text-ink-700" }}
                  />
                  <Input
                    type="number"
                    min={0}
                    label={t("manualBreak")}
                    value={editForm.breakMinutes}
                    onValueChange={(v) =>
                      setEditForm((f) => ({ ...f, breakMinutes: v }))
                    }
                    labelPlacement="outside"
                    variant="bordered"
                    radius="lg"
                    classNames={{ label: "text-sm font-medium text-ink-700" }}
                  />
                  <Input
                    label={t("manualNote")}
                    value={editForm.note}
                    onValueChange={(v) => setEditForm((f) => ({ ...f, note: v }))}
                    labelPlacement="outside"
                    variant="bordered"
                    radius="lg"
                    classNames={{ label: "text-sm font-medium text-ink-700" }}
                  />
                </div>
                {editError ? (
                  <p className="mt-1 text-sm text-red-600">{editError}</p>
                ) : null}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("manualCancel")}
                </Button>
                <Button
                  color="primary"
                  isLoading={editMutation.isPending}
                  isDisabled={!editForm.clockIn || !editForm.clockOut || editMutation.isPending}
                  onPress={submitEdit}
                >
                  {t("editSubmit")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {embedded ? (
        <Modal
          isOpen={manualModal.isOpen}
          onOpenChange={manualModal.onOpenChange}
          size="lg"
        >
          <ModalContent>
            {(onClose) => (
              <>
                <ModalHeader className="flex flex-col gap-1">
                  <span className="font-title text-base text-ink-900">
                    {t("manualTitle")}
                  </span>
                  <span className="text-xs font-normal text-ink-500">
                    {t("manualHint")}
                  </span>
                </ModalHeader>
                <ModalBody>
                  {manualFields}
                  {formError ? (
                    <p className="mt-1 text-sm text-red-600">{formError}</p>
                  ) : null}
                </ModalBody>
                <ModalFooter>
                  <Button variant="light" onPress={onClose}>
                    {t("manualCancel")}
                  </Button>
                  <Button
                    color="primary"
                    startContent={<Plus className="h-4 w-4" />}
                    isLoading={createMutation.isPending}
                    isDisabled={manualDisabled}
                    onPress={submitManual}
                  >
                    {createMutation.isPending
                      ? t("manualSubmitting")
                      : t("manualSubmit")}
                  </Button>
                </ModalFooter>
              </>
            )}
          </ModalContent>
        </Modal>
      ) : null}
    </section>
  );
}

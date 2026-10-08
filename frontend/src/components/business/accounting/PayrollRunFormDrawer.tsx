"use client";

import React, {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { Button, Input, Textarea } from "@nextui-org/react";
import { useQuery } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import type { Locale } from "@/i18n/localeRegistry";
import type { CreatePayrollLineItemRequest, CreatePayrollRunRequest } from "@/api/accounting";
import { getBusinessStaff, type StaffMember } from "@/api/staff";
import { queryKeys } from "@/api/queryKeys";
import {
  useCreatePayrollRun,
  useMarkPayrollRunPaid,
} from "@/hooks/accounting/useAccountingQueries";
import { asDollars } from "@/types/money";
import { StaffSelect } from "@/components/ui/fields/StaffSelect";
import {
  formatDay,
  formatMoney,
} from "@/components/business/accounting/format";
import DetailDrawer from "../shared/DetailDrawer";
import StatusBadge, { type StatusTone } from "./StatusBadge";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
  formatDateInput,
} from "./accountingShared";

export type PayrollRunFormDrawerProps = {
  open: boolean;
  onClose: () => void;
  businessId: string;
  currency: string;
  locale: Locale;
  /**
   * Period end of the most recent known payroll run (YYYY-MM-DD).
   * When set, the form defaults to the next 7-day window starting the day after.
   * When null/omitted, defaults to the current Mon–Sun week.
   */
  lastPeriodEnd?: string | null;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
  /** Called after a successful create (and mark-paid when chained). */
  onSuccess?: () => void;
};

type FormStep = 1 | 2 | 3;

type FormLine = {
  /** Stable local key for React lists. */
  key: string;
  payee_type: "staff" | "contractor";
  staff_id: number | null;
  payee_name: string;
  gross: string;
  bonus: string;
  deduction: string;
};

function mutationErrorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === "object") {
    const maybeAxios = err as {
      response?: { data?: { error?: string; message?: string } };
      message?: string;
    };
    const data = maybeAxios.response?.data;
    if (data?.error && typeof data.error === "string") return data.error;
    if (data?.message && typeof data.message === "string") return data.message;
    if (typeof maybeAxios.message === "string" && maybeAxios.message) {
      return maybeAxios.message;
    }
  }
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

function parseDay(value: string): Date | null {
  const m = (value || "").slice(0, 10).match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!m) return null;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
}

/**
 * Default payroll period for the create form.
 * - With a prior run end: next day → next day + 6 (7-day window).
 * - Otherwise: current calendar week Monday–Sunday (local).
 */
export function defaultPayrollPeriod(
  lastPeriodEnd?: string | null,
  now: Date = new Date(),
): { start: string; end: string } {
  if (lastPeriodEnd) {
    const end = parseDay(lastPeriodEnd);
    if (end) {
      end.setDate(end.getDate() + 1);
      const startStr = formatDateInput(end);
      end.setDate(end.getDate() + 6);
      return { start: startStr, end: formatDateInput(end) };
    }
  }
  const day = now.getDay(); // 0 = Sunday
  const mondayOffset = day === 0 ? -6 : 1 - day;
  const mon = new Date(
    now.getFullYear(),
    now.getMonth(),
    now.getDate() + mondayOffset,
  );
  const sun = new Date(mon.getFullYear(), mon.getMonth(), mon.getDate() + 6);
  return { start: formatDateInput(mon), end: formatDateInput(sun) };
}

function lineNet(line: FormLine): number {
  const gross = Number(line.gross) || 0;
  const bonus = Number(line.bonus) || 0;
  const deduction = Number(line.deduction) || 0;
  return gross + bonus - deduction;
}

function staffToLines(staff: StaffMember[], idPrefix: string): FormLine[] {
  return staff.map((s, i) => ({
    key: `${idPrefix}-staff-${s.id}-${i}`,
    payee_type: "staff" as const,
    staff_id: s.id,
    payee_name: s.name,
    gross: "",
    bonus: "0",
    deduction: "0",
  }));
}

function emptyContractorLine(key: string): FormLine {
  return {
    key,
    payee_type: "contractor",
    staff_id: null,
    payee_name: "",
    gross: "",
    bonus: "0",
    deduction: "0",
  };
}

function emptyStaffLine(key: string): FormLine {
  return {
    key,
    payee_type: "staff",
    staff_id: null,
    payee_name: "",
    gross: "",
    bonus: "0",
    deduction: "0",
  };
}

function stepTone(step: FormStep, current: FormStep): StatusTone {
  if (step < current) return "success";
  if (step === current) return "info";
  return "neutral";
}

export default function PayrollRunFormDrawer({
  open,
  onClose,
  businessId,
  currency,
  locale,
  lastPeriodEnd = null,
  t,
  onSuccess,
}: PayrollRunFormDrawerProps) {
  const createMutation = useCreatePayrollRun(businessId);
  const markPaidMutation = useMarkPayrollRunPaid(businessId);
  const lineKeySeed = useId();

  const staffQuery = useQuery({
    queryKey: queryKeys.staff.list(businessId),
    queryFn: () => getBusinessStaff(businessId),
    enabled: open && !!businessId,
    staleTime: 5 * 60 * 1000,
  });

  const roster = useMemo(
    () => staffQuery.data?.staff ?? [],
    [staffQuery.data?.staff],
  );

  const [step, setStep] = useState<FormStep>(1);
  const [periodStart, setPeriodStart] = useState("");
  const [periodEnd, setPeriodEnd] = useState("");
  const [notes, setNotes] = useState("");
  const [lines, setLines] = useState<FormLine[]>([]);
  const [periodError, setPeriodError] = useState<string | null>(null);
  const [linesError, setLinesError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  const [lineKeyCounter, setLineKeyCounter] = useState(0);
  const rosterRef = useRef(roster);
  const stepRef = useRef(step);
  rosterRef.current = roster;
  stepRef.current = step;

  // Reset form whenever the drawer opens.
  useEffect(() => {
    if (!open) return;
    const defaults = defaultPayrollPeriod(lastPeriodEnd);
    setStep(1);
    setPeriodStart(defaults.start);
    setPeriodEnd(defaults.end);
    setNotes("");
    setPeriodError(null);
    setLinesError(null);
    setServerError(null);
    setLineKeyCounter(0);
    // Prefill from currently cached roster; staff effect may refine once loaded.
    setLines(staffToLines(rosterRef.current, lineKeySeed));
  }, [open, lastPeriodEnd, lineKeySeed]);

  // When roster arrives (or refreshes while open on step ≤ 2 with empty/unedited defaults),
  // seed one line per active staff member.
  useEffect(() => {
    if (!open) return;
    if (staffQuery.isLoading) return;
    if (stepRef.current !== 1 && stepRef.current !== 2) return;
    setLines((currentLines) => {
      // Only reseed while every row is an untouched staff default. Functional
      // state gives this roster-driven effect the freshest edits without
      // putting `lines` in the dependency list and creating a reseed loop.
      const blank =
        currentLines.length === 0 ||
        currentLines.every(
          (line) =>
            line.payee_type === "staff" &&
            !line.gross &&
            (line.bonus === "0" || !line.bonus) &&
            (line.deduction === "0" || !line.deduction),
        );
      if (!blank && currentLines.length > 0) return currentLines;

      const nextLines = staffToLines(roster, lineKeySeed);
      const unchanged =
        currentLines.length === nextLines.length &&
        currentLines.every((line, index) => {
          const nextLine = nextLines[index];
          return (
            nextLine !== undefined &&
            line.key === nextLine.key &&
            line.staff_id === nextLine.staff_id &&
            line.payee_name === nextLine.payee_name
          );
        });
      return unchanged ? currentLines : nextLines;
    });
  }, [open, staffQuery.isLoading, roster, lineKeySeed]);

  const isPending = createMutation.isPending || markPaidMutation.isPending;

  const eligibleLines = useMemo(
    () =>
      lines.filter((l) => {
        const g = Number(l.gross);
        return Number.isFinite(g) && g > 0;
      }),
    [lines],
  );

  const reviewTotals = useMemo(() => {
    let gross = 0;
    let bonus = 0;
    let deduction = 0;
    eligibleLines.forEach((l) => {
      gross += Number(l.gross) || 0;
      bonus += Number(l.bonus) || 0;
      deduction += Number(l.deduction) || 0;
    });
    return {
      gross,
      bonus,
      deduction,
      net: gross + bonus - deduction,
      count: eligibleLines.length,
    };
  }, [eligibleLines]);

  const validatePeriod = useCallback((): string | null => {
    if (!periodStart || !periodEnd) {
      return t("payrollForm.errors.periodInvalid");
    }
    if (periodEnd < periodStart) {
      return t("payrollForm.errors.periodInvalid");
    }
    return null;
  }, [periodStart, periodEnd, t]);

  const validateLines = useCallback((): string | null => {
    if (eligibleLines.length === 0) {
      return t("payrollForm.errors.linesRequired");
    }
    for (const line of eligibleLines) {
      if (line.payee_type === "staff" && (line.staff_id == null || line.staff_id <= 0)) {
        return t("payrollForm.errors.staffRequired");
      }
      if (line.payee_type === "contractor" && !line.payee_name.trim()) {
        return t("payrollForm.errors.staffRequired");
      }
      const g = Number(line.gross);
      if (!Number.isFinite(g) || g <= 0) {
        return t("payrollForm.errors.grossRequired");
      }
    }
    return null;
  }, [eligibleLines, t]);

  const goNext = useCallback(() => {
    setServerError(null);
    if (step === 1) {
      const err = validatePeriod();
      setPeriodError(err);
      if (err) return;
      setStep(2);
      return;
    }
    if (step === 2) {
      const err = validateLines();
      setLinesError(err);
      if (err) return;
      setStep(3);
    }
  }, [step, validatePeriod, validateLines]);

  const goBack = useCallback(() => {
    setServerError(null);
    setPeriodError(null);
    setLinesError(null);
    setStep((s) => (s === 3 ? 2 : 1));
  }, []);

  const updateLine = useCallback(
    (key: string, patch: Partial<FormLine>) => {
      setLines((prev) =>
        prev.map((l) => (l.key === key ? { ...l, ...patch } : l)),
      );
      setLinesError(null);
    },
    [],
  );

  const removeLine = useCallback((key: string) => {
    setLines((prev) => prev.filter((l) => l.key !== key));
    setLinesError(null);
  }, []);

  const addContractor = useCallback(() => {
    setLineKeyCounter((n) => {
      const next = n + 1;
      setLines((prev) => [
        ...prev,
        emptyContractorLine(`${lineKeySeed}-c-${next}`),
      ]);
      return next;
    });
    setLinesError(null);
  }, [lineKeySeed]);

  const addStaffLine = useCallback(() => {
    setLineKeyCounter((n) => {
      const next = n + 1;
      setLines((prev) => [
        ...prev,
        emptyStaffLine(`${lineKeySeed}-s-${next}`),
      ]);
      return next;
    });
    setLinesError(null);
  }, [lineKeySeed]);

  const buildPayload = useCallback((): CreatePayrollRunRequest => {
    const line_items: CreatePayrollLineItemRequest[] = eligibleLines.map(
      (l) => {
        const base: CreatePayrollLineItemRequest = {
          payee_type: l.payee_type,
          gross_amount: asDollars(Number(l.gross)),
          bonus_amount: asDollars(Number(l.bonus) || 0),
          deduction_amount: asDollars(Number(l.deduction) || 0),
        };
        if (l.payee_type === "staff" && l.staff_id != null) {
          base.staff_id = l.staff_id;
          const name =
            l.payee_name.trim() ||
            roster.find((s) => s.id === l.staff_id)?.name ||
            undefined;
          if (name) base.payee_name = name;
        } else {
          base.payee_name = l.payee_name.trim();
        }
        return base;
      },
    );
    return {
      period_start: periodStart,
      period_end: periodEnd,
      notes: notes.trim() || undefined,
      line_items,
    };
  }, [eligibleLines, periodStart, periodEnd, notes, roster]);

  const handleCreate = useCallback(
    (andMarkPaid: boolean) => {
      setServerError(null);
      const periodErr = validatePeriod();
      if (periodErr) {
        setPeriodError(periodErr);
        setStep(1);
        return;
      }
      const linesErr = validateLines();
      if (linesErr) {
        setLinesError(linesErr);
        setStep(2);
        return;
      }

      const payload = buildPayload();
      createMutation.mutate(payload, {
        onSuccess: (run) => {
          if (!andMarkPaid) {
            onSuccess?.();
            onClose();
            return;
          }
          markPaidMutation.mutate(run.id, {
            onSuccess: () => {
              onSuccess?.();
              onClose();
            },
            onError: (err) => {
              setServerError(
                mutationErrorMessage(err, t("errors.markPaid")),
              );
            },
          });
        },
        onError: (err) => {
          setServerError(mutationErrorMessage(err, t("errors.savePayroll")));
        },
      });
    },
    [
      validatePeriod,
      validateLines,
      buildPayload,
      createMutation,
      markPaidMutation,
      onSuccess,
      onClose,
      t,
    ],
  );

  const inputWrapperClass =
    "bg-white border-warm-200 data-[hover=true]:border-brand/40";

  const stepHeader = (
    <div
      className="mb-5 flex flex-wrap items-center gap-2"
      data-testid="payroll-form-steps"
      aria-label={t("payrollForm.title")}
    >
      <StatusBadge
        tone={stepTone(1, step)}
        label={`1. ${t("payrollForm.stepPeriod")}`}
        size="sm"
      />
      <StatusBadge
        tone={stepTone(2, step)}
        label={`2. ${t("payrollForm.stepLines")}`}
        size="sm"
      />
      <StatusBadge
        tone={stepTone(3, step)}
        label={`3. ${t("payrollForm.stepReview")}`}
        size="sm"
      />
    </div>
  );

  const footer = (
    <div className="flex w-full flex-wrap items-center justify-between gap-2">
      <div>
        {step > 1 ? (
          <button
            type="button"
            className={accountingSecondaryButtonClass}
            onClick={goBack}
            disabled={isPending}
          >
            {t("payrollForm.back")}
          </button>
        ) : (
          <button
            type="button"
            className={accountingSecondaryButtonClass}
            onClick={onClose}
            disabled={isPending}
          >
            {t("entryForm.cancel")}
          </button>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {step < 3 ? (
          <Button
            type="button"
            className={accountingPrimaryButtonClass}
            onPress={goNext}
            isDisabled={isPending}
          >
            {t("payrollForm.next")}
          </Button>
        ) : (
          <>
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={() => handleCreate(false)}
              disabled={isPending}
            >
              {t("payrollForm.createDraft")}
            </button>
            <Button
              type="button"
              className={accountingPrimaryButtonClass}
              onPress={() => handleCreate(true)}
              isLoading={isPending}
              isDisabled={isPending}
            >
              {t("payrollForm.createAndMarkPaid")}
            </Button>
          </>
        )}
      </div>
    </div>
  );

  // L6-18: multi-step wizard — any progress past step 1 or notes/line edits is dirty.
  const dirty =
    step > 1 ||
    notes.trim() !== "" ||
    lines.some(
      (line) =>
        (line.gross && line.gross !== "0") ||
        (line.bonus && line.bonus !== "0") ||
        (line.deduction && line.deduction !== "0"),
    );

  return (
    <DetailDrawer
      open={open}
      onClose={onClose}
      title={t("payrollForm.title")}
      size="lg"
      footer={footer}
      dirty={dirty}
      discardConfirm={{
        title: t("discardConfirm.payroll.title"),
        description: t("discardConfirm.payroll.description"),
        confirmLabel: t("discardConfirm.confirm"),
        cancelLabel: t("discardConfirm.cancel"),
      }}
    >
      <div className="space-y-4">
        {stepHeader}

        {serverError ? (
          <div
            role="alert"
            className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700"
          >
            {serverError}
          </div>
        ) : null}

        {step === 1 ? (
          <div className="space-y-4" data-testid="payroll-form-step-period">
            {periodError ? (
              <div
                role="alert"
                className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700"
              >
                {periodError}
              </div>
            ) : null}
            <div className="grid gap-4 sm:grid-cols-2">
              <Input
                type="date"
                label={t("payrollForm.periodStart")}
                aria-label={t("payrollForm.periodStart")}
                labelPlacement="outside"
          placeholder=" "
                variant="bordered"
                value={periodStart}
                onValueChange={(v) => {
                  setPeriodStart(v);
                  setPeriodError(null);
                }}
                onChange={(e) => {
                  setPeriodStart(e.target.value);
                  setPeriodError(null);
                }}
                isInvalid={Boolean(periodError)}
                classNames={{ inputWrapper: inputWrapperClass }}
              />
              <Input
                type="date"
                label={t("payrollForm.periodEnd")}
                aria-label={t("payrollForm.periodEnd")}
                labelPlacement="outside"
          placeholder=" "
                variant="bordered"
                value={periodEnd}
                onValueChange={(v) => {
                  setPeriodEnd(v);
                  setPeriodError(null);
                }}
                onChange={(e) => {
                  setPeriodEnd(e.target.value);
                  setPeriodError(null);
                }}
                isInvalid={Boolean(periodError)}
                classNames={{ inputWrapper: inputWrapperClass }}
              />
            </div>
            <Textarea
              label={t("payrollForm.notes")}
              aria-label={t("payrollForm.notes")}
              labelPlacement="outside"
          placeholder=" "
              variant="bordered"
              minRows={2}
              value={notes}
              onValueChange={setNotes}
              onChange={(e) => setNotes(e.target.value)}
              classNames={{ inputWrapper: inputWrapperClass }}
            />
          </div>
        ) : null}

        {step === 2 ? (
          <div className="space-y-3" data-testid="payroll-form-step-lines">
            <p className="text-sm text-ink-600">{t("payrollForm.prefillHint")}</p>
            {linesError ? (
              <div
                role="alert"
                className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700"
              >
                {linesError}
              </div>
            ) : null}

            <div className="space-y-3">
              {lines.map((line, index) => (
                <div
                  key={line.key}
                  data-testid={`payroll-form-line-${index}`}
                  className="rounded-2xl border border-warm-200/80 bg-white p-3 shadow-sm shadow-warm-900/5 sm:p-4"
                >
                  <div className="mb-3 flex items-start justify-between gap-2">
                    <span className="text-xs font-semibold uppercase tracking-wide text-ink-500">
                      {line.payee_type === "staff"
                        ? t("payrollRuns.staff")
                        : t("payrollRuns.contractor")}
                    </span>
                    <button
                      type="button"
                      className="inline-flex h-8 items-center gap-1 rounded-lg px-2 text-xs font-medium text-rose-600 transition-colors hover:bg-rose-50 disabled:opacity-40"
                      aria-label={t("payrollForm.removeLine")}
                      onClick={() => removeLine(line.key)}
                      disabled={lines.length <= 1}
                    >
                      <Trash2 className="h-3.5 w-3.5" aria-hidden />
                      {t("payrollForm.removeLine")}
                    </button>
                  </div>

                  {line.payee_type === "staff" ? (
                    <div className="mb-3">
                      <StaffSelect
                        label={t("payrollRuns.staffMember")}
                        staff={roster}
                        value={line.staff_id}
                        onChange={(id) => {
                          const name =
                            roster.find((s) => s.id === id)?.name ?? "";
                          updateLine(line.key, {
                            staff_id: id,
                            payee_name: name,
                          });
                        }}
                      />
                    </div>
                  ) : (
                    <div className="mb-3">
                      <Input
                        label={t("payrollForm.contractorName")}
                        aria-label={t("payrollForm.contractorName")}
                        labelPlacement="outside"
          placeholder=" "
                        variant="bordered"
                        value={line.payee_name}
                        onValueChange={(v) =>
                          updateLine(line.key, { payee_name: v })
                        }
                        onChange={(e) =>
                          updateLine(line.key, { payee_name: e.target.value })
                        }
                        classNames={{ inputWrapper: inputWrapperClass }}
                      />
                    </div>
                  )}

                  <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                    <Input
                      type="number"
                      step="0.01"
                      min="0"
                      label={t("payrollForm.gross")}
                      aria-label={t("payrollForm.gross")}
                      labelPlacement="outside"
          placeholder=" "
                      variant="bordered"
                      value={line.gross}
                      onValueChange={(v) => updateLine(line.key, { gross: v })}
                      onChange={(e) =>
                        updateLine(line.key, { gross: e.target.value })
                      }
                      startContent={
                        <span className="pointer-events-none text-xs text-ink-500">
                          {currency}
                        </span>
                      }
                      classNames={{ inputWrapper: inputWrapperClass }}
                    />
                    <Input
                      type="number"
                      step="0.01"
                      min="0"
                      label={t("payrollForm.bonus")}
                      aria-label={t("payrollForm.bonus")}
                      labelPlacement="outside"
          placeholder=" "
                      variant="bordered"
                      value={line.bonus}
                      onValueChange={(v) => updateLine(line.key, { bonus: v })}
                      onChange={(e) =>
                        updateLine(line.key, { bonus: e.target.value })
                      }
                      startContent={
                        <span className="pointer-events-none text-xs text-ink-500">
                          {currency}
                        </span>
                      }
                      classNames={{ inputWrapper: inputWrapperClass }}
                    />
                    <Input
                      type="number"
                      step="0.01"
                      min="0"
                      label={t("payrollForm.deduction")}
                      aria-label={t("payrollForm.deduction")}
                      labelPlacement="outside"
          placeholder=" "
                      variant="bordered"
                      value={line.deduction}
                      onValueChange={(v) =>
                        updateLine(line.key, { deduction: v })
                      }
                      onChange={(e) =>
                        updateLine(line.key, { deduction: e.target.value })
                      }
                      startContent={
                        <span className="pointer-events-none text-xs text-ink-500">
                          {currency}
                        </span>
                      }
                      classNames={{ inputWrapper: inputWrapperClass }}
                    />
                    <div className="flex flex-col gap-1">
                      <span className="text-sm font-medium text-ink-700">
                        {t("payrollForm.net")}
                      </span>
                      <div
                        data-testid={`payroll-line-net-${index}`}
                        className="flex h-10 items-center rounded-xl border border-warm-200 bg-warm-50/80 px-3 text-sm font-semibold tabular-nums text-ink-900"
                      >
                        {lineNet(line)}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>

            <div className="flex flex-wrap gap-2 pt-1">
              <button
                type="button"
                className={accountingSecondaryButtonClass}
                onClick={addStaffLine}
              >
                <Plus className="h-4 w-4" aria-hidden />
                {t("payrollForm.addStaffLine")}
              </button>
              <button
                type="button"
                className={accountingSecondaryButtonClass}
                onClick={addContractor}
              >
                <Plus className="h-4 w-4" aria-hidden />
                {t("payrollForm.addContractorLine")}
              </button>
            </div>
          </div>
        ) : null}

        {step === 3 ? (
          <div className="space-y-4" data-testid="payroll-form-step-review">
            <div className="rounded-2xl border border-warm-200/80 bg-white px-4 py-3 text-sm text-ink-700 shadow-sm">
              <p>
                <span className="font-medium text-ink-500">
                  {t("payrollForm.periodStart")}:{" "}
                </span>
                {formatDay(periodStart, locale)}
              </p>
              <p className="mt-1">
                <span className="font-medium text-ink-500">
                  {t("payrollForm.periodEnd")}:{" "}
                </span>
                {formatDay(periodEnd, locale)}
              </p>
              {notes.trim() ? (
                <p className="mt-1">
                  <span className="font-medium text-ink-500">
                    {t("payrollForm.notes")}:{" "}
                  </span>
                  {notes.trim()}
                </p>
              ) : null}
            </div>

            <div className="overflow-x-auto rounded-2xl border border-warm-200/80 bg-white shadow-sm shadow-warm-900/5">
              <table className="min-w-full divide-y divide-warm-200/80 text-sm">
                <thead>
                  <tr className="bg-warm-50/70 text-left text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
                    <th className="px-3 py-2.5 sm:px-4">
                      {t("payrollDetail.lines.payee")}
                    </th>
                    <th className="px-3 py-2.5 text-right sm:px-4">
                      {t("payrollForm.gross")}
                    </th>
                    <th className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                      {t("payrollForm.bonus")}
                    </th>
                    <th className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                      {t("payrollForm.deduction")}
                    </th>
                    <th className="px-3 py-2.5 text-right sm:px-4">
                      {t("payrollForm.net")}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-warm-100">
                  {eligibleLines.map((line) => {
                    const name =
                      line.payee_name.trim() ||
                      (line.staff_id != null
                        ? roster.find((s) => s.id === line.staff_id)?.name
                        : null) ||
                      "—";
                    return (
                      <tr key={line.key}>
                        <td className="px-3 py-2.5 text-ink-900 sm:px-4">
                          {name}
                        </td>
                        <td className="px-3 py-2.5 text-right tabular-nums sm:px-4">
                          {formatMoney(Number(line.gross) || 0, currency, locale)}
                        </td>
                        <td className="hidden px-3 py-2.5 text-right tabular-nums sm:table-cell sm:px-4">
                          {formatMoney(Number(line.bonus) || 0, currency, locale)}
                        </td>
                        <td className="hidden px-3 py-2.5 text-right tabular-nums sm:table-cell sm:px-4">
                          {formatMoney(
                            Number(line.deduction) || 0,
                            currency,
                            locale,
                          )}
                        </td>
                        <td className="px-3 py-2.5 text-right tabular-nums sm:px-4">
                          {formatMoney(lineNet(line), currency, locale)}
                        </td>
                      </tr>
                    );
                  })}
                  <tr className="bg-warm-50/50 font-semibold">
                    <td className="px-3 py-2.5 text-ink-700 sm:px-4">
                      {t("payrollForm.totals")}
                    </td>
                    <td className="px-3 py-2.5 text-right tabular-nums sm:px-4">
                      {formatMoney(reviewTotals.gross, currency, locale)}
                    </td>
                    <td className="hidden px-3 py-2.5 text-right tabular-nums sm:table-cell sm:px-4">
                      {formatMoney(reviewTotals.bonus, currency, locale)}
                    </td>
                    <td className="hidden px-3 py-2.5 text-right tabular-nums sm:table-cell sm:px-4">
                      {formatMoney(reviewTotals.deduction, currency, locale)}
                    </td>
                    <td className="px-3 py-2.5 text-right tabular-nums sm:px-4">
                      {formatMoney(reviewTotals.net, currency, locale)}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
      </div>
    </DetailDrawer>
  );
}

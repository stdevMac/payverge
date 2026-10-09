"use client";

import React, { useCallback, useMemo, useState } from "react";
import type { Locale } from "@/i18n/localeRegistry";
import type {
  LedgerActorStaff,
  PayrollLineItem,
  PayrollRun,
} from "@/api/accounting";
import {
  useDeletePayrollRun,
  useMarkPayrollRunPaid,
  usePayrollRun,
  useVoidPayrollRun,
} from "@/hooks/accounting/useAccountingQueries";
import {
  formatDay,
  formatMoney,
  formatPeriod,
} from "@/components/business/accounting/format";
import { AnimatedNumberText } from "../premium";
import DetailDrawer from "../shared/DetailDrawer";
import ConfirmationModal from "../modals/ConfirmationModal";
import StatusBadge, { type StatusTone } from "./StatusBadge";
import { accountingPrimaryButtonClass } from "./accountingShared";

export type PayrollRunDetailDrawerProps = {
  open: boolean;
  runId: number | null;
  businessId: string;
  locale: Locale;
  currency: string;
  /** When false, hide Mark paid / Void / Delete. */
  canWrite?: boolean;
  onClose: () => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
};

type PendingAction = "markPaid" | "void" | "delete" | null;

function actorName(
  staff: LedgerActorStaff | null | undefined,
  fallbackUserId: number | null | undefined,
  ownerLabel: string,
): string {
  const name = staff?.name?.trim();
  if (name) return name;
  // Owner-performed actions carry a user id but no staff row — show "Owner"
  // instead of a dash.
  if (fallbackUserId != null) return ownerLabel;
  return "—";
}

function statusTone(status: string): StatusTone {
  if (status === "paid") return "success";
  if (status === "void") return "neutral";
  return "pending";
}

function statusLabel(status: string, t: (key: string) => string): string {
  if (status === "paid") return t("payroll.statusPaid");
  if (status === "void") return t("payroll.statusVoid");
  return t("payroll.statusDraft");
}

function linePayeeName(line: PayrollLineItem): string {
  const name = line.payee_name?.trim();
  if (name) return name;
  // Backend copies staff name into payee_name at create; fall back for older rows.
  return "—";
}

function amountCell(
  value: number,
  currency: string,
  locale: Locale,
): React.ReactNode {
  return (
    <span className="tabular-nums text-ink-900">
      {formatMoney(value, currency, locale)}
    </span>
  );
}

export default function PayrollRunDetailDrawer({
  open,
  runId,
  businessId,
  locale,
  currency,
  canWrite = true,
  onClose,
  t,
  tWith,
}: PayrollRunDetailDrawerProps) {
  const resolvedId = runId ?? 0;
  const { data: run, isLoading } = usePayrollRun(businessId, resolvedId, {
    enabled: open && resolvedId > 0,
  });

  const markPaidMutation = useMarkPayrollRunPaid(businessId);
  const voidMutation = useVoidPayrollRun(businessId);
  const deleteMutation = useDeletePayrollRun(businessId);

  const [pendingAction, setPendingAction] = useState<PendingAction>(null);

  const interpolate = useCallback(
    (key: string, replacements: Record<string, string | number>) => {
      if (tWith) return tWith(key, replacements);
      let value = t(key);
      Object.entries(replacements).forEach(([name, replacement]) => {
        value = value.replace(
          new RegExp(`\\{${name}\\}`, "g"),
          String(replacement),
        );
      });
      return value;
    },
    [t, tWith],
  );

  const isPending =
    markPaidMutation.isPending ||
    voidMutation.isPending ||
    deleteMutation.isPending;

  const periodLabel = useMemo(() => {
    if (!run) return "";
    return formatPeriod(run.period_start, run.period_end, locale);
  }, [run, locale]);

  const runCurrency = run?.currency || currency;
  const status = run?.status ?? "draft";

  const handleConfirm = useCallback(() => {
    if (!run || !pendingAction) return;
    const action = pendingAction;
    const id = run.id;
    setPendingAction(null);

    const callbacks = {
      onSuccess: () => onClose(),
      // Keep the destructive action available after a transient failure. The
      // shared API layer surfaces the error toast; restoring confirmation gives
      // the operator an explicit retry without closing or losing context.
      onError: () => setPendingAction(action),
    };
    if (action === "markPaid") {
      markPaidMutation.mutate(id, callbacks);
      return;
    }
    if (action === "void") {
      voidMutation.mutate(id, callbacks);
      return;
    }
    deleteMutation.mutate(id, callbacks);
  }, [
    run,
    pendingAction,
    markPaidMutation,
    voidMutation,
    deleteMutation,
    onClose,
  ]);

  const confirmCopy = useMemo(() => {
    if (!pendingAction) {
      return {
        title: "",
        description: "",
        confirmLabel: "",
        isDanger: false,
      };
    }
    if (pendingAction === "markPaid") {
      return {
        title: t("payrollDetail.confirmMarkPaidTitle"),
        description: interpolate("payrollDetail.confirmMarkPaidBody", {
          period: periodLabel,
        }),
        confirmLabel: t("payrollDetail.actions.markPaid"),
        isDanger: false,
      };
    }
    if (pendingAction === "delete") {
      return {
        title: t("payrollDetail.confirmDeleteTitle"),
        description: interpolate("payrollDetail.confirmDeleteBody", {
          period: periodLabel,
        }),
        confirmLabel: t("payrollDetail.actions.delete"),
        isDanger: true,
      };
    }
    return {
      title: t("payrollDetail.confirmVoidTitle"),
      description: interpolate("payrollDetail.confirmVoidBody", {
        period: periodLabel,
      }),
      confirmLabel: t("payrollDetail.actions.void"),
      isDanger: true,
    };
  }, [pendingAction, t, interpolate, periodLabel]);

  const footer = useMemo(() => {
    if (!run || !canWrite) return null;
    if (status === "void") return null;

    if (status === "draft") {
      return (
        <div className="flex w-full flex-wrap items-center justify-end gap-2">
          <button
            type="button"
            className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-rose-200 bg-white px-3 text-sm font-medium text-rose-700 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => setPendingAction("delete")}
            disabled={isPending}
          >
            {t("payrollDetail.actions.delete")}
          </button>
          <button
            type="button"
            className={accountingPrimaryButtonClass}
            onClick={() => setPendingAction("markPaid")}
            disabled={isPending}
          >
            {t("payrollDetail.actions.markPaid")}
          </button>
        </div>
      );
    }

    // paid
    return (
      <div className="flex w-full flex-wrap items-center justify-end gap-2">
        <button
          type="button"
          className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-rose-200 bg-white px-3 text-sm font-medium text-rose-700 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
          onClick={() => setPendingAction("void")}
          disabled={isPending}
        >
          {t("payrollDetail.actions.void")}
        </button>
      </div>
    );
  }, [run, canWrite, status, isPending, t]);

  return (
    <>
      <DetailDrawer
        open={open && resolvedId > 0}
        onClose={onClose}
        title={t("payrollDetail.title")}
        subtitle={run ? periodLabel : undefined}
        size="lg"
        footer={footer}
      >
        {isLoading && !run ? (
          <div
            data-testid="payroll-detail-loading"
            className="space-y-4 animate-pulse"
          >
            <div className="h-24 rounded-2xl bg-warm-100" />
            <div className="h-40 rounded-2xl bg-warm-100" />
            <div className="h-16 rounded-2xl bg-warm-100" />
          </div>
        ) : run ? (
          <PayrollRunDetailBody
            run={run}
            locale={locale}
            currency={runCurrency}
            t={t}
            interpolate={interpolate}
          />
        ) : null}
      </DetailDrawer>

      <ConfirmationModal
        isOpen={!!pendingAction && !!run}
        onOpenChange={() => setPendingAction(null)}
        title={confirmCopy.title}
        description={confirmCopy.description}
        confirmLabel={confirmCopy.confirmLabel}
        isDanger={confirmCopy.isDanger}
        onConfirm={handleConfirm}
      />
    </>
  );
}

function PayrollRunDetailBody({
  run,
  locale,
  currency,
  t,
  interpolate,
}: {
  run: PayrollRun;
  locale: Locale;
  currency: string;
  t: (key: string) => string;
  interpolate: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
}) {
  const lines = run.line_items ?? [];
  const isVoided = run.status === "void";

  return (
    <div className="space-y-5">
      {/* Net total hero */}
      <div className="rounded-2xl border border-warm-200/80 bg-white px-4 py-4 shadow-sm shadow-warm-900/5">
        <p className="text-xs font-medium uppercase tracking-wide text-ink-500">
          {t("payroll.table.netTotal")}
        </p>
        <AnimatedNumberText
          value={Number(run.net_total) || 0}
          format={(v) => formatMoney(v, currency, locale)}
          className={`mt-1 block font-title text-display-md tabular-nums ${
            isVoided ? "text-warm-400 line-through" : "text-ink-900"
          }`}
        />
        <div className="mt-2">
          <StatusBadge
            tone={statusTone(run.status)}
            label={statusLabel(run.status, t)}
            size="sm"
          />
        </div>
      </div>

      {/* Line items mini-table */}
      <div className="overflow-x-auto rounded-2xl border border-warm-200/80 bg-white shadow-sm shadow-warm-900/5">
        <table className="min-w-full divide-y divide-warm-200/80 text-sm">
          <thead>
            <tr className="bg-warm-50/70 text-left text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              <th className="px-3 py-2.5 sm:px-4">
                {t("payrollDetail.lines.payee")}
              </th>
              <th className="px-3 py-2.5 text-right sm:px-4">
                {t("payrollDetail.lines.gross")}
              </th>
              <th className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                {t("payrollDetail.lines.bonus")}
              </th>
              <th className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                {t("payrollDetail.lines.deduction")}
              </th>
              <th className="px-3 py-2.5 text-right sm:px-4">
                {t("payrollDetail.lines.net")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-warm-100">
            {lines.map((line, index) => (
              <tr
                key={line.id ?? `line-${index}`}
                className="transition-colors hover:bg-brand/5"
              >
                <td className="px-3 py-2.5 text-ink-900 sm:px-4">
                  {linePayeeName(line)}
                </td>
                <td className="px-3 py-2.5 text-right sm:px-4">
                  {amountCell(Number(line.gross_amount) || 0, currency, locale)}
                </td>
                <td className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                  {amountCell(Number(line.bonus_amount) || 0, currency, locale)}
                </td>
                <td className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                  {amountCell(
                    Number(line.deduction_amount) || 0,
                    currency,
                    locale,
                  )}
                </td>
                <td className="px-3 py-2.5 text-right sm:px-4">
                  {amountCell(Number(line.net_amount) || 0, currency, locale)}
                </td>
              </tr>
            ))}
            {/* Totals row */}
            <tr className="bg-warm-50/50 font-semibold">
              <td className="px-3 py-2.5 text-ink-700 sm:px-4">
                {t("payrollDetail.totals")}
              </td>
              <td className="px-3 py-2.5 text-right sm:px-4">
                {amountCell(Number(run.gross_total) || 0, currency, locale)}
              </td>
              <td className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                {amountCell(Number(run.bonus_total) || 0, currency, locale)}
              </td>
              <td className="hidden px-3 py-2.5 text-right sm:table-cell sm:px-4">
                {amountCell(Number(run.deduction_total) || 0, currency, locale)}
              </td>
              <td className="px-3 py-2.5 text-right sm:px-4">
                {amountCell(Number(run.net_total) || 0, currency, locale)}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      {/* Notes */}
      {run.notes?.trim() ? (
        <div className="rounded-2xl border border-warm-200/80 bg-white px-4 py-4 shadow-sm shadow-warm-900/5">
          <p className="text-xs font-medium uppercase tracking-wide text-ink-500">
            {t("payrollDetail.notes")}
          </p>
          <p className="mt-1 whitespace-pre-wrap text-sm text-ink-900">
            {run.notes}
          </p>
        </div>
      ) : null}

      {/* Audit trail */}
      <div className="space-y-1 text-xs text-warm-500">
        {run.created_at || run.created_by_staff ? (
          <p>
            {interpolate("payrollDetail.audit.createdBy", {
              name: actorName(
                run.created_by_staff,
                run.created_by_user_id,
                t("payrollDetail.audit.owner"),
              ),
              date: run.created_at ? formatDay(run.created_at, locale) : "—",
            })}
          </p>
        ) : null}
        {run.paid_at ? (
          <p>
            {interpolate("payrollDetail.audit.paidBy", {
              name: actorName(
                run.paid_by_staff,
                run.paid_by_user_id,
                t("payrollDetail.audit.owner"),
              ),
              date: formatDay(run.paid_at, locale),
            })}
          </p>
        ) : null}
        {run.voided_at ? (
          <p>
            {interpolate("payrollDetail.audit.voidedBy", {
              name: actorName(
                run.voided_by_staff,
                run.voided_by_user_id,
                t("payrollDetail.audit.owner"),
              ),
              date: formatDay(run.voided_at, locale),
            })}
          </p>
        ) : null}
      </div>
    </div>
  );
}

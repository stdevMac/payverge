"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Users, Download, Plus } from "lucide-react";
import type { Locale } from "@/i18n/localeRegistry";
import {
  accountingApi,
  type PayrollRunWithCount,
} from "@/api/accounting";
import {
  useDeletePayrollRun,
  useMarkPayrollRunPaid,
  usePayrollRuns,
  useVoidPayrollRun,
} from "@/hooks/accounting/useAccountingQueries";
import {
  formatMoney,
  formatPeriod,
} from "@/components/business/accounting/format";
import { PremiumPanel } from "../premium";
import DataTable, { type DataTableColumn } from "../shared/DataTable";
import RowActionsMenu, { type RowActionItem } from "../shared/RowActionsMenu";
import SegmentedTabs from "../shared/SegmentedTabs";
import ConfirmationModal from "../modals/ConfirmationModal";
import StatusBadge, { type StatusTone } from "./StatusBadge";
import PayrollRunDetailDrawer from "./PayrollRunDetailDrawer";
import PayrollRunFormDrawer from "./PayrollRunFormDrawer";
import {
  accountingPanelClass,
  accountingPanelHeaderClass,
  accountingPanelTitleClass,
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
} from "./accountingShared";

const PAGE_SIZE = 20;

type PayrollStatusFilter = "all" | "draft" | "paid" | "void";

type ConfirmAction =
  | { kind: "markPaid"; runId: number; period: string }
  | { kind: "delete"; runId: number; period: string }
  | { kind: "void"; runId: number; period: string };

export type PayrollTabProps = {
  businessId: string;
  start: string;
  end: string;
  locale: Locale;
  currency: string;
  /** When false, hide New run / mutation controls. */
  canWrite?: boolean;
  /** Deep-link seed from `?status=draft|paid|void` (Task 26). */
  initialStatusFilter?: PayrollStatusFilter;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
};

function statusTone(status: string): StatusTone {
  if (status === "paid") return "success";
  if (status === "void") return "neutral";
  return "pending";
}

function statusLabel(
  status: string,
  t: (key: string) => string,
): string {
  if (status === "paid") return t("payroll.statusPaid");
  if (status === "void") return t("payroll.statusVoid");
  return t("payroll.statusDraft");
}

function resolveInitialStatusFilter(
  value: PayrollStatusFilter | undefined,
): PayrollStatusFilter {
  if (value === "draft" || value === "paid" || value === "void" || value === "all") {
    return value;
  }
  return "all";
}

export default function PayrollTab({
  businessId,
  start,
  end,
  locale,
  currency,
  canWrite = true,
  initialStatusFilter,
  t,
  tWith,
}: PayrollTabProps) {
  const [statusFilter, setStatusFilter] = useState<PayrollStatusFilter>(() =>
    resolveInitialStatusFilter(initialStatusFilter),
  );
  const [page, setPage] = useState(1);
  const [selectedRunId, setSelectedRunId] = useState<number | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);

  // Reset page when filters / period change.
  useEffect(() => {
    setPage(1);
  }, [statusFilter, start, end]);

  const listParams = useMemo(
    () => ({
      start,
      end,
      page,
      page_size: PAGE_SIZE,
      ...(statusFilter !== "all"
        ? { status: statusFilter as "draft" | "paid" | "void" }
        : {}),
    }),
    [start, end, page, statusFilter],
  );

  const { data, isLoading, isFetching } = usePayrollRuns(
    businessId,
    listParams,
  );
  const markPaidMutation = useMarkPayrollRunPaid(businessId);
  const voidMutation = useVoidPayrollRun(businessId);
  const deleteMutation = useDeletePayrollRun(businessId);

  const runs = useMemo(() => data?.runs ?? [], [data?.runs]);
  const total = data?.total ?? 0;

  /** Latest period_end among loaded runs — seeds the create form's next window. */
  const lastPeriodEnd = useMemo(() => {
    if (runs.length === 0) return null;
    let max = "";
    for (const r of runs) {
      const end = (r.period_end || "").slice(0, 10);
      if (end && end > max) max = end;
    }
    return max || null;
  }, [runs]);

  const exportUrl = useMemo(
    () =>
      accountingApi.payrollExportUrl(businessId, {
        start,
        end,
        // The server localizes the CSV header row and otherwise falls back to
        // Accept-Language — send the locale the dashboard is actually rendering
        // in so the file matches the screen, not the browser (#930).
        lang: locale,
        ...(statusFilter !== "all" ? { status: statusFilter } : {}),
      }),
    [businessId, start, end, statusFilter, locale],
  );

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

  const openDetail = useCallback((run: PayrollRunWithCount) => {
    setSelectedRunId(run.id);
  }, []);

  const handleRowAction = useCallback(
    (key: string, run: PayrollRunWithCount) => {
      const period = formatPeriod(run.period_start, run.period_end, locale);
      if (key === "markPaid" && run.status === "draft" && canWrite) {
        setConfirm({ kind: "markPaid", runId: run.id, period });
        return;
      }
      if (key === "delete" && run.status === "draft" && canWrite) {
        setConfirm({ kind: "delete", runId: run.id, period });
        return;
      }
      if (key === "void" && run.status === "paid" && canWrite) {
        setConfirm({ kind: "void", runId: run.id, period });
      }
    },
    [canWrite, locale],
  );

  const menuItemsFor = useCallback(
    (run: PayrollRunWithCount): RowActionItem[] => {
      if (!canWrite) return [];
      if (run.status === "draft") {
        return [
          { key: "markPaid", label: t("payrollRuns.markPaid") },
          {
            key: "delete",
            label: t("payrollRuns.delete"),
            tone: "danger",
          },
        ];
      }
      if (run.status === "paid") {
        return [
          {
            key: "void",
            label: t("payrollRuns.voidAction"),
            tone: "danger",
          },
        ];
      }
      return [];
    },
    [canWrite, t],
  );

  const columns: DataTableColumn<PayrollRunWithCount>[] = useMemo(
    () => [
      {
        key: "period",
        header: t("payroll.table.period"),
        render: (r) => (
          <span
            className={
              r.status === "void" ? "text-warm-400 line-through" : ""
            }
          >
            {formatPeriod(r.period_start, r.period_end, locale)}
          </span>
        ),
      },
      {
        key: "payees",
        header: t("payroll.table.payees"),
        hideBelow: "sm",
        render: (r) =>
          interpolate("payroll.payeeCount", {
            count: r.payee_count ?? r.line_items?.length ?? 0,
          }),
      },
      {
        key: "status",
        header: t("payroll.table.status"),
        render: (r) => (
          <StatusBadge
            tone={statusTone(r.status)}
            label={statusLabel(r.status, t)}
            size="sm"
          />
        ),
      },
      {
        key: "netTotal",
        header: t("payroll.table.netTotal"),
        align: "right",
        render: (r) => (
          <span className="tabular-nums text-ink-950">
            {formatMoney(r.net_total, r.currency || currency, locale)}
          </span>
        ),
      },
    ],
    [t, locale, currency, interpolate],
  );

  const statusTabs = useMemo(
    () => [
      { key: "all", label: t("payroll.filters.all") },
      { key: "draft", label: t("payroll.statusDraft") },
      { key: "paid", label: t("payroll.statusPaid") },
      { key: "void", label: t("payroll.statusVoid") },
    ],
    [t],
  );

  const confirmCopy = useMemo(() => {
    if (!confirm) {
      return {
        title: "",
        description: "",
        confirmLabel: "",
        isDanger: false,
      };
    }
    if (confirm.kind === "markPaid") {
      return {
        title: t("payrollRuns.confirmMarkPaidTitle"),
        description: interpolate("payrollRuns.confirmMarkPaidBody", {
          period: confirm.period,
        }),
        confirmLabel: t("payrollRuns.markPaid"),
        isDanger: false,
      };
    }
    if (confirm.kind === "delete") {
      return {
        title: t("payrollRuns.confirmDeleteTitle"),
        description: interpolate("payrollRuns.confirmDeleteBody", {
          period: confirm.period,
        }),
        confirmLabel: t("payrollRuns.confirmDelete"),
        isDanger: true,
      };
    }
    return {
      title: t("payrollRuns.confirmVoidTitle"),
      description: interpolate("payrollRuns.confirmVoidBody", {
        period: confirm.period,
      }),
      confirmLabel: t("payrollRuns.confirmVoid"),
      isDanger: true,
    };
  }, [confirm, t, interpolate]);

  return (
    <>
      <PremiumPanel className={accountingPanelClass} withTexture={false}>
        <div className={accountingPanelHeaderClass}>
          <h3 className={accountingPanelTitleClass}>{t("payroll.title")}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <a
              href={exportUrl}
              className={accountingSecondaryButtonClass}
              download
            >
              <Download className="h-4 w-4" aria-hidden />
              {t("payroll.exportCsv")}
            </a>
            {canWrite ? (
              <button
                type="button"
                className={accountingPrimaryButtonClass}
                onClick={() => setCreateOpen(true)}
              >
                <Plus className="h-4 w-4" aria-hidden />
                {t("payroll.newRun")}
              </button>
            ) : null}
          </div>
        </div>

        <div
          className="border-b border-warm-200/80 px-4 py-3 sm:px-5"
          data-testid="payroll-status-filters"
        >
          <SegmentedTabs
            tabs={statusTabs}
            activeKey={statusFilter}
            onChange={(key) => setStatusFilter(key as PayrollStatusFilter)}
            size="sm"
            ariaLabel={t("payroll.filters.status")}
          />
        </div>

        <div className="px-2 py-3 sm:px-4">
          <DataTable<PayrollRunWithCount>
            aria-label={t("payroll.title")}
            columns={columns}
            rows={runs}
            rowKey={(row) => row.id}
            loading={isLoading || (isFetching && runs.length === 0)}
            emptyState={{
              icon: Users,
              title: t("payroll.empty.title"),
              subtitle: t("payroll.empty.subtitle"),
            }}
            pagination={{
              page,
              pageSize: PAGE_SIZE,
              total,
              onPageChange: setPage,
            }}
            onRowClick={openDetail}
            rowActions={(row) => {
              const items = menuItemsFor(row);
              if (items.length === 0) return null;
              return (
                <RowActionsMenu
                  aria-label={`payroll-actions-${row.id}`}
                  items={items}
                  onAction={(key) => handleRowAction(key, row)}
                />
              );
            }}
          />
        </div>
      </PremiumPanel>

      <PayrollRunDetailDrawer
        open={selectedRunId != null}
        runId={selectedRunId}
        businessId={businessId}
        locale={locale}
        currency={currency}
        canWrite={canWrite}
        onClose={() => setSelectedRunId(null)}
        t={t}
        tWith={tWith}
      />

      <PayrollRunFormDrawer
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        businessId={businessId}
        locale={locale}
        currency={currency}
        lastPeriodEnd={lastPeriodEnd}
        t={t}
        tWith={tWith}
      />

      <ConfirmationModal
        isOpen={!!confirm}
        onOpenChange={() => setConfirm(null)}
        title={confirmCopy.title}
        description={confirmCopy.description}
        confirmLabel={confirmCopy.confirmLabel}
        isDanger={confirmCopy.isDanger}
        onConfirm={() => {
          if (!confirm) return;
          const { kind, runId } = confirm;
          setConfirm(null);
          if (kind === "markPaid") {
            markPaidMutation.mutate(runId);
            return;
          }
          if (kind === "delete") {
            deleteMutation.mutate(runId);
            return;
          }
          voidMutation.mutate(runId);
        }}
      />
    </>
  );
}

"use client";

import React from "react";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Button,
} from "@nextui-org/react";
import {
  Eye,
  DollarSign,
  Printer as PrinterIcon,
  CheckCircle2,
} from "lucide-react";
import toast from "react-hot-toast";
import { Bill } from "../../api/bills";
import { getBillLocationLabel } from "@/lib/tableLabel";
import { operatorBillDisplayNumber } from "@/lib/operatorBillNumber";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { StatusChip } from "../ui/StatusChip";
import { EmptyState } from "../ui/EmptyState";
import { createBillPrintJob } from "@/api/print";
import OperationalAlertClaimStatus from "./operational-alerts/OperationalAlertClaimStatus";
import { usePendingPaymentBills } from "./usePendingPaymentBills";
import { hasPerm } from "@/constants/permissions";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { useAuth } from "@/providers/HybridAuthProvider";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { OperatorBillNumber } from "./OperatorBillNumber";
import RowActionsMenu, { type RowActionItem } from "./shared/RowActionsMenu";

interface BillsTableProps {
  bills: Bill[];
  isActive: boolean;
  actionLoading: number | null;
  /** L2-37: true while a non-blocking list fetch is in flight — show skeleton
   *  before the empty state so filters stay mounted in the parent. */
  loading?: boolean;
  onViewBill: (billId: number) => void;
  onCloseBill: (billId: number) => void;
  onCreateBill: () => void;
  onResetFilters: () => void;
  tString: (key: string) => string;
  // Business default currency; defaults to USD so callers that don't have
  // the business currency ready still render a valid amount.
  currency?: string;
  // Active UI locale (e.g. "en", "es") used for the short timestamp format.
  locale?: string;
  // Required for print job creation when print actions are shown.
  businessId?: number;
  businessTimezone?: string | null;
}

export const BillsTable = React.memo(function BillsTable({
  bills,
  isActive,
  actionLoading,
  loading = false,
  onViewBill,
  onCloseBill,
  onCreateBill,
  onResetFilters,
  tString,
  currency = "USD",
  locale = "en",
  businessId,
  businessTimezone = null,
}: BillsTableProps) {
  // Route through the shared Intl-based formatter so an AED business shows
  // "AED 322.88" instead of "$322.88". Hardcoded `$` was the prior gap.
  // Thread the operator locale so number grouping follows it (e.g. es/es-AR
  // render "€1.234,56", not the en-US "€1,234.56"). (Audit A-04.)
  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency, undefined, locale);

  const formatShortTimestamp = React.useCallback(
    (value: string) =>
      formatBusinessDateTime(value, locale, businessTimezone, DATE_TIME_SHORT),
    [businessTimezone, locale],
  );

  // Ephemeral "awaiting confirmation" badges for guest-initiated payments
  // (payment.pending). Only tracked for the active bills surface — a settled
  // bill can't have a payment awaiting confirmation.
  const pendingPaymentBillIds = usePendingPaymentBills(
    businessId ?? 0,
    isActive && !!businessId,
  );

  const { isStaffUser } = useAuth();
  const { permissions: staffPermissions } = useStaffPermissionsContext();
  // Owners always; staff need print:bill (no feature flags).
  const canPrint = !isStaffUser || hasPerm(staffPermissions, "print:bill");

  const renderPendingBadge = (billId: number) =>
    pendingPaymentBillIds.has(billId) ? (
      <span
        className="inline-flex items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-[11px] font-semibold text-amber-800"
        title={tString("bills.awaitingConfirmation")}
      >
        <span className="h-1.5 w-1.5 rounded-full bg-amber-500" aria-hidden />
        {tString("bills.awaitingConfirmation")}
      </span>
    ) : null;

  const [printingId, setPrintingId] = React.useState<number | null>(null);
  const [isMobileLayout, setIsMobileLayout] = React.useState(false);

  React.useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;

    const query = window.matchMedia("(max-width: 767px)");
    const updateLayout = () => setIsMobileLayout(query.matches);
    updateLayout();
    query.addEventListener("change", updateLayout);
    return () => query.removeEventListener("change", updateLayout);
  }, []);

  const handlePrintBill = React.useCallback(
    async (billId: number) => {
      if (!businessId) return;
      setPrintingId(billId);
      try {
        const job = await createBillPrintJob(businessId, billId, locale);
        if (job.payload_html) {
          // The assigned browser station is the sole physical consumer. Printing
          // here as well leaves the same routed row available to the background
          // agent and produces two copies.
          toast.success(tString("print.queued"));
        } else {
          toast.error(tString("print.noPrinter"));
        }
      } catch (e) {
        toast.error(
          tString("print.failed").replace("{message}", (e as Error).message),
        );
      } finally {
        setPrintingId(null);
      }
    },
    [businessId, locale, tString],
  );

  const getTableLabel = (bill: Bill) =>
    getBillLocationLabel(bill, {
      table: tString("table"),
      counter: tString("counter"),
      delivery: tString("delivery"),
    });

  const renderActions = (bill: Bill) => {
    const displayNumber = operatorBillDisplayNumber(bill);
    // Laptop widths (~1024px with the dashboard sidebar) cannot fit View +
    // "Close without payment" + Print in one cell. Wrapping still clipped
    // NextUI button labels. Keep View as the primary action; put the rest in
    // the shared ellipsis menu so every label stays fully readable.
    const overflowItems: RowActionItem[] = [];
    if (bill.status === "open") {
      overflowItems.push({
        key: "close",
        label: tString("buttons.closeWithoutPayment"),
        icon: <CheckCircle2 className="h-4 w-4" />,
        disabled: actionLoading === bill.id,
      });
    }
    if (canPrint) {
      overflowItems.push({
        key: "print",
        label: tString("buttons.print"),
        icon: <PrinterIcon className="h-4 w-4" />,
        disabled: printingId === bill.id,
      });
    }

    return (
      <div
        className="flex items-center justify-end gap-1 whitespace-nowrap"
        data-testid="bill-row-actions"
      >
        <Button
          aria-label={`${tString("view")} ${tString("billLabel")} ${displayNumber}`}
          size="sm"
          variant="light"
          startContent={<Eye className="h-4 w-4" />}
          onPress={() => onViewBill(bill.id)}
          className="font-medium text-ink-600 hover:bg-warm-50 hover:text-ink-900"
        >
          {tString("view")}
        </Button>
        {overflowItems.length > 0 ? (
          <RowActionsMenu
            aria-label={tString("buttons.moreActionsAria").replace(
              "{number}",
              displayNumber,
            )}
            data-testid="bill-row-overflow"
            items={overflowItems}
            placement="bottom-end"
            onAction={(key) => {
              if (key === "close") onCloseBill(bill.id);
              if (key === "print") void handlePrintBill(bill.id);
            }}
          />
        ) : null}
      </div>
    );
  };

  // L2-37: never paint the empty state while a fetch is in flight — but only
  // on the FIRST load. `loading` is the shared list-fetching flag, which flips
  // true on every SSE-driven background refetch; swapping live rows for pulse
  // bars several times a minute is a worse flicker than the empty flash.
  if (loading && bills.length === 0) {
    return (
      <div
        className="space-y-3 rounded-2xl border border-warm-200/90 bg-white/90 p-4"
        role="status"
        aria-busy="true"
        aria-label={tString("loading")}
        data-testid="bills-table-loading"
      >
        {[0, 1, 2, 3].map((i) => (
          <div
            key={i}
            className="h-14 animate-pulse rounded-xl bg-warm-100"
          />
        ))}
      </div>
    );
  }

  if (bills.length === 0) {
    return (
      <EmptyState
        panel
        icon={DollarSign}
        title={
          isActive
            ? tString("emptyState.noActiveBills")
            : tString("emptyState.noMatchingBills")
        }
        subtitle={
          isActive
            ? tString("emptyState.createFirstBill")
            : tString("emptyState.adjustFilters")
        }
        action={
          isActive ? (
            <Button
              onPress={onCreateBill}
              radius="full"
              className="bg-brand px-6 font-medium text-white shadow-[0_18px_34px_rgba(26,107,106,0.22)] hover:bg-brand-dark"
            >
              {tString("buttons.createBill")}
            </Button>
          ) : (
            <Button
              onPress={onResetFilters}
              radius="full"
              variant="bordered"
              className="border-warm-200 bg-white px-6 font-medium text-ink-700 hover:bg-warm-50"
            >
              {tString("buttons.resetFilters")}
            </Button>
          )
        }
      />
    );
  }

  return (
    <div className="overflow-x-auto rounded-2xl border border-warm-200/90 bg-white/90 shadow-[0_18px_46px_rgba(46,42,37,0.07)]">
      {isMobileLayout ? (
        <div className="divide-y divide-warm-200/80">
          {bills.map((bill) => (
            <article
              key={bill.id}
              className="p-4 transition-colors hover:bg-warm-50/70"
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="break-words text-sm font-semibold text-ink-900">
                    <OperatorBillNumber
                      id={bill.id}
                      bill_number={bill.bill_number}
                      prefix=""
                      copyLabel={tString("bills.copyBillNumber")}
                      copiedLabel={tString("bills.copiedBillNumber")}
                    />
                  </p>
                  <p className="mt-1 text-xs font-medium text-ink-500">
                    {getTableLabel(bill)}
                  </p>
                </div>
                <div className="flex flex-col items-end gap-1">
                  <StatusChip
                    kind="bill"
                    status={bill.status}
                    labelOverride={tString(`billStatuses.${bill.status}`)}
                  />
                  {renderPendingBadge(bill.id)}
                </div>
              </div>

              <div className="mt-4 grid grid-cols-2 gap-3 rounded-2xl border border-warm-200/80 bg-warm-50/80 p-3">
                <div>
                  <p className="text-xs font-semibold text-ink-500">
                    {tString("tableColumns.total")}
                  </p>
                  <p className="mt-1 font-mono text-base font-semibold text-ink-950">
                    {formatCurrency(bill.total_amount)}
                  </p>
                </div>
                <div>
                  <p className="text-xs font-semibold text-ink-500">
                    {tString("tableColumns.created")}
                  </p>
                  <p className="mt-1 text-sm tabular-nums text-ink-700">
                    {formatShortTimestamp(bill.created_at)}
                  </p>
                </div>
              </div>

              <OperationalAlertClaimStatus
                resourceType="bill"
                resourceId={bill.id}
                className="mt-3"
              />

              <div className="mt-4">{renderActions(bill)}</div>
            </article>
          ))}
        </div>
      ) : (
        <Table aria-label={tString("tableAria")} removeWrapper>
          <TableHeader>
            <TableColumn className="bg-warm-50/90 font-semibold text-ink-600">
              {tString("tableColumns.billNumber")}
            </TableColumn>
            {/*
             * Round 4 audit (Task 8): mobile-tighten. "Table" is second-least-
             * critical for a glanceable bills list — the bill number + amount
             * + status + actions are what operators scan first. Hide below md.
             */}
            <TableColumn className="hidden bg-warm-50/90 font-semibold text-ink-600 md:table-cell">
              {tString("tableColumns.table")}
            </TableColumn>
            <TableColumn className="bg-warm-50/90 text-right font-semibold text-ink-600">
              {tString("tableColumns.total")}
            </TableColumn>
            <TableColumn className="bg-warm-50/90 font-semibold text-ink-600">
              {tString("tableColumns.status")}
            </TableColumn>
            {/*
             * "Created" timestamp is useful but not the row's primary signal —
             * hiding below md keeps mobile to a tight 4-column layout.
             */}
            <TableColumn className="hidden bg-warm-50/90 font-semibold text-ink-600 md:table-cell">
              {tString("tableColumns.created")}
            </TableColumn>
            <TableColumn className="w-[1%] bg-warm-50/90 font-semibold text-ink-600">
              {tString("tableColumns.actions")}
            </TableColumn>
          </TableHeader>
          <TableBody>
            {bills.map((bill) => (
              <TableRow
                key={bill.id}
                className="group transition-colors hover:bg-warm-50/80"
              >
                <TableCell>
                  <OperatorBillNumber
                    id={bill.id}
                    bill_number={bill.bill_number}
                    prefix=""
                    copyLabel={tString("bills.copyBillNumber")}
                    copiedLabel={tString("bills.copiedBillNumber")}
                    className="text-sm font-medium text-ink-900 group-hover:text-brand-dark"
                  />
                </TableCell>
                <TableCell className="hidden md:table-cell">
                  <span className="text-sm text-ink-600">
                    {/* Prefer the friendly name the operator assigned in
                      Table Management ("Indoor 1", "Patio 2"). List endpoints
                      send it as the flat `table_name` (BillListRow projection);
                      full-Bill endpoints send the nested `table` relation. Use
                      `||` so the projection's COALESCE'd empty string still
                      falls through to "Table <id>" for tableless/counter bills. */}
                    {getTableLabel(bill)}
                  </span>
                </TableCell>
                <TableCell className="text-right tabular-nums font-medium">
                  <span className="text-ink-900">
                    {formatCurrency(bill.total_amount)}
                  </span>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <StatusChip
                      kind="bill"
                      status={bill.status}
                      labelOverride={tString(`billStatuses.${bill.status}`)}
                    />
                    {renderPendingBadge(bill.id)}
                  </div>
                  <OperationalAlertClaimStatus
                    resourceType="bill"
                    resourceId={bill.id}
                    className="mt-1"
                  />
                </TableCell>
                <TableCell className="hidden md:table-cell">
                  <span className="text-sm tabular-nums text-ink-600">
                    {formatShortTimestamp(bill.created_at)}
                  </span>
                </TableCell>
                <TableCell className="w-[1%] whitespace-nowrap align-middle">
                  {renderActions(bill)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
});

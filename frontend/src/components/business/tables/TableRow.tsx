import React, { useCallback, useState } from "react";
import { Download, ExternalLink, Receipt } from "lucide-react";
import Link from "next/link";
import toast from "react-hot-toast";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { StatusChip } from "@/components/ui/StatusChip";
import { downloadTableQR, QuickQRColors } from "./qrDownload";
import { formatPreparedUnits } from "@/utils/formatPreparedUnits";
import {
  TIME_SHORT,
  formatBusinessTime,
  resolveBusinessTimeZone,
} from "@/utils/businessTime";
import TableHostActions, { type HostActionTarget } from "./TableHostActions";
import {
  SEATED_EMPTY,
  isMissingTranslationLeaf,
  resolveSeatedIso,
  translationOrFallback,
} from "./seatedAge";

export type TableStatus = "available" | "occupied" | "reserved";

interface TableRowReservation {
  id: number;
  customer_name: string;
  party_size: number;
  reservation_time: string;
  status: string;
}

export interface TableRowData {
  id: number;
  name: string;
  table_code: string;
  is_active: boolean;
  status: TableStatus;
  // Number of seats. Defaults to 0 when the backend hasn't returned a
  // value yet so the column can still render the dash placeholder.
  capacity: number;
  active_bill: {
    id: number;
    total: number;
    physical_item_quantity: number;
    created_at: string | null;
    updated_at?: string | null;
  } | null;
  /** Staff who opened the active bill, when known. */
  server_name: string | null;
  /** Next upcoming pending/confirmed reservation for this table. */
  next_reservation: TableRowReservation | null;
  last_seen: string | null;
  qr_foreground_color?: string;
  qr_background_color?: string;
}

type TimeAgoTranslator = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Exported for aging tone tests (host Live View). */
function minutesSince(iso: string | null, nowMs = Date.now()): number | null {
  if (!iso) return null;
  const ms = nowMs - new Date(iso).getTime();
  if (!Number.isFinite(ms) || ms < 0) return 0;
  return Math.floor(ms / 60000);
}

/** Amber ≥60m, rose ≥120m — long-open checks need attention on a busy floor. */
function agingToneClass(minutes: number | null): string {
  if (minutes == null) return "text-ink-600";
  if (minutes >= 120) return "text-rose-700 font-semibold";
  if (minutes >= 60) return "text-amber-800 font-medium";
  return "text-ink-600";
}

function timeAgo(iso: string | null, t: TimeAgoTranslator): string {
  if (!iso) return SEATED_EMPTY;
  const m = minutesSince(iso) ?? 0;
  const ago = (
    key: string,
    fallback: string,
    params?: Record<string, string | number>,
  ) => translationOrFallback(t(key, params), key, fallback);
  if (m < 1) return ago("timeAgo.justNow", SEATED_EMPTY);
  if (m < 60) return ago("timeAgo.minutesAgo", `${m}m`, { n: m });
  const h = Math.floor(m / 60);
  if (h < 24) return ago("timeAgo.hoursAgo", `${h}h`, { n: h });
  const d = Math.floor(h / 24);
  if (d < 7) return ago("timeAgo.daysAgo", `${d}d`, { n: d });
  if (d < 30) {
    const w = Math.floor(d / 7);
    return ago("timeAgo.weeksAgo", `${w}w`, { n: w });
  }
  const mo = Math.floor(d / 30);
  return ago("timeAgo.monthsAgo", `${mo}mo`, { n: mo });
}

interface TableRowProps {
  table: TableRowData;
  onSelect: (t: TableRowData) => void;
  // Currency code (e.g. "USD", "AED") for formatting the active-bill
  // total. Defaults to USD so call sites that don't pass it (or where
  // the business has no preference set) still produce a readable value.
  currency?: string;
  /** Business IANA timezone for reservation / seated clock labels. */
  businessTimezone?: string | null;
  // Business-level QR colors used as fallback when the row's own
  // qr_foreground_color / qr_background_color aren't set. Lets the
  // quick-download action match the operator's branding without
  // opening the detail drawer.
  qrFallback?: QuickQRColors;
  /** Required for seat / clear / transfer / merge from Live View. */
  businessId?: number;
  /** Other tables the host can transfer/merge into. */
  hostTargets?: HostActionTarget[];
  /** Refresh Live View after a floor mutation. */
  onHostActionComplete?: () => void | Promise<void>;
}

export default function TableRow({
  table,
  onSelect,
  currency = "USD",
  businessTimezone = null,
  qrFallback,
  businessId,
  hostTargets = [],
  onHostActionComplete,
}: TableRowProps) {
  const { locale } = useSimpleLocale();
  const tString = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.dashboard.tableManager.${key}`;
      const result = getTranslation(fullKey, locale, params);
      return typeof result === "string" ? result : key;
    },
    [locale],
  );
  const statusLabel = tString(`tableStatus.${table.status}`);
  const [downloading, setDownloading] = useState(false);
  const tz = resolveBusinessTimeZone(businessTimezone);

  const handleDownload = async (e: React.MouseEvent) => {
    // Row's onClick opens the drawer — stop that so the operator can
    // grab a QR without leaving the list view.
    e.stopPropagation();
    if (downloading) return;
    setDownloading(true);
    try {
      await downloadTableQR(table.table_code, {
        foregroundColor: table.qr_foreground_color || qrFallback?.foregroundColor,
        backgroundColor: table.qr_background_color || qrFallback?.backgroundColor,
        poweredByText: tString("qrCustomization.poweredBy"),
      });
    } catch (err) {
      console.error("QR download failed", err);
      toast.error(tString("qrDownloadError"));
    } finally {
      setDownloading(false);
    }
  };

  const previewUrl = `/t/${table.table_code}`;
  const openBillHref = table.active_bill
    ? `?tab=bills&billId=${table.active_bill.id}`
    : null;

  const covers =
    table.next_reservation?.party_size && table.next_reservation.party_size > 0
      ? table.next_reservation.party_size
      : null;

  const reservationTimeLabel = table.next_reservation
    ? formatBusinessTime(
        table.next_reservation.reservation_time,
        locale,
        tz,
        TIME_SHORT,
      )
    : null;

  const seatedIso = resolveSeatedIso({
    created_at: table.active_bill?.created_at,
    updated_at: table.active_bill?.updated_at,
    last_seen: table.last_seen,
  });
  const seatedMinutes = minutesSince(seatedIso);
  const unknownLabel = translationOrFallback(
    tString("aging.seatedUnknown"),
    "aging.seatedUnknown",
    SEATED_EMPTY,
  );
  const warningTitle = tString("aging.openCheckWarning");
  const warningIsReal = !isMissingTranslationLeaf(
    warningTitle,
    "aging.openCheckWarning",
  );
  const hasSeatedParty =
    Boolean(table.active_bill) || table.status === "occupied";
  const seatedLabel = seatedIso
    ? timeAgo(seatedIso, tString)
    : hasSeatedParty
      ? unknownLabel
      : SEATED_EMPTY;
  const seatedTitle = seatedIso
    ? seatedMinutes != null && seatedMinutes >= 60 && warningIsReal
      ? warningTitle
      : formatBusinessTime(seatedIso, locale, tz, TIME_SHORT)
    : undefined;
  const lastSeenMinutes = minutesSince(table.last_seen);

  return (
    <tr
      role="row"
      onClick={() => onSelect(table)}
      className={`cursor-pointer hover:bg-warm-50 hover:shadow-sm transition-all group${
        table.is_active ? "" : " bg-warm-50/40 text-ink-500"
      }`}
    >
      {/* Show the operator-facing table_code ("CORE-T01") instead of
          the internal DB primary key. Long slug codes must not wrap and
          steal width from the Name column (host Live View). */}
      <td className="px-3 py-3 w-[9rem] max-w-[9rem]">
        <span
          className="block truncate font-mono text-xs uppercase tracking-wider text-ink-600"
          title={table.table_code}
        >
          {table.table_code}
        </span>
      </td>
      <td
        className="px-3 py-3 min-w-[8rem] font-medium text-ink-900 whitespace-nowrap"
        data-testid="table-name"
      >
        <span className="block max-w-[12rem] truncate" title={table.name}>
          {table.name}
        </span>
      </td>
      <td className="px-3 py-3 text-ink-700 tabular-nums">
        {table.capacity > 0 ? `${table.capacity}` : <span className="text-ink-400">—</span>}
      </td>
      <td className="px-3 py-3">
        {/* A soft-deleted table is neither available/occupied/reserved — it is
            hidden from guests. Read it as neutral "Inactive" so the operator
            knows it needs reactivating, not that it is somehow free. */}
        {table.is_active ? (
          <div className="flex flex-col items-start gap-0.5">
            <StatusChip kind="table" status={table.status} labelOverride={statusLabel} />
            {table.status === "reserved" && reservationTimeLabel ? (
              <span className="text-xs text-ink-600 tabular-nums">
                {tString("nextReservation", { time: reservationTimeLabel })}
              </span>
            ) : null}
            {table.status === "available" && reservationTimeLabel ? (
              <span className="text-xs text-amber-800 tabular-nums">
                {tString("nextReservation", { time: reservationTimeLabel })}
              </span>
            ) : null}
          </div>
        ) : (
          <StatusChip
            tone="neutral"
            label={tString("tableStatus.inactive")}
          />
        )}
      </td>
      <td className="px-3 py-3 text-ink-700 tabular-nums">
        {covers != null ? covers : <span className="text-ink-400">—</span>}
      </td>
      <td className="px-3 py-3 text-ink-700">
        {table.server_name ? (
          <span className="block max-w-[8rem] truncate" title={table.server_name}>
            {table.server_name}
          </span>
        ) : (
          <span className="text-ink-400">—</span>
        )}
      </td>
      <td
        className={`px-3 py-3 whitespace-nowrap ${agingToneClass(seatedMinutes)}`}
        title={seatedTitle}
        data-testid="table-seated-age"
      >
        {seatedLabel}
      </td>
      <td
        className="px-3 py-3 min-w-[9rem] whitespace-nowrap"
        data-testid="table-current-bill"
      >
        {/* #794: occupied rows are never dead ends. An open check links to
            its bill; a leftover-kitchen table (occupied, no bill) links to
            the kitchen queue instead of a mute em dash. */}
        {table.active_bill && openBillHref ? (
          <Link
            href={openBillHref}
            onClick={(e) => e.stopPropagation()}
            aria-label={tString("openBillAria").replace("{name}", table.name)}
            title={tString("openBillTitle")}
            className="text-ink-700 tabular-nums whitespace-nowrap underline decoration-warm-300 underline-offset-2 hover:text-brand hover:decoration-brand transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded-sm"
          >
            {formatCurrency(table.active_bill.total, currency, undefined, intlLocaleFor(locale))}
            {table.active_bill.physical_item_quantity > 0 ? (
              <>
                {" "}
                (
                {formatPreparedUnits(
                  table.active_bill.physical_item_quantity,
                  tString,
                  "billItems",
                )}
                )
              </>
            ) : null}
          </Link>
        ) : table.status === "occupied" ? (
          <Link
            /* O5: the row has no ticket-status data — leftover tickets can be
               approved / in_kitchen / ready, so land on the unfiltered queue
               rather than a status filter that may render empty. */
            href="?tab=kitchen&kitchenStatus=all"
            onClick={(e) => e.stopPropagation()}
            aria-label={tString("inKitchenLeftoverAria").replace(
              "{name}",
              table.name,
            )}
            className="text-xs text-amber-800 underline decoration-amber-300 underline-offset-2 hover:text-amber-900 hover:decoration-amber-500 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded-sm"
          >
            {tString("inKitchenLeftover")}
          </Link>
        ) : (
          <span className="text-ink-400">—</span>
        )}
      </td>
      <td
        className={`px-3 py-3 ${agingToneClass(lastSeenMinutes)}`}
        data-testid="table-last-seen-age"
      >
        {timeAgo(table.last_seen, tString)}
      </td>
      <td className="px-3 py-3">
        <div className="flex items-center gap-1 justify-end">
          {businessId != null && onHostActionComplete ? (
            <TableHostActions
              businessId={businessId}
              table={table}
              targets={hostTargets}
              onChanged={onHostActionComplete}
              tString={tString}
            />
          ) : null}
          {openBillHref ? (
            <Link
              href={openBillHref}
              onClick={(e) => e.stopPropagation()}
              aria-label={tString("openBillAria").replace("{name}", table.name)}
              title={tString("openBillTitle")}
              className="inline-flex items-center justify-center w-8 h-8 rounded-md text-ink-500 hover:text-brand hover:bg-warm-100 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            >
              <Receipt className="w-4 h-4" />
            </Link>
          ) : null}
          <button
            type="button"
            onClick={handleDownload}
            disabled={downloading}
            aria-label={tString("downloadQrAria").replace("{name}", table.name)}
            title={tString("downloadQrTitle")}
            className="inline-flex items-center justify-center w-8 h-8 rounded-md text-ink-500 hover:text-brand hover:bg-warm-100 transition-colors disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            <Download className="w-4 h-4" />
          </button>
          <a
            href={previewUrl}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            aria-label={tString("openPreviewAria").replace("{name}", table.name)}
            title={tString("openPreviewTitle")}
            className="inline-flex items-center justify-center w-8 h-8 rounded-md text-ink-500 hover:text-brand hover:bg-warm-100 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            <ExternalLink className="w-4 h-4" />
          </a>
        </div>
      </td>
    </tr>
  );
}

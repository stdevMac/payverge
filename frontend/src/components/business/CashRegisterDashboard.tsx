"use client";

import React, { FormEvent, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Drawer,
  DrawerContent,
  DrawerBody,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import toast from "react-hot-toast";
import {
  ArrowLeftRight,
  Download,
  History,
  LockKeyhole,
  Minus,
  Plus,
  Printer,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  cashRegisterApi,
  type CashRegisterMovement,
  type CashRegisterSession,
} from "@/api/cashRegister";
import { getBusiness } from "@/api/business";
import { queryKeys } from "@/api/queryKeys";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { asDollars, formatDollars } from "@/types/money";
import { normalizeMoneyInput } from "@/types/alternativePayments";
import { intlLocaleFor } from "@/utils/intlLocale";
import { DATE_TIME_SHORT, formatBusinessDateTime } from "@/utils/businessTime";
import { operatorTenderLabel } from "@/lib/operatorBillNumber";
import { CashRegisterSkeleton } from "./CashRegisterSkeleton";
import { PremiumPanel } from "./premium";
import { sectionHeadingClass } from "@/components/ui/headingStyles";
import DashboardTabShell from "./shared/DashboardTabShell";
import { btnPrimary } from "@/components/ui/buttonStyles";
import {
  tableBody,
  tableBodyCell,
  tableHeaderCell,
  tableRoot,
} from "./shared/tableStyles";
import { shouldOfferVarianceAdjustment } from "./cashRegisterCloseActions";
import {
  suggestedOpeningFloatFromClosedSession,
  suggestedOpeningFloatFromDeclaredBank,
} from "./cashRegisterSuggestedFloat";
import {
  buildCashRegisterZReportHTML,
  buildCashRegisterZReportText,
  downloadTextFile,
} from "./cashRegisterZReport";

interface CashRegisterDashboardProps {
  businessId: string;
  /** Wave 4: jump into Accounting after close-of-day. */
  onNavigateToTab?: (tab: string) => void;
  /**
   * Fix 7: currency + IANA timezone threaded from the dashboard page so this
   * component doesn't re-fetch getBusiness for them. Optional — a standalone
   * render (prop absent) falls back to a single local fetch.
   */
  currency?: string;
  businessTimezone?: string | null;
}

type MovementType = "cash_in" | "cash_out";

// Fix 11: session history is a real paginated list now (backend returns total +
// honors offset). 20 per page.
const SESSION_PAGE_SIZE = 20;

// Money inputs accept a comma OR dot decimal separator (es-AR operators type
// "10,50") and reject >2 decimals. normalizeMoneyInput enforces >0, so the
// zero-allowing variant handles "0"/"0,00" explicitly (opening float and
// drawer count may legitimately be zero).
function parseMoneyInput(value: string) {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const normalized = normalizeMoneyInput(trimmed);
  if (normalized === null) {
    // normalizeMoneyInput rejects 0; a literal zero drawer count is valid.
    return /^0([.,]0{1,2})?$/.test(trimmed) ? asDollars(0) : null;
  }
  return asDollars(Number(normalized));
}

function parsePositiveMoneyInput(value: string) {
  const normalized = normalizeMoneyInput(value);
  return normalized === null ? null : asDollars(Number(normalized));
}

function formatDateTime(
  value: string | null,
  locale: string,
  timeZone: string | null,
) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return formatBusinessDateTime(date, locale, timeZone, DATE_TIME_SHORT);
}

function fieldClass() {
  return "mt-1 w-full rounded-xl border border-warm-200 bg-white/90 px-3 py-2 text-sm text-ink-900 shadow-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand/20";
}

function labelClass() {
  // #828: `block` is load-bearing — inline label boxes inside a space-y
  // column collapse into one line ("AmountReasonNote") because vertical
  // margins do not apply to inline boxes.
  return "block text-sm font-medium text-ink-700";
}

const cashRegisterMetricPanelClass =
  "rounded-2xl border border-warm-200/80 bg-white/90 px-4 py-3 shadow-sm shadow-warm-900/5";
const cashRegisterMetricLabelClass =
  "text-xs font-semibold uppercase tracking-[0.16em] text-ink-500";
// Color-role buttons the shared kit doesn't carry (destructive-outline header
// action, and the paired emerald/rose movement CTAs). Reshaped to the kit's
// geometry — rounded-full pill, font-medium, focus ring, opacity-50 disabled —
// so they read as one hand with btnPrimary. The plain brand primary now comes
// straight from shared/buttonStyles (btnPrimary).
const cashRegisterDangerOutlineButtonClass =
  "inline-flex h-10 items-center justify-center gap-2 rounded-full border border-rose-200 bg-white px-4 text-sm font-medium text-rose-700 transition-colors hover:bg-rose-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50";
const cashRegisterSuccessButtonClass =
  "inline-flex h-10 items-center justify-center gap-2 rounded-full bg-emerald-600 px-4 text-sm font-medium text-white transition-colors hover:bg-emerald-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-400/50 disabled:cursor-not-allowed disabled:opacity-50";
const cashRegisterCashOutButtonClass =
  "inline-flex h-10 items-center justify-center gap-2 rounded-full bg-rose-600 px-4 text-sm font-medium text-white transition-colors hover:bg-rose-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400/50 disabled:cursor-not-allowed disabled:opacity-50";

// Variance tone: a perfectly-reconciled drawer (variance 0) is the only "good"
// (green) outcome; both a shortage AND an overage are exceptions the operator
// should notice, so any non-zero variance reads amber, not green (a large
// overage previously rendered as reassuring green). Small float noise below a
// cent counts as reconciled.
function varianceTone(
  variance: number | null | undefined,
): "positive" | "warning" {
  return Math.abs(Number(variance ?? 0)) < 0.005 ? "positive" : "warning";
}

function Metric({
  label,
  value,
  tone = "neutral",
}: {
  label: string;
  value: string;
  tone?: "neutral" | "positive" | "negative" | "warning";
}) {
  const toneClass =
    tone === "positive"
      ? "text-emerald-700"
      : tone === "negative"
        ? "text-rose-700"
        : tone === "warning"
          ? "text-amber-700"
          : "text-ink-900";

  return (
    <div className={cashRegisterMetricPanelClass}>
      <p className={cashRegisterMetricLabelClass}>{label}</p>
      <p className={`mt-2 text-xl font-semibold ${toneClass}`}>{value}</p>
    </div>
  );
}

type CashRegisterEmptyPanelProps = {
  icon: LucideIcon;
  subtitle: string;
  title: string;
  variant: "movements" | "history";
};

function CashRegisterEmptyPanel({
  icon: Icon,
  subtitle,
  title,
  variant,
}: CashRegisterEmptyPanelProps) {
  return (
    <div className="mt-4 overflow-hidden rounded-2xl border border-brand/10 bg-brand/5 p-4 sm:p-5">
      <div>
        <div>
          <div className="mb-4 flex h-11 w-11 items-center justify-center rounded-2xl border border-white/80 bg-white shadow-sm">
            <Icon className="h-5 w-5 text-brand" strokeWidth={1.7} />
          </div>
          <h3 className="font-title text-xl text-ink-950">{title}</h3>
          <p className="mt-2 max-w-md text-sm leading-6 text-ink-600">
            {subtitle}
          </p>
        </div>
      </div>
    </div>
  );
}

function historyClosedBy(item: CashRegisterSession): string {
  // CLOSED BY is only meaningful after close. Falling back to
  // opened_by_label on open rows made operators think the opener
  // "closed" the drawer (and often rendered opaque "user:8" labels).
  return item.status === "closed" ? item.closed_by_label || "—" : "—";
}

type HistorySessionCardProps = {
  item: CashRegisterSession;
  locale: string;
  businessTimezone: string | null;
  formatMoney: (value: number | undefined | null) => string;
  formatVariance: (value: number | undefined | null) => string;
  t: (key: string) => string;
  onOpen: (sessionId: number) => void;
};

function HistorySessionCard({
  item,
  locale,
  businessTimezone,
  formatMoney,
  formatVariance,
  t,
  onOpen,
}: HistorySessionCardProps) {
  const tone = varianceTone(item.variance);
  return (
    <PremiumPanel
      as="button"
      interactive
      withTexture={false}
      data-testid={`cash-register-history-card-${item.id}`}
      aria-label={t("viewSessionDetail").replace("{id}", String(item.id))}
      onClick={() => onOpen(item.id)}
      className="w-full p-4 text-left"
    >
      <div className="flex items-start justify-between gap-3">
        <span className="font-mono font-semibold text-ink-950">
          {t("sessionNumber").replace("{id}", String(item.id))}
        </span>
        <span className="shrink-0 text-sm font-medium text-ink-700">
          {t(`statuses.${item.status}`)}
        </span>
      </div>
      <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-3">
        <div>
          <dt className={cashRegisterMetricLabelClass}>{t("opened")}</dt>
          <dd className="mt-1 text-sm text-ink-600">
            {formatDateTime(item.opened_at, locale, businessTimezone)}
          </dd>
        </div>
        <div>
          <dt className={cashRegisterMetricLabelClass}>{t("closed")}</dt>
          <dd className="mt-1 text-sm text-ink-600">
            {formatDateTime(item.closed_at, locale, businessTimezone)}
          </dd>
        </div>
        <div>
          <dt className={cashRegisterMetricLabelClass}>{t("openingFloat")}</dt>
          <dd className="mt-1 whitespace-nowrap tabular-nums text-sm font-medium text-ink-950">
            {formatMoney(item.opening_float)}
          </dd>
        </div>
        <div>
          <dt className={cashRegisterMetricLabelClass}>{t("countedCash")}</dt>
          <dd className="mt-1 whitespace-nowrap tabular-nums text-sm font-medium text-ink-950">
            {formatMoney(item.counted_cash)}
          </dd>
        </div>
        <div>
          <dt className={cashRegisterMetricLabelClass}>{t("variance")}</dt>
          <dd
            className={`mt-1 whitespace-nowrap font-medium tabular-nums ${
              tone === "warning" ? "text-amber-700" : "text-emerald-700"
            }`}
          >
            {formatVariance(item.variance)}
          </dd>
        </div>
        <div>
          <dt className={cashRegisterMetricLabelClass}>{t("closedBy")}</dt>
          <dd className="mt-1 text-sm text-ink-600">{historyClosedBy(item)}</dd>
        </div>
      </dl>
    </PremiumPanel>
  );
}

export default function CashRegisterDashboard({
  businessId,
  onNavigateToTab,
  currency: currencyProp,
  businessTimezone: businessTimezoneProp,
}: CashRegisterDashboardProps) {
  const { locale } = useSimpleLocale();
  const queryClient = useQueryClient();
  const [openingFloat, setOpeningFloat] = useState("");
  const [openingNote, setOpeningNote] = useState("");
  const [movementAmount, setMovementAmount] = useState("");
  const [movementReason, setMovementReason] = useState("");
  const [movementNote, setMovementNote] = useState("");
  const [closeModalOpen, setCloseModalOpen] = useState(false);
  const [countedCash, setCountedCash] = useState("");
  const [closingNote, setClosingNote] = useState("");
  const [closeResult, setCloseResult] = useState<CashRegisterSession | null>(
    null,
  );
  // Business default currency for amount formatting. Prefer the threaded props
  // (Fix 7); only fetch when the parent didn't supply them. Defaults to USD so
  // an in-flight load shows a valid amount. Without this, AED / ARS drawers
  // rendered "$" everywhere (R3-BP-2, same recipe BillManager uses).
  const [fetchedCurrency, setFetchedCurrency] = useState<string>("USD");
  const [fetchedTimezone, setFetchedTimezone] = useState<string | null>(null);
  const businessCurrency = currencyProp ?? fetchedCurrency;
  const businessTimezone =
    businessTimezoneProp !== undefined ? businessTimezoneProp : fetchedTimezone;

  useEffect(() => {
    let cancelled = false;
    if (currencyProp !== undefined && businessTimezoneProp !== undefined)
      return;
    setFetchedTimezone(null);
    getBusiness(Number(businessId))
      .then((biz) => {
        if (!cancelled) {
          if (biz?.default_currency) setFetchedCurrency(biz.default_currency);
          setFetchedTimezone(biz?.timezone ?? null);
        }
      })
      .catch(() => {
        // Best-effort — leave the USD fallback in place.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, currencyProp, businessTimezoneProp]);

  // Fix 11: session-history pagination (0-based page index).
  const [historyPage, setHistoryPage] = useState(0);
  // #652: the newest CLOSED shift, pinned to history page 0. The opening-float
  // suggestion is a claim about the last close, so it must not follow whatever
  // page of the history table the operator happens to be looking at.
  const [newestClosedSession, setNewestClosedSession] =
    useState<CashRegisterSession | null>(null);
  // Fix 12: closed-session detail drawer — holds the session id being inspected,
  // plus how many of its movements are revealed (collapsed by default, not the
  // full dump).
  const [detailSessionId, setDetailSessionId] = useState<number | null>(null);
  const [detailMovementsShown, setDetailMovementsShown] = useState(10);
  const DETAIL_MOVEMENTS_PAGE = 10;
  // Fix 14: unassigned-cash inspection drawer (list of the individual tenders
  // that make up the unassigned total), paginated.
  const [unassignedDrawerOpen, setUnassignedDrawerOpen] = useState(false);
  const [unassignedPage, setUnassignedPage] = useState(0);
  const UNASSIGNED_PAGE_SIZE = 20;

  const t = (key: string, params?: Record<string, string | number>) => {
    const result = getTranslation(
      `businessDashboard.cashRegisterDashboard.${key}`,
      locale,
      params,
    );
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const formatMoney = (value: number | undefined | null) =>
    formatDollars(asDollars(Number(value ?? 0)), {
      locale: intlLocaleFor(locale),
      currency: businessCurrency,
    });

  // Variance is signed: a positive value means the drawer was OVER (more cash
  // than expected). Without an explicit "+" it's ambiguous vs. a short drawer,
  // so prefix positives. Negatives already carry their "-"; zero (reconciled)
  // gets no sign at all. The sign is a strict-direction cue and intentionally
  // independent of varianceTone, whose green/amber split is magnitude-based
  // (|n| < 0.005 = reconciled) — don't try to align the two thresholds.
  const formatVariance = (value: number | undefined | null) => {
    const n = Number(value ?? 0);
    const formatted = formatMoney(Math.abs(n));
    if (Math.abs(n) < 0.005) {
      return t("evenAmount").replace("{amount}", formatted);
    }
    if (n > 0) {
      return t("overAmount").replace("{amount}", formatted);
    }
    return t("shortAmount").replace("{amount}", formatted);
  };

  const sessionParams = useMemo(
    () => ({
      limit: SESSION_PAGE_SIZE,
      offset: historyPage * SESSION_PAGE_SIZE,
    }),
    [historyPage],
  );

  const currentKey = queryKeys.cashRegister.current(businessId);
  const historyKey = queryKeys.cashRegister.sessions(businessId, sessionParams);
  const unassignedKey = queryKeys.cashRegister.unassigned(businessId);

  const currentQuery = useQuery({
    queryKey: currentKey,
    queryFn: () => cashRegisterApi.getCurrent(businessId),
  });

  const historyQuery = useQuery({
    queryKey: historyKey,
    queryFn: () => cashRegisterApi.listSessions(businessId, sessionParams),
    // Keep the previous page visible while the next page loads (no flash).
    placeholderData: (prev) => prev,
  });

  const unassignedQuery = useQuery({
    queryKey: unassignedKey,
    queryFn: () => cashRegisterApi.getUnassigned(businessId),
  });

  const session = currentQuery.data?.session;
  const sessionId = session?.id;
  const sessionDetailKey = sessionId
    ? queryKeys.cashRegister.session(businessId, sessionId)
    : null;

  const sessionDetailQuery = useQuery({
    queryKey: sessionDetailKey ?? [
      "cash-register",
      businessId,
      "sessions",
      "none",
    ],
    queryFn: () => {
      if (!sessionId) throw new Error("missing_session_id");
      return cashRegisterApi.getSession(businessId, sessionId);
    },
    enabled: Boolean(sessionId),
  });

  // Fix 12: closed-session detail drawer — fetches the selected history row's
  // full session (movements included) via the existing getSession endpoint.
  const detailQuery = useQuery({
    queryKey: detailSessionId
      ? queryKeys.cashRegister.session(businessId, detailSessionId)
      : ["cash-register", businessId, "sessions", "detail-none"],
    queryFn: () => {
      if (!detailSessionId) throw new Error("missing_session_id");
      return cashRegisterApi.getSession(businessId, detailSessionId);
    },
    enabled: Boolean(detailSessionId),
  });
  // Collapse the movements list back to the first page whenever a different
  // session is opened in the drawer.
  useEffect(() => {
    setDetailMovementsShown(DETAIL_MOVEMENTS_PAGE);
  }, [detailSessionId]);

  // Fix 14: fetch the unassigned-cash line items only while the drawer is open.
  const unassignedListParams = useMemo(
    () => ({
      limit: UNASSIGNED_PAGE_SIZE,
      offset: unassignedPage * UNASSIGNED_PAGE_SIZE,
    }),
    [unassignedPage],
  );
  const unassignedListQuery = useQuery({
    queryKey: queryKeys.cashRegister.unassignedList(
      businessId,
      unassignedListParams,
    ),
    queryFn: () =>
      cashRegisterApi.listUnassigned(businessId, unassignedListParams),
    enabled: unassignedDrawerOpen,
    placeholderData: (prev) => prev,
  });
  useEffect(() => {
    if (!unassignedDrawerOpen) setUnassignedPage(0);
  }, [unassignedDrawerOpen]);

  useEffect(() => {
    setOpeningFloat("");
    setOpeningNote("");
    setMovementAmount("");
    setMovementReason("");
    setMovementNote("");
    setCloseModalOpen(false);
    setCountedCash("");
    setClosingNote("");
    setCloseResult(null);
    setHistoryPage(0);
    setNewestClosedSession(null);
    setDetailSessionId(null);
  }, [businessId]);

  // Fix 15: the close-session dialog is now a NextUI Modal, which owns its own
  // Escape/backdrop dismissal and focus trap — the hand-rolled keydown listener
  // that used to live here is no longer needed.

  const invalidateCaja = async (affectedSessionId?: number) => {
    const invalidations = [
      queryClient.invalidateQueries({
        queryKey: queryKeys.cashRegister.all(businessId),
      }),
      queryClient.invalidateQueries({ queryKey: currentKey }),
      queryClient.invalidateQueries({ queryKey: historyKey }),
      queryClient.invalidateQueries({ queryKey: unassignedKey }),
    ];

    if (affectedSessionId) {
      invalidations.push(
        queryClient.invalidateQueries({
          queryKey: queryKeys.cashRegister.session(
            businessId,
            affectedSessionId,
          ),
        }),
      );
    }

    await Promise.all(invalidations);
  };

  // Map a thrown mutation error to a translated toast. Invalid-input sentinels
  // (thrown synchronously by mutationFn before any network call) get the
  // field-level "enter a valid amount" message; everything else (network /
  // server) gets the operation-specific failure copy. Before, all three
  // mutations had NO onError and rendered no error state — a bad money input or
  // a failed request just silently did nothing (R3-BP-3).
  const invalidSentinels = new Set([
    "invalid_opening_float",
    "invalid_movement",
    "invalid_close",
  ]);
  const toastMutationError = (error: unknown, failedKey: string) => {
    const message = error instanceof Error ? error.message : "";
    if (invalidSentinels.has(message)) {
      toast.error(t("errors.amountInvalid"));
      return;
    }
    toast.error(t(failedKey));
  };

  const openMutation = useMutation({
    mutationFn: () => {
      const amount = parseMoneyInput(openingFloat);
      if (amount === null) throw new Error("invalid_opening_float");
      return cashRegisterApi.openSession(businessId, {
        opening_float: amount,
        opening_note: openingNote.trim(),
      });
    },
    onSuccess: async (session) => {
      setOpeningFloat("");
      setOpeningNote("");
      setCloseResult(null);
      await invalidateCaja(session.id);
    },
    onError: (error) => toastMutationError(error, "errors.openFailed"),
  });

  const movementMutation = useMutation({
    mutationFn: (movement_type: MovementType) => {
      const amount = parsePositiveMoneyInput(movementAmount);
      const session = currentQuery.data?.session;
      if (amount === null || !session) throw new Error("invalid_movement");
      return cashRegisterApi.createMovement(businessId, session.id, {
        movement_type,
        amount,
        reason: movementReason.trim(),
        note: movementNote.trim(),
      });
    },
    onSuccess: async () => {
      setMovementAmount("");
      setMovementReason("");
      setMovementNote("");
      await invalidateCaja(currentQuery.data?.session?.id);
    },
    onError: (error) => toastMutationError(error, "errors.movementFailed"),
  });

  const closeMutation = useMutation({
    mutationFn: () => {
      const amount = parseMoneyInput(countedCash);
      const session = currentQuery.data?.session;
      if (amount === null || !session) throw new Error("invalid_close");
      return cashRegisterApi.closeSession(businessId, session.id, {
        counted_cash: amount,
        closing_note: closingNote.trim(),
      });
    },
    onSuccess: async (session) => {
      setCloseResult(session);
      setCloseModalOpen(false);
      setCountedCash("");
      setClosingNote("");
      await invalidateCaja(session.id);
    },
    onError: (error) => toastMutationError(error, "errors.closeFailed"),
  });

  const currentSucceeded = currentQuery.isSuccess;
  const isOpen = currentSucceeded && session?.status === "open";
  const isLoading =
    currentQuery.isLoading ||
    historyQuery.isLoading ||
    unassignedQuery.isLoading;
  const isError =
    currentQuery.isError || historyQuery.isError || unassignedQuery.isError;

  const movements = useMemo(
    () =>
      [
        ...(sessionDetailQuery.data?.movements ?? session?.movements ?? []),
      ].sort((a, b) => b.occurred_at.localeCompare(a.occurred_at)),
    [session?.movements, sessionDetailQuery.data?.movements],
  );

  // Fix 7: the dedicated /unassigned query is the single source of truth. The
  // `current` response also carries unassigned_cash_* but reading both was the
  // audit's "unassigned cash fetched twice"; keep the current-response fields
  // only as a first-paint fallback before the dedicated query resolves.
  const unassignedTotal =
    unassignedQuery.data?.total ?? currentQuery.data?.unassigned_cash_total;
  const unassignedCount =
    unassignedQuery.data?.count ??
    currentQuery.data?.unassigned_cash_count ??
    0;
  const validMovementAmount = parsePositiveMoneyInput(movementAmount) !== null;
  // Inline validity for the money fields — accepts comma decimals, rejects >2
  // decimals. Empty is "not yet invalid" (no error shown until they type).
  const movementAmountInvalid =
    movementAmount.trim() !== "" && !validMovementAmount;
  const validOpeningFloat = parseMoneyInput(openingFloat) !== null;
  const openingFloatInvalid = openingFloat.trim() !== "" && !validOpeningFloat;
  const validCountedCash = parseMoneyInput(countedCash) !== null;
  const countedCashInvalid = countedCash.trim() !== "" && !validCountedCash;
  // L2-30: preview expected vs counted vs variance before confirm. Open
  // sessions intentionally omit expected_cash on the wire (blind-close); derive
  // from float + movements already on the session payload (read-shape only).
  const expectedCashPreview = useMemo(() => {
    if (!session || session.status !== "open") return null;
    const opening = Number(session.opening_float) || 0;
    const sales = Number(session.cash_sales) || 0;
    const refunds = Number(session.cash_refunds) || 0;
    const cashIn = Number(session.cash_in) || 0;
    const cashOut = Number(session.cash_out) || 0;
    return opening + sales - refunds + cashIn - cashOut;
  }, [session]);
  const countedCashPreview = parseMoneyInput(countedCash);
  const variancePreview =
    expectedCashPreview !== null && countedCashPreview !== null
      ? countedCashPreview - expectedCashPreview
      : null;
  // Fix 11: the header "History" stat must reflect the true total (backend
  // returns it), not the current page length (was hardcoded 20).
  const historyTotal = historyQuery.data?.total ?? 0;
  // Newest close first — matches backend closed_at DESC and guards against any
  // out-of-order page payload (dinner QA #108 / #157).
  const historyPageSessions = useMemo(() => {
    const sessions = [...(historyQuery.data?.sessions ?? [])];
    sessions.sort((a, b) => {
      const aClosed = a.closed_at ?? a.opened_at ?? "";
      const bClosed = b.closed_at ?? b.opened_at ?? "";
      const byClosed = bClosed.localeCompare(aClosed);
      return byClosed !== 0 ? byClosed : b.id - a.id;
    });
    return sessions;
  }, [historyQuery.data?.sessions]);
  // Only page 0 can define "the last close" — paging the history table below
  // must not redefine it (#652).
  useEffect(() => {
    if (historyPage !== 0) return;
    setNewestClosedSession(historyPageSessions[0] ?? null);
  }, [historyPage, historyPageSessions]);
  // Suggested opening float = last declared starting bank, never counted cash
  // that already includes over/short (#652). Prefer the /current contract;
  // fall back to the newest closed history row's opening_float.
  const suggestedOpeningFloat = useMemo(() => {
    const fromCurrent = suggestedOpeningFloatFromDeclaredBank(
      currentQuery.data?.suggested_opening_float,
    );
    if (fromCurrent !== null) return fromCurrent;
    return suggestedOpeningFloatFromClosedSession(newestClosedSession);
  }, [currentQuery.data?.suggested_opening_float, newestClosedSession]);
  // Header stats carry money with currency — never a bare "0 Movements" count
  // that races the movements fetch, and never mixed unitless counts (#113/#126).
  const shellStats: { label: string; value: string }[] = [];
  if (isOpen && expectedCashPreview !== null) {
    shellStats.push({
      label: t("shell.inDrawer"),
      value: formatMoney(expectedCashPreview),
    });
  }
  if (unassignedCount > 0 || Number(unassignedTotal ?? 0) > 0) {
    shellStats.push({
      label: t("shell.unassigned"),
      value: formatMoney(unassignedTotal),
    });
  }
  if (historyTotal > 0) {
    shellStats.push({
      label: t("shell.history"),
      value: String(historyTotal),
    });
  }

  const submitOpen = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    openMutation.mutate();
  };

  const submitMovement = (movementType: MovementType) => {
    if (!validMovementAmount) return;
    movementMutation.mutate(movementType);
  };

  const submitClose = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    closeMutation.mutate();
  };

  return (
    <DashboardTabShell
      loading={isLoading ? <CashRegisterSkeleton /> : null}
      header={{
        title: t("title"),
        subtitle: t("subtitle"),
        status: {
          label: isOpen ? t("shell.activeMode") : t("shell.readyMode"),
          tone: isOpen ? "positive" : "neutral",
        },
        stats: shellStats,
      }}
    >
      {isError ? (
        <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          {t("error")}
        </div>
      ) : null}

      {closeResult ? (
        <PremiumPanel
          as="section"
          tone="accent"
          className="p-5"
          data-testid="cash-register-close-result"
        >
          <div className="mb-4 flex items-center gap-2">
            <LockKeyhole className="h-4 w-4 text-brand" aria-hidden />
            <h2 className={sectionHeadingClass}>{t("closeResult")}</h2>
          </div>
          <div className="grid gap-3 md:grid-cols-3">
            <Metric
              label={t("expectedCash")}
              value={formatMoney(closeResult.expected_cash)}
            />
            <Metric
              label={t("countedCash")}
              value={formatMoney(closeResult.counted_cash)}
            />
            <Metric
              label={t("variance")}
              value={formatVariance(closeResult.variance)}
              tone={varianceTone(closeResult.variance)}
            />
          </div>
          {onNavigateToTab ? (
            <div className="mt-4 flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => onNavigateToTab("accounting?sub=entries")}
                className={btnPrimary}
              >
                {t("closeActions.viewInAccounting")}
              </button>
              {shouldOfferVarianceAdjustment(closeResult.variance) ? (
                <button
                  type="button"
                  onClick={() => onNavigateToTab("accounting?sub=entries")}
                  className="inline-flex items-center rounded-xl border border-amber-300 bg-amber-50 px-4 py-2 text-sm font-semibold text-amber-800 transition-colors hover:bg-amber-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                >
                  {t("closeActions.recordVariance")}
                </button>
              ) : null}
            </div>
          ) : null}
        </PremiumPanel>
      ) : null}

      {currentSucceeded && !isOpen ? (
        <PremiumPanel as="section" className="p-5" withTexture={false}>
          <div className="mb-4">
            <h2 className={sectionHeadingClass}>{t("noOpenSession")}</h2>
            <p className="mt-1 text-sm leading-5 text-ink-600">
              {t("openSession")}
            </p>
          </div>
          <form
            className="grid gap-4 md:grid-cols-[1fr_2fr_auto]"
            onSubmit={submitOpen}
          >
            <div>
              <label className={labelClass()}>
                {t("openingFloat")}
                <input
                  aria-invalid={
                    openingFloatInvalid || openingFloat.trim() === ""
                  }
                  aria-describedby="cash-register-opening-float-hint"
                  className={fieldClass()}
                  inputMode="decimal"
                  name="opening_float"
                  onChange={(event) => setOpeningFloat(event.target.value)}
                  required
                  type="text"
                  value={openingFloat}
                />
              </label>
              {openingFloatInvalid ? (
                <span
                  id="cash-register-opening-float-hint"
                  className="mt-1 block text-xs text-rose-600"
                >
                  {t("errors.amountInvalid")}
                </span>
              ) : openingFloat.trim() === "" ? (
                <span
                  id="cash-register-opening-float-hint"
                  className="mt-1 block text-xs text-ink-500"
                >
                  {t("openingFloatRequired")}
                </span>
              ) : (
                <span id="cash-register-opening-float-hint" className="sr-only">
                  {t("openingFloatRequired")}
                </span>
              )}
              {suggestedOpeningFloat !== null ? (
                <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-ink-600">
                  <span>
                    {t("suggestedFloat", {
                      amount: formatMoney(suggestedOpeningFloat),
                    })}
                  </span>
                  <button
                    type="button"
                    className="font-semibold text-brand-700 underline-offset-2 hover:underline"
                    onClick={() =>
                      setOpeningFloat(String(Number(suggestedOpeningFloat)))
                    }
                  >
                    {t("useSuggestedFloat")}
                  </button>
                </div>
              ) : null}
            </div>
            <label className={labelClass()}>
              {t("openingNote")}
              <input
                className={fieldClass()}
                name="opening_note"
                onChange={(event) => setOpeningNote(event.target.value)}
                type="text"
                value={openingNote}
              />
            </label>
            <button
              className={`${btnPrimary} mt-6 h-10 justify-center`}
              disabled={openMutation.isPending || !validOpeningFloat}
              title={!validOpeningFloat ? t("openingFloatRequired") : undefined}
              type="submit"
            >
              <Plus className="h-4 w-4" aria-hidden />
              {t("buttons.open")}
            </button>
          </form>
        </PremiumPanel>
      ) : null}

      {currentSucceeded && isOpen && session ? (
        <section className="space-y-6">
          <PremiumPanel className="p-5" withTexture={false}>
            <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 className={sectionHeadingClass}>{t("currentSession")}</h2>
                <p className="mt-1 text-sm leading-5 text-ink-600">
                  {formatDateTime(session.opened_at, locale, businessTimezone)}
                </p>
              </div>
              <button
                className={cashRegisterDangerOutlineButtonClass}
                onClick={() => setCloseModalOpen(true)}
                type="button"
              >
                <LockKeyhole className="h-4 w-4" aria-hidden />
                {t("closeSession")}
              </button>
            </div>
            <div className="grid gap-3 md:grid-cols-5">
              <Metric
                label={t("openingFloat")}
                value={formatMoney(session.opening_float)}
              />
              <Metric
                label={t("cashSales")}
                value={formatMoney(session.cash_sales)}
                tone="positive"
              />
              <Metric
                label={t("cashRefunds")}
                value={formatMoney(session.cash_refunds)}
                tone="negative"
              />
              <Metric
                label={t("cashIn")}
                value={formatMoney(session.cash_in)}
                tone="positive"
              />
              <Metric
                label={t("cashOut")}
                value={formatMoney(session.cash_out)}
                tone="negative"
              />
            </div>
          </PremiumPanel>

          <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
            <PremiumPanel as="section" className="p-5" withTexture={false}>
              <h2 className={sectionHeadingClass}>{t("movements")}</h2>
              {movements.length === 0 ? (
                <CashRegisterEmptyPanel
                  icon={ArrowLeftRight}
                  title={t("emptyMovements")}
                  subtitle={t("emptyMovementsSubtitle")}
                  variant="movements"
                />
              ) : (
                <div className="mt-4 overflow-x-auto">
                  <table className={`${tableRoot} min-w-[560px]`}>
                    <thead>
                      <tr>
                        <th className={tableHeaderCell}>{t("type")}</th>
                        <th className={tableHeaderCell}>{t("amount")}</th>
                        <th className={tableHeaderCell}>{t("reason")}</th>
                        <th className={tableHeaderCell}>{t("time")}</th>
                      </tr>
                    </thead>
                    <tbody className={tableBody}>
                      {movements.map((movement: CashRegisterMovement) => (
                        <tr key={movement.id}>
                          <td className={`${tableBodyCell} text-ink-600`}>
                            {t(`movementTypes.${movement.movement_type}`)}
                          </td>
                          <td
                            className={`${tableBodyCell} font-medium text-ink-950`}
                          >
                            {formatMoney(movement.amount)}
                          </td>
                          <td className={`${tableBodyCell} text-ink-600`}>
                            {movement.reason || "-"}
                          </td>
                          <td className={`${tableBodyCell} text-ink-500`}>
                            {formatDateTime(
                              movement.occurred_at,
                              locale,
                              businessTimezone,
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </PremiumPanel>

            <PremiumPanel as="section" className="p-5" withTexture={false}>
              <h2 className={sectionHeadingClass}>{t("manualMovement")}</h2>
              <div className="mt-4 space-y-4">
                <label className={labelClass()}>
                  {t("amount")}
                  <input
                    aria-invalid={movementAmountInvalid}
                    className={fieldClass()}
                    inputMode="decimal"
                    onChange={(event) => setMovementAmount(event.target.value)}
                    required
                    type="text"
                    value={movementAmount}
                  />
                  {movementAmountInvalid ? (
                    <span className="mt-1 block text-xs text-rose-600">
                      {t("errors.amountInvalid")}
                    </span>
                  ) : null}
                </label>
                <label className={labelClass()}>
                  {t("reason")}
                  <input
                    className={fieldClass()}
                    onChange={(event) => setMovementReason(event.target.value)}
                    required
                    type="text"
                    value={movementReason}
                  />
                </label>
                <label className={labelClass()}>
                  {t("note")}
                  <input
                    className={fieldClass()}
                    onChange={(event) => setMovementNote(event.target.value)}
                    type="text"
                    value={movementNote}
                  />
                </label>
                <div className="grid gap-3 sm:grid-cols-2">
                  <button
                    className={cashRegisterSuccessButtonClass}
                    disabled={
                      movementMutation.isPending ||
                      !validMovementAmount ||
                      !movementReason.trim()
                    }
                    onClick={() => submitMovement("cash_in")}
                    type="button"
                  >
                    <Plus className="h-4 w-4" aria-hidden />
                    {t("buttons.addCashIn")}
                  </button>
                  <button
                    className={cashRegisterCashOutButtonClass}
                    disabled={
                      movementMutation.isPending ||
                      !validMovementAmount ||
                      !movementReason.trim()
                    }
                    onClick={() => submitMovement("cash_out")}
                    type="button"
                  >
                    <Minus className="h-4 w-4" aria-hidden />
                    {t("buttons.addCashOut")}
                  </button>
                </div>
              </div>
            </PremiumPanel>
          </div>
        </section>
      ) : null}

      {unassignedCount > 0 || Number(unassignedTotal ?? 0) > 0 ? (
        // Fix 14: the panel is now inspectable — clicking opens a drawer listing
        // the individual unassigned tenders.
        <button
          type="button"
          onClick={() => setUnassignedDrawerOpen(true)}
          aria-label={t("inspectUnassigned")}
          className="w-full rounded-xl border border-amber-200 bg-amber-50 p-5 text-left transition hover:bg-amber-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
        >
          <div className="flex items-center justify-between gap-3">
            <h2 className={sectionHeadingClass}>{t("unassignedCash")}</h2>
            <span className="text-sm font-medium text-brand-700">
              {t("inspect")}
            </span>
          </div>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <Metric label={t("amount")} value={formatMoney(unassignedTotal)} />
            <Metric label={t("count")} value={String(unassignedCount)} />
          </div>
        </button>
      ) : null}

      {/* Fix 13: history is ALWAYS shown. Past sessions are read-only and
          independent of whether a session is currently open — the old
          canShowHistory gate hid all history while a drawer was open. */}
      <PremiumPanel as="section" className="p-5" withTexture={false}>
        <h2 className={sectionHeadingClass}>{t("history")}</h2>
        {historyPageSessions.length === 0 ? (
          <CashRegisterEmptyPanel
            icon={History}
            title={t("emptyHistory")}
            subtitle={t("emptyHistorySubtitle")}
            variant="history"
          />
        ) : (
          <>
            <ul
              className="mt-4 space-y-3 lg:hidden"
              data-testid="cash-register-history-cards"
            >
              {historyPageSessions.map((item) => (
                <li key={item.id}>
                  <HistorySessionCard
                    item={item}
                    locale={locale}
                    businessTimezone={businessTimezone}
                    formatMoney={formatMoney}
                    formatVariance={formatVariance}
                    t={t}
                    onOpen={setDetailSessionId}
                  />
                </li>
              ))}
            </ul>
            <div className="mt-4 hidden overflow-x-auto lg:block">
              <table
                aria-label={t("history")}
                className={`${tableRoot} min-w-[780px]`}
              >
                <thead>
                  <tr>
                    <th className={tableHeaderCell}>{t("session")}</th>
                    <th className={tableHeaderCell}>{t("status")}</th>
                    <th className={tableHeaderCell}>{t("opened")}</th>
                    <th className={tableHeaderCell}>{t("closed")}</th>
                    <th className={tableHeaderCell}>{t("closedBy")}</th>
                    <th className={tableHeaderCell}>{t("openingFloat")}</th>
                    <th className={tableHeaderCell}>{t("countedCash")}</th>
                    <th
                      className={`${tableHeaderCell} whitespace-nowrap min-w-[7.5rem]`}
                    >
                      {t("variance")}
                    </th>
                  </tr>
                </thead>
                <tbody className={tableBody}>
                  {historyPageSessions.map((item) => (
                    // Fix 12: clicking a history row opens a read-only detail
                    // drawer (existing getSession, movements included).
                    <tr
                      key={item.id}
                      role="button"
                      tabIndex={0}
                      aria-label={t("viewSessionDetail").replace(
                        "{id}",
                        String(item.id),
                      )}
                      onClick={() => setDetailSessionId(item.id)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          setDetailSessionId(item.id);
                        }
                      }}
                      className="cursor-pointer transition hover:bg-warm-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    >
                      <td className={tableBodyCell}>
                        <span className="font-mono font-semibold text-ink-950">
                          {t("sessionNumber").replace("{id}", String(item.id))}
                        </span>
                      </td>
                      <td className={tableBodyCell}>
                        {t(`statuses.${item.status}`)}
                      </td>
                      <td className={`${tableBodyCell} text-ink-600`}>
                        {formatDateTime(
                          item.opened_at,
                          locale,
                          businessTimezone,
                        )}
                      </td>
                      <td className={`${tableBodyCell} text-ink-600`}>
                        {formatDateTime(
                          item.closed_at,
                          locale,
                          businessTimezone,
                        )}
                      </td>
                      <td className={`${tableBodyCell} text-ink-600`}>
                        {historyClosedBy(item)}
                      </td>
                      <td
                        className={`${tableBodyCell} whitespace-nowrap tabular-nums text-ink-950`}
                      >
                        {formatMoney(item.opening_float)}
                      </td>
                      <td
                        className={`${tableBodyCell} whitespace-nowrap tabular-nums text-ink-950`}
                      >
                        {formatMoney(item.counted_cash)}
                      </td>
                      <td
                        className={`${tableBodyCell} min-w-[7.5rem] whitespace-nowrap font-medium tabular-nums ${
                          varianceTone(item.variance) === "warning"
                            ? "text-amber-700"
                            : "text-emerald-700"
                        }`}
                      >
                        {formatVariance(item.variance)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Fix 11: real pagination — backend returns total + honors offset */}
            {historyTotal > SESSION_PAGE_SIZE && (
              <div className="mt-4 flex items-center justify-between gap-3">
                <span className="text-sm text-ink-600">
                  {t("historyRange")
                    .replace(
                      "{from}",
                      String(historyPage * SESSION_PAGE_SIZE + 1),
                    )
                    .replace(
                      "{to}",
                      String(
                        historyPage * SESSION_PAGE_SIZE +
                          historyPageSessions.length,
                      ),
                    )
                    .replace("{total}", String(historyTotal))}
                </span>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    disabled={historyPage <= 0}
                    onClick={() => setHistoryPage((p) => Math.max(0, p - 1))}
                    className="inline-flex h-9 items-center rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5 disabled:opacity-50"
                  >
                    {t("buttons.previous")}
                  </button>
                  <button
                    type="button"
                    disabled={
                      (historyPage + 1) * SESSION_PAGE_SIZE >= historyTotal
                    }
                    onClick={() => setHistoryPage((p) => p + 1)}
                    className="inline-flex h-9 items-center rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5 disabled:opacity-50"
                  >
                    {t("buttons.next")}
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </PremiumPanel>

      {/* Fix 12: closed-session detail drawer. Read-only view of a past session
          with its movements (collapsed, not the full dump). Uses NextUI Drawer
          for the focus trap + Esc/backdrop close. */}
      <Drawer
        isOpen={detailSessionId !== null}
        onClose={() => setDetailSessionId(null)}
        placement="right"
        size="lg"
        hideCloseButton
      >
        <DrawerContent>
          <DrawerBody className="p-6">
            <div className="mb-4 flex items-center justify-between gap-3">
              <h2 className={sectionHeadingClass}>
                {detailSessionId !== null
                  ? t("sessionNumber").replace("{id}", String(detailSessionId))
                  : t("history")}
              </h2>
              <button
                type="button"
                aria-label={t("buttons.closeDrawer")}
                onClick={() => setDetailSessionId(null)}
                className="inline-flex h-9 w-9 items-center justify-center rounded-xl text-ink-600 transition hover:bg-brand/5 hover:text-brand-700"
              >
                <X className="h-4 w-4" aria-hidden />
              </button>
            </div>

            {detailQuery.isLoading ? (
              <p className="text-sm text-ink-500">{t("loading")}</p>
            ) : detailQuery.isError || !detailQuery.data ? (
              <p className="text-sm text-rose-600">{t("errors.loadFailed")}</p>
            ) : (
              <div className="space-y-5">
                <div className="grid gap-3 sm:grid-cols-2">
                  <Metric
                    label={t("status")}
                    value={t(`statuses.${detailQuery.data.status}`)}
                  />
                  <Metric
                    label={t("openingFloat")}
                    value={formatMoney(detailQuery.data.opening_float)}
                  />
                  <Metric
                    label={t("opened")}
                    value={formatDateTime(
                      detailQuery.data.opened_at,
                      locale,
                      businessTimezone,
                    )}
                  />
                  <Metric
                    label={t("closed")}
                    value={formatDateTime(
                      detailQuery.data.closed_at,
                      locale,
                      businessTimezone,
                    )}
                  />
                  <Metric
                    label={t("countedCash")}
                    value={formatMoney(detailQuery.data.counted_cash)}
                  />
                  <Metric
                    label={t("variance")}
                    value={formatVariance(detailQuery.data.variance)}
                  />
                </div>

                {detailQuery.data.status === "closed" ? (
                  <div className="flex flex-wrap gap-2">
                    <button
                      type="button"
                      className="inline-flex h-9 items-center gap-2 rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5"
                      onClick={() => {
                        const session = detailQuery.data;
                        if (!session) return;
                        const zLabels = {
                          title: t("zReport.title"),
                          session: t("session"),
                          status: t("status"),
                          opened: t("opened"),
                          closed: t("closed"),
                          closedBy: t("closedBy"),
                          openingFloat: t("openingFloat"),
                          cashSales: t("cashSales"),
                          cashRefunds: t("cashRefunds"),
                          cashIn: t("cashIn"),
                          cashOut: t("cashOut"),
                          expectedCash: t("expectedCash"),
                          countedCash: t("countedCash"),
                          variance: t("variance"),
                          movements: t("movements"),
                          type: t("type"),
                          amount: t("amount"),
                          reason: t("reason"),
                          time: t("time"),
                          generatedAt: t("zReport.generatedAt"),
                        };
                        const movementTypeLabels = {
                          cash_sale: t("movementTypes.cash_sale"),
                          cash_refund: t("movementTypes.cash_refund"),
                          cash_in: t("movementTypes.cash_in"),
                          cash_out: t("movementTypes.cash_out"),
                        };
                        const generatedAt = formatDateTime(
                          new Date().toISOString(),
                          locale,
                          businessTimezone,
                        );
                        const html = buildCashRegisterZReportHTML({
                          session,
                          labels: zLabels,
                          movementTypeLabels,
                          formatMoney,
                          formatDateTime: (value) =>
                            formatDateTime(value, locale, businessTimezone),
                          generatedAt,
                        });
                        const printWindow = window.open("", "_blank");
                        if (!printWindow) {
                          toast.error(t("zReport.printBlocked"));
                          return;
                        }
                        printWindow.document.write(html);
                        printWindow.document.close();
                        printWindow.focus();
                        printWindow.print();
                        toast.success(t("zReport.printOpened"));
                      }}
                    >
                      <Printer className="h-4 w-4" aria-hidden />
                      {t("buttons.printZReport")}
                    </button>
                    <button
                      type="button"
                      className="inline-flex h-9 items-center gap-2 rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5"
                      onClick={() => {
                        const session = detailQuery.data;
                        if (!session) return;
                        const text = buildCashRegisterZReportText({
                          session,
                          labels: {
                            title: t("zReport.title"),
                            session: t("session"),
                            status: t("status"),
                            opened: t("opened"),
                            closed: t("closed"),
                            closedBy: t("closedBy"),
                            openingFloat: t("openingFloat"),
                            cashSales: t("cashSales"),
                            cashRefunds: t("cashRefunds"),
                            cashIn: t("cashIn"),
                            cashOut: t("cashOut"),
                            expectedCash: t("expectedCash"),
                            countedCash: t("countedCash"),
                            variance: t("variance"),
                            movements: t("movements"),
                            type: t("type"),
                            amount: t("amount"),
                            reason: t("reason"),
                            time: t("time"),
                            generatedAt: t("zReport.generatedAt"),
                          },
                          movementTypeLabels: {
                            cash_sale: t("movementTypes.cash_sale"),
                            cash_refund: t("movementTypes.cash_refund"),
                            cash_in: t("movementTypes.cash_in"),
                            cash_out: t("movementTypes.cash_out"),
                          },
                          formatMoney,
                          formatDateTime: (value) =>
                            formatDateTime(value, locale, businessTimezone),
                          generatedAt: formatDateTime(
                            new Date().toISOString(),
                            locale,
                            businessTimezone,
                          ),
                        });
                        downloadTextFile(
                          `z-report-session-${session.id}.txt`,
                          text,
                        );
                        toast.success(t("zReport.exported"));
                      }}
                    >
                      <Download className="h-4 w-4" aria-hidden />
                      {t("buttons.exportZReport")}
                    </button>
                  </div>
                ) : null}

                <div>
                  <h3 className={sectionHeadingClass}>{t("movements")}</h3>
                  {(detailQuery.data.movements ?? []).length === 0 ? (
                    <p className="mt-3 text-sm text-ink-500">
                      {t("emptyMovements")}
                    </p>
                  ) : (
                    <>
                      <div className="mt-3 overflow-x-auto">
                        <table className={`${tableRoot} min-w-[420px]`}>
                          <thead>
                            <tr>
                              <th className={tableHeaderCell}>{t("type")}</th>
                              <th className={tableHeaderCell}>{t("amount")}</th>
                              <th className={tableHeaderCell}>{t("time")}</th>
                            </tr>
                          </thead>
                          <tbody className={tableBody}>
                            {(detailQuery.data.movements ?? [])
                              .slice(0, detailMovementsShown)
                              .map((movement) => (
                                <tr key={movement.id}>
                                  <td
                                    className={`${tableBodyCell} text-ink-600`}
                                  >
                                    {t(
                                      `movementTypes.${movement.movement_type}`,
                                    )}
                                  </td>
                                  <td
                                    className={`${tableBodyCell} font-medium text-ink-950`}
                                  >
                                    {formatMoney(movement.amount)}
                                  </td>
                                  <td
                                    className={`${tableBodyCell} text-ink-500`}
                                  >
                                    {formatDateTime(
                                      movement.occurred_at,
                                      locale,
                                      businessTimezone,
                                    )}
                                  </td>
                                </tr>
                              ))}
                          </tbody>
                        </table>
                      </div>
                      {(detailQuery.data.movements ?? []).length >
                        detailMovementsShown && (
                        <button
                          type="button"
                          onClick={() =>
                            setDetailMovementsShown(
                              (c) => c + DETAIL_MOVEMENTS_PAGE,
                            )
                          }
                          className="mt-3 inline-flex h-9 items-center rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5"
                        >
                          {t("showMoreMovements").replace(
                            "{count}",
                            String(
                              Math.min(
                                DETAIL_MOVEMENTS_PAGE,
                                (detailQuery.data.movements ?? []).length -
                                  detailMovementsShown,
                              ),
                            ),
                          )}
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>
            )}
          </DrawerBody>
        </DrawerContent>
      </Drawer>

      {/* Fix 14: unassigned-cash inspection drawer — the individual tenders that
          make up the unassigned total, paginated. List-only (an assign action
          would require a session-attach flow not exposed here — deferred). */}
      <Drawer
        isOpen={unassignedDrawerOpen}
        onClose={() => setUnassignedDrawerOpen(false)}
        placement="right"
        size="lg"
        hideCloseButton
      >
        <DrawerContent>
          <DrawerBody className="p-6">
            <div className="mb-4 flex items-center justify-between gap-3">
              <h2 className={sectionHeadingClass}>{t("unassignedCash")}</h2>
              <button
                type="button"
                aria-label={t("buttons.closeDrawer")}
                onClick={() => setUnassignedDrawerOpen(false)}
                className="inline-flex h-9 w-9 items-center justify-center rounded-xl text-ink-600 transition hover:bg-brand/5 hover:text-brand-700"
              >
                <X className="h-4 w-4" aria-hidden />
              </button>
            </div>

            {unassignedListQuery.isLoading ? (
              <p className="text-sm text-ink-500">{t("loading")}</p>
            ) : unassignedListQuery.isError ? (
              <p className="text-sm text-rose-600">{t("errors.loadFailed")}</p>
            ) : (unassignedListQuery.data?.items ?? []).length === 0 ? (
              <p className="text-sm text-ink-500">{t("emptyUnassigned")}</p>
            ) : (
              <>
                <div className="overflow-x-auto">
                  <table
                    aria-label={t("unassignedCash")}
                    className={`${tableRoot} min-w-[420px]`}
                  >
                    <thead>
                      <tr>
                        <th className={tableHeaderCell}>{t("session")}</th>
                        <th className={tableHeaderCell}>{t("amount")}</th>
                        <th className={tableHeaderCell}>{t("time")}</th>
                      </tr>
                    </thead>
                    <tbody className={tableBody}>
                      {(unassignedListQuery.data?.items ?? []).map((item) => (
                        <tr key={item.id}>
                          <td className={`${tableBodyCell} text-ink-950`}>
                            {/* R2-10: bill_number (with #) -> participant name
                                -> #bill_id. operatorBillDisplayNumber alone
                                dropped both the "#" and the name rung, so a
                                tender with a bill_id but no bill_number read as
                                a bare internal number. */}
                            {operatorTenderLabel({
                              id: item.bill_id,
                              bill_number: item.bill_number,
                              participant_name: item.participant_name,
                            })}
                          </td>
                          <td
                            className={`${tableBodyCell} font-medium text-ink-950`}
                          >
                            {formatMoney(item.amount)}
                          </td>
                          <td className={`${tableBodyCell} text-ink-500`}>
                            {formatDateTime(
                              item.created_at,
                              locale,
                              businessTimezone,
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                {(unassignedListQuery.data?.total ?? 0) >
                  UNASSIGNED_PAGE_SIZE && (
                  <div className="mt-4 flex items-center justify-between gap-3">
                    <span className="text-sm text-ink-600">
                      {t("historyRange")
                        .replace(
                          "{from}",
                          String(unassignedPage * UNASSIGNED_PAGE_SIZE + 1),
                        )
                        .replace(
                          "{to}",
                          String(
                            unassignedPage * UNASSIGNED_PAGE_SIZE +
                              (unassignedListQuery.data?.items?.length ?? 0),
                          ),
                        )
                        .replace(
                          "{total}",
                          String(unassignedListQuery.data?.total ?? 0),
                        )}
                    </span>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        disabled={unassignedPage <= 0}
                        onClick={() =>
                          setUnassignedPage((p) => Math.max(0, p - 1))
                        }
                        className="inline-flex h-9 items-center rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5 disabled:opacity-50"
                      >
                        {t("buttons.previous")}
                      </button>
                      <button
                        type="button"
                        disabled={
                          (unassignedPage + 1) * UNASSIGNED_PAGE_SIZE >=
                          (unassignedListQuery.data?.total ?? 0)
                        }
                        onClick={() => setUnassignedPage((p) => p + 1)}
                        className="inline-flex h-9 items-center rounded-xl border border-warm-200 bg-white px-3 text-sm font-medium text-ink-700 shadow-sm transition hover:bg-brand/5 disabled:opacity-50"
                      >
                        {t("buttons.next")}
                      </button>
                    </div>
                  </div>
                )}
              </>
            )}
          </DrawerBody>
        </DrawerContent>
      </Drawer>

      {/* Fix 15: the close-session dialog is now a NextUI Modal (focus trap,
          Esc/backdrop close, aria-modal) instead of the hand-rolled fixed div. */}
      <Modal
        isOpen={closeModalOpen}
        onClose={() => setCloseModalOpen(false)}
        size="md"
      >
        <ModalContent>
          <form onSubmit={submitClose}>
            <ModalHeader className={sectionHeadingClass}>
              {t("closeSession")}
            </ModalHeader>
            <ModalBody className="space-y-4">
              {/* L2-30: count FIRST. Preview only after a count is entered so
                  blind-close is not defeated by showing expected cash up front. */}
              <label className={labelClass()}>
                {t("cashCountInput")}
                <input
                  aria-invalid={countedCashInvalid}
                  autoFocus
                  className={fieldClass()}
                  inputMode="decimal"
                  onChange={(event) => setCountedCash(event.target.value)}
                  required
                  type="text"
                  value={countedCash}
                  data-testid="cash-register-counted-input"
                />
                {countedCashInvalid ? (
                  <span className="mt-1 block text-xs text-rose-600">
                    {t("errors.amountInvalid")}
                  </span>
                ) : null}
              </label>
              {expectedCashPreview !== null && countedCashPreview !== null ? (
                <div
                  data-testid="cash-register-close-preview"
                  className="rounded-2xl border border-warm-200 bg-warm-50/80 px-3 py-3 text-sm text-ink-800"
                >
                  <div className="flex justify-between gap-2">
                    <span>{t("expectedCash")}</span>
                    <span className="font-semibold tabular-nums">
                      {formatMoney(expectedCashPreview)}
                    </span>
                  </div>
                  <div className="mt-1 flex justify-between gap-2">
                    <span>{t("countedCash")}</span>
                    <span className="font-semibold tabular-nums">
                      {formatMoney(countedCashPreview)}
                    </span>
                  </div>
                  <div className="mt-1 flex justify-between gap-2 border-t border-warm-200/80 pt-1">
                    <span>{t("variance")}</span>
                    <span className="font-semibold tabular-nums">
                      {variancePreview !== null
                        ? formatVariance(variancePreview)
                        : "—"}
                    </span>
                  </div>
                </div>
              ) : null}
              <label className={labelClass()}>
                {t("closingNote")}
                <textarea
                  className={`${fieldClass()} min-h-24 resize-y`}
                  onChange={(event) => setClosingNote(event.target.value)}
                  value={closingNote}
                />
              </label>
            </ModalBody>
            <ModalFooter>
              <Button
                type="button"
                variant="bordered"
                onPress={() => setCloseModalOpen(false)}
                className="font-semibold text-ink-700"
              >
                {t("buttons.cancel")}
              </Button>
              <button
                className={`${btnPrimary} h-10 justify-center`}
                disabled={closeMutation.isPending || !validCountedCash}
                type="submit"
              >
                <LockKeyhole className="h-4 w-4" aria-hidden />
                {t("buttons.submitClose")}
              </button>
            </ModalFooter>
          </form>
        </ModalContent>
      </Modal>
    </DashboardTabShell>
  );
}

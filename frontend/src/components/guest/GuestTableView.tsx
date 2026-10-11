"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import { Spinner, Image } from "@nextui-org/react";
import {
  Menu,
  Receipt,
  QrCode,
  MapPin,
  ChevronRight,
  Sparkles,
  Info,
} from "lucide-react";
import {
  getTableByCode,
  getOpenBillByTableCode,
  BillWithItemsResponse,
} from "../../api/bills";
import { Business, MenuCategory, Offer, Bundle } from "../../api/business";
import { CurrencyPrice } from "../common/CurrencyConverter";
import { usePolling } from "../../hooks/usePolling";
import { useGuestBillSync } from "@/hooks/useGuestBillSync";
import {
  GUEST_TABLE_LANDING_FALLBACK_POLL_MS,
  shouldPollGuestTableLanding,
} from "./guestTableLandingPoll";
import PaymentNotification from "../notifications/PaymentNotification";
import BillUpdateNotification from "../notifications/BillUpdateNotification";
import PersistentGuestNav from "../navigation/PersistentGuestNav";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import CRMSignupCard from "./CRMSignupCard";
import CallWaiterButton from "./CallWaiterButton";
import OpenClosedPill from "./OpenClosedPill";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { formatEntityName } from "@/lib/tableLabel";
import {
  isBusinessClosedFromOrderability,
  isGuestOrderingEnabled,
} from "@/lib/guestBusinessClosed";
import {
  filterGuestSellableBundles,
  filterGuestSellableOffers,
} from "@/lib/guestPromotionAvailability";
import type { Orderability } from "@/api/orders";
import { useCustomerAuth } from "@/contexts/CustomerAuthContext";
import GuestClosedBanner from "./GuestClosedBanner";
import { AiWaiter, type AiWaiterCartActionOutcome } from "./AiWaiter";
import { countGuestFacingBillItems } from "@/lib/billBundleGrouping";
import { shouldShowAiWaiter } from "@/app/t/[tableCode]/menu/aiGate";
import { classifyTableLoadFailure } from "@/app/t/[tableCode]/menu/classifyTableLoadFailure";
import { guestLandingBillPresentation } from "./guestLandingBill";
import {
  canonicalPendingCartTableCode,
  consumePendingCartIntent,
  getOrCreatePendingCartSessionBinding,
  isPendingCartUUID,
  writePendingCartIntent,
  type PendingCartStorageOptions,
} from "./pendingCartIntent";

// Matches the public guest table payload (buildPublicGuestTableResponse).
interface Table {
  table_code: string;
  name: string;
  capacity: number;
  is_active: boolean;
}

interface GuestTableViewProps {
  tableCode: string;
}

/**
 * Why the table home has no data. Issue 815: a 429 (per-IP rate limit) or a
 * transient network/5xx failure must never paint the dead-QR "table isn't
 * active" 404 while the table is actually live — only a true 404 may.
 */
type TableLoadFailure = "not_found" | "rate_limited" | "transient";

function classifyGuestTableFailure(error: unknown): TableLoadFailure {
  const status =
    (error as { response?: { status?: number }; status?: number })?.response
      ?.status ??
    (error as { status?: number })?.status;
  if (status === 429) return "rate_limited";
  return classifyTableLoadFailure(error) === "not_found"
    ? "not_found"
    : "transient";
}

type StagePendingCartNavigationInput = {
  tableCode: string;
  quantity: number;
  notes?: string;
  metadata?: {
    itemType?: "menu_item" | "bundle";
    menuItemId?: string;
    bundleId?: number;
  };
  navigate: (href: string) => void;
};

export function stagePendingCartNavigation(
  input: StagePendingCartNavigationInput,
  options: PendingCartStorageOptions = {},
): AiWaiterCartActionOutcome {
  const tableCode = canonicalPendingCartTableCode(input.tableCode);
  if (tableCode === null) return "rejected";
  const binding = getOrCreatePendingCartSessionBinding(tableCode, options);
  if (!binding.ok) return "rejected";

  const randomUUID =
    options.randomUUID ??
    (typeof globalThis.crypto?.randomUUID === "function"
      ? globalThis.crypto.randomUUID.bind(globalThis.crypto)
      : null);
  if (randomUUID === null) return "rejected";

  let nonce: string;
  try {
    nonce = randomUUID();
  } catch {
    return "rejected";
  }
  if (!isPendingCartUUID(nonce)) return "rejected";
  const now = options.now ?? Date.now();
  const isBundle = input.metadata?.itemType === "bundle";
  if (
    (isBundle &&
      (input.metadata?.bundleId === undefined ||
        input.metadata.menuItemId !== undefined)) ||
    (!isBundle &&
      (input.metadata?.menuItemId === undefined ||
        input.metadata.bundleId !== undefined))
  ) {
    return "rejected";
  }
  const written = writePendingCartIntent(
    {
      version: 1,
      tableCode,
      sessionId: binding.sessionId,
      nonce,
      expiresAt: now + 120_000,
      ...(isBundle
        ? { bundleId: input.metadata?.bundleId as number }
        : { menuItemId: input.metadata?.menuItemId as string }),
      quantity: input.quantity,
      ...(input.notes === undefined ? {} : { notes: input.notes }),
    },
    options,
  );
  if (!written.ok) return "rejected";

  try {
    input.navigate(`/t/${encodeURIComponent(tableCode)}/menu`);
  } catch {
    consumePendingCartIntent(tableCode, binding.sessionId, options);
    return "rejected";
  }
  return "deferred";
}

interface TableData {
  table: Table;
  business: Business;
  menu: {
    categories: string;
    item_orderability?: Record<string, Orderability>;
  };
  categories: MenuCategory[];
  offers?: Offer[];
  bundles?: Bundle[];
}

export const GuestTableView: React.FC<GuestTableViewProps> = ({
  tableCode,
}) => {
  const { t, setBusinessId, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const { isAuthenticated: isCustomerSignedIn } = useCustomerAuth();
  const router = useRouter();
  const [tableData, setTableData] = useState<TableData | null>(null);
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [loadFailure, setLoadFailure] = useState<TableLoadFailure | null>(
    null,
  );
  const [paymentNotification, setPaymentNotification] = useState<any>(null);
  const [billUpdateNotification, setBillUpdateNotification] =
    useState<any>(null);

  const businessData = tableData?.business || null;

  const promotionCatalog = {
    categories: tableData?.categories || [],
    orderability: tableData?.menu?.item_orderability || {},
    bundles: tableData?.bundles || [],
  };
  const activeOffers = filterGuestSellableOffers(
    tableData?.offers,
    promotionCatalog,
  );
  const activeBundles = filterGuestSellableBundles(
    tableData?.bundles,
    promotionCatalog,
  );
  const promotionCount = activeOffers.length + activeBundles.length;

  // Hours closed from orderability projection (not kitchen toggle).
  const isBusinessClosed = isBusinessClosedFromOrderability(
    tableData?.menu?.item_orderability,
  );
  const kitchenOrdersOn = Boolean(
    businessData?.kitchen_enabled && businessData?.orders_enabled,
  );
  // Compose closed hours so add-to-cart / order CTAs stay off after hours.
  // Open bills remain payable on /bill regardless of this flag.
  const isOrderingEnabled = isGuestOrderingEnabled({
    kitchenEnabled: businessData?.kitchen_enabled,
    ordersEnabled: businessData?.orders_enabled,
    businessClosed: isBusinessClosed,
  });

  const totalItemQuantity = countGuestFacingBillItems(currentBill?.items);

  const loadTableData = useCallback(
    async (isInitialLoad = false) => {
      if (isInitialLoad) {
        setLoading(true);
      }

      try {
        const tableResponse = await getTableByCode(tableCode, currentLanguage);
        setTableData(tableResponse);
        setLoadFailure(null);

        if (tableResponse?.business?.id) {
          setBusinessId(tableResponse.business.id);
        }

        try {
          const billResponse = await getOpenBillByTableCode(tableCode);
          // No-active-bill now resolves to { bill: null } (H3) — coerce to a
          // falsy currentBill so downstream `!!currentBill` checks stay honest.
          setCurrentBill(billResponse.bill ? billResponse : null);
        } catch {
          setCurrentBill(null);
        }
      } catch (error) {
        console.error("Error loading table data:", error);
        // Issue 815: remember WHY the load failed so the empty-data render
        // can distinguish a dead QR (404) from throttling/transient outages.
        setLoadFailure(classifyGuestTableFailure(error));
      } finally {
        if (isInitialLoad) {
          setLoading(false);
        }
      }
    },
    [tableCode, setBusinessId, currentLanguage],
  );

  useEffect(() => {
    if (tableCode) {
      loadTableData(true);
    }
  }, [tableCode, loadTableData]);

  // Issue 815 (GM6): the manual retry on the throttled/transient screen must
  // not let a guest hammer the very limiter that throttled them. One retry in
  // flight at a time; the button disables and the screen stays put (no
  // full-page spinner flash) until the fetch settles.
  const retryInFlightRef = useRef(false);
  const [retrying, setRetrying] = useState(false);
  const handleTransientRetry = useCallback(() => {
    if (retryInFlightRef.current) return;
    retryInFlightRef.current = true;
    setRetrying(true);
    void loadTableData(false).finally(() => {
      retryInFlightRef.current = false;
      setRetrying(false);
    });
  }, [loadTableData]);

  const silentLoadTableData = useCallback(() => {
    return loadTableData(false);
  }, [loadTableData]);

  const { connected: streamConnected } = useGuestBillSync({
    tableCode,
    hasActiveBill: !!currentBill,
    onActiveBillChange: () => void silentLoadTableData(),
    onBillEvent: () => void silentLoadTableData(),
  });

  usePolling({
    callback: silentLoadTableData,
    interval: GUEST_TABLE_LANDING_FALLBACK_POLL_MS,
    enabled: shouldPollGuestTableLanding({
      tableCode,
      streamConnected,
      hasTableData: !!tableData,
    }),
    immediate: false,
  });

  // The stream is push-based. Refetch when the guest comes back to the tab so
  // a parked or quiet connection does not leave the landing stale until the
  // 30s fallback fires.
  useEffect(() => {
    if (!tableCode) return;
    const refresh = () => {
      void silentLoadTableData();
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") {
        refresh();
      }
    };
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [tableCode, silentLoadTableData]);

  if (loading) {
    return (
      <div className="flex min-h-[100dvh] items-center justify-center bg-warm-50">
        <Spinner size="lg" color="primary" />
      </div>
    );
  }

  if (!tableData && loadFailure !== "not_found") {
    // Issue 815: throttled (429) or transient network/5xx failure. The table
    // may be perfectly active — never claim the QR is dead here. Polling
    // keeps retrying every 30s; the button offers a manual retry too.
    const rateLimited = loadFailure === "rate_limited";
    return (
      <div
        data-testid="table-load-transient"
        className="flex min-h-[100dvh] items-center justify-center bg-warm-50 px-6"
      >
        <div className="w-full max-w-md text-center">
          <h2 className="font-title text-heading-lg text-ink-950 mb-3">
            {rateLimited ? t("errors.billBusyTitle") : t("errors.networkError")}
          </h2>
          <p className="text-body-sm text-ink-600 mb-6">
            {rateLimited
              ? t("errors.billBusy")
              : t("errors.networkErrorDescription")}
          </p>
          <div className="flex flex-col gap-2">
            <button
              onClick={handleTransientRetry}
              disabled={retrying}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-60"
            >
              {t("errors.tableNotFoundRetry")}
            </button>
            <p className="text-body-sm text-ink-500 mt-3">
              {t("errors.tableNotFoundHelp")}
            </p>
          </div>
        </div>
      </div>
    );
  }

  if (!tableData) {
    return (
      <div className="flex min-h-[100dvh] items-center justify-center bg-warm-50 px-6">
        <div className="w-full max-w-md text-center">
          <p className="text-label uppercase text-ink-500 mb-3">404</p>
          <h2 className="font-title text-heading-lg text-ink-950 mb-3">
            {t("errors.tableNotFound")}
          </h2>
          <p className="text-body-sm text-ink-600 mb-6">
            {t("errors.tableNotFoundDescription", { tableCode })}
          </p>
          <div className="flex flex-col gap-2">
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
            >
              {t("errors.tableNotFoundRetry")}
            </button>
            <Link
              href="/"
              className="inline-flex items-center justify-center rounded-full border border-ink-200 bg-white px-6 py-3 text-sm font-semibold text-ink-700 transition-colors hover:bg-ink-50"
            >
              {t("errors.tableNotFoundGoHome")}
            </Link>
            <p className="text-body-sm text-ink-500 mt-3">
              {t("errors.tableNotFoundHelp")}
            </p>
          </div>
        </div>
      </div>
    );
  }

  const { table, business } = tableData;

  return (
    <div className="relative min-h-[100dvh] bg-warm-50">
      <div
        data-testid="guest-table-main"
        className="mx-auto w-full max-w-xl px-5 pt-8 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom)+5.5rem)] sm:max-w-2xl sm:px-8 sm:pt-16"
      >
        <div className="flex flex-col gap-6">
          {/* Status pill row — table chip, open/closed pill, sign-in link */}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white px-3 py-1.5 text-label text-ink-600">
                <span className="relative flex h-1.5 w-1.5">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-brand opacity-60" />
                  <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-brand" />
                </span>
                {formatEntityName(t("menu.tableLabel"), table.name)}
              </span>
              {/* Always pass business timezone — device TZ (e.g. Asia/Dubai)
                  would show emerald "Open until" while the closed banner
                  (orderability in venue TZ) correctly says closed. */}
              <OpenClosedPill
                hours={business?.hours}
                timezone={business?.timezone}
              />
            </div>
            <div className="flex items-center gap-3">
              {businessData?.crm_enabled && !isCustomerSignedIn && (
                <button
                  type="button"
                  data-testid="landing-header-signin"
                  onClick={() => {
                    // Lift the auth modal open via a custom event the
                    // CRMSignupCard listens for. Avoids navigating to
                    // /login which is a 404 on this app.
                    window.dispatchEvent(
                      new CustomEvent("guest-auth-open", {
                        detail: { mode: "login" },
                      }),
                    );
                  }}
                  // RSP-1: bare uppercase label rendered a 47×16 tap target, the
                  // smallest on the guest tier. `py-1.5 -my-1.5` takes it to 28px
                  // (WCAG 2.5.8 AA floor is 24) without moving the header row.
                  className="text-label uppercase tracking-wider text-ink-600 transition-colors hover:text-brand py-1.5 -my-1.5"
                >
                  {t("crm.signIn")}
                </button>
              )}
            </div>
          </div>

          {/* Hero — centered on mobile, start-aligned from sm up; branded type */}
          <header className="flex flex-col gap-5 sm:gap-6 items-center text-center sm:items-start sm:text-start">
            {business.logo && (
              <div className="h-16 w-16 overflow-hidden rounded-2xl border border-warm-200 bg-white shadow-sm sm:h-20 sm:w-20">
                <Image
                  src={business.logo}
                  alt={business.name}
                  className="h-full w-full object-cover"
                />
              </div>
            )}

            <h1 className="font-title text-display-lg text-ink-950 sm:text-display-xl">
              {business.name}
            </h1>

            {business.address?.street && business.address?.city && (
              <p className="inline-flex items-center gap-2 text-body-sm text-ink-600">
                <MapPin className="h-4 w-4 text-ink-400" strokeWidth={1.75} />
                {business.address.street}, {business.address.city}
              </p>
            )}
          </header>

          {/* Banner priority: kitchen off > hours closed (never double-stack). */}
          {!kitchenOrdersOn ? (
            <div
              role="status"
              className="flex items-start gap-3 rounded-2xl border border-warm-200 bg-warm-50 px-5 py-3"
            >
              <Info
                className="mt-0.5 h-5 w-5 flex-shrink-0 text-brand"
                strokeWidth={1.75}
              />
              <p className="flex-1 text-body-sm text-ink-700">
                {t("menu.orderingDisabled")}
              </p>
            </div>
          ) : isBusinessClosed ? (
            <GuestClosedBanner />
          ) : null}

          {/* Promotions sit above the tall service stack so Today's deals
              never rest under the sticky dock on short phones (issue 83). */}
          {promotionCount > 0 && !isBusinessClosed && (
            <Link
              href={`/t/${tableCode}/menu`}
              data-testid="landing-promotions-teaser"
              className="group flex items-center gap-4 rounded-2xl border border-warm-200 bg-white px-5 py-4 transition-colors hover:border-brand/40"
            >
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
                <Sparkles className="h-5 w-5" strokeWidth={1.75} />
              </div>
              <div className="flex-1">
                <p className="text-heading-sm text-ink-900">
                  {t("table.promotions.title")}
                </p>
                <p className="text-body-sm text-ink-600">
                  {t("table.promotions.subtitle")}
                </p>
              </div>
              <ChevronRight
                className="h-5 w-5 text-ink-400 transition-transform group-hover:translate-x-0.5 group-hover:text-brand"
                strokeWidth={1.75}
              />
            </Link>
          )}

          {/* Service actions first (menu/bill) — rewards come after for unsigned-in guests. */}
          <div
            data-testid="landing-service-actions"
            className="flex flex-col gap-3 pt-2"
          >
            {currentBill ? (
              <Link
                href={`/t/${tableCode}/bill`}
                data-testid="landing-current-bill"
                className="group relative flex items-center gap-4 overflow-hidden rounded-2xl border border-brand/30 bg-white px-5 py-5 transition-all hover:border-brand/60 hover:shadow-sm active:translate-y-px"
              >
                <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-brand/10 text-brand">
                  <Receipt className="h-6 w-6" strokeWidth={1.75} />
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <p className="font-title text-heading-md text-ink-950">
                      {t("table.currentBill")}
                    </p>
                    <span className="rounded-full bg-brand/10 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider text-brand">
                      {t(
                        `bill.billStatus.${guestLandingBillPresentation(currentBill.bill).statusKey}`,
                      )}
                    </span>
                  </div>
                  <p className="font-mono text-body-sm font-semibold tabular-nums text-brand">
                    <CurrencyPrice
                      amount={
                        guestLandingBillPresentation(currentBill.bill).amount
                      }
                      fromCurrency={business.default_currency || "USD"}
                      displayCurrency={
                        business.display_currency ||
                        business.default_currency ||
                        "USD"
                      }
                      locale={moneyLocale}
                    />
                    <span className="ml-2 font-sans font-normal text-ink-500">
                      · {totalItemQuantity} {t("menu.items")}
                    </span>
                  </p>
                </div>
                <ChevronRight
                  className="h-5 w-5 text-ink-400 transition-transform group-hover:translate-x-0.5 group-hover:text-brand"
                  strokeWidth={1.75}
                />
              </Link>
            ) : null}

            <Link
              href={`/t/${tableCode}/menu`}
              data-testid="landing-browse-menu"
              className="group relative flex items-center gap-4 overflow-hidden rounded-2xl border border-warm-200 bg-white px-5 py-5 transition-all hover:border-brand/40 hover:shadow-sm active:translate-y-px"
            >
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-brand text-white shadow-sm transition-transform group-hover:scale-[1.04]">
                <Menu className="h-6 w-6" strokeWidth={1.75} />
              </div>
              <div className="flex-1">
                <p className="font-title text-heading-md text-ink-950">
                  {t("table.browseMenu")}
                </p>
                <p className="text-body-sm text-ink-600">
                  {isBusinessClosed
                    ? t("table.browseMenuDescriptionClosed")
                    : t("table.browseMenuDescription")}
                </p>
              </div>
              <ChevronRight
                className="h-5 w-5 text-ink-400 transition-transform group-hover:translate-x-0.5 group-hover:text-brand"
                strokeWidth={1.75}
              />
            </Link>

            {isOrderingEnabled && !currentBill ? (
              <div className="flex items-center gap-4 rounded-2xl border border-dashed border-warm-200 bg-white px-5 py-5 text-ink-500">
                <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-warm-100">
                  <Receipt
                    className="h-6 w-6 text-ink-400"
                    strokeWidth={1.75}
                  />
                </div>
                <div className="flex-1">
                  <p className="font-title text-heading-md text-ink-700">
                    {t("table.noBill")}
                  </p>
                  <p className="text-body-sm text-ink-500">
                    {t("table.noBillDescription")}
                  </p>
                </div>
              </div>
            ) : null}

            {/* Raise a hand — service call with staff acknowledgement loop */}
            <CallWaiterButton
              tableCode={tableCode}
              businessClosed={isBusinessClosed}
            />
          </div>

          {/* CRM rewards after service CTAs so QR diners hit Menu/Bill first. */}
          {businessData && (
            <div data-testid="landing-crm-rewards">
              <CRMSignupCard
                businessName={businessData.name}
                tableCode={tableCode}
                crmEnabled={businessData.crm_enabled}
                variant={isBusinessClosed || !!currentBill ? "compact" : "full"}
              />
            </div>
          )}

          {/* Footer rail — extra bottom margin so “Powered by” clears the dock
              at max scroll (shell already pads with --guest-nav-height). */}
          <div
            data-testid="landing-footer-rail"
            className="mt-2 mb-2 flex flex-col items-center gap-3 border-t border-warm-200 pt-6 pb-2 text-center"
          >
            <Link
              href="/scan"
              className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:border-ink-300 hover:text-ink-900"
            >
              <QrCode className="h-4 w-4" strokeWidth={1.75} />
              {t("table.scanQR")}
            </Link>
            <p className="text-label text-ink-500">
              {t("businessPage.poweredBySecure")}
            </p>
          </div>
        </div>
      </div>

      <PersistentGuestNav
        tableCode={tableCode}
        currentBill={currentBill}
        isOrderingEnabled={isOrderingEnabled}
        defaultCurrency={business?.default_currency || "USD"}
        displayCurrency={
          business?.display_currency || business?.default_currency || "USD"
        }
      />

      {/* AI Waiter on landing when backend reports ai_available (an LLM provider
          is configured and the venue enabled the waiter).
          Guest QR path uses the real AiWaiter only (never marketing concierge). */}
      {shouldShowAiWaiter(business) && (
        <AiWaiter
          businessId={business.id}
          businessName={business.name}
          aiName={business.ai_settings?.ai_name}
          waiterMode={business.ai_waiter_mode}
          menuData={tableData.categories || []}
          itemOrderability={tableData.menu?.item_orderability || {}}
          bundles={(activeBundles || []).map((b) => ({
            id: b.id,
            name: b.name,
            price: b.price,
            currency: b.currency,
            is_active: b.is_active,
          }))}
          language={currentLanguage || "en"}
          tableCode={tableCode}
          isOrderingEnabled={isOrderingEnabled}
          onAddToCart={(_itemName, _price, quantity, notes, metadata) =>
            stagePendingCartNavigation({
              tableCode,
              quantity,
              notes,
              metadata: {
                itemType: metadata?.itemType,
                menuItemId: metadata?.menuItemId,
                bundleId: metadata?.bundleId,
              },
              navigate: router.push,
            })
          }
          hasActiveBill={!!currentBill}
          billItems={
            currentBill?.items.map((i) => ({
              name: i.menu_item?.name || i.name || t("bill.unknownItem"),
              price: i.price,
              quantity: i.quantity,
            })) || []
          }
          currency={business.default_currency || "USD"}
          cartVisible={false}
        />
      )}

      <PaymentNotification
        payment={paymentNotification}
        onDismiss={() => setPaymentNotification(null)}
      />

      <BillUpdateNotification
        update={billUpdateNotification}
        onDismiss={() => setBillUpdateNotification(null)}
      />
    </div>
  );
};

"use client";

import React, { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useParams } from "next/navigation";
import { Spinner, Button, Image } from "@nextui-org/react";
import toast from "react-hot-toast";
import { useGuestTranslation } from "../../../../i18n/GuestTranslationProvider";
import { nextGuestMenuLanguage } from "@/i18n/localeRegistry";
import { ArrowLeft, Menu, ScrollText } from "lucide-react";
import Link from "next/link";
import dynamic from "next/dynamic";
import PersistentGuestNav from "../../../../components/navigation/PersistentGuestNav";

// Lazy load heavy components
const GuestBill = dynamic(
  () => import("../../../../components/guest/GuestBill"),
  {
    loading: () => (
      <div className="flex justify-center p-8">
        <Spinner size="lg" />
      </div>
    ),
  },
);
import {
  getBillByNumber,
  getTableByCode,
  getOpenBillByTableCode,
  getBusinessByTableCode,
  tryGuestBillRef,
  BillWithItemsResponse,
  RateLimitCooldownError,
  parseRetryAfterSeconds,
} from "../../../../api/bills";
import { getApiErrorStatus } from "@/utils/apiError";
import {
  classifyTableLoadFailure,
  isTransientTableMiss,
} from "../menu/classifyTableLoadFailure";
import { Business, MenuCategory } from "../../../../api/business";
import { getMenuByTableCode } from "../../../../api/bills";
import ReceiptGenerator from "../../../../components/receipt/ReceiptGenerator";
import PaymentStatusChecker from "../../../../components/payment/PaymentStatusChecker";
import { CurrencyPrice } from "../../../../components/common/CurrencyConverter";
import LoyaltyEarnedCard from "../../../../components/guest/LoyaltyEarnedCard";
import GuestFiscalReceiptCard from "../../../../components/table/GuestFiscalReceiptCard";
import CRMSignupCard from "../../../../components/guest/CRMSignupCard";
import CallWaiterButton from "../../../../components/guest/CallWaiterButton";
import { getRouteParam } from "@/utils/nextRouteParams";
import { SplittingAPI } from "@/api/splitting";
import { paymentMethodLabel } from "@/lib/paymentMethodLabels";
import { useGuestBillSync } from "@/hooks/useGuestBillSync";
import { formatEntityName } from "@/lib/tableLabel";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";

// Matches the public guest table payload (buildPublicGuestTableResponse).
interface Table {
  table_code: string;
  name: string;
  capacity: number;
  is_active: boolean;
}

interface TableData {
  table: Table;
  business: Business;
  menu: {
    categories: string;
  };
  categories: MenuCategory[];
  trustpilot_enabled?: boolean;
  trustpilot_review_url?: string;
}

// Internal component that uses the hook
function GuestBillPageContent() {
  const { t, setBusinessId, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const params = useParams();
  const tableCode = getRouteParam(params, "tableCode");

  const [tableData, setTableData] = useState<TableData | null>(null);
  const [tableLoadError, setTableLoadError] = useState<
    "not_found" | "network" | null
  >(null);
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(
    null,
  );
  const [liveRevision, setLiveRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [showThankYou, setShowThankYou] = useState(false);
  const [paidBill, setPaidBill] = useState<BillWithItemsResponse | null>(null);
  const [paymentDetails, setPaymentDetails] = useState<{
    totalPaid: number;
    tipAmount: number;
    paymentMethod?: string;
    transactionId?: string;
    splitShareId?: number;
  } | null>(null);
  const [selectedLanguage, setSelectedLanguage] = useState<string>("");
  const [_translatedCategories, setTranslatedCategories] = useState<
    MenuCategory[]
  >([]);
  const [businessWithTrustpilot, setBusinessWithTrustpilot] =
    useState<any>(null);

  // Currency settings
  const [businessCurrencies, setBusinessCurrencies] = useState({
    default_currency: "USD",
    display_currency: "USD",
  });

  // Payment status checking
  const [showPaymentStatusChecker, setShowPaymentStatusChecker] =
    useState(false);
  const [pendingPayment, setPendingPayment] = useState<{
    paymentId: string;
    paymentMethod: string;
    /** Guest capability token (public_token); used for /guest/bill routes. */
    billToken: string;
  } | null>(null);

  // Tracks whether the current load cycle is still active. Set to false by
  // the effect cleanup when tableCode changes or the component unmounts, so
  // late-returning parallel responses don't clobber fresh state. (Full request
  // cancellation would need AbortSignal threaded through the API layer.)
  const loadGenerationRef = useRef(0);
  // Guest token of the bill whose close-out lookup is in flight (cashier
  // settlement detection in handleActiveBillChange).
  const closingBillTokenRef = useRef<string | null>(null);
  // NEW-4 / REV-4: when the edge/app returns 429, show a localized countdown
  // and auto-retry instead of a blank/empty bill page. Cap auto-retries so a
  // sticky edge limit cannot spin forever — after the cap, only manual retry.
  const MAX_BILL_AUTO_RETRIES = 3;
  const [rateLimitSeconds, setRateLimitSeconds] = useState<number | null>(null);
  const [rateLimitManualOnly, setRateLimitManualOnly] = useState(false);
  const autoRetryCountRef = useRef(0);
  const rateLimitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rateLimitTickRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const clearRateLimitTimers = useCallback(() => {
    if (rateLimitTimerRef.current) {
      clearTimeout(rateLimitTimerRef.current);
      rateLimitTimerRef.current = null;
    }
    if (rateLimitTickRef.current) {
      clearInterval(rateLimitTickRef.current);
      rateLimitTickRef.current = null;
    }
  }, []);

  const beginRateLimitCooldown = useCallback(
    (seconds: number) => {
      const wait = Math.max(1, Math.ceil(seconds || 5));
      clearRateLimitTimers();
      const remainingAuto =
        MAX_BILL_AUTO_RETRIES - autoRetryCountRef.current;
      if (remainingAuto <= 0) {
        // Cap hit: show busy card without auto-retry timer.
        setRateLimitManualOnly(true);
        setRateLimitSeconds(0);
        return;
      }
      setRateLimitManualOnly(false);
      setRateLimitSeconds(wait);
      rateLimitTickRef.current = setInterval(() => {
        setRateLimitSeconds((prev) => {
          if (prev == null) return prev;
          if (prev <= 1) return 0;
          return prev - 1;
        });
      }, 1000);
      rateLimitTimerRef.current = setTimeout(() => {
        clearRateLimitTimers();
        setRateLimitSeconds(null);
        autoRetryCountRef.current += 1;
        void loadTableDataRef.current();
      }, wait * 1000);
    },
    [clearRateLimitTimers],
  );

  const extractRateLimitSeconds = (reason: unknown): number | null => {
    if (reason instanceof RateLimitCooldownError) {
      return reason.retryAfterSeconds > 0 ? reason.retryAfterSeconds : 5;
    }
    if (getApiErrorStatus(reason) === 429) {
      return parseRetryAfterSeconds(reason, 5);
    }
    return null;
  };

  // Stable ref so the cooldown timeout can call the latest loadTableData.
  const loadTableDataRef = useRef<() => Promise<void>>(async () => {});

  const loadTableData = useCallback(async () => {
    const generation = ++loadGenerationRef.current;
    const isStillActive = () => loadGenerationRef.current === generation;

    setLoading(true);
    try {
      if (!tableCode) {
        setTableData(null);
        setTableLoadError("network");
        return;
      }
      // Load table data, bill data, and business data in parallel for better performance
      let [tableResponse, billResponse, businessResponse] =
        await Promise.allSettled([
          getTableByCode(tableCode, currentLanguage),
          getOpenBillByTableCode(tableCode),
          getBusinessByTableCode(tableCode),
        ]);
      // A live table can miss on the first hop and resolve on reload. The miss
      // is not always a rejection: an edge/proxy hiccup answers 200 with an
      // empty body, which axios resolves with a falsy `data`. Retry the table
      // lookup once for BOTH shapes before rendering Mesa No Encontrada.
      if (isTransientTableMiss(tableResponse)) {
        try {
          tableResponse = {
            status: "fulfilled",
            value: await getTableByCode(tableCode, currentLanguage),
          };
        } catch (retryReason) {
          tableResponse = { status: "rejected", reason: retryReason };
        }
      }

      if (!isStillActive()) return;

      // Prefer the longest retry window if multiple legs 429.
      let rateLimited: number | null = null;
      for (const res of [tableResponse, billResponse, businessResponse]) {
        if (res.status === "rejected") {
          const wait = extractRateLimitSeconds(res.reason);
          if (wait != null) {
            rateLimited = Math.max(rateLimited ?? 0, wait);
          }
        }
      }
      if (rateLimited != null) {
        setCurrentBill(null);
        beginRateLimitCooldown(rateLimited);
        return;
      }

      // Successful path clears any prior cooldown UI and auto-retry budget.
      clearRateLimitTimers();
      setRateLimitSeconds(null);
      setRateLimitManualOnly(false);
      autoRetryCountRef.current = 0;

      if (tableResponse.status === "fulfilled" && tableResponse.value?.table) {
        setTableData(tableResponse.value);
        setTableLoadError(null);

        // Set business ID for translation provider
        if (tableResponse.value?.business?.id) {
          setBusinessId(tableResponse.value.business.id);
        }
      } else if (tableResponse.status === "fulfilled") {
        // Retried and the payload is still empty: the transport lost the body,
        // the code is not proven missing. Never render Mesa No Encontrada here.
        console.error("Empty table payload for", tableCode);
        setTableData(null);
        setTableLoadError("network");
      } else {
        console.error("Error loading table data:", tableResponse.reason);
        setTableData(null);
        setTableLoadError(classifyTableLoadFailure(tableResponse.reason));
      }

      // Store business data with Trustpilot information and set currency settings
      if (businessResponse.status === "fulfilled") {
        setBusinessWithTrustpilot(businessResponse.value);
        // Set business currency settings from the business response
        if (businessResponse.value.business) {
          setBusinessCurrencies({
            default_currency:
              businessResponse.value.business.default_currency || "USD",
            display_currency:
              businessResponse.value.business.display_currency || "USD",
          });
        }
      } else {
        console.error(
          "Error loading business with Trustpilot data:",
          businessResponse.reason,
        );
      }

      if (billResponse.status === "fulfilled") {
        // No-active-bill now resolves to { bill: null } (H3) — coerce to a
        // falsy currentBill so downstream `!currentBill` guards stay honest.
        setCurrentBill(billResponse.value.bill ? billResponse.value : null);
      } else {
        setCurrentBill(null);
      }
    } catch (error) {
      if (!isStillActive()) return;
      const wait = extractRateLimitSeconds(error);
      if (wait != null) {
        beginRateLimitCooldown(wait);
        return;
      }
      console.error("Error loading data:", error);
    } finally {
      if (isStillActive()) {
        setLoading(false);
      }
    }
  }, [
    tableCode,
    setBusinessId,
    beginRateLimitCooldown,
    clearRateLimitTimers,
    currentLanguage,
  ]);

  loadTableDataRef.current = loadTableData;

  useEffect(() => {
    return () => {
      clearRateLimitTimers();
    };
  }, [clearRateLimitTimers]);

  const loadTranslatedMenu = useCallback(
    async (languageCode: string) => {
      try {
        const menuData = await getMenuByTableCode(tableCode, languageCode);
        // Use translated categories if available, otherwise fall back to original
        const categories = menuData.parsed_categories || menuData.categories;
        setTranslatedCategories(categories || []);
      } catch (error) {
        console.error("Error loading translated menu:", error);
        // Fall back to original categories if translation fails
        if (tableData?.categories) {
          setTranslatedCategories(tableData.categories);
        }
      }
    },
    [tableCode, tableData?.categories],
  );

  // Keep bill-line translation locale in lockstep with GuestTranslationProvider
  // (explicit ?lang=, cookie, in-session switch). Do not wait only on a
  // per-business localStorage key — that left selectedLanguage empty while
  // chrome already rendered in Spanish.
  useEffect(() => {
    const next = nextGuestMenuLanguage(currentLanguage, selectedLanguage);
    if (!next) return;
    setSelectedLanguage(next);
    if (tableData?.categories) {
      void loadTranslatedMenu(next);
    }
  }, [
    currentLanguage,
    selectedLanguage,
    tableData?.categories,
    loadTranslatedMenu,
  ]);

  // React to in-session language switches from the bottom-nav selector, the
  // same way the menu page does. Without this the bill screen stayed frozen on
  // the language it first loaded — item translations and dates never updated.
  useEffect(() => {
    const handleGuestLanguageChange = (event: Event) => {
      const { language, businessId } = (event as CustomEvent).detail || {};
      if (businessId != null && businessId !== tableData?.business?.id) {
        return;
      }
      if (language) {
        setSelectedLanguage(language);
        loadTranslatedMenu(language);
      }
    };
    window.addEventListener("guestLanguageChange", handleGuestLanguageChange);
    return () => {
      window.removeEventListener("guestLanguageChange", handleGuestLanguageChange);
    };
  }, [tableData?.business?.id, loadTranslatedMenu]);

  // Helper function to display payment method names (shared module — Wave 5).
  // Legacy quirk preserved: a missing method historically meant USDC crypto.
  const getPaymentMethodDisplay = (paymentMethod?: string): string => {
    if (!paymentMethod) return t("paymentMethods.cryptoUsdc");
    return paymentMethodLabel(paymentMethod, t);
  };
  // Payment-return handling is intentionally one-shot. Snapshot the mount
  // callbacks explicitly so locale/provider rerenders cannot replay a toast or
  // payment transition, while async cleanup still uses the same load cycle.
  const paymentReturnDependenciesRef = useRef({
    translate: t,
    displayPaymentMethod: getPaymentMethodDisplay,
    reloadTableData: loadTableData,
  });

  const handleBillPaidAtCashier = useCallback((bill: BillWithItemsResponse) => {
    if (showPaymentStatusChecker) return;
    setPaidBill(bill);
    setPaymentDetails(null);
    setShowThankYou(true);
    setCurrentBill(null);
  }, [showPaymentStatusChecker]);

  const handlePaymentComplete = useCallback(
    async (paymentDetails: {
      totalPaid: number;
      tipAmount: number;
      paymentMethod?: string;
      transactionId?: string;
      splitShareId?: number;
    }, billToken?: string) => {
      try {
        const resolvedBill =
          currentBill ||
          (billToken ? await getBillByNumber(billToken) : null);

        if (resolvedBill) {
          setPaidBill(resolvedBill);
          setPaymentDetails(paymentDetails);
          setShowThankYou(true);
          setCurrentBill(null);
        }
      } catch (error) {
        console.error("Error handling payment completion:", error);
        if (currentBill) {
          setPaidBill(currentBill);
          setPaymentDetails(paymentDetails);
          setShowThankYou(true);
          setCurrentBill(null);
        }
      }
    },
    [currentBill],
  );

  const handleActiveBillChange = useCallback((hasActiveBill: boolean) => {
    if (hasActiveBill) {
      void loadTableData();
      return;
    }
    if (!currentBill) return;
    // The table's open bill disappears the moment staff settle it at the
    // counter, and this live sync fires before GuestBill's own poll can see
    // the paid status. Read the bill once by its guest token so a cashier
    // payment lands on the thank-you receipt instead of an empty table.
    const token = tryGuestBillRef(currentBill.bill);
    const clear = () => {
      setCurrentBill(null);
      toast(t("bill.billClosedMessage"));
    };
    if (!token) {
      clear();
      return;
    }
    // The sync hook keeps firing (SSE, focus, poll) until currentBill clears,
    // so skip re-entry while this bill's lookup is in flight.
    if (closingBillTokenRef.current === token) return;
    closingBillTokenRef.current = token;
    // A newer loadTableData (a new bill opened) or unmount bumps the
    // generation; a late result must not wipe that newer state.
    const generation = loadGenerationRef.current;
    const settle = (apply: () => void) => {
      if (closingBillTokenRef.current === token) {
        closingBillTokenRef.current = null;
      }
      if (loadGenerationRef.current !== generation) return;
      apply();
    };
    void getBillByNumber(token)
      .then((latest) =>
        settle(() => {
          const status = latest?.bill?.status;
          if (status === "paid" || status === "closed") {
            handleBillPaidAtCashier(latest);
            return;
          }
          clear();
        }),
      )
      .catch(() => settle(clear));
  }, [currentBill, handleBillPaidAtCashier, loadTableData, t]);

  const { connected: liveConnected } = useGuestBillSync({
    tableCode,
    hasActiveBill: Boolean(currentBill?.bill),
    onActiveBillChange: handleActiveBillChange,
    onBillEvent: () => setLiveRevision((revision) => revision + 1),
  });

  useEffect(() => {
    loadTableData();
    return () => {
      // Bumping the generation marks in-flight promises as stale so they
      // can't update React state after unmount.
      loadGenerationRef.current += 1;
    };
  }, [loadTableData]);

  // Check for payment return parameters
  useEffect(() => {
    const {
      translate,
      displayPaymentMethod,
      reloadTableData,
    } = paymentReturnDependenciesRef.current;
    if (typeof window !== "undefined") {
      const urlParams = new URLSearchParams(window.location.search);
      const paymentStatus = urlParams.get("payment");
      const paymentMethodFromStatus = urlParams.get("method");
      const paymentId = urlParams.get("payment_id");
      const paymentMethod = urlParams.get("payment_method");
      // Prefer bill_token (public_token capability); accept legacy bill_number
      // for one-release resume of in-flight redirects written before this cutover.
      const urlBillToken =
        urlParams.get("bill_token")?.trim() ||
        urlParams.get("bill_number")?.trim() ||
        "";
      const storedPaymentRaw = sessionStorage.getItem("payverge_payment");
      // Only an opaque paymentId/method/billToken reference survives in storage
      // — no amounts or bill details, to limit XSS exfiltration.
      let storedPayment: {
        paymentId?: string;
        method?: string;
        billToken?: string;
        splitShareId?: number;
      } | null = null;

      if (storedPaymentRaw) {
        try {
          const parsed = JSON.parse(storedPaymentRaw);
          if (parsed && typeof parsed === "object") {
            const legacyBillNumber =
              typeof parsed.billNumber === "string" ? parsed.billNumber : undefined;
            const preferredToken =
              typeof parsed.billToken === "string" ? parsed.billToken : undefined;
            storedPayment = {
              paymentId:
                typeof parsed.paymentId === "string" ? parsed.paymentId : undefined,
              method:
                typeof parsed.method === "string" ? parsed.method : undefined,
              billToken: preferredToken || legacyBillNumber,
              splitShareId:
                typeof parsed.splitShareId === "number" &&
                Number.isFinite(parsed.splitShareId) &&
                parsed.splitShareId > 0
                  ? parsed.splitShareId
                  : undefined,
            };
          }
        } catch (error) {
          console.error("Error parsing stored payment state:", error);
        }
      }

      const normalizedStoredBillToken = storedPayment?.billToken?.trim() || "";
      const storedPaymentMatchesBill =
        !normalizedStoredBillToken ||
        !urlBillToken ||
        normalizedStoredBillToken === urlBillToken;

      const resolvedBillToken =
        urlBillToken ||
        (storedPaymentMatchesBill ? normalizedStoredBillToken : "");
      const safeStoredPayment =
        storedPaymentMatchesBill ? storedPayment : null;

      if (paymentId && paymentMethod) {
        // Set up payment status checking. Amount/tip will be fetched from the
        // authoritative server response by PaymentStatusChecker.
        setPendingPayment({
          paymentId,
          paymentMethod,
          billToken: resolvedBillToken,
        });
        setShowPaymentStatusChecker(true);
      } else if (paymentStatus && paymentMethodFromStatus) {
        if (paymentStatus === "cancelled") {
          toast(
            translate("payment.paymentCancelled", {
              method: displayPaymentMethod(
                paymentMethodFromStatus || undefined,
              ),
            }),
          );
          if (safeStoredPayment?.splitShareId && resolvedBillToken) {
            void SplittingAPI.releaseHeldShare(
              resolvedBillToken,
              safeStoredPayment.splitShareId,
            )
              .then(() => {
                void reloadTableData();
              })
              .catch((error) => {
                console.error("Failed to release cancelled split share:", error);
              });
          }
          sessionStorage.removeItem("payverge_payment");
        } else if (paymentStatus === "success") {
          if (safeStoredPayment?.paymentId && safeStoredPayment?.method) {
            setPendingPayment({
              paymentId: safeStoredPayment.paymentId,
              paymentMethod: safeStoredPayment.method,
              billToken: resolvedBillToken,
            });
            setShowPaymentStatusChecker(true);
          } else {
            // We returned from a successful provider redirect (?payment=success)
            // but the opaque paymentId reference didn't survive in sessionStorage
            // — e.g. the tab was discarded during the external redirect, or Safari
            // ITP cleared it across the cross-site hop. The success URL carries no
            // payment_id, so we can't open the status checker; but the payment is
            // settling server-side via webhook. Refresh the bill so the paid state
            // surfaces and reassure the guest, instead of silently leaving an
            // unpaid-looking bill with live payment UI (which invites paying again).
            toast(translate("bill.paymentProcessing"));
            void reloadTableData();
          }
        }
      }

      if (paymentId || paymentMethod || paymentStatus || paymentMethodFromStatus) {
        // Clean up URL parameters
        const newUrl = window.location.pathname;
        window.history.replaceState({}, "", newUrl);
      }
    }
  }, []);

  const paymentStatusChecker =
    showPaymentStatusChecker && pendingPayment ? (
      <PaymentStatusChecker
        isOpen={showPaymentStatusChecker}
        onClose={() => {
          setShowPaymentStatusChecker(false);
          setPendingPayment(null);
        }}
        billToken={
          pendingPayment.billToken ||
          (currentBill ? tryGuestBillRef(currentBill.bill) ?? "" : "")
        }
        paymentId={pendingPayment.paymentId}
        paymentMethod={pendingPayment.paymentMethod}
        onPaymentConfirmed={(paymentDetails) => {
          sessionStorage.removeItem("payverge_payment");
          if (paymentDetails.splitShareId) {
            setShowPaymentStatusChecker(false);
            setPendingPayment(null);
            setShowThankYou(false);
            setPaidBill(null);
            setPaymentDetails(null);
            void (async () => {
              try {
                const refreshedBill = await getOpenBillByTableCode(tableCode);
                // H3: no-active-bill resolves to { bill: null } — coerce.
                setCurrentBill(refreshedBill.bill ? refreshedBill : null);
              } catch {
                void loadTableData();
              }
            })();
            return;
          }
          handlePaymentComplete(
            paymentDetails,
            pendingPayment.billToken ||
              (currentBill ? tryGuestBillRef(currentBill.bill) ?? undefined : undefined),
          );
          setShowPaymentStatusChecker(false);
          setPendingPayment(null);
        }}
      />
    ) : null;

  // Memoize business data to prevent unnecessary re-renders
  const businessData = useMemo(
    () => tableData?.business,
    [tableData?.business],
  );

  if (loading && rateLimitSeconds == null) {
    return (
      <div className="min-h-[100dvh] bg-warm-50">
        <PersistentGuestNav
          tableCode={tableCode}
          currentBill={null}
          defaultCurrency={businessCurrencies.default_currency}
          displayCurrency={businessCurrencies.display_currency}
        />

        <div className="mx-auto w-full max-w-2xl px-4 py-6 sm:px-6 sm:py-8">
          {/* Layout-matched skeleton */}
          <div className="animate-pulse space-y-4">
            <div className="h-7 w-1/3 rounded-full bg-warm-200" />
            <div className="space-y-3 rounded-2xl border border-warm-200 bg-white p-5">
              <div className="h-3 w-1/4 rounded-full bg-warm-200" />
              <div className="h-9 w-1/2 rounded-md bg-warm-200" />
              <div className="h-px w-full bg-warm-200" />
              <div className="h-4 w-2/3 rounded-full bg-warm-200" />
              <div className="h-4 w-1/2 rounded-full bg-warm-200" />
            </div>
            <div className="h-12 w-full rounded-xl bg-warm-200" />
          </div>
        </div>
        {paymentStatusChecker}
      </div>
    );
  }

  // NEW-4 / REV-4: 429 cooldown — localized busy message + auto-retry countdown
  // (or manual-retry-only after the auto-retry cap).
  if (rateLimitSeconds != null) {
    return (
      <div
        className="flex min-h-[100dvh] items-center justify-center bg-warm-50 px-6"
        data-testid="bill-rate-limit-cooldown"
        data-manual-only={rateLimitManualOnly ? "true" : "false"}
      >
        <div className="max-w-md w-full text-center">
          <p className="text-label uppercase text-ink-500 mb-3">429</p>
          <h1 className="font-title text-heading-lg text-ink-950 mb-4">
            {t("errors.billBusyTitle")}
          </h1>
          <p className="text-body text-ink-600 mb-4">{t("errors.billBusy")}</p>
          {!rateLimitManualOnly ? (
            <p
              className="text-body-sm font-semibold text-brand mb-8"
              aria-live="polite"
              data-testid="bill-rate-limit-countdown"
            >
              {t("errors.billBusyRetryIn", { seconds: rateLimitSeconds })}
            </p>
          ) : (
            <p className="text-body-sm text-ink-500 mb-8" data-testid="bill-rate-limit-manual-only">
              {t("errors.tableNotFoundRetry")}
            </p>
          )}
          <button
            type="button"
            data-testid="bill-rate-limit-retry"
            onClick={() => {
              clearRateLimitTimers();
              setRateLimitSeconds(null);
              setRateLimitManualOnly(false);
              // Manual retry does not consume the auto budget further; a
              // successful load resets the counter in loadTableData.
              void loadTableData();
            }}
            className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
          >
            {t("errors.tableNotFoundRetry")}
          </button>
        </div>
        {paymentStatusChecker}
      </div>
    );
  }

  if (!tableData) {
    // Only a CONFIRMED 404 may claim the code does not exist. Anything else
    // (network miss, empty payload, unclassified throw) is a reachability
    // problem — a transient fetch miss must never read as Mesa No Encontrada.
    const isNetwork = tableLoadError !== "not_found";
    return (
      <div className="flex min-h-[100dvh] items-center justify-center bg-warm-50 px-6">
        <div className="max-w-md w-full text-center">
          <p className="text-label uppercase text-ink-500 mb-3">
            {isNetwork ? "…" : "404"}
          </p>
          <h1 className="font-title text-heading-lg text-ink-950 mb-4">
            {isNetwork ? t("errors.networkError") : t("errors.tableNotFound")}
          </h1>
          <p className="text-body text-ink-600 mb-8">
            {isNetwork
              ? t("errors.networkErrorDescription")
              : t("errors.tableNotFoundDescription", { tableCode })}
          </p>
          <div className="flex flex-col gap-3">
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {t("errors.tableNotFoundRetry")}
            </button>
            <Link
              href="/"
              className="inline-flex items-center justify-center rounded-full border border-ink-200 bg-white px-6 py-3 text-sm font-semibold text-ink-700 transition-colors hover:bg-ink-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink-400 focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {t("errors.tableNotFoundGoHome")}
            </Link>
            {!isNetwork && (
              <p className="text-body-sm text-ink-500 mt-2">
                {t("errors.tableNotFoundHelp")}
              </p>
            )}
          </div>
        </div>
        {paymentStatusChecker}
      </div>
    );
  }

  const business = businessData;

  // Create enhanced business object with root-level fields from business endpoint
  const enhancedBusiness = business
    ? {
        ...business,
        trustpilot_enabled: businessWithTrustpilot?.trustpilot_enabled,
        trustpilot_review_url: businessWithTrustpilot?.trustpilot_review_url
          ? businessWithTrustpilot.trustpilot_review_url.replace(
              "/write-review",
              "/evaluate/",
            )
          : undefined,
      }
    : null;

  // Early return if business data is not available
  if (!enhancedBusiness) {
    return (
      <div className="flex min-h-[100dvh] items-center justify-center bg-warm-50 px-6">
        <div className="w-full max-w-md rounded-2xl border border-warm-200 bg-white p-8 text-center">
          <h2 className="font-title text-heading-lg text-ink-950 mb-2">
            {t("errors.businessNotFound")}
          </h2>
          <p className="text-body-sm text-ink-600">
            {t("errors.businessNotFoundDescription")}
          </p>
        </div>
        {paymentStatusChecker}
      </div>
    );
  }

  // Show thank you screen after payment
  if (showThankYou && paidBill) {
    return (
      <div className="min-h-[100dvh] bg-warm-50">
        <div className="mx-auto w-full max-w-2xl px-4 py-6 sm:px-6 sm:py-8">
          <div className="mb-6 text-center">
            <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-brand text-white shadow-sm">
              <svg
                className="h-8 w-8"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2.25}
                  d="M5 13l4 4L19 7"
                />
              </svg>
            </div>
            <h1 className="font-title text-display-md text-ink-950 mb-1 sm:text-display-lg">
              {t("bill.thankYouTitle")}
            </h1>
            {paymentDetails ? (
              <p className="text-body-sm text-ink-600">
                {t("bill.paymentProcessedSuccessfully")}
              </p>
            ) : (
              <p className="text-body-sm text-ink-500">
                {t("bill.paidAtCashier")}
              </p>
            )}
          </div>

          <div className="mb-5 overflow-hidden rounded-2xl border border-warm-200 bg-white">
            <header className="flex items-start justify-between gap-3 px-5 pt-5 pb-3">
              <div>
                <p className="text-label uppercase text-ink-500">
                  {t("bill.orderSummary")}
                </p>
                <p className="font-mono text-xs tabular-nums text-ink-500">
                  #{paidBill.bill.bill_number}
                </p>
              </div>
              <div className="text-end">
                <p className="text-label uppercase text-ink-500">
                  {t("bill.tableLabel")}
                </p>
                <p className="text-sm font-semibold text-ink-900">
                  {formatEntityName(
                    t("bill.tableLabel"),
                    tableData.table.name,
                  )}
                </p>
              </div>
            </header>
            <ul className="divide-y divide-warm-200/70">
              {paidBill.items.map((item, index) => (
                <li
                  key={index}
                  className="flex items-start justify-between gap-3 px-5 py-3"
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium text-ink-900">
                      {item.name}
                    </p>
                    <p className="font-mono text-xs tabular-nums text-ink-500">
                      {t("bill.qtyLabel", { quantity: item.quantity })}
                    </p>
                  </div>
                  <p className="font-mono text-sm font-semibold tabular-nums text-ink-900">
                    <CurrencyPrice
                      amount={item.subtotal}
                      fromCurrency={businessCurrencies.default_currency}
                      displayCurrency={businessCurrencies.display_currency}
                      locale={moneyLocale}
                    />
                  </p>
                </li>
              ))}
            </ul>
            <dl className="space-y-1.5 border-t border-warm-200 bg-warm-50/60 px-5 py-4 text-sm">
              <div className="flex justify-between">
                <dt className="text-ink-600">{t("bill.subtotal")}</dt>
                <dd className="font-mono tabular-nums text-ink-900">
                  <CurrencyPrice
                    amount={paidBill.bill.subtotal}
                    fromCurrency={businessCurrencies.default_currency}
                    displayCurrency={businessCurrencies.display_currency}
                    locale={moneyLocale}
                  />
                </dd>
              </div>
              {paidBill.bill.tax_amount > 0 && (
                <div className="flex justify-between">
                  <dt className="text-ink-600">{t("bill.tax")}</dt>
                  <dd className="font-mono tabular-nums text-ink-900">
                    <CurrencyPrice
                      amount={paidBill.bill.tax_amount}
                      fromCurrency={businessCurrencies.default_currency}
                      displayCurrency={businessCurrencies.display_currency}
                      locale={moneyLocale}
                    />
                  </dd>
                </div>
              )}
              {paidBill.bill.service_fee_amount > 0 && (
                <div className="flex justify-between">
                  <dt className="text-ink-600">{t("bill.serviceFee")}</dt>
                  <dd className="font-mono tabular-nums text-ink-900">
                    <CurrencyPrice
                      amount={paidBill.bill.service_fee_amount}
                      fromCurrency={businessCurrencies.default_currency}
                      displayCurrency={businessCurrencies.display_currency}
                      locale={moneyLocale}
                    />
                  </dd>
                </div>
              )}
              {/* Loyalty redemption is baked into total_amount by the backend
                  (subtotal/tax/service_fee are NOT reduced). Without this line
                  the itemized rows won't reconcile with the lower Total paid on
                  the thank-you receipt. Mirrors the in-bill GuestBill summary. */}
              {(paidBill.bill.loyalty_discount ?? 0) > 0 && (
                <div className="flex justify-between">
                  <dt className="text-emerald-700">{t("bill.loyaltyDiscount")}</dt>
                  <dd className="font-mono tabular-nums text-emerald-700">
                    {"-"}
                    <CurrencyPrice
                      amount={paidBill.bill.loyalty_discount ?? 0}
                      fromCurrency={businessCurrencies.default_currency}
                      displayCurrency={businessCurrencies.display_currency}
                      locale={moneyLocale}
                    />
                  </dd>
                </div>
              )}
              {paymentDetails && paymentDetails.tipAmount > 0 && (
                <div className="flex justify-between">
                  <dt className="text-ink-600">{t("bill.tip")}</dt>
                  <dd className="font-mono tabular-nums text-brand">
                    <CurrencyPrice
                      amount={paymentDetails.tipAmount}
                      fromCurrency={businessCurrencies.default_currency}
                      displayCurrency={businessCurrencies.display_currency}
                      locale={moneyLocale}
                    />
                  </dd>
                </div>
              )}
              <div className="flex items-center justify-between border-t border-warm-200 pt-2">
                <dt className="text-base font-semibold text-ink-900">
                  {t("bill.totalPaid")}
                </dt>
                <dd className="font-mono text-lg font-semibold tabular-nums text-ink-950">
                  <CurrencyPrice
                    amount={paymentDetails ? paymentDetails.totalPaid : paidBill.bill.total_amount}
                    fromCurrency={businessCurrencies.default_currency}
                    displayCurrency={businessCurrencies.display_currency}
                    locale={moneyLocale}
                  />
                </dd>
              </div>
            </dl>
          </div>

          {/* Loyalty capture at the highest-intent moment: members see this visit's
              points; anonymous guests get the join prompt (self-hides when signed in
              or when the business has CRM off). */}
          <LoyaltyEarnedCard
            tableCode={tableCode}
            billNumber={paidBill.bill.bill_number}
          />
          <GuestFiscalReceiptCard
            billToken={
              pendingPayment?.billToken || paidBill.bill.public_token || undefined
            }
          />
          {tableData.business.crm_enabled && (
            <div className="mb-6">
              <CRMSignupCard
                businessName={enhancedBusiness.name}
                tableCode={tableCode}
                crmEnabled={tableData.business.crm_enabled}
              />
            </div>
          )}

          {/* Receipt Generator */}
          <div className="mb-8">
            <ReceiptGenerator
              data={{
                business: {
                  name: enhancedBusiness.name,
                  logo: enhancedBusiness.logo,
                  address: enhancedBusiness.address,
                  phone: enhancedBusiness.phone,
                  tax_rate: enhancedBusiness.tax_rate,
                  service_fee_rate: enhancedBusiness.service_fee_rate,
                  timezone: enhancedBusiness.timezone,
                  google_reviews_enabled:
                    enhancedBusiness.google_reviews_enabled,
                  google_review_link: enhancedBusiness.google_review_link,
                  trustpilot_enabled: enhancedBusiness.trustpilot_enabled,
                  trustpilot_review_url: enhancedBusiness.trustpilot_review_url,
                },
                bill: {
                  bill_number: paidBill.bill.bill_number,
                  created_at: paidBill.bill.created_at,
                  subtotal: paidBill.bill.subtotal,
                  tax_amount: paidBill.bill.tax_amount,
                  service_fee_amount: paidBill.bill.service_fee_amount,
                  // Baked into total_amount by the backend; surfaced as a line
                  // so the printed receipt's items reconcile with the total.
                  loyalty_discount: paidBill.bill.loyalty_discount,
                  total_amount: paidBill.bill.total_amount,
                },
                table: {
                  name: tableData.table.name,
                },
                items: paidBill.items.map((item) => ({
                  name: item.name,
                  quantity: item.quantity,
                  subtotal: item.subtotal,
                })),
                paymentDetails: paymentDetails
                  ? {
                      totalPaid: paymentDetails.totalPaid,
                      tipAmount: paymentDetails.tipAmount || 0,
                      paymentMethod: getPaymentMethodDisplay(
                        paymentDetails.paymentMethod,
                      ),
                      transactionId: paymentDetails.transactionId,
                    }
                  : undefined,
              }}
              emailReceipt={(() => {
                const token = tryGuestBillRef(paidBill.bill);
                return token ? { billToken: token } : undefined;
              })()}
              onDownloadReceipt={(pdfBlob) => {
                // Create a download link for the PDF
                const url = URL.createObjectURL(pdfBlob);
                const a = document.createElement("a");
                a.href = url;
                a.download = `receipt-${paidBill.bill.bill_number}.pdf`;
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                URL.revokeObjectURL(url);
              }}
              currency={businessCurrencies.default_currency}
            />
          </div>

          {(enhancedBusiness.google_reviews_enabled ||
            enhancedBusiness.trustpilot_enabled) && (
            <div className="mb-6 overflow-hidden rounded-2xl border border-warm-200 bg-white">
              <p className="border-b border-warm-200 px-5 py-3 text-label uppercase text-ink-500">
                {t("bill.shareYourExperience") || "Share your experience"}
              </p>
              <div className="divide-y divide-warm-200/70">
                {enhancedBusiness.google_reviews_enabled &&
                  enhancedBusiness.google_review_link && (
                    <button
                      className="flex w-full items-center justify-between px-5 py-4 text-left transition-colors hover:bg-warm-50/60"
                      onClick={() =>
                        window.open(
                          enhancedBusiness.google_review_link,
                          "_blank",
                          "noopener,noreferrer",
                        )
                      }
                    >
                      <span className="inline-flex items-center gap-3">
                        <svg
                          className="h-5 w-5 text-ink-700"
                          fill="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" />
                          <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
                          <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
                          <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
                        </svg>
                        <span className="text-sm font-semibold text-ink-900">
                          {t("bill.leaveGoogleReview")}
                        </span>
                      </span>
                      <ArrowLeft
                        className="h-4 w-4 rotate-180 text-ink-400 rtl:rotate-0"
                        strokeWidth={1.75}
                      />
                    </button>
                  )}
                {enhancedBusiness.trustpilot_enabled &&
                  enhancedBusiness.trustpilot_review_url && (
                    <button
                      className="flex w-full items-center justify-between px-5 py-4 text-left transition-colors hover:bg-warm-50/60"
                      onClick={() =>
                        window.open(
                          enhancedBusiness.trustpilot_review_url,
                          "_blank",
                          "noopener,noreferrer",
                        )
                      }
                    >
                      <span className="inline-flex items-center gap-3">
                        <svg
                          className="h-5 w-5 text-emerald-600"
                          fill="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path d="M12 2L15.09 8.26L22 9L17 13.74L18.18 20.5L12 17.27L5.82 20.5L7 13.74L2 9L8.91 8.26L12 2Z" />
                        </svg>
                        <span className="text-sm font-semibold text-ink-900">
                          {t("bill.leaveTrustpilotReview")}
                        </span>
                      </span>
                      <ArrowLeft
                        className="h-4 w-4 rotate-180 text-ink-400 rtl:rotate-0"
                        strokeWidth={1.75}
                      />
                    </button>
                  )}
              </div>
            </div>
          )}

          {/* Actions */}
          <div className="space-y-3 text-center">
            <Button
              size="lg"
              variant="light"
              className="text-ink-600 hover:text-ink-900"
              startContent={
                <ArrowLeft
                  className="h-5 w-5 rtl:rotate-180"
                  strokeWidth={1.75}
                />
              }
              onPress={() => {
                setShowThankYou(false);
                setPaidBill(null);
              }}
            >
              {t("bill.backToTable")}
            </Button>

            <p className="text-xs text-ink-500">
              {t("bill.thankYouMessage", {
                businessName: enhancedBusiness.name,
              })}
              <br />
              {t("bill.hopeToSeeYouSoon")}
            </p>
          </div>
        </div>
      </div>
    );
  }

  if (!currentBill) {
    return (
      <div className="min-h-[100dvh] bg-warm-50">
        <header className="sticky top-0 z-40 border-b border-warm-200 bg-warm-50/85 backdrop-blur-xl">
          <div className="mx-auto flex w-full max-w-2xl items-center gap-3 px-4 py-3 sm:px-6">
            <Link
              href={`/t/${tableCode}`}
              className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-warm-200 bg-white text-ink-700 hover:border-ink-300 hover:text-ink-900"
              aria-label={t("navigation.table")}
            >
              <ArrowLeft className="h-5 w-5 rtl:rotate-180" strokeWidth={1.75} />
            </Link>

            {enhancedBusiness.logo && (
              <div className="h-9 w-9 flex-shrink-0 overflow-hidden rounded-lg border border-warm-200 bg-white">
                <Image
                  src={enhancedBusiness.logo}
                  alt={enhancedBusiness.name}
                  className="h-full w-full object-cover"
                />
              </div>
            )}
            <div className="min-w-0 flex-1">
              <p className="text-label uppercase text-ink-500">
                {t("navigation.bill")}
              </p>
              <p className="truncate font-title text-heading-sm sm:text-heading-md text-ink-950">
                {enhancedBusiness.name}
              </p>
            </div>

            <Link
              href={`/t/${tableCode}/menu`}
              className="hidden sm:inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 hover:border-ink-300 hover:text-ink-900"
            >
              <Menu className="h-4 w-4" strokeWidth={1.75} />
              {t("bill.addItems")}
            </Link>
          </div>
        </header>

        <div className="mx-auto w-full max-w-2xl px-4 py-10 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom)+1.5rem)] sm:px-6">
          <div className="rounded-2xl border border-warm-200 bg-white p-8 text-center sm:p-12">
            <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-brand/10 text-brand">
              <ScrollText className="h-7 w-7" strokeWidth={1.5} />
            </div>
            <h2 className="font-title text-heading-lg text-ink-950 mb-3">
              {t("bill.noActiveBill")}
            </h2>
            <p className="mx-auto mb-8 max-w-sm text-body-sm text-ink-600">
              {t("bill.noActiveBillDescription")}
            </p>
            <Link
              href={`/t/${tableCode}/menu`}
              className="inline-flex items-center gap-2 rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
            >
              <Menu className="h-4 w-4" strokeWidth={1.75} />
              {t("bill.browseMenu")}
            </Link>
          </div>
          <div className="pointer-events-none relative z-[60] mt-5">
            <CallWaiterButton tableCode={tableCode} />
          </div>
        </div>

        <PersistentGuestNav
          tableCode={tableCode}
          currentBill={currentBill}
          defaultCurrency={businessCurrencies.default_currency}
          displayCurrency={businessCurrencies.display_currency}
        />
        {paymentStatusChecker}
      </div>
    );
  }

  const remainingDue = Math.max(
    currentBill.bill.total_amount - currentBill.bill.paid_amount,
    0,
  );
  const isPayable =
    currentBill.bill.status === "open" ||
    currentBill.bill.status === "partial";

  return (
    <div className="min-h-[100dvh] bg-warm-50">
      <header className="sticky top-0 z-40 border-b border-warm-200 bg-warm-50/85 backdrop-blur-xl">
        <div className="mx-auto flex w-full max-w-2xl items-center gap-3 px-4 py-3 sm:px-6">
          <Link
            href={`/t/${tableCode}`}
            className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl border border-warm-200 bg-white text-ink-700 hover:border-ink-300 hover:text-ink-900"
            aria-label={t("navigation.table")}
          >
            <ArrowLeft className="h-5 w-5 rtl:rotate-180" strokeWidth={1.75} />
          </Link>
          <div className="min-w-0 flex-1">
            <p className="truncate text-label uppercase text-ink-500">
              {t("navigation.bill")}
            </p>
            {/* RSP-2: this slot is squeezed between the back link and Add Items,
                so at 390px `heading-md` (20px) ellipsized the venue's own name —
                the restaurant's brand, on the screen a paying guest looks at.
                `heading-sm` (17px) fits; the larger size returns from sm: up. */}
            <p className="truncate font-title text-heading-sm sm:text-heading-md text-ink-950">
              {enhancedBusiness.name}
            </p>
          </div>
          <Link
            href={`/t/${tableCode}/menu`}
            aria-label={t("bill.addItems")}
            className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white px-3 py-2 text-xs font-semibold text-ink-700 hover:border-ink-300 hover:text-ink-900 sm:px-4 sm:text-sm"
          >
            <Menu className="h-4 w-4" strokeWidth={1.75} />
            <span className="hidden sm:inline">{t("bill.addItems")}</span>
          </Link>
        </div>
      </header>

      <div className="mx-auto w-full max-w-2xl px-4 py-5 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom)+1.5rem)] sm:px-6 sm:py-8">
        {/* Amount-due hero — single most important number on this screen */}
        <section className="mb-5 rounded-2xl border border-warm-200 bg-white p-5 sm:p-6">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="text-label uppercase text-ink-500">
                {isPayable ? t("bill.amountToPay") : t("bill.total")}
              </h1>
              <p className="mt-1 font-mono text-display-md tabular-nums text-ink-950 sm:text-display-lg">
                <CurrencyPrice
                  amount={isPayable ? remainingDue : currentBill.bill.total_amount}
                  fromCurrency={businessCurrencies.default_currency}
                  displayCurrency={businessCurrencies.display_currency}
                  locale={moneyLocale}
                />
              </p>
              <p className="mt-1 font-mono text-xs tabular-nums text-ink-500">
                #{currentBill.bill.bill_number} ·{" "}
                {formatEntityName(
                  t("bill.tableLabel"),
                  tableData.table.name,
                )}
              </p>
            </div>
            <span
              className={`flex-shrink-0 rounded-full px-3 py-1 text-[11px] font-semibold uppercase tracking-wider ${
                currentBill.bill.status === "open"
                  ? "bg-emerald-100 text-emerald-800"
                  : currentBill.bill.status === "partial"
                    ? "bg-amber-100 text-amber-800"
                    : currentBill.bill.status === "paid"
                      ? "bg-brand/10 text-brand-dark"
                      : "bg-warm-200 text-ink-700"
              }`}
            >
              {t(`bill.billStatus.${currentBill.bill.status}`)}
            </span>
          </div>
        </section>

        <GuestBill
          bill={currentBill}
          business={enhancedBusiness}
          tableCode={tableCode}
          selectedLanguage={selectedLanguage}
          defaultCurrency={businessCurrencies.default_currency}
          displayCurrency={businessCurrencies.display_currency}
          onPaymentComplete={handlePaymentComplete}
          onBillUpdate={setCurrentBill}
          onBillPaidAtCashier={handleBillPaidAtCashier}
          liveConnected={liveConnected}
          liveRevision={liveRevision}
        />
        <div className="pointer-events-none relative z-[60] mt-5">
          <CallWaiterButton tableCode={tableCode} />
        </div>
      </div>

      <PersistentGuestNav
        tableCode={tableCode}
        currentBill={currentBill}
        defaultCurrency={businessCurrencies.default_currency}
        displayCurrency={businessCurrencies.display_currency}
      />

      {paymentStatusChecker}
    </div>
  );
}

// PG-21: GuestTranslationProvider is seeded by t/[tableCode]/layout.tsx so
// SSR nav labels match hydration. Do not wrap with an empty provider here —
// that would reset messages to English until the client load finishes.
export default function GuestBillPage() {
  return <GuestBillPageContent />;
}

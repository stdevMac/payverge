"use client";

import React, { useState, useMemo, useEffect, useCallback, useRef } from "react";
import toast from "react-hot-toast";
import { Chip, Button, Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from "@nextui-org/react";
import { Receipt, Clock, CreditCard, AlertCircle, Gift, ChevronDown } from "lucide-react";
import OrderStatusTimeline from "./OrderStatusTimeline";
import { detectOrderTransitions } from "./orderTransitions";
import { getApiErrorCode, getLocalizedApiError } from "@/utils/apiError";
import { guestCancelConflictFromError } from "./guestCancelConflict";
import {
  BillWithItemsResponse,
  BillItem,
  getBillByNumber,
  getOpenBillByTableCode,
  tryGuestBillRef,
  isActiveBillStatus,
} from "../../api/bills";
import { Business } from "../../api/business";
import { Order, OrderItem, getGuestOrdersByBillNumber, guestCancelOrder, parseOrderItems } from "../../api/orders";
import { usePolling } from "../../hooks/usePolling";
import PaymentSection from "./PaymentSection";
import { guestBillPollInterval } from "./guestBillPollInterval";
import GuestBillSplitPanel from "../splitting/GuestBillSplitPanel";
import { requestAlternativePayment } from "@/api/alternativePayments";
import { PaymentMethod, asMoneyString } from "@/types/alternativePayments";
import { guestPaymentToastMessage } from "@/components/guest/guestPaymentToast";
import { resolveBillItemTranslatedName } from "@/components/guest/billItemTranslation";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import CurrencyConverter, { CurrencyPrice } from "../common/CurrencyConverter";
import { useCustomerAuth } from "../../contexts/CustomerAuthContext";
import { crmAPI, CustomerBusiness } from "../../api/crm";
import { redeemPoints, undoRedemption, getLoyaltyRate } from "../../api/loyalty";
import {
  formatGuestRate,
  normalizeGuestLocale,
} from "@/utils/guestCurrencyFormatter";
import { asDollars } from "@/types/money";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import {
  buildBundleGroupKeys,
  isIncludedBundleChild,
  resolveBundleGroupKey,
} from "@/lib/billBundleGrouping";
// import ParticipantTracker from '../blockchain/ParticipantTracker'; // Temporarily disabled for debugging

interface GuestBillProps {
  bill: BillWithItemsResponse;
  business: Business;
  tableCode: string;
  selectedLanguage?: string;
  defaultCurrency?: string;
  displayCurrency?: string;
  onPaymentComplete: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod?: string;
    transactionId?: string;
  }) => void;
  onBillUpdate?: (updatedBill: BillWithItemsResponse) => void;
  onBillPaidAtCashier?: (bill: BillWithItemsResponse) => void;
  compact?: boolean;
  /** True while the table SSE stream is connected. Quiet polling slows to a backstop. */
  liveConnected?: boolean;
  /** Increments on each `table.bill_changed` event so the bill refetches immediately. */
  liveRevision?: number;
}

const GuestBill: React.FC<GuestBillProps> = ({
  bill,
  business,
  tableCode,
  selectedLanguage,
  defaultCurrency = "USD",
  displayCurrency = "USD",
  onPaymentComplete,
  onBillUpdate,
  onBillPaidAtCashier,
  compact = false,
  liveConnected = false,
  liveRevision,
}) => {
  const ACTIVE_GUEST_ORDER_STATUSES = useMemo(
    () => ["pending", "approved", "in_kitchen", "ready"],
    [],
  );

  const { t, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  // W2T9b: unguessable public_token for /guest/bill/:id capability routes.
  // Never fall back to bill_number (weakens the capability boundary).
  // tryGuestBillRef avoids a render throw when the open-bill projection
  // omits the token (PV-LIVE-20260720-001) — recoverable UI below.
  const billRef = tryGuestBillRef(bill.bill);
  const { customer, isAuthenticated } = useCustomerAuth();
  const [activeOrders, setActiveOrders] = useState<Order[]>([]);
  const [cancelledOrders, setCancelledOrders] = useState<Order[]>([]);
  const [_loadingOrders, setLoadingOrders] = useState(false);

  // G-6: one-time staff-cancel toast bookkeeping. Orders the guest cancelled
  // themselves (Task 20 records into guestCancelledIdsRef) never toast.
  const guestCancelledIdsRef = useRef<Set<number>>(new Set());
  const staffCancelToastedIdsRef = useRef<Set<number>>(new Set());
  const readyToastedIdsRef = useRef<Set<number>>(new Set());
  const knownOrderStatusRef = useRef<Map<number, string>>(new Map());

  // Loyalty redemption state
  const [loyaltyPoints, setLoyaltyPoints] = useState<number | null>(null);
  const [loyaltyRate, setLoyaltyRate] = useState<number>(100);
  const [isRedeeming, setIsRedeeming] = useState(false);
  const [isUndoing, setIsUndoing] = useState(false);
  const [redemptionApplied, setRedemptionApplied] = useState<{
    points: number;
    discountCents: number;
    remainingPoints: number;
  } | null>(null);
  // allOrderIds held in a ref: including it in useCallback deps would re-create
  // loadPendingOrders every poll tick, tearing down the polling interval.
  const allOrderIdsRef = useRef<string>("");
  const pollFailureToastShownRef = useRef(false);
  const orderPollFailureLoggedRef = useRef(false);
  const prevBillStatusRef = useRef<string | null>(null);
  // Generation guard for loadBillData. The 10s poll and the order-signature
  // change both call loadBillData concurrently; without this a slower earlier
  // "open" response can resolve after a newer "paid" one and clobber the fresh
  // status back to open. Because the cashier-detection effect has already seen
  // "paid", it won't re-fire — so the guest keeps seeing an already-paid bill
  // as still-due and can pay twice. Mirrors the page-level loadGenerationRef.
  const loadBillGenRef = useRef(0);
  const {
    isOpen: isCashierModalOpen,
    onOpen: onCashierModalOpen,
    onClose: onCashierModalClose,
  } = useDisclosure();
  const [cashierRequestSent, setCashierRequestSent] = useState(false);
  const [cashierRequestLoading, setCashierRequestLoading] = useState(false);
  // True while the guest holds/settles a split share; hides the full-bill
  // "Pay Now" section so they can't also pay the whole bill at an amount that
  // ignores held shares.
  const [splitShareActive, setSplitShareActive] = useState(false);
  // Progressive disclosure: split stays collapsed; kitchen/active-orders open
  // by default so diners see pending → kitchen → ready after Place Order (#79).
  const [splitExpanded, setSplitExpanded] = useState(false);
  const [kitchenExpanded, setKitchenExpanded] = useState(true);
  const [paymentBusy, setPaymentBusy] = useState(false);

  const handleCashierPayment = useCallback(
    async (details?: {
      totalPaid: number;
      tipAmount: number;
      paymentMethod: string;
    }) => {
      if (!billRef) {
        toast.error(t("bill.missingAccessTokenBody"));
        return;
      }
      if (!details || details.totalPaid <= 0) {
        toast.error(t("bill.cashierAmountInvalid"));
        return;
      }
      setCashierRequestLoading(true);
      setCashierRequestSent(false);
      try {
        const result = await requestAlternativePayment(
          billRef,
          asMoneyString(details.totalPaid),
          PaymentMethod.CASH,
          t("bill.cashierGuestLabel"),
        );
        if (result.success) {
          setCashierRequestSent(true);
          onCashierModalOpen();
        } else {
          toast.error(
            guestPaymentToastMessage(result, t) ||
              t("payment.errors.cashierRequestFailed"),
          );
        }
      } catch (err) {
        // A repeat tap while staff already hold a request for this balance:
        // show the "staff are on the way" state, not an error.
        if (getApiErrorCode(err) === "payment_request_pending") {
          setCashierRequestSent(true);
          onCashierModalOpen();
          return;
        }
        toast.error(
          guestPaymentToastMessage(err, t) ||
            t("payment.errors.cashierRequestFailed"),
        );
      } finally {
        setCashierRequestLoading(false);
      }
    },
    [billRef, onCashierModalOpen, t],
  );

  // Detect bill status transition to paid/closed (cashier payment)
  useEffect(() => {
    const currentStatus = bill.bill.status;
    const prevStatus = prevBillStatusRef.current;

    if (
      prevStatus &&
      (prevStatus === "open" || prevStatus === "partial") &&
      (currentStatus === "paid" || currentStatus === "closed")
    ) {
      onBillPaidAtCashier?.(bill);
    }

    prevBillStatusRef.current = currentStatus;
  }, [bill.bill.status, bill, onBillPaidAtCashier]);

  // Menu item translations fetched from backend
  const [itemTranslations, setItemTranslations] = useState<Record<string, Record<string, string>>>({});

  useEffect(() => {
    if (!selectedLanguage || selectedLanguage === "en") {
      setItemTranslations({});
      return;
    }
    let cancelled = false;
    // Guest bill is rendered on the unauthenticated /t/[tableCode] pages, so
    // we use the public guest endpoint instead of the authenticated
    // /inside/menu-translations route to avoid a silent 401 that leaves
    // non-English diners with untranslated menus.
    import("../../api/currency").then(({ getGuestMenuTranslations }) => {
      getGuestMenuTranslations(tableCode, selectedLanguage)
        .then((data) => {
          if (cancelled) return;
          const menuItems = data?.translations?.menu_item ?? {};
          const bundles = data?.translations?.bundle ?? {};
          const merged: Record<string, Record<string, string>> = {
            ...menuItems,
          };
          for (const [id, value] of Object.entries(bundles)) {
            if (!value || typeof value !== "object") continue;
            merged[id] = value;
            merged[`bundle:${id}`] = value;
          }
          setItemTranslations(merged);
        })
        .catch((err) => {
          if (cancelled) return;
          // Translations unavailable — fall back to original names. Logged at
          // warn level so ops can distinguish "guest translated menu is
          // unavailable" from "translations just don't exist."
          console.warn("[GuestBill] guest menu translations unavailable:", err?.message);
        });
    });
    return () => { cancelled = true; };
  }, [selectedLanguage, tableCode]);

  // Load updated bill data
  const loadBillData = useCallback(async () => {
    if (!onBillUpdate || !billRef) return true;

    const gen = ++loadBillGenRef.current;
    try {
      const latestBill = await getBillByNumber(billRef);
      // Discard a stale response: only the latest load may publish the bill,
      // so an older "open" can't overwrite a newer "paid".
      if (loadBillGenRef.current !== gen) return true;
      onBillUpdate(latestBill);
      return true;
    } catch (error) {
      try {
        const updatedBill = await getOpenBillByTableCode(tableCode, {
          disableCache: true,
        });
        if (loadBillGenRef.current !== gen) return true;
        // No-active-bill now resolves to { bill: null } (H3). Publishing that
        // would hand consumers a truthy bill-less object — keep the legacy
        // "don't update" behavior instead.
        if (updatedBill.bill) {
          onBillUpdate(updatedBill);
        }
        // else: the fallback succeeded but found no active bill (paid/closed)
        // — an expected terminal state, not an error worth logging.
        return true;
      } catch (fallbackError) {
        console.error("Error loading bill by number:", error);
        console.error("Error loading updated bill:", fallbackError);
        return false;
      }
    }
  }, [billRef, tableCode, onBillUpdate]);

  // Load all orders for this bill (pending and cancelled)
  const loadPendingOrders = useCallback(
    async (isInitialLoad = false) => {
      if (!billRef) return;
      if (isInitialLoad) {
        setLoadingOrders(true);
      }

      try {
        const ordersResponse = await getGuestOrdersByBillNumber(billRef, {
          disableCache: true,
        });
        const orders = ordersResponse.orders || [];
        orderPollFailureLoggedRef.current = false;

        // G-6: render the full active lifecycle, not just pending.
        const active = orders.filter((order) =>
          ACTIVE_GUEST_ORDER_STATUSES.includes(order.status),
        );
        const cancelled = orders.filter(
          (order) => order.status === "cancelled",
        );

        // One-time toasts on observed transitions (pure detector; refs own
        // dedup). Guest-triggered cancels and orders already in a terminal
        // state at mount never toast.
        const transitions = detectOrderTransitions(
          orders,
          knownOrderStatusRef.current,
        );
        transitions.staffCancelled.forEach((orderId) => {
          if (
            guestCancelledIdsRef.current.has(orderId) ||
            staffCancelToastedIdsRef.current.has(orderId)
          ) {
            return;
          }
          staffCancelToastedIdsRef.current.add(orderId);
          toast(t("orders.staffCancelledToast"), {
            icon: "⚠️",
            duration: 8000,
          });
        });
        transitions.becameReady.forEach((orderId) => {
          if (readyToastedIdsRef.current.has(orderId)) return;
          readyToastedIdsRef.current.add(orderId);
          toast(t("orders.readyToast"), { icon: "🔔", duration: 8000 });
          // Haptic nudge where supported; feature-guarded — vibrate is absent
          // on iOS Safari and in jsdom.
          if (
            typeof navigator !== "undefined" &&
            typeof navigator.vibrate === "function"
          ) {
            navigator.vibrate(200);
          }
        });
        orders.forEach((order) => {
          knownOrderStatusRef.current.set(order.id, order.status);
        });

        // Create a unique signature of all order IDs and statuses
        const currentOrderSignature = orders
          .map((o) => `${o.id}:${o.status}`)
          .sort()
          .join(",");

        // Check if ANY order status changed (not just count)
        if (
          !isInitialLoad &&
          allOrderIdsRef.current &&
          currentOrderSignature !== allOrderIdsRef.current
        ) {
          // Orders changed status, immediately refresh bill data
          await loadBillData();
        }

        setActiveOrders(active);
        setCancelledOrders(cancelled);
        allOrderIdsRef.current = currentOrderSignature;
      } catch (error) {
        if (!orderPollFailureLoggedRef.current) {
          console.warn("Order status is temporarily unavailable:", error);
          orderPollFailureLoggedRef.current = true;
        }
        // Do NOT clobber the last known orders list — a transient fetch
        // failure would otherwise flash an empty list to the guest. Re-throw
        // so usePolling can count repeated failures and notify the user.
        throw error;
      } finally {
        if (isInitialLoad) {
          setLoadingOrders(false);
        }
      }
    },
    [billRef, loadBillData, t, ACTIVE_GUEST_ORDER_STATUSES],
  );

  // Initial load of pending orders — swallow here so initial load never
  // rejects the effect; subsequent polls surface failures via onRepeatedFailure.
  useEffect(() => {
    loadPendingOrders(true).catch(() => undefined);
  }, [loadPendingOrders]);

  // Silent polling for pending orders (no loading state)
  const silentLoadPendingOrders = useCallback(() => {
    return loadPendingOrders(false);
  }, [loadPendingOrders]);

  // Synchronized polling for all updates (orders and bill)
  const pollAllUpdates = useCallback(async () => {
    // Bill totals are the critical source of truth. Order status is useful
    // supporting context, but a temporary failure there must not mark a
    // successfully refreshed bill as stale or discard a rejected-order total.
    const [, billResult] = await Promise.allSettled([
      silentLoadPendingOrders(),
      loadBillData(),
    ]);
    if (billResult.status === "rejected" || billResult.value === false) {
      throw billResult.status === "rejected"
        ? billResult.reason
        : new Error("Bill refresh failed");
    }
  }, [silentLoadPendingOrders, loadBillData]);

  const splitFallbackPollInterval = guestBillPollInterval({
    isActive: isActiveBillStatus(bill.bill.status),
    splitPanelOpen: splitExpanded || splitShareActive,
    paymentInFlight:
      paymentBusy || cashierRequestLoading || cashierRequestSent,
    sseConnected: Boolean(liveConnected),
  });

  // Default 10s. Poll at 3s only while the bill is active and the split panel
  // is open (including an active split share) or a payment is in flight. When
  // the live table stream is connected, quiet polling slows to a 30s backstop
  // because that stream already pushes bill lifecycle and payment changes.
  // A liveRevision bump refetches immediately. Hidden tabs still pause via usePolling.
  // On repeated failure, surface a toast once so guests know their bill view may
  // be stale before checkout.
  const handlePollFailure = useCallback(() => {
    if (pollFailureToastShownRef.current) return;
    pollFailureToastShownRef.current = true;
    toast.error(
      t("bill.pollFailure") ||
        "We're having trouble keeping this bill up to date. Please refresh.",
      { duration: 8000 },
    );
  }, [t]);

  usePolling({
    callback: pollAllUpdates,
    interval: splitFallbackPollInterval,
    enabled: true,
    immediate: false,
    onRepeatedFailure: handlePollFailure,
    failureThreshold: 3,
  });

  // Skip the initial render and undefined/0 so mounting does not double-fetch.
  // Later liveRevision bumps (one per table.bill_changed) refresh at once.
  const liveRevisionPrev = useRef(liveRevision);
  const liveRevisionMounted = useRef(false);
  useEffect(() => {
    const prev = liveRevisionPrev.current;
    liveRevisionPrev.current = liveRevision;
    if (!liveRevisionMounted.current) {
      liveRevisionMounted.current = true;
      return;
    }
    if (liveRevision == null || liveRevision === 0 || prev === liveRevision) {
      return;
    }
    pollAllUpdates().catch(() => undefined);
  }, [liveRevision, pollAllUpdates]);

  // Cancel a pending order on behalf of the guest (G-7: two-tap arm-to-confirm)
  // + in-flight lock). The first tap arms the control ("Tap again to cancel");
  // a second tap within ~3s performs the cancel. Auto-disarms after the window
  // so a stray first tap never leaves the button primed. Mirrors the
  // arm-to-confirm clear-cart pattern in CartModal, avoiding window.confirm.
  const [cancellingOrderIds, setCancellingOrderIds] = useState<Set<number>>(
    new Set(),
  );
  const [armedCancelOrderId, setArmedCancelOrderId] = useState<number | null>(
    null,
  );
  const cancelArmTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (cancelArmTimerRef.current) clearTimeout(cancelArmTimerRef.current);
    },
    [],
  );
  const performGuestCancel = useCallback(
    async (orderId: number) => {
      setCancellingOrderIds((prev) => new Set(prev).add(orderId));
      try {
        await guestCancelOrder(tableCode, orderId);
        guestCancelledIdsRef.current.add(orderId);
        toast.success(t("orders.cancelled"));
        await loadPendingOrders();
      } catch (error: unknown) {
        // Always refresh: a 409 means kitchen state moved; the stale Pending
        // row + Cancel control must not linger under the toast (#528).
        const key = guestCancelConflictFromError(error);
        // Mark before refresh so a pending→cancelled snap does not also toast
        // "staff cancelled" on a guest-initiated conflict.
        if (key !== "orders.cancelFailed") {
          guestCancelledIdsRef.current.add(orderId);
        }
        await Promise.allSettled([loadPendingOrders(), loadBillData()]);
        toast.error(t(key));
      } finally {
        setCancellingOrderIds((prev) => {
          const next = new Set(prev);
          next.delete(orderId);
          return next;
        });
      }
    },
    [tableCode, loadPendingOrders, loadBillData, t],
  );
  const handleGuestCancel = useCallback(
    (orderId: number) => {
      // Second tap within the arm window: perform the cancel.
      if (armedCancelOrderId === orderId) {
        if (cancelArmTimerRef.current) clearTimeout(cancelArmTimerRef.current);
        setArmedCancelOrderId(null);
        void performGuestCancel(orderId);
        return;
      }
      // First tap: arm this order and auto-disarm after 3s.
      if (cancelArmTimerRef.current) clearTimeout(cancelArmTimerRef.current);
      setArmedCancelOrderId(orderId);
      cancelArmTimerRef.current = setTimeout(
        () => setArmedCancelOrderId(null),
        3000,
      );
    },
    [armedCancelOrderId, performGuestCancel],
  );

  // Translate item names using backend translations when available.
  // Prefer menu_item_id over bill-line UUID so names match the guest menu
  // (GUEST-002). See resolveBillItemTranslatedName for type-aware fallbacks.
  const getTranslatedItemName = useCallback(
    (item: BillItem) =>
      resolveBillItemTranslatedName(item, itemTranslations),
    [itemTranslations],
  );

  // Load guest's loyalty points for this business
  useEffect(() => {
    if (!isAuthenticated || !customer) {
      setLoyaltyPoints(null);
      return;
    }
    let cancelled = false;
    crmAPI.getBusinesses().then((businesses: CustomerBusiness[]) => {
      if (cancelled) return;
      const match = businesses.find((b) => b.business_id === business.id);
      setLoyaltyPoints(match ? match.loyalty_points : 0);
    }).catch(() => {
      if (!cancelled) setLoyaltyPoints(null);
    });
    getLoyaltyRate(tableCode).then((rate) => {
      if (!cancelled && rate > 0) setLoyaltyRate(rate);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, [isAuthenticated, customer, business.id, tableCode]);

  // Guest payloads emit loyalty_discount as dollars (0 when none). Derive
  // applied/undo from that field so a reload does not hide Undo or re-offer
  // Redeem against a bill that already has a redemption.
  const loyaltyDiscount = bill.bill.loyalty_discount ?? 0;
  const hasLoyaltyDiscount = loyaltyDiscount > 0;

  const handleRedeemPoints = useCallback(async () => {
    if (loyaltyPoints === null || loyaltyPoints <= 0) return;
    setIsRedeeming(true);
    try {
      // Only redeem the points the bill is actually worth. Redeeming the whole
      // balance against a smaller bill would spend points whose value exceeds the
      // bill; the backend clamps the discount at the bill total, so the surplus
      // points would be consumed for no benefit. Cap the request at what the
      // remaining owed can absorb (ceil so the discount fully covers it).
      const remainingOwed = Math.max(
        bill.bill.total_amount - bill.bill.paid_amount,
        0,
      );
      const pointsNeeded = Math.ceil(remainingOwed * loyaltyRate);
      const pointsToRedeem =
        pointsNeeded > 0 ? Math.min(loyaltyPoints, pointsNeeded) : loyaltyPoints;
      const result = await redeemPoints(tableCode, pointsToRedeem, bill.bill.bill_number);
      setRedemptionApplied({
        points: result.points_deducted,
        discountCents: result.discount_cents,
        remainingPoints: result.remaining_points,
      });
      setLoyaltyPoints(result.remaining_points);
      // Refresh bill to reflect the applied discount
      if (onBillUpdate && billRef) {
        try {
          const updated = await getBillByNumber(billRef);
          onBillUpdate(updated);
        } catch {
          // non-fatal: discount chip is shown regardless
        }
      }
    } catch (err: unknown) {
      // Localized in the diner's selected storefront language; coded backend
      // errors resolve via apiErrors.json, uncoded ones keep the raw backend
      // string, and a string-less throw degrades to a generic localized message.
      toast.error(getLocalizedApiError(err, currentLanguage));
    } finally {
      setIsRedeeming(false);
    }
  }, [
    loyaltyPoints,
    loyaltyRate,
    tableCode,
    bill.bill.bill_number,
    billRef,
    bill.bill.total_amount,
    bill.bill.paid_amount,
    onBillUpdate,
    currentLanguage,
  ]);

  const handleUndoRedemption = useCallback(async () => {
    setIsUndoing(true);
    try {
      await undoRedemption(tableCode, bill.bill.bill_number);
      // Restore points locally. After a reload there is no session redeem
      // record, so estimate from the payload dollars × rate.
      const pointsToRestore =
        redemptionApplied?.points ?? Math.round(loyaltyDiscount * loyaltyRate);
      if (pointsToRestore > 0) {
        setLoyaltyPoints((prev) => (prev ?? 0) + pointsToRestore);
      }
      setRedemptionApplied(null);
      if (onBillUpdate && billRef) {
        try {
          const updated = await getBillByNumber(billRef);
          onBillUpdate(updated);
        } catch {
          // non-fatal
        }
      }
    } catch (err: unknown) {
      toast.error(getLocalizedApiError(err, currentLanguage));
    } finally {
      setIsUndoing(false);
    }
  }, [
    tableCode,
    bill.bill.bill_number,
    billRef,
    redemptionApplied,
    loyaltyDiscount,
    loyaltyRate,
    onBillUpdate,
    currentLanguage,
  ]);

  const formatDate = (dateString: string) => {
    // Guest-selected language for numerals/month names, but the *venue*
    // timezone for the wall clock. Device TZ (e.g. Asia/Dubai agent host)
    // otherwise shows 5:14 AM for a 9:14 PM New York order — same honesty
    // rule as OpenClosedPill (FIND-013).
    return formatBusinessDateTime(
      dateString,
      selectedLanguage || currentLanguage || "en",
      business.timezone ?? null,
      DATE_TIME_SHORT,
    );
  };

  const getItemType = useCallback((item: BillItem) => item.item_type || item.itemType || "menu_item", []);

  const itemTypeBadge = useCallback((itemType: string) => {
    switch (itemType) {
      case "bundle":
        return { label: t("bill.itemTypeBundle"), color: "primary" as const };
      case "bundle_item":
        return { label: t("bill.itemTypeBundleItem"), color: "secondary" as const };
      case "discount":
        return { label: t("bill.itemTypeDiscount"), color: "success" as const };
      default:
        return { label: t("bill.itemTypeMenuItem"), color: "default" as const };
    }
  }, [t]);

  const billItems = useMemo(() => bill.items || [], [bill.items]);
  const displayItems = useMemo(
    () => billItems.filter((item: BillItem) => {
      const type = getItemType(item);
      return type !== "discount" && type !== "bundle_item";
    }),
    [billItems, getItemType],
  );
  const discountItems = useMemo(
    () => billItems.filter((item: BillItem) => getItemType(item) === "discount"),
    [billItems, getItemType],
  );
  const billItemsForSplitting = useMemo(
    () =>
      displayItems.map((item) => ({
        id: item.id,
        name: getTranslatedItemName(item),
        price: item.price,
        quantity: item.quantity,
        subtotal: item.subtotal,
        item_type: item.item_type,
      })),
    [displayItems, getTranslatedItemName],
  );
  // Key children by bundle_occurrence_id (or reconstructed legacy occurrence)
  // so repeated Date Night lines do not nest every child's qty under every parent.
  const bundleGroupKeys = useMemo(
    () => buildBundleGroupKeys(billItems),
    [billItems],
  );
  const bundleChildrenByParent = useMemo(
    () =>
      billItems
        .filter((item: BillItem) => getItemType(item) === "bundle_item")
        .reduce(
          (acc: Record<string, BillItem[]>, item: BillItem) => {
            const key = resolveBundleGroupKey(item, bundleGroupKeys);
            if (!key) return acc;
            if (!acc[key]) acc[key] = [];
            acc[key].push(item);
            return acc;
          },
          {},
        ),
    [billItems, getItemType, bundleGroupKeys],
  );

  // Clamp at zero to match the page-level amount-due hero. This value drives the
  // PaymentSection charge base, the tip presets, and the cashier-modal amount,
  // so an overpaid/edge partial bill (paid_amount > total_amount) must not show
  // a negative "Pay Now"/tip or forward a negative amount downstream.
  const remainingAmount = useMemo(
    () => Math.max(bill.bill.total_amount - bill.bill.paid_amount, 0),
    [bill.bill.total_amount, bill.bill.paid_amount],
  );
  // Subtotal-proportioned tip base for the full-bill path, so tip presets are
  // computed pre-tax/pre-fee — matching heldShareTipBaseAmount in
  // GuestBillSplitPanel so both the split and full-bill paths tip on the same
  // (pre-tax/pre-fee) base. Money stays float64 dollars.
  const fullBillTipBaseAmount = useMemo(() => {
    const subtotal = Math.max(0, bill.bill.subtotal ?? 0);
    const gross =
      subtotal +
      Math.max(0, bill.bill.tax_amount ?? 0) +
      Math.max(0, bill.bill.service_fee_amount ?? 0);
    if (subtotal > 0 && gross > 0) {
      return Math.min(remainingAmount, remainingAmount * (subtotal / gross));
    }
    return remainingAmount;
  }, [
    bill.bill.subtotal,
    bill.bill.tax_amount,
    bill.bill.service_fee_amount,
    remainingAmount,
  ]);
  const isPayableBill = isActiveBillStatus(bill.bill.status);

  // PV-LIVE-20260720-001: missing capability must not uncaught-throw into
  // TableErrorBoundary. Guests keep a recoverable surface (refresh / staff).
  if (!billRef) {
    return (
      <div
        className="rounded-2xl border border-amber-200 bg-amber-50 px-5 py-6 text-center"
        data-testid="guest-bill-missing-token"
        role="alert"
      >
        <AlertCircle
          className="mx-auto mb-3 h-8 w-8 text-amber-700"
          strokeWidth={1.75}
        />
        <h3 className="text-base font-semibold text-ink-900">
          {t("bill.missingAccessTokenTitle")}
        </h3>
        <p className="mt-2 text-sm text-ink-600">
          {t("bill.missingAccessTokenBody")}
        </p>
        <Button
          className="mt-4 rounded-xl bg-brand font-semibold text-white"
          onPress={() => {
            if (typeof window !== "undefined") {
              window.location.reload();
            }
          }}
        >
          {t("bill.refreshBill")}
        </Button>
      </div>
    );
  }

  // Convert bill data for splitting component (use stable IDs and memoize)

  return (
    <div className="space-y-4 sm:space-y-5">
      {/* Items list — single card, divider rows, no per-item card overuse */}
      <div className="overflow-hidden rounded-2xl border border-warm-200 bg-white">
        <header className="flex items-center justify-between px-5 pt-5 pb-3">
          <h3 className="text-label uppercase text-ink-500">
            {t("bill.orderItems")}
          </h3>
          <span className="text-xs text-ink-500">
            <Clock className="me-1 inline h-3 w-3 align-[-2px]" strokeWidth={1.75} />
            {formatDate(bill.bill.created_at)}
          </span>
        </header>
        <ul className="divide-y divide-warm-200/70">
          {displayItems.map((item, index) => {
            const type = getItemType(item);
            const badge = itemTypeBadge(type);
            const parentKey = resolveBundleGroupKey(item, bundleGroupKeys);
            const bundleChildren = parentKey
              ? bundleChildrenByParent[parentKey] || []
              : [];

            return (
              <li key={item.id || index} className="px-5 py-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <p className="text-sm font-semibold text-ink-900">
                        {getTranslatedItemName(item)}
                      </p>
                      {type !== "menu_item" && (
                        <Chip size="sm" variant="flat" color={badge.color}>
                          {badge.label}
                        </Chip>
                      )}
                    </div>
                    <p className="mt-0.5 font-mono text-xs tabular-nums text-ink-500">
                      {item.quantity} ×{" "}
                      <CurrencyPrice
                        amount={item.price}
                        fromCurrency={defaultCurrency}
                        displayCurrency={displayCurrency}
                      locale={moneyLocale}
                  />
                    </p>
                  </div>
                  <p className="flex-shrink-0 font-mono text-sm font-semibold tabular-nums text-ink-900">
                    <CurrencyPrice
                      amount={item.subtotal}
                      fromCurrency={defaultCurrency}
                      displayCurrency={displayCurrency}
                    locale={moneyLocale}
                  />
                  </p>
                </div>

                {bundleChildren.length > 0 && (
                  <ul className="mt-2 space-y-1 border-s-2 border-brand/20 ps-3">
                    {bundleChildren.map((child: BillItem, childIndex: number) => {
                      const included = isIncludedBundleChild(child);
                      return (
                      <li
                        key={child.id || `${index}-child-${childIndex}`}
                        className="flex justify-between text-xs text-ink-600"
                      >
                        <span>
                          {child.quantity || 1} × {getTranslatedItemName(child)}
                        </span>
                        <span className="font-mono tabular-nums text-ink-500">
                          {included ? (
                            t("bill.includedInBundle")
                          ) : (
                            <CurrencyPrice
                              amount={child.subtotal || 0}
                              fromCurrency={defaultCurrency}
                              displayCurrency={displayCurrency}
                              locale={moneyLocale}
                            />
                          )}
                        </span>
                      </li>
                      );
                    })}
                  </ul>
                )}
              </li>
            );
          })}
        </ul>

        {discountItems.length > 0 && (
          <div className="space-y-1 border-t border-warm-200 bg-emerald-50/60 px-5 py-3">
            <p className="text-label uppercase text-emerald-800">
              {t("bill.appliedOffers")}
            </p>
            {discountItems.map((discount: BillItem, index: number) => (
              <div
                key={`discount-${discount.id || index}`}
                className="flex items-center justify-between text-sm"
              >
                <span className="text-emerald-800">
                  {discount.name || t("bill.discount")}
                </span>
                <span className="font-mono font-semibold tabular-nums text-emerald-800">
                  <CurrencyPrice
                    amount={discount.subtotal || discount.price || 0}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  />
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Bill Summary */}
      <div className="rounded-2xl border border-warm-200 bg-white p-5 sm:p-6">
        <h3 className="mb-4 text-label uppercase text-ink-500">
          {t("bill.summary")}
        </h3>
        <dl className="space-y-2 text-sm">
          <div className="flex justify-between">
            <dt className="text-ink-600">{t("bill.subtotal")}</dt>
            <dd className="font-mono tabular-nums text-ink-900">
              <CurrencyPrice
                amount={bill.bill.subtotal}
                fromCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
              locale={moneyLocale}
                  />
            </dd>
          </div>

          {bill.bill.tax_amount > 0 && (
            <div className="flex justify-between">
              <dt className="text-ink-600">
                {t("bill.tax", {
                  rate: formatGuestRate(business.tax_rate, moneyLocale),
                })}
              </dt>
              <dd className="font-mono tabular-nums text-ink-900">
                <CurrencyPrice
                  amount={bill.bill.tax_amount}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                locale={moneyLocale}
                  />
              </dd>
            </div>
          )}

          {bill.bill.service_fee_amount > 0 && (
            <div className="flex justify-between">
              <dt className="text-ink-600">
                {t("bill.serviceFee", {
                  rate: formatGuestRate(business.service_fee_rate, moneyLocale),
                })}
              </dt>
              <dd className="font-mono tabular-nums text-ink-900">
                <CurrencyPrice
                  amount={bill.bill.service_fee_amount}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                locale={moneyLocale}
                  />
              </dd>
            </div>
          )}

          {(bill.bill.loyalty_discount ?? 0) > 0 && (
            <div className="flex justify-between">
              <dt className="text-emerald-700">
                {t("bill.loyaltyDiscount")}
              </dt>
              <dd className="font-mono tabular-nums text-emerald-700">
                {"-"}
                <CurrencyPrice
                  amount={bill.bill.loyalty_discount ?? 0}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                locale={moneyLocale}
                  />
              </dd>
            </div>
          )}

          <div className="flex items-center justify-between border-t border-warm-200 pt-3">
            <dt className="text-base font-semibold text-ink-900">
              {t("bill.total")}
            </dt>
            <dd className="text-end">
              <CurrencyConverter
                amount={bill.bill.total_amount}
                fromCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
                showUSDCConversion={true}
                // PG-8 / PG-20: same locale as line items so guest bill does
                // not mix en-US ($67.27) with es (68,00 US$) formats.
                locale={moneyLocale}
              />
            </dd>
          </div>

          {bill.bill.paid_amount > 0 && (
            <>
              <div className="flex justify-between">
                <dt className="text-ink-600">{t("bill.paid")}</dt>
                <dd className="font-mono tabular-nums text-emerald-700">
                  <CurrencyPrice
                    amount={bill.bill.paid_amount}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  />
                </dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-ink-600">{t("bill.remaining")}</dt>
                <dd
                  className={`font-mono tabular-nums font-semibold ${
                    remainingAmount > 0 ? "text-amber-700" : "text-emerald-700"
                  }`}
                >
                  <CurrencyPrice
                    amount={remainingAmount}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  />
                </dd>
              </div>
            </>
          )}

          {bill.bill.tip_amount > 0 && (
            <div className="flex justify-between">
              <dt className="text-ink-600">{t("bill.tip")}</dt>
              <dd className="font-mono tabular-nums text-brand">
                <CurrencyPrice
                  amount={bill.bill.tip_amount}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                locale={moneyLocale}
                  />
              </dd>
            </div>
          )}
        </dl>
      </div>

      {/* Participant Tracker - Temporarily disabled for debugging */}
      {/* {bill.bill.status === 'open' && (
        <ParticipantTracker
          billId={bill.bill.id}
          totalAmount={bill.bill.total_amount * 1000000} // Convert to wei (6 decimals)
          className="mb-6"
          refreshInterval={5000} // Refresh every 5 seconds
        />
      )} */}

      {/* Loyalty Redemption Card — shown above payment methods */}
      {isPayableBill && isAuthenticated && loyaltyPoints !== null && loyaltyPoints > 0 && !redemptionApplied && !hasLoyaltyDiscount && (() => {
        // Show only what the guest will actually get and spend. The discount can
        // never exceed the amount still owed (the backend clamps it there), so a
        // full-balance estimate would over-promise and imply the surplus points
        // are spent. Cap the displayed discount at the remaining amount and the
        // points at what that discount is worth.
        const pointsNeeded = Math.ceil(remainingAmount * loyaltyRate);
        const pointsToRedeem =
          pointsNeeded > 0 ? Math.min(loyaltyPoints, pointsNeeded) : loyaltyPoints;
        const estimatedDiscount = (pointsToRedeem / loyaltyRate).toFixed(2);
        return (
          <div className="overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/60">
            <div className="flex items-center gap-3 px-5 py-4">
              <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-amber-100">
                <Gift className="h-4 w-4 text-amber-700" strokeWidth={1.75} />
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-amber-900">
                  {t("bill.loyaltyPoints", { count: loyaltyPoints })}
                </p>
                <p className="text-xs text-amber-700">
                  {(() => {
                    const sentence = t("bill.loyaltyEstimate", { amount: "__AMT__" });
                    const [before, after] = sentence.split("__AMT__");
                    return (
                      <>
                        {before}
                        <CurrencyPrice
                          amount={Number(estimatedDiscount)}
                          fromCurrency={defaultCurrency}
                          displayCurrency={displayCurrency}
                        locale={moneyLocale}
                  />
                        {after ?? ""}
                      </>
                    );
                  })()}
                </p>
              </div>
              <Button
                size="sm"
                isLoading={isRedeeming}
                onPress={handleRedeemPoints}
                className="flex-shrink-0 rounded-lg bg-amber-600 text-xs font-semibold text-white hover:bg-amber-700"
              >
                {(() => {
                  const sentence = t("bill.redeemPoints", {
                    points: pointsToRedeem,
                    discount: "__AMT__",
                  });
                  const [before, after] = sentence.split("__AMT__");
                  return (
                    <>
                      {before}
                      <CurrencyPrice
                        amount={Number(estimatedDiscount)}
                        fromCurrency={defaultCurrency}
                        displayCurrency={displayCurrency}
                      locale={moneyLocale}
                  />
                      {after ?? ""}
                    </>
                  );
                })()}
              </Button>
            </div>
          </div>
        );
      })()}

      {/* Loyalty Applied Chip — session redeem OR payload after reload */}
      {isPayableBill && isAuthenticated && (redemptionApplied || hasLoyaltyDiscount) && (
        <div className="overflow-hidden rounded-2xl border border-emerald-200 bg-emerald-50/60">
          <div className="flex items-center gap-3 px-5 py-4">
            <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-emerald-100">
              <Gift className="h-4 w-4 text-emerald-700" strokeWidth={1.75} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-emerald-900">
                {(() => {
                  const appliedPoints =
                    redemptionApplied?.points ??
                    Math.round(loyaltyDiscount * loyaltyRate);
                  const appliedDiscount =
                    redemptionApplied != null
                      ? redemptionApplied.discountCents / 100
                      : loyaltyDiscount;
                  const sentence = t("bill.pointsApplied", {
                    points: appliedPoints,
                    discount: "__AMT__",
                  });
                  const [before, after] = sentence.split("__AMT__");
                  return (
                    <>
                      {before}
                      <CurrencyPrice
                        amount={appliedDiscount}
                        fromCurrency={defaultCurrency}
                        displayCurrency={displayCurrency}
                      locale={moneyLocale}
                  />
                      {after ?? ""}
                    </>
                  );
                })()}
              </p>
            </div>
            <Button
              size="sm"
              variant="light"
              isLoading={isUndoing}
              onPress={handleUndoRedemption}
              className="flex-shrink-0 rounded-lg text-xs font-semibold text-emerald-700"
            >
              {t("bill.undoRedemption")}
            </Button>
          </div>
        </div>
      )}

      {/* Kitchen / active orders — open by default so pending→kitchen→ready is
          obvious after Place Order; sits above pay so it isn't buried. */}
      {activeOrders.length > 0 && (
        <div
          data-testid="guest-bill-kitchen-disclosure"
          className="overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/60"
        >
          <button
            type="button"
            data-testid="guest-bill-kitchen-toggle"
            aria-expanded={kitchenExpanded}
            onClick={() => setKitchenExpanded((v) => !v)}
            className="flex w-full items-center justify-between gap-3 px-5 py-4 text-start transition-colors hover:bg-amber-50"
          >
            <span className="inline-flex items-center gap-2 text-sm font-semibold text-amber-900">
              <Clock className="h-3.5 w-3.5" strokeWidth={1.75} />
              {t("bill.activeOrders")} ({activeOrders.length})
            </span>
            <ChevronDown
              className={`h-4 w-4 flex-shrink-0 text-amber-800 transition-transform ${
                kitchenExpanded ? "rotate-180" : ""
              }`}
              strokeWidth={1.75}
              aria-hidden
            />
          </button>
          {kitchenExpanded ? (
            <>
              <ul className="divide-y divide-amber-200/70 border-t border-amber-200 bg-white">
                {activeOrders.map((order) => (
                  <li key={order.id} className="px-4 py-3">
                    <OrderStatusTimeline status={order.status} t={t} />
                    <ul className="mt-2 space-y-1">
                      {parseOrderItems(order.items).map(
                        (item: OrderItem, itemIndex: number) => (
                          <li
                            key={`${order.id}-${itemIndex}`}
                            className="flex items-start justify-between gap-3"
                          >
                            <div className="min-w-0 flex-1">
                              <p className="text-sm font-medium text-ink-900">
                                <span className="text-ink-500">
                                  {item.quantity} ×
                                </span>{" "}
                                {resolveBillItemTranslatedName(
                                  {
                                    id: item.id,
                                    menu_item_id: item.menu_item_id || "",
                                    name: item.menu_item_name,
                                    item_type: item.item_type,
                                  },
                                  itemTranslations,
                                )}
                              </p>
                              {item.special_requests && (
                                <p className="mt-0.5 text-xs text-ink-500">
                                  {t("bill.specialRequests")}:{" "}
                                  {item.special_requests}
                                </p>
                              )}
                            </div>
                            <span className="font-mono text-sm tabular-nums text-ink-500">
                              <CurrencyPrice
                                amount={item.subtotal}
                                fromCurrency={defaultCurrency}
                                displayCurrency={displayCurrency}
                                locale={moneyLocale}
                              />
                            </span>
                          </li>
                        ),
                      )}
                    </ul>
                    {order.status === "pending" &&
                      (() => {
                        const cancelling = cancellingOrderIds.has(order.id);
                        const armed = armedCancelOrderId === order.id;
                        return (
                          <button
                            type="button"
                            onClick={() => handleGuestCancel(order.id)}
                            disabled={cancelling}
                            aria-label={t("orders.cancel")}
                            className={`mt-2 inline-flex min-h-11 items-center rounded-lg px-3 py-2 text-xs font-semibold transition-colors ${
                              cancelling
                                ? "cursor-not-allowed text-ink-400"
                                : armed
                                  ? "bg-rose-600 text-white hover:bg-rose-700"
                                  : "text-rose-600 underline hover:bg-rose-50"
                            }`}
                          >
                            {cancelling
                              ? "…"
                              : armed
                                ? t("orders.cancelConfirmTap")
                                : t("orders.cancel")}
                          </button>
                        );
                      })()}
                  </li>
                ))}
              </ul>
              <p className="border-t border-amber-200 px-5 py-3 text-xs text-amber-800">
                {t("bill.activeOrdersNote")}
              </p>
            </>
          ) : null}
        </div>
      )}

      {/* Payment primary — after kitchen status so "kitchen got it" is visible. */}
      {isPayableBill && !splitShareActive && (
        <PaymentSection
          billId={bill.bill.id}
          billToken={billRef}
          businessId={business.id}
          amount={remainingAmount}
          businessName={business.name}
          businessAddress={bill.bill.settlement_address}
          tipAddress={bill.bill.tipping_address}
          tableCode={tableCode}
          defaultCurrency={defaultCurrency}
          displayCurrency={displayCurrency}
          tipBaseAmount={fullBillTipBaseAmount}
          onPaymentComplete={(paymentDetails: {
            totalPaid: number;
            tipAmount: number;
            paymentMethod: string;
            transactionId?: string;
          }) => {
            // Forward the full detail object so the receipt labels the real
            // payment method and carries the transaction reference instead of
            // defaulting to "Crypto (USDC)" with an empty tx id.
            onPaymentComplete({
              totalPaid: paymentDetails.totalPaid,
              tipAmount: paymentDetails.tipAmount,
              paymentMethod: paymentDetails.paymentMethod,
              transactionId: paymentDetails.transactionId,
            });
          }}
          onCashierPayment={handleCashierPayment}
          cashierPaymentLoading={cashierRequestLoading}
          onPaymentBusyChange={setPaymentBusy}
        />
      )}

      {/* Split bill — collapsed by default (disclosure). */}
      {isPayableBill && (
        <div
          data-testid="guest-bill-split-disclosure"
          className="overflow-hidden rounded-2xl border border-warm-200 bg-white"
        >
          <button
            type="button"
            data-testid="guest-bill-split-toggle"
            aria-expanded={splitExpanded}
            onClick={() => setSplitExpanded((v) => !v)}
            className="flex w-full items-center justify-between gap-3 px-5 py-4 text-start transition-colors hover:bg-warm-50"
          >
            <span className="text-sm font-semibold text-ink-900">
              {t("bill.splitBill") || "Split bill"}
            </span>
            <ChevronDown
              className={`h-4 w-4 flex-shrink-0 text-ink-500 transition-transform ${
                splitExpanded ? "rotate-180" : ""
              }`}
              strokeWidth={1.75}
              aria-hidden
            />
          </button>
          {splitExpanded ? (
            <div className="border-t border-warm-200 px-3 pb-4 pt-2 sm:px-4">
              <GuestBillSplitPanel
                billId={bill.bill.id}
                billToken={billRef}
                businessId={business.id}
                businessName={business.name}
                businessAddress={bill.bill.settlement_address}
                tipAddress={bill.bill.tipping_address}
                tableCode={tableCode}
                remainingAmount={asDollars(remainingAmount)}
                billSubtotal={bill.bill.subtotal}
                billTaxAmount={bill.bill.tax_amount}
                billServiceFeeAmount={bill.bill.service_fee_amount}
                defaultCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
                defaultExpanded
                items={billItemsForSplitting}
                onPaymentComplete={(paymentDetails) => {
                  onPaymentComplete({
                    totalPaid: paymentDetails.totalPaid,
                    tipAmount: paymentDetails.tipAmount,
                    paymentMethod: paymentDetails.paymentMethod,
                    transactionId: paymentDetails.transactionId,
                  });
                }}
                onBillRefresh={async () => {
                  await loadBillData();
                }}
                onActiveChange={setSplitShareActive}
              />
            </div>
          ) : null}
        </div>
      )}

      {/* Cancelled Orders — single card, row list */}
      {cancelledOrders.length > 0 && (
        <div className="overflow-hidden rounded-2xl border border-rose-200 bg-rose-50/60">
          <header className="flex items-center justify-between px-5 pt-4 pb-2">
            <h3 className="inline-flex items-center gap-2 text-label uppercase text-rose-800">
              <AlertCircle className="h-3.5 w-3.5" strokeWidth={1.75} />
              {t("bill.cancelledOrders")}
            </h3>
          </header>
          <ul className="divide-y divide-rose-200/70 bg-white">
            {cancelledOrders.map((order) => (
              <li key={order.id} className="px-4 py-3">
                <ul className="space-y-1">
                  {parseOrderItems(order.items).map(
                    (item: OrderItem, itemIndex: number) => (
                      <li
                        key={`${order.id}-${itemIndex}`}
                        className="flex items-start justify-between gap-3"
                      >
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium text-ink-700 line-through">
                            <span className="sr-only">
                              {t("orders.cancelled")}:{" "}
                            </span>
                            <span className="text-ink-600">
                              {item.quantity} ×
                            </span>{" "}
                            {item.menu_item_name}
                          </p>
                        </div>
                        <span className="font-mono text-sm tabular-nums text-ink-600 line-through">
                          <CurrencyPrice
                            amount={item.subtotal}
                            fromCurrency={defaultCurrency}
                            displayCurrency={displayCurrency}
                            locale={moneyLocale}
                          />
                        </span>
                      </li>
                    ),
                  )}
                </ul>
                {order.cancel_reason && (
                  <p className="mt-1.5 text-xs text-ink-500">
                    <span className="font-medium">
                      {t("orders.cancelReasonLabel")}:{" "}
                    </span>
                    {order.cancel_reason}
                  </p>
                )}
              </li>
            ))}
          </ul>
          <p className="border-t border-rose-200 px-5 py-3 text-xs text-rose-800">
            {t("bill.cancelledOrdersNote")}
          </p>
        </div>
      )}

      {/* Bill Status Messages */}
      {bill.bill.status === "paid" && (
        <div
          data-testid="guest-bill-paid-status"
          role="status"
          aria-live="polite"
          className="bg-emerald-50 border border-emerald-200 rounded-2xl p-8 text-center shadow-sm"
        >
          <span className="sr-only">{t("bill.paidAnnouncement")}</span>
          <div className="w-16 h-16 bg-emerald-100 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <Receipt className="w-8 h-8 text-emerald-600" />
          </div>
          <h3 className="text-heading-md font-title text-ink-900 tracking-wide mb-2">
            {t("bill.billPaid")}
          </h3>
          <p className="text-ink-600">{t("bill.thankYou")}</p>
        </div>
      )}

      {bill.bill.status === "closed" && (
        <div className="bg-warm-50 border border-warm-200 rounded-2xl p-8 text-center shadow-sm">
          <div className="w-16 h-16 bg-warm-100 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <Receipt className="w-8 h-8 text-ink-500" />
          </div>
          <h3 className="text-heading-md font-title text-ink-800 tracking-wide mb-2">
            {t("bill.billClosed")}
          </h3>
          <p className="text-ink-500">
            {t("bill.billClosedMessage")}
          </p>
        </div>
      )}

      {/* Cashier Payment Modal */}
      <Modal
        isOpen={isCashierModalOpen}
        onClose={() => {
          setCashierRequestSent(false);
          onCashierModalClose();
        }}
        size="lg"
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            {t("bill.payWithCashier")}
          </ModalHeader>
          <ModalBody>
            <div className="text-center space-y-4">
              <div className="w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center mx-auto">
                <CreditCard className="w-8 h-8 text-brand" />
              </div>

              <div>
                <h3 className="text-lg font-semibold text-ink-900 mb-2">
                  {t("bill.pleasePayAtCashier")}
                </h3>
                <p className="text-ink-600 mb-4">
                  {t("bill.cashierInstructions")}
                </p>
              </div>

              <div className="bg-warm-50 rounded-lg p-4">
                <div className="flex justify-between items-center">
                  <span className="text-ink-600">
                    {t("bill.amountToPay")}:
                  </span>
                  <span className="text-xl font-bold text-ink-900">
                    <CurrencyPrice
                      amount={remainingAmount}
                      fromCurrency={defaultCurrency}
                      displayCurrency={displayCurrency}
                    locale={moneyLocale}
                  />
                  </span>
                </div>
                <div className="flex justify-between items-center mt-2">
                  <span className="text-sm text-ink-500">
                    {t("bill.billNumber", { number: bill.bill.bill_number })}
                  </span>
                </div>
              </div>

              <div className="text-sm text-ink-500">
                {cashierRequestSent
                  ? t("bill.cashierRequestSentNote")
                  : t("bill.cashierUpdateNote")}
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button
              color="primary"
              onPress={onCashierModalClose}
              className="w-full"
            >
              {t("bill.gotIt")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
};

export default GuestBill;

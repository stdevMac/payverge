"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import toast from "react-hot-toast";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
} from "@nextui-org/react";
import {
  CheckCircle2,
  CreditCard,
  ListChecks,
  ReceiptText,
  Users,
  WalletCards,
} from "lucide-react";
import PaymentSection from "../guest/PaymentSection";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import {
  getSplitEventsURL,
  useSplittingAPI,
  type SplitShare,
  type SplitShareReceipt,
  type SplitState,
} from "@/api/splitting";
import { requestAlternativePayment } from "@/api/alternativePayments";
import type { BillItem } from "@/api/bills";
import { asDollars, type Dollars } from "@/types/money";
import { asMoneyString, PaymentMethod } from "@/types/alternativePayments";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { CurrencyPrice } from "../common/CurrencyConverter";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import { guestPaymentToastMessage } from "@/components/guest/guestPaymentToast";
import {
  guestPaymentErrorInfo,
  presentSplitHoldError,
} from "@/lib/guestPaymentErrors";
import { splitTenderLabel } from "@/lib/paymentMethodLabels";
import { equalPreviewDollars } from "@/utils/equalSplitPreview";

// Upper bound on the guest split stream's reconnect backoff (mirrors the
// operator SharedSSEConnection): keep retrying forever, just stop escalating.
const SPLIT_STREAM_MAX_BACKOFF_MS = 30_000;
const MAX_SPLIT_PEOPLE = 100;

type SplitMode = "equal" | "items" | "custom";
type FractionOption = "1" | "1/2" | "1/3" | "1/4" | "custom";

interface GuestBillSplitPanelProps {
  billId: number;
  billToken: string;
  businessId: number;
  businessName: string;
  businessAddress: string;
  tipAddress: string;
  tableCode: string;
  remainingAmount: Dollars;
  billSubtotal?: Dollars;
  billTaxAmount?: Dollars;
  billServiceFeeAmount?: Dollars;
  defaultCurrency: string;
  displayCurrency: string;
  items?: Pick<BillItem, "id" | "name" | "price" | "quantity" | "subtotal" | "item_type">[];
  onPaymentComplete: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId?: string;
  }) => void;
  onBillRefresh?: () => Promise<void> | void;
  // Reports whether the guest is currently engaged in a split share (holding an
  // active hold or viewing a settled receipt). The parent uses this to hide the
  // full-bill "Pay Now" section — paying the whole bill while holding a partial
  // share is contradictory and its amount ignores everyone's held shares.
  onActiveChange?: (active: boolean) => void;
  /**
   * When the parent already wraps this panel in a disclosure (GuestBill),
   * start expanded so guests do not have to tap "Split Bill" twice.
   */
  defaultExpanded?: boolean;
}

const fractionOptions: FractionOption[] = ["1", "1/2", "1/3", "1/4"];

function parseFractionValue(fraction: string) {
  const trimmed = fraction.trim();
  if (trimmed.includes("/")) {
    const [numerator, denominator] = trimmed.split("/").map((part) => Number(part));
    if (
      Number.isFinite(numerator) &&
      Number.isFinite(denominator) &&
      numerator > 0 &&
      denominator > 0
    ) {
      return numerator / denominator;
    }
    return null;
  }
  const decimal = Number(trimmed);
  if (Number.isFinite(decimal) && decimal > 0) {
    return decimal;
  }
  return null;
}

function fractionToNumber(fraction: string) {
  const parsed = parseFractionValue(fraction);
  if (parsed !== null) return parsed;
  if (fraction === "1/2") return 0.5;
  if (fraction === "1/3") return 1 / 3;
  if (fraction === "1/4") return 0.25;
  return 1;
}

function gcd(a: number, b: number): number {
  while (b !== 0) {
    const next = a % b;
    a = b;
    b = next;
  }
  return Math.abs(a);
}

function formatFractionAmount(value: number) {
  const clamped = Math.max(0, Math.min(1, value));
  const commonFractions = [
    { value: 1, label: "1" },
    { value: 3 / 4, label: "3/4" },
    { value: 2 / 3, label: "2/3" },
    { value: 1 / 2, label: "1/2" },
    { value: 1 / 3, label: "1/3" },
    { value: 1 / 4, label: "1/4" },
  ];
  const match = commonFractions.find((candidate) => Math.abs(candidate.value - clamped) < 0.01);
  if (match) return match.label;
  const numerator = Math.max(1, Math.round(clamped * 100));
  const divisor = gcd(numerator, 100);
  return `${numerator / divisor}/${100 / divisor}`;
}

function clampFractionToRemaining(fraction: string, remainingFraction: number) {
  const trimmed = fraction.trim();
  const parsed = parseFractionValue(trimmed);
  const available = Math.max(0, remainingFraction);
  if (parsed !== null && parsed <= available + 0.001) {
    return trimmed;
  }
  return formatFractionAmount(Math.min(parsed ?? 1, available));
}

function paymentMethodToSplitTender(method: string) {
  if (method === "usdc_payment") return "crypto";
  if (method === "cross_chain_payment") return "cross-chain";
  if (method === "cashier") return "cash";
  return "plugin";
}

function formatHoldCountdown(seconds: number) {
  const safeSeconds = Math.max(0, seconds);
  const minutes = Math.floor(safeSeconds / 60);
  const remainingSeconds = safeSeconds % 60;
  return `${minutes}:${String(remainingSeconds).padStart(2, "0")}`;
}

function clampWholeNumber(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min;
  return Math.min(max, Math.max(min, Math.trunc(value)));
}

export default function GuestBillSplitPanel({
  billId,
  billToken,
  businessId,
  businessName,
  businessAddress,
  tipAddress,
  tableCode,
  remainingAmount,
  billSubtotal,
  billTaxAmount,
  billServiceFeeAmount,
  defaultCurrency,
  displayCurrency,
  items = [],
  onPaymentComplete,
  onBillRefresh,
  onActiveChange,
  defaultExpanded = false,
}: GuestBillSplitPanelProps) {
  const { t, currentLanguage } = useGuestTranslation();
  const onBillRefreshRef = useRef(onBillRefresh);
  onBillRefreshRef.current = onBillRefresh;
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const splitting = useSplittingAPI();
  // Offer discount lines and combo children are not claimable — the backend
  // folds discounts into positive menu bases, and PG-24 requires claim-parent
  // semantics for bundles (children stay kitchen-only). Filter both so a
  // stale options payload cannot break item-mode preview/holds.
  const claimableItems = useMemo(
    () =>
      items.filter((item) => {
        const subtotal = Number(item.subtotal);
        if (!Number.isFinite(subtotal) || subtotal <= 0) return false;
        const itemType = String(item.item_type ?? "").toLowerCase();
        return itemType !== "discount" && itemType !== "bundle_item";
      }),
    [items],
  );
  // GuestBill wraps this panel in its own disclosure; when true we skip a
  // second "Split Bill" gate (audit FIND-017). Standalone / unit tests leave
  // the default collapsed so existing click-to-expand flows stay stable.
  const [expanded, setExpanded] = useState(Boolean(defaultExpanded));
  const [mode, setMode] = useState<SplitMode>("equal");
  const [displayName, setDisplayName] = useState("");
  // String-backed so clearing the field mid-edit does not snap back to 1
  // (Number("") → 0 → clamp → 1). Numeric consumers normalize via clampWholeNumber.
  const [numPeople, setNumPeople] = useState("2");
  const [sharesCovered, setSharesCovered] = useState("1");
  const [customAmount, setCustomAmount] = useState("");
  const [coverRemaining, setCoverRemaining] = useState(false);
  const [selectedFraction, setSelectedFraction] = useState<FractionOption>("1");
  const [customFraction, setCustomFraction] = useState("1/2");
  const [selectedItemIDs, setSelectedItemIDs] = useState<string[]>([]);
  const [selectedFractions, setSelectedFractions] = useState<Record<string, string>>({});
  const [liveState, setLiveState] = useState<SplitState | null>(null);
  const [heldShare, setHeldShare] = useState<SplitShare | null>(null);
  const [settledReceipt, setSettledReceipt] = useState<SplitShareReceipt | null>(null);
  const [isHolding, setIsHolding] = useState(false);
  const [isReleasingShare, setIsReleasingShare] = useState(false);
  // Cashier confirmation modal parity with the full-bill path (GuestBill).
  const [cashierModalOpen, setCashierModalOpen] = useState(false);
  const [cashierRequestAmount, setCashierRequestAmount] = useState<number | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const liveReceiptLoadingShareIDRef = useRef<number | null>(null);
  const recoveredBillNumberRef = useRef<string | null>(null);

  // Tell the parent when the guest is engaged in a split share so it can hide
  // the full-bill "Pay Now" section (which would otherwise let them pay the
  // whole bill while holding a partial share, at an amount that ignores held
  // shares). Fires on hold/settle/release transitions.
  const splitShareActive = Boolean(heldShare || settledReceipt);
  useEffect(() => {
    onActiveChange?.(splitShareActive);
  }, [splitShareActive, onActiveChange]);

  useEffect(() => {
    if (typeof EventSource === "undefined") return;

    // The guest split stream must survive drops: EventSource's built-in retry
    // gives up permanently on a non-200/fatal close, which used to freeze the
    // guest's split view silently. We manage the lifecycle ourselves — always
    // tear down on error and recreate with capped, jittered exponential
    // backoff, refetching the split/bill state on every recovery to cover
    // updates that fired during the gap.
    let cancelled = false;
    let source: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let attempts = 0;

    const applyPayload = (event: MessageEvent) => {
      try {
        const payload = JSON.parse(event.data) as {
          data?: SplitState;
          state?: SplitState;
          bill_number?: string;
        };
        // Stream is already scoped by billToken in the EventSource URL; do not
        // compare state.bill_number (display number) to the public_token.
        const nextState = payload.data ?? payload.state ?? (payload.bill_number ? (payload as SplitState) : null);
        if (nextState) {
          setLiveState(nextState);
        }
      } catch {
        // Ignore malformed stream frames; polling still refreshes the bill.
      }
    };

    const connect = () => {
      if (cancelled) return;
      const es = new EventSource(getSplitEventsURL(billToken), { withCredentials: true });
      source = es;
      es.addEventListener("connected", (event: MessageEvent) => {
        const isRecoveryFromDrop = attempts > 0;
        attempts = 0;
        applyPayload(event);
        // Splits that settled during the gap aren't replayed — refetch.
        if (isRecoveryFromDrop) void onBillRefreshRef.current?.();
      });
      es.addEventListener("bill.split.updated", applyPayload);
      // The server lost a frame for this bill (buffer overflow) and resent the
      // authoritative split state; refetch the bill too, since a settlement
      // may be among the lost frames.
      es.addEventListener("sync.reset", (event: MessageEvent) => {
        applyPayload(event);
        void onBillRefreshRef.current?.();
      });
      es.onerror = () => {
        es.close();
        if (source === es) source = null;
        void onBillRefreshRef.current?.();
        if (cancelled) return;
        attempts += 1;
        const ceiling = Math.min(1000 * 2 ** (attempts - 1), SPLIT_STREAM_MAX_BACKOFF_MS);
        const delay = ceiling / 2 + Math.random() * (ceiling / 2);
        retryTimer = setTimeout(() => {
          retryTimer = null;
          connect();
        }, delay);
      };
    };

    connect();

    return () => {
      cancelled = true;
      if (retryTimer) {
        clearTimeout(retryTimer);
        retryTimer = null;
      }
      source?.close();
      source = null;
    };
  }, [billToken]);

  useEffect(() => {
    if (recoveredBillNumberRef.current === billToken) return;
    recoveredBillNumberRef.current = billToken;
    let cancelled = false;

    void (async () => {
      try {
        const shares = await splitting.getMySplitShares(billToken);
        if (cancelled || heldShare || settledReceipt || shares.length === 0) return;

        const held = shares.find((share) => share.status === "held");
        if (held) {
          setHeldShare(held);
          setExpanded(true);
          return;
        }

        const settled = shares.find((share) => share.status === "settled");
        if (!settled) return;
        liveReceiptLoadingShareIDRef.current = settled.id;
        const receipt = await splitting.getSplitShareReceipt(billToken, settled.id);
        if (cancelled) return;
        setSettledReceipt(receipt);
        setExpanded(true);
      } catch (error) {
        if (!cancelled) {
          console.error("Failed to recover guest split shares:", error);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [billToken, heldShare, settledReceipt, splitting]);

  // PG-30: keep typed values visible; normalize only for math/hold, and surface
  // out-of-range reasons instead of silently snapping 0 → 1 or uncapped 100+.
  const peopleFieldEmpty = numPeople.trim() === "";
  const peopleParsed = Number(numPeople);
  const peopleOutOfRange =
    mode === "equal" &&
    !peopleFieldEmpty &&
    (!Number.isFinite(peopleParsed) ||
      !Number.isInteger(peopleParsed) ||
      peopleParsed < 1 ||
      peopleParsed > MAX_SPLIT_PEOPLE);
  const normalizedNumPeople = peopleOutOfRange
    ? 0
    : clampWholeNumber(peopleParsed, 1, MAX_SPLIT_PEOPLE);
  const sharesFieldEmpty = sharesCovered.trim() === "";
  const sharesParsed = Number(sharesCovered);
  const sharesOutOfRange =
    mode === "equal" &&
    !peopleOutOfRange &&
    normalizedNumPeople > 0 &&
    !sharesFieldEmpty &&
    (!Number.isFinite(sharesParsed) ||
      !Number.isInteger(sharesParsed) ||
      sharesParsed < 1 ||
      sharesParsed > normalizedNumPeople);
  const normalizedSharesCovered =
    peopleOutOfRange || sharesOutOfRange
      ? 0
      : clampWholeNumber(sharesParsed, 1, Math.max(1, normalizedNumPeople));

  // PG-23: pure dual of BE seat count + floor re-split. nowMs is wall-clock
  // (ticked while any live equal hold has an expiry, not only local heldShare).
  const equalPreview = useMemo(() => {
    if (peopleOutOfRange || sharesOutOfRange || peopleFieldEmpty || sharesFieldEmpty) {
      return asDollars(0);
    }
    const requestedPeople = clampWholeNumber(Number(numPeople), 1, MAX_SPLIT_PEOPLE);
    const availableCents =
      liveState?.available_cents ?? Math.round(Number(remainingAmount) * 100);
    const covered = clampWholeNumber(Number(sharesCovered), 1, requestedPeople);
    return asDollars(
      equalPreviewDollars({
        availableCents,
        requestedPeople,
        sharesCovered: covered,
        shares: liveState?.shares,
        nowMs,
      }),
    );
  }, [
    liveState?.available_cents,
    liveState?.shares,
    nowMs,
    numPeople,
    peopleFieldEmpty,
    peopleOutOfRange,
    remainingAmount,
    sharesCovered,
    sharesFieldEmpty,
    sharesOutOfRange,
  ]);

  const liveAvailableAmount = asDollars((liveState?.available_cents ?? Math.round(Number(remainingAmount) * 100)) / 100);
  const activeSelectedFraction = selectedFraction === "custom" ? customFraction : selectedFraction;
  const payerRoster = useMemo(
    () => (liveState?.shares ?? []).filter((share) => share.status === "held" || share.status === "settled"),
    [liveState?.shares],
  );
  const claimedFractionByItem = useMemo(() => {
    const claimed: Record<string, number> = {};
    for (const share of liveState?.shares ?? []) {
      if (share.status !== "held" && share.status !== "settled") continue;
      for (const itemID of share.claimed_item_ids ?? []) {
        claimed[itemID] =
          (claimed[itemID] ?? 0) +
          fractionToNumber(share.claimed_fractions?.[itemID] || "1");
      }
    }
    return claimed;
  }, [liveState?.shares]);
  const availableItemSubtotal = useMemo(
    () =>
      claimableItems.reduce((total, item) => {
        const remainingFraction = Math.max(0, 1 - (claimedFractionByItem[item.id] ?? 0));
        return total + Number(item.subtotal) * remainingFraction;
      }, 0),
    [claimedFractionByItem, claimableItems],
  );

  // PG-25: keep the raw parse separate from the hold amount so over-balance
  // and non-positive inputs can explain themselves inline instead of silently
  // clamping / disabling the hold button with no reason.
  const rawCustomParsed = parseLocaleDecimal(customAmount);
  const customFieldEmpty = customAmount.trim() === "";
  const customIsNonPositive =
    mode === "custom" &&
    !coverRemaining &&
    !customFieldEmpty &&
    (!Number.isFinite(rawCustomParsed) || rawCustomParsed <= 0);
  const customIsOverRemaining =
    mode === "custom" &&
    !coverRemaining &&
    !customFieldEmpty &&
    Number.isFinite(rawCustomParsed) &&
    rawCustomParsed > Number(liveAvailableAmount) + 0.001;
  const customAmountValue = asDollars(
    Math.min(
      Math.max(0, Number.isFinite(rawCustomParsed) ? rawCustomParsed : 0),
      Number(liveAvailableAmount),
    ),
  );
  const itemPreview = useMemo(() => {
    const selected = new Set(selectedItemIDs);
    const selectedSubtotal = claimableItems.reduce((total, item) => {
      if (!selected.has(item.id)) return total;
      return total + Number(item.subtotal) * fractionToNumber(selectedFractions[item.id] || "1");
    }, 0);
    // Denominator is gross positive claimable subtotal (discounts already
    // excluded). Proportional share of live available mirrors backend
    // net-base allocation: available * selectedGross / remainingGross.
    const denominator =
      availableItemSubtotal > 0
        ? availableItemSubtotal
        : Number(billSubtotal) > 0
          ? Number(billSubtotal)
          : claimableItems.reduce((total, item) => total + Math.max(0, Number(item.subtotal)), 0);
    const proportionalShare =
      denominator > 0
        ? Number(liveAvailableAmount) * (selectedSubtotal / denominator)
        : selectedSubtotal;
    return asDollars(Math.min(Math.max(0, proportionalShare), Number(liveAvailableAmount)));
  }, [availableItemSubtotal, billSubtotal, claimableItems, liveAvailableAmount, selectedFractions, selectedItemIDs]);
  const priorPaidAmount = useMemo(() => {
    const gross =
      Number(billSubtotal ?? 0) +
      Number(billTaxAmount ?? 0) +
      Number(billServiceFeeAmount ?? 0);
    return Math.max(0, gross - Number(remainingAmount));
  }, [billServiceFeeAmount, billSubtotal, billTaxAmount, remainingAmount]);
  const hasPriorPayment = priorPaidAmount > 0.009;
  const selectedAmount =
    mode === "equal"
      ? equalPreview
      : mode === "items"
        ? itemPreview
        : coverRemaining
          ? liveAvailableAmount
          : customAmountValue;
  const heldShareTipBaseAmount = useMemo(() => {
    if (!heldShare) return undefined;

    const shareAmount = Math.max(0, Number(heldShare.amount));
    if (heldShare.mode === "items") {
      const itemByID = new Map(claimableItems.map((item) => [item.id, item]));
      const itemSubtotal = (heldShare.claimed_item_ids ?? []).reduce((total, itemID) => {
        const item = itemByID.get(itemID);
        if (!item) return total;
        return total + Math.max(0, Number(item.subtotal)) * fractionToNumber(heldShare.claimed_fractions?.[itemID] || "1");
      }, 0);
      if (itemSubtotal > 0) {
        return asDollars(Math.min(itemSubtotal, shareAmount));
      }
    }

    const subtotal = Math.max(0, Number(billSubtotal ?? 0));
    const gross = subtotal + Math.max(0, Number(billTaxAmount ?? 0)) + Math.max(0, Number(billServiceFeeAmount ?? 0));
    if (subtotal > 0 && gross > 0) {
      return asDollars(Math.min(shareAmount, shareAmount * (subtotal / gross)));
    }
    return asDollars(shareAmount);
  }, [billServiceFeeAmount, billSubtotal, billTaxAmount, heldShare, claimableItems]);
  const canHoldShare =
    mode === "items"
      ? selectedItemIDs.length > 0
      : mode === "custom"
        ? coverRemaining || (!customIsNonPositive && selectedAmount > 0)
        : !peopleOutOfRange &&
          !sharesOutOfRange &&
          !peopleFieldEmpty &&
          !sharesFieldEmpty &&
          selectedAmount > 0;

  const toggleItem = (itemID: string) => {
    setSelectedItemIDs((current) => {
      if (current.includes(itemID)) {
        setSelectedFractions((fractions) => {
          const next = { ...fractions };
          delete next[itemID];
          return next;
        });
        return current.filter((id) => id !== itemID);
      }
      const remainingFraction = Math.max(0, 1 - (claimedFractionByItem[itemID] ?? 0));
      if (remainingFraction <= 0.001) return current;
      const nextFraction = clampFractionToRemaining(activeSelectedFraction, remainingFraction);
      setSelectedFractions((fractions) => ({ ...fractions, [itemID]: nextFraction }));
      return [...current, itemID];
    });
  };

  const handleHoldShare = async () => {
    if (!canHoldShare) {
      toast.error(t("bill.splitEnterAmount"));
      return;
    }
    setIsHolding(true);
    try {
      liveReceiptLoadingShareIDRef.current = null;
      setSettledReceipt(null);
      const claimedFractions = selectedItemIDs.reduce<Record<string, string>>((acc, itemID) => {
        acc[itemID] = selectedFractions[itemID] || "1";
        return acc;
      }, {});
      const response = await splitting.createSplitHold(billToken, {
        mode,
        display_name: displayName.trim() || undefined,
        amount: mode === "custom" && !coverRemaining ? selectedAmount : undefined,
        cover_remaining: mode === "custom" && coverRemaining ? true : undefined,
        num_people: mode === "equal" ? normalizedNumPeople : undefined,
        shares_covered: mode === "equal" ? normalizedSharesCovered : undefined,
        claimed_item_ids: mode === "items" ? selectedItemIDs : undefined,
        claimed_fractions: mode === "items" ? claimedFractions : undefined,
      });
      setHeldShare(response.share);
      toast.success(t("bill.splitShareHeld"));
    } catch (error) {
      console.error("Failed to hold split share:", error);
      toast.error(t(presentSplitHoldError(guestPaymentErrorInfo(error)).messageKey));
    } finally {
      setIsHolding(false);
    }
  };

  const handleSharePaymentComplete = async (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId?: string;
  }) => {
    if (!heldShare) {
      onPaymentComplete(paymentDetails);
      await onBillRefresh?.();
      return;
    }

    let settledShareID = heldShare?.id ?? null;
    let canLoadReceipt = false;
    if (paymentDetails.transactionId) {
      try {
        const settlement = await splitting.executeHeldShare(billToken, {
          share_id: heldShare.id,
          payment_method: paymentMethodToSplitTender(paymentDetails.paymentMethod),
          transaction_hash: paymentDetails.transactionId,
          tip_amount: asDollars(paymentDetails.tipAmount),
        });
        settledShareID = settlement.share.id;
        canLoadReceipt = settlement.share.status === "settled";
        toast.success(t("bill.splitShareSettled"));
      } catch (error) {
        console.error("Failed to settle split share:", error);
        toast.error(t("bill.splitSettleFailed"));
      }
    }
    if (settledShareID && canLoadReceipt) {
      try {
        const receipt = await splitting.getSplitShareReceipt(billToken, settledShareID);
        setSettledReceipt(receipt);
        setHeldShare(null);
      } catch (error) {
        console.error("Failed to load split share receipt:", error);
      }
    }
    await onBillRefresh?.();
  };

  // In-flight lock for the share cashier request. Mirrors the full-bill
  // wiring (GuestBill.tsx cashierRequestLoading): without it a double-tap
  // files duplicate cash requests before the first response lands.
  const [isCashierRequestInFlight, setIsCashierRequestInFlight] =
    useState(false);

  const handleShareCashierPayment = async (paymentDetails?: {
    tipAmount?: number;
  }) => {
    if (!heldShare || isCashierRequestInFlight) return;

    const tipAmount = Math.max(
      0,
      Math.round((paymentDetails?.tipAmount ?? 0) * 100) / 100,
    );
    setIsCashierRequestInFlight(true);
    try {
      const result = await requestAlternativePayment(
        billToken,
        asMoneyString(Number(heldShare.amount)),
        PaymentMethod.CASH,
        heldShare.display_name,
        heldShare.id,
        tipAmount > 0 ? asMoneyString(tipAmount) : "0.00",
      );
      if (result.success) {
        setCashierRequestAmount(Number(heldShare.amount));
        setCashierModalOpen(true);
        await onBillRefresh?.();
        return;
      }
      toast.error(
        guestPaymentToastMessage(result, t) || t("bill.contactBusiness"),
      );
    } catch (err) {
      toast.error(
        guestPaymentToastMessage(err, t) || t("bill.contactBusiness"),
      );
    } finally {
      setIsCashierRequestInFlight(false);
    }
  };

  const handleReleaseShare = async () => {
    if (!heldShare || isReleasingShare) return;
    setIsReleasingShare(true);
    try {
      const response = await splitting.releaseHeldShare(billToken, heldShare.id);
      setLiveState(response.state);
      setHeldShare(null);
      liveReceiptLoadingShareIDRef.current = null;
      toast.success(t("bill.splitShareReleased"));
      await onBillRefresh?.();
    } catch (error) {
      console.error("Failed to release split share:", error);
      toast.error(t("bill.splitReleaseFailed"));
    } finally {
      setIsReleasingShare(false);
    }
  };

  useEffect(() => {
    if (!heldShare || settledReceipt) return;

    const liveShare = liveState?.shares.find((share) => share.id === heldShare.id);
    if (!liveShare) return;

    if (liveShare.status === "settled") {
      if (liveReceiptLoadingShareIDRef.current === heldShare.id) return;
      liveReceiptLoadingShareIDRef.current = heldShare.id;
      void (async () => {
        try {
          const receipt = await splitting.getSplitShareReceipt(billToken, heldShare.id);
          if (liveReceiptLoadingShareIDRef.current !== heldShare.id) return;
          setSettledReceipt(receipt);
          setHeldShare(null);
          toast.success(t("bill.splitShareSettled"));
          await onBillRefresh?.();
        } catch (error) {
          if (liveReceiptLoadingShareIDRef.current !== heldShare.id) return;
          liveReceiptLoadingShareIDRef.current = null;
          console.error("Failed to load live split share receipt:", error);
          toast.error(t("bill.splitSettleFailed"));
        }
      })();
      return;
    }

    if (liveShare.status === "released" || liveShare.status === "failed") {
      liveReceiptLoadingShareIDRef.current = null;
      setHeldShare(null);
      toast.error(t("bill.splitHoldFailed"));
      void onBillRefresh?.();
      return;
    }

    // Still held: pick up a server-side hold extension. When the guest starts a
    // payment the backend lengthens hold_expires_at (splitGuestPaymentHoldTTL) so
    // they have time to finish. The local countdown was keyed on the ORIGINAL
    // (shorter) expiry captured at hold time, so it would fire and tear down the
    // in-flight payment UI while the server still holds the share — booting the
    // guest mid-payment and inviting a duplicate hold / cash request. Sync the
    // later expiry so the countdown restarts instead.
    if (liveShare.status === "held" && liveShare.hold_expires_at) {
      const liveExpiry = Date.parse(liveShare.hold_expires_at);
      const currentExpiry = heldShare.hold_expires_at
        ? Date.parse(heldShare.hold_expires_at)
        : 0;
      if (Number.isFinite(liveExpiry) && liveExpiry > currentExpiry) {
        setHeldShare((prev) =>
          prev && prev.id === liveShare.id
            ? { ...prev, hold_expires_at: liveShare.hold_expires_at }
            : prev,
        );
      }
    }
  }, [billToken, heldShare, liveState?.shares, onBillRefresh, settledReceipt, splitting, t]);

  // Tick wall-clock while (a) this guest's hold is counting down, or (b) any
  // live equal held share has hold_expires_at — so equal seat preview re-runs
  // when OTHER guests' holds expire without an SSE update (PG-23).
  const equalHoldExpiryKey = useMemo(() => {
    return (liveState?.shares ?? [])
      .filter(
        (share) =>
          share.mode === "equal" &&
          share.status === "held" &&
          Boolean(share.hold_expires_at),
      )
      .map((share) => `${share.id}:${share.hold_expires_at}`)
      .join("|");
  }, [liveState?.shares]);

  useEffect(() => {
    const localExpiry = heldShare?.hold_expires_at
      ? Date.parse(heldShare.hold_expires_at)
      : Number.NaN;
    const needsTick =
      (Number.isFinite(localExpiry) && localExpiry > 0) || equalHoldExpiryKey.length > 0;
    if (!needsTick) return;

    let retired = false;
    const retireLocalHoldIfExpired = () => {
      if (!Number.isFinite(localExpiry) || retired) return false;
      if (Date.now() < localExpiry) return false;
      retired = true;
      liveReceiptLoadingShareIDRef.current = null;
      setHeldShare(null);
      setExpanded(true);
      toast.error(t("bill.splitShareReleased"));
      void onBillRefresh?.();
      return true;
    };

    setNowMs(Date.now());
    if (retireLocalHoldIfExpired()) return;

    const timer = window.setInterval(() => {
      setNowMs(Date.now());
      retireLocalHoldIfExpired();
    }, 1000);
    return () => window.clearInterval(timer);
  }, [equalHoldExpiryKey, heldShare?.hold_expires_at, onBillRefresh, t]);

  if (settledReceipt) {
    return (
      <section className="space-y-4 rounded-2xl border border-brand/20 bg-white p-5 sm:p-6">
        <div aria-live="polite" className="flex items-start gap-3">
          <span className="mt-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-700">
            <CheckCircle2 className="h-5 w-5" strokeWidth={1.75} />
          </span>
          <div className="min-w-0">
            <p className="text-label uppercase text-brand">{t("bill.splitBill")}</p>
            <h3 className="mt-1 text-lg font-semibold text-ink-950">
              {t("bill.splitReceiptTitle")}
            </h3>
            <p className="mt-1 text-sm text-ink-600">
              {t("bill.splitReceiptSubtitle")}
            </p>
          </div>
        </div>

        {settledReceipt.items.length > 0 && (
          <div className="rounded-xl border border-warm-200 bg-warm-50 p-4">
            <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
              <ReceiptText className="h-4 w-4 text-brand" strokeWidth={1.75} />
              <span>{t("bill.splitReceiptItems")}</span>
            </div>
            <div className="space-y-2">
              {settledReceipt.items.map((item) => (
                <div key={item.id} className="flex items-center justify-between gap-3 text-sm">
                  <span className="min-w-0 truncate text-ink-700">
                    {item.name} · {item.fraction}
                  </span>
                  <span className="font-mono font-semibold tabular-nums text-ink-950">
                    <CurrencyPrice
                      amount={item.subtotal}
                      fromCurrency={defaultCurrency}
                      displayCurrency={displayCurrency}
                    locale={moneyLocale}
                  />
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        <dl className="space-y-2 rounded-xl border border-warm-200 bg-warm-50 p-4 text-sm">
          {[
            [t("bill.splitReceiptSubtotal"), settledReceipt.subtotal],
            [t("bill.splitReceiptTax"), settledReceipt.tax],
            [t("bill.splitReceiptService"), settledReceipt.service_fee],
            [t("bill.splitReceiptTip"), settledReceipt.tip_amount],
          ].map(([label, amount]) => (
            <div key={String(label)} className="flex items-center justify-between gap-3">
              <dt className="text-ink-600">{label}</dt>
              <dd className="font-mono font-semibold tabular-nums text-ink-950">
                <CurrencyPrice
                  amount={amount as Dollars}
                  fromCurrency={defaultCurrency}
                  displayCurrency={displayCurrency}
                locale={moneyLocale}
                  />
              </dd>
            </div>
          ))}
          <div className="flex items-center justify-between gap-3 border-t border-warm-200 pt-3">
            <dt className="font-semibold text-ink-950">{t("bill.splitReceiptTotalPaid")}</dt>
            <dd className="font-mono text-base font-semibold tabular-nums text-ink-950">
              <CurrencyPrice
                amount={settledReceipt.grand_total}
                fromCurrency={defaultCurrency}
                displayCurrency={displayCurrency}
              locale={moneyLocale}
                  />
            </dd>
          </div>
          <div className="flex items-center justify-between gap-3 text-xs">
            <dt className="text-ink-500">{t("bill.splitReceiptTender")}</dt>
            <dd className="font-semibold uppercase tracking-normal text-ink-700">
              {settledReceipt.tender ? splitTenderLabel(settledReceipt.tender, t) : "-"}
            </dd>
          </div>
        </dl>

        <button
          type="button"
          onClick={() => {
            setSettledReceipt(null);
            setHeldShare(null);
            setExpanded(true);
          }}
          className="h-11 w-full rounded-xl border border-warm-200 bg-white text-sm font-semibold text-ink-800 transition-colors hover:border-brand hover:text-brand"
        >
          {t("bill.splitNewShare")}
        </button>
      </section>
    );
  }

  if (heldShare) {
    // "Remaining after you" = what the table still owes once YOUR share is
    // settled. liveState.available already nets out all holds (including this
    // one), so subtracting heldShare.amount again wrongly showed $0.00 after a
    // half-bill hold (audit FIND-018). Prefer cents: total - paid - your share.
    const totalCents =
      liveState?.total_cents ??
      Math.round(Number(remainingAmount) * 100);
    const paidCents = liveState?.paid_cents ?? 0;
    const shareCents =
      heldShare.amount_cents ?? Math.round(Number(heldShare.amount) * 100);
    const remainingAfterHeldShare = asDollars(
      Math.max(totalCents - paidCents - shareCents, 0) / 100,
    );
    const holdExpiresAtMs = heldShare.hold_expires_at ? Date.parse(heldShare.hold_expires_at) : Number.NaN;
    const holdSecondsRemaining = Number.isFinite(holdExpiresAtMs)
      ? Math.max(0, Math.ceil((holdExpiresAtMs - nowMs) / 1000))
      : null;
    return (
      <section className="space-y-4 rounded-2xl border border-brand/20 bg-white p-5 sm:p-6">
        <div aria-live="polite" className="space-y-1">
          <p className="text-label uppercase text-brand">{t("bill.splitBill")}</p>
          <h3 className="text-lg font-semibold text-ink-950">
            {t("bill.splitShareReady")}
          </h3>
          <p className="text-sm text-ink-600">
            {(() => {
              const sentence = t("bill.splitRemainingAfterYou", { amount: "__AMT__" });
              const [before, after] = sentence.split("__AMT__");
              return (
                <>
                  {before}
                  <CurrencyPrice
                    amount={remainingAfterHeldShare}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  />
                  {after ?? ""}
                </>
              );
            })()}
          </p>
          {holdSecondsRemaining !== null && (
            <p className="pt-1 text-xs font-medium text-amber-700">
              {t("bill.splitHoldCountdown", {
                time: formatHoldCountdown(holdSecondsRemaining),
              })}
            </p>
          )}
        </div>
        <PaymentSection
          billId={billId}
          billToken={billToken}
          businessId={businessId}
          amount={heldShare.amount}
          businessName={businessName}
          businessAddress={businessAddress}
          tipAddress={tipAddress}
          tableCode={tableCode}
          defaultCurrency={defaultCurrency}
          displayCurrency={displayCurrency}
          tipBaseAmount={heldShareTipBaseAmount}
          onPaymentComplete={handleSharePaymentComplete}
          onCashierPayment={handleShareCashierPayment}
          cashierPaymentLoading={isCashierRequestInFlight}
          splitShareId={heldShare.id}
        />
        <button
          type="button"
          onClick={handleReleaseShare}
          disabled={isReleasingShare}
          className="h-11 w-full rounded-xl border border-warm-200 bg-white text-sm font-semibold text-ink-700 transition-colors hover:border-rose-300 hover:text-rose-700 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {t("bill.splitReleaseShare")}
        </button>
        {/* Cashier confirmation — only reachable from the held-share payment path. */}
        <Modal isOpen={cashierModalOpen} onClose={() => setCashierModalOpen(false)} size="lg">
          <ModalContent>
            <ModalHeader className="flex flex-col gap-1">{t("bill.payWithCashier")}</ModalHeader>
            <ModalBody>
              <div className="text-center space-y-4">
                <div className="w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center mx-auto">
                  <CreditCard className="w-8 h-8 text-brand" />
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-ink-900 mb-2">
                    {t("bill.pleasePayAtCashier")}
                  </h3>
                  <p className="text-ink-600 mb-4">{t("bill.cashierInstructions")}</p>
                </div>
                {cashierRequestAmount !== null && (
                  <div className="bg-warm-50 rounded-lg p-4">
                    <div className="flex justify-between items-center">
                      <span className="text-ink-600">{t("bill.amountToPay")}:</span>
                      <span className="text-xl font-bold text-ink-900">
                        <CurrencyPrice
                          amount={cashierRequestAmount}
                          fromCurrency={defaultCurrency}
                          displayCurrency={displayCurrency}
                          locale={moneyLocale}
                        />
                      </span>
                    </div>
                  </div>
                )}
                <div className="text-sm text-ink-500">{t("bill.cashierRequestSentNote")}</div>
              </div>
            </ModalBody>
            <ModalFooter>
              <Button color="primary" onPress={() => setCashierModalOpen(false)} className="w-full">
                {t("bill.gotIt")}
              </Button>
            </ModalFooter>
          </ModalContent>
        </Modal>
      </section>
    );
  }

  return (
    <section className="rounded-2xl border border-warm-200 bg-white p-5 sm:p-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="text-label uppercase text-ink-500">{t("bill.splitBill")}</p>
          <h3 className="mt-1 text-lg font-semibold text-ink-950">
            {t("bill.splitLiveTitle")}
          </h3>
          <p className="mt-1 text-sm text-ink-600">
            {t("bill.splitLiveDescription")}
          </p>
        </div>
        {/* PG-28: when parent (GuestBill) owns the disclosure via defaultExpanded,
            do not render a second control with the same accessible name. */}
        {!defaultExpanded ? (
          <button
            type="button"
            onClick={() => setExpanded((value) => !value)}
            className="inline-flex h-11 items-center justify-center gap-2 rounded-xl bg-brand px-4 text-sm font-semibold text-white transition-colors hover:bg-brand-dark active:translate-y-px motion-reduce:transition-none motion-reduce:active:translate-y-0"
          >
            <WalletCards className="h-4 w-4" strokeWidth={1.75} aria-hidden="true" />
            {t("bill.splitBill")}
          </button>
        ) : null}
      </div>

      {expanded && (
        <div className="mt-5 space-y-5 border-t border-warm-200 pt-5">
          {liveState && (
            <div aria-live="polite" className="rounded-xl border border-warm-200 bg-warm-50 p-4">
              <div className="flex items-center justify-between gap-3 text-sm">
                <span className="text-ink-600">{t("bill.splitProgress")}</span>
                <span className="font-mono font-semibold tabular-nums text-ink-950">
                  <CurrencyPrice
                    amount={liveState.paid_amount}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  />{" "}
                  /{" "}
                  <CurrencyPrice
                    amount={liveState.total_amount}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                  locale={moneyLocale}
                  />
                </span>
              </div>
              <div
                aria-hidden="true"
                className="mt-3 h-2 overflow-hidden rounded-full bg-warm-200"
              >
                <div
                  className="h-full bg-brand transition-[width] motion-reduce:transition-none"
                  style={{
                    width: `${Math.min(
                      100,
                      Math.max(0, (liveState.paid_cents / Math.max(1, liveState.total_cents)) * 100),
                    )}%`,
                  }}
                />
              </div>
              <div className="mt-2 flex items-center justify-between gap-3 text-xs text-ink-600">
                <span>
                  {(() => {
                    const sentence = t("bill.splitHeld", { amount: "__AMT__" });
                    const [before, after] = sentence.split("__AMT__");
                    return (
                      <>
                        {before}
                        <CurrencyPrice
                          amount={liveState.held_amount}
                          fromCurrency={defaultCurrency}
                          displayCurrency={displayCurrency}
                        locale={moneyLocale}
                  />
                        {after ?? ""}
                      </>
                    );
                  })()}
                </span>
                <span>
                  {(() => {
                    const sentence = t("bill.splitAvailable", { amount: "__AMT__" });
                    const [before, after] = sentence.split("__AMT__");
                    return (
                      <>
                        {before}
                        <CurrencyPrice
                          amount={liveState.available_amount}
                          fromCurrency={defaultCurrency}
                          displayCurrency={displayCurrency}
                        locale={moneyLocale}
                  />
                        {after ?? ""}
                      </>
                    );
                  })()}
                </span>
              </div>
              {payerRoster.length > 0 && (
                <div className="mt-4 border-t border-warm-200 pt-4">
                  <p className="text-xs font-semibold uppercase text-ink-500">
                    {t("bill.splitRoster")}
                  </p>
                  <ul className="mt-3 space-y-2">
                    {payerRoster.map((share) => (
                      <li
                        key={share.id}
                        className="flex min-h-12 items-center justify-between gap-3 rounded-lg bg-white px-3 py-2"
                      >
                        <span className="min-w-0">
                          <span className="block truncate text-sm font-semibold text-ink-950">
                            {share.display_name || t("bill.splitGuestFallback", { id: share.id })}
                          </span>
                          <span className="mt-0.5 flex flex-wrap items-center gap-1 text-xs text-ink-600">
                            <span>
                              {share.status === "settled"
                                ? t("bill.splitStatusSettled")
                                : t("bill.splitStatusPaying")}
                            </span>
                            {share.tender && (
                              <>
                                <span aria-hidden="true">·</span>
                                <span>{splitTenderLabel(share.tender, t)}</span>
                              </>
                            )}
                          </span>
                        </span>
                        <span className="shrink-0 font-mono text-sm font-semibold tabular-nums text-ink-950">
                          <CurrencyPrice
                            amount={share.amount}
                            fromCurrency={defaultCurrency}
                            displayCurrency={displayCurrency}
                          locale={moneyLocale}
                  />
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}

          <label className="block">
            <span className="text-xs font-semibold uppercase text-ink-500">
              {t("bill.splitDisplayNameLabel")}
            </span>
            <input
              aria-label={t("bill.splitDisplayNameLabel")}
              autoComplete="given-name"
              maxLength={40}
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              placeholder={t("bill.splitDisplayNamePlaceholder")}
              className="mt-1 h-11 w-full rounded-xl border border-warm-200 bg-warm-50 px-3 text-ink-950 outline-none focus:border-brand"
            />
          </label>

          <div>
            <p className="mb-3 text-label uppercase text-ink-500">
              {t("bill.splitChooseMethod")}
            </p>
            <div className="grid gap-2 sm:grid-cols-3">
              <button
                type="button"
                aria-label={t("bill.splitEqually")}
                onClick={() => setMode("equal")}
                className={`rounded-xl border p-4 text-start transition-colors ${
                  mode === "equal"
                    ? "border-brand bg-brand/5"
                    : "border-warm-200 bg-white hover:border-ink-300"
                }`}
              >
                <Users className="mb-3 h-5 w-5 text-brand" strokeWidth={1.75} />
                <span className="block text-sm font-semibold text-ink-950">
                  {t("bill.splitEqually")}
                </span>
                <span className="mt-1 block text-xs text-ink-600">
                  {t("bill.splitEquallyDescription")}
                </span>
              </button>
              <button
                type="button"
                aria-label={t("bill.splitItems")}
                onClick={() => setMode("items")}
                disabled={claimableItems.length === 0}
                className={`rounded-xl border p-4 text-start transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${
                  mode === "items"
                    ? "border-brand bg-brand/5"
                    : "border-warm-200 bg-white hover:border-ink-300"
                }`}
              >
                <ListChecks className="mb-3 h-5 w-5 text-brand" strokeWidth={1.75} />
                <span className="block text-sm font-semibold text-ink-950">
                  {t("bill.splitItems")}
                </span>
                <span className="mt-1 block text-xs text-ink-600">
                  {t("bill.splitItemsDescription")}
                </span>
              </button>
              <button
                type="button"
                aria-label={t("bill.splitCustom")}
                onClick={() => setMode("custom")}
                className={`rounded-xl border p-4 text-start transition-colors ${
                  mode === "custom"
                    ? "border-brand bg-brand/5"
                    : "border-warm-200 bg-white hover:border-ink-300"
                }`}
              >
                <WalletCards className="mb-3 h-5 w-5 text-brand" strokeWidth={1.75} />
                <span className="block text-sm font-semibold text-ink-950">
                  {t("bill.splitCustom")}
                </span>
                <span className="mt-1 block text-xs text-ink-600">
                  {t("bill.splitCustomDescription")}
                </span>
              </button>
            </div>
          </div>

          {mode === "equal" ? (
            <div className="space-y-3">
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="block">
                  <span className="text-xs font-semibold uppercase text-ink-500">
                    {t("bill.splitPeopleCount")}
                  </span>
                  <input
                    type="number"
                    min={1}
                    max={MAX_SPLIT_PEOPLE}
                    aria-label={t("bill.splitPeopleCount")}
                    aria-invalid={peopleOutOfRange}
                    aria-describedby={
                      peopleOutOfRange ? "split-people-range-reason" : undefined
                    }
                    value={numPeople}
                    onChange={(event) => {
                      // PG-30: do not silent-snap; keep the typed value and
                      // explain bounds inline when out of range.
                      setNumPeople(event.target.value);
                    }}
                    className={`mt-1 h-11 w-full rounded-xl border bg-warm-50 px-3 font-mono tabular-nums text-ink-950 outline-none focus:border-brand ${
                      peopleOutOfRange ? "border-rose-400" : "border-warm-200"
                    }`}
                  />
                </label>
                <label className="block">
                  <span className="text-xs font-semibold uppercase text-ink-500">
                    {t("bill.splitSharesCovered")}
                  </span>
                  <input
                    type="number"
                    min={1}
                    max={Math.max(1, normalizedNumPeople || MAX_SPLIT_PEOPLE)}
                    aria-label={t("bill.splitSharesCovered")}
                    aria-invalid={sharesOutOfRange}
                    aria-describedby={
                      sharesOutOfRange ? "split-people-range-reason" : undefined
                    }
                    value={sharesCovered}
                    onChange={(event) => {
                      setSharesCovered(event.target.value);
                    }}
                    className={`mt-1 h-11 w-full rounded-xl border bg-warm-50 px-3 font-mono tabular-nums text-ink-950 outline-none focus:border-brand ${
                      sharesOutOfRange ? "border-rose-400" : "border-warm-200"
                    }`}
                  />
                </label>
              </div>
              {peopleOutOfRange ? (
                <p
                  id="split-people-range-reason"
                  data-testid="split-people-range-reason"
                  role="status"
                  className="text-sm text-rose-700"
                >
                  {t("bill.splitPeopleOutOfRange", { min: 1, max: MAX_SPLIT_PEOPLE })}
                </p>
              ) : null}
              {sharesOutOfRange ? (
                <p
                  id="split-people-range-reason"
                  data-testid="split-people-range-reason"
                  role="status"
                  className="text-sm text-rose-700"
                >
                  {t("bill.splitSharesOutOfRange", {
                    min: 1,
                    max: Math.max(1, normalizedNumPeople),
                  })}
                </p>
              ) : null}
            </div>
          ) : mode === "items" ? (
            <div className="space-y-3">
              <label className="block">
                <span className="text-xs font-semibold uppercase text-ink-500">
                  {t("bill.splitItemFraction")}
                </span>
                <select
                  aria-label={t("bill.splitItemFraction")}
                  value={selectedFraction}
                  onChange={(event) => setSelectedFraction(event.target.value as FractionOption)}
                  className="mt-1 h-11 w-full rounded-xl border border-warm-200 bg-warm-50 px-3 font-mono tabular-nums text-ink-950 outline-none focus:border-brand"
                >
                  {fractionOptions.map((fraction) => (
                    <option key={fraction} value={fraction}>
                      {fraction === "1" ? t("bill.splitFullItem") : fraction}
                    </option>
                  ))}
                  <option value="custom">{t("bill.splitCustomFraction")}</option>
                </select>
              </label>
              {selectedFraction === "custom" && (
                <label className="block">
                  <span className="text-xs font-semibold uppercase text-ink-500">
                    {t("bill.splitCustomFraction")}
                  </span>
                  <input
                    aria-label={t("bill.splitCustomFraction")}
                    inputMode="text"
                    value={customFraction}
                    placeholder={t("bill.splitCustomFractionPlaceholder")}
                    onChange={(event) => setCustomFraction(event.target.value)}
                    className="mt-1 h-11 w-full rounded-xl border border-warm-200 bg-warm-50 px-3 font-mono tabular-nums text-ink-950 outline-none focus:border-brand"
                  />
                </label>
              )}
              <div className="grid gap-2">
                {claimableItems.map((item) => {
                  const selected = selectedItemIDs.includes(item.id);
                  const remainingFraction = Math.max(0, 1 - (claimedFractionByItem[item.id] ?? 0));
                  const itemFraction = selectedFractions[item.id] || clampFractionToRemaining(activeSelectedFraction, remainingFraction);
                  const fullyClaimed = !selected && remainingFraction <= 0.001;
                  const partiallyClaimed = !selected && remainingFraction < 0.999;
                  return (
                    <button
                      key={item.id}
                      type="button"
                      aria-pressed={selected}
                      disabled={fullyClaimed}
                      onClick={() => toggleItem(item.id)}
                      className={`flex min-h-14 items-center justify-between gap-3 rounded-xl border p-3 text-start transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${
                        selected
                          ? "border-brand bg-brand/5"
                          : "border-warm-200 bg-white hover:border-ink-300"
                      }`}
                    >
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-semibold text-ink-950">
                          {item.name}
                        </span>
                        <span className="mt-0.5 block text-xs text-ink-500">
                          {fullyClaimed
                            ? t("bill.splitItemUnavailable")
                            : selected
                              ? `${itemFraction} ${t("bill.splitItemSelected")}`
                              : partiallyClaimed
                                ? t("bill.splitItemRemaining", {
                                    fraction: formatFractionAmount(remainingFraction),
                                  })
                                : t("bill.splitTapToClaim")}
                        </span>
                      </span>
                      <span className="shrink-0 text-end">
                        <span
                          data-testid="split-item-line-total-label"
                          className="block text-[10px] font-semibold uppercase tracking-wide text-ink-500"
                        >
                          {t("bill.splitItemLineTotal")}
                        </span>
                        <span
                          data-testid="split-item-line-total-amount"
                          className="font-mono text-sm font-semibold tabular-nums text-ink-900"
                        >
                          <CurrencyPrice
                            amount={item.subtotal}
                            fromCurrency={defaultCurrency}
                            displayCurrency={displayCurrency}
                            locale={moneyLocale}
                          />
                        </span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              <label className="block">
                <span className="text-xs font-semibold uppercase text-ink-500">
                  {t("bill.splitAmountLabel")}
                </span>
                <input
                  aria-label={t("bill.splitAmountLabel")}
                  inputMode="decimal"
                  value={customAmount}
                  aria-invalid={customIsNonPositive || customIsOverRemaining}
                  aria-describedby={
                    customIsNonPositive || customIsOverRemaining
                      ? "split-custom-amount-reason"
                      : undefined
                  }
                  onChange={(event) => {
                    setCoverRemaining(false);
                    setCustomAmount(event.target.value);
                  }}
                  className={`mt-1 h-11 w-full rounded-xl border bg-warm-50 px-3 font-mono tabular-nums text-ink-950 outline-none focus:border-brand ${
                    customIsNonPositive || customIsOverRemaining
                      ? "border-rose-400"
                      : "border-warm-200"
                  }`}
                />
              </label>
              {customIsOverRemaining ? (
                <p
                  id="split-custom-amount-reason"
                  data-testid="split-custom-amount-reason"
                  role="status"
                  className="text-sm text-amber-800"
                >
                  {t("bill.splitAmountOverRemaining")}
                </p>
              ) : null}
              {customIsNonPositive ? (
                <p
                  id="split-custom-amount-reason"
                  data-testid="split-custom-amount-reason"
                  role="status"
                  className="text-sm text-rose-700"
                >
                  {t("bill.splitAmountNonPositive")}
                </p>
              ) : null}
              <button
                type="button"
                onClick={() => {
                  setCoverRemaining(true);
                  // Prefill in the guest's own decimal shape ("12,50" for comma
                  // locales) so the value round-trips through parseLocaleDecimal
                  // unchanged. No grouping — parseLocaleDecimal treats the LAST
                  // separator as decimal.
                  setCustomAmount(
                    new Intl.NumberFormat(currentLanguage || "en", {
                      minimumFractionDigits: 2,
                      maximumFractionDigits: 2,
                      useGrouping: false,
                    }).format(Number(liveAvailableAmount)),
                  );
                }}
                disabled={Number(liveAvailableAmount) <= 0}
                className="h-11 w-full rounded-xl border border-brand/30 bg-brand/5 text-sm font-semibold text-brand transition-colors hover:border-brand disabled:cursor-not-allowed disabled:opacity-50"
              >
                {t("bill.splitPayRemaining")}
              </button>
            </div>
          )}

          <div className="rounded-xl border border-warm-200 bg-warm-50 p-4">
            {mode === "items" && selectedItemIDs.length > 0 ? (
              <>
                {/* PG-26: when line totals differ from the allocated share
                    (tax/fees/discounts), surface both figures with labels so
                    guests can reconcile them. Allocation math is unchanged. */}
                <div className="flex items-center justify-between gap-3">
                  <span
                    data-testid="split-selected-items-total-label"
                    className="text-sm text-ink-600"
                  >
                    {t("bill.splitSelectedItemsTotal")}
                  </span>
                  <span
                    data-testid="split-selected-items-total-amount"
                    className="font-mono text-sm font-semibold tabular-nums text-ink-700"
                  >
                    <CurrencyPrice
                      amount={asDollars(
                        claimableItems.reduce((total, item) => {
                          if (!selectedItemIDs.includes(item.id)) return total;
                          return (
                            total +
                            Number(item.subtotal) *
                              fractionToNumber(selectedFractions[item.id] || "1")
                          );
                        }, 0),
                      )}
                      fromCurrency={defaultCurrency}
                      displayCurrency={displayCurrency}
                      locale={moneyLocale}
                    />
                  </span>
                </div>
                <div className="mt-2 flex items-center justify-between gap-3 border-t border-warm-200 pt-2">
                  <span
                    data-testid="split-your-share-label"
                    className="text-sm font-medium text-ink-800"
                  >
                    {hasPriorPayment
                      ? t("bill.splitYourShareRemaining")
                      : t("bill.splitYourShareWithTax")}
                  </span>
                  <span
                    data-testid="split-your-share-amount"
                    className="font-mono text-lg font-semibold tabular-nums text-ink-950"
                  >
                    <CurrencyPrice
                      amount={selectedAmount}
                      fromCurrency={defaultCurrency}
                      displayCurrency={displayCurrency}
                      locale={moneyLocale}
                    />
                  </span>
                </div>
                {hasPriorPayment ? (
                  <p
                    data-testid="split-prior-payment-note"
                    className="mt-1 text-xs text-ink-500"
                  >
                    {t("bill.splitPriorPaymentNote", {
                      amount: priorPaidAmount.toFixed(2),
                    })}
                  </p>
                ) : null}
              </>
            ) : (
              <div className="flex items-center justify-between gap-3">
                <span
                  data-testid="split-your-share-label"
                  className="text-sm text-ink-600"
                >
                  {t("bill.splitYourShare")}
                </span>
                <span
                  data-testid="split-your-share-amount"
                  className="font-mono text-lg font-semibold tabular-nums text-ink-950"
                >
                  <CurrencyPrice
                    amount={selectedAmount}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                    locale={moneyLocale}
                  />
                </span>
              </div>
            )}
          </div>

          <button
            type="button"
            onClick={handleHoldShare}
            disabled={isHolding || !canHoldShare}
            className="h-12 w-full rounded-xl bg-brand font-semibold text-white transition-colors hover:bg-brand-dark active:translate-y-px disabled:cursor-not-allowed disabled:opacity-40 motion-reduce:transition-none motion-reduce:active:translate-y-0"
          >
            {isHolding ? t("bill.processing") : t("bill.splitHoldShare")}
          </button>
        </div>
      )}
    </section>
  );
}

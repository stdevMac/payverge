"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { AccessibleTextarea } from "@/components/ui/AccessibleTextarea";

import React, { useState, useRef, useEffect, useCallback } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,

  Divider,
  Card,
  CardBody,
  Chip,
} from "@nextui-org/react";
import { ShoppingCart, MapPin, Truck, Clock, AlertCircle, CreditCard, Banknote } from "lucide-react";
import { guestDeliveryApi, DeliveryCheckoutItemInput } from "@/api/delivery";
import { asDollars } from "@/types/money";
import { formatGuestCurrency } from "@/utils/guestCurrencyFormatter";
import { parseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { isSafeExternalTrackingUrl } from "@/lib/externalUrl";
import type { FulfillmentContext } from "@/hooks/useFulfillmentContext";
import { useRouter } from "next/navigation";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

import { localizeBackendError } from "./deliveryGuestLocalization";
import {
  getOrCreateCheckoutRequestId,
  clearCheckoutRequestId,
  computeCheckoutFingerprint,
} from "./checkoutRequestId";
import { GuestDemoPersonalDataNotice } from "@/components/demo/DemoPersonalDataNotice";

export interface CartItem {
  /** Unique key for the cart line (item id + selected options hash) */
  key: string;
  menu_item_name: string;
  menu_item_id?: string;
  quantity: number;
  unit_price: number;
  item_type?: string;
  bundle_id?: number;
  options: Array<{ name: string; price_change: number }>;
  special_requests?: string;
}

interface GuestDeliveryCheckoutProps {
  isOpen: boolean;
  onClose: () => void;
  businessId: number;
  businessName: string;
  fulfillmentContext: FulfillmentContext;
  cart: CartItem[];
  onSuccess?: () => void;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /** Business tax rate as a percent (e.g. 8 = 8%). Used in the price
   *  breakdown estimate. Matches backend ComputeBillTotals. Defaults to 0 when
   *  not available. */
  taxRate?: number;
  /** Business service fee rate as a percent (e.g. 5 = 5%). Defaults to 0. */
  serviceFeeRate?: number;
}

function cartToCheckoutItems(cart: CartItem[]): DeliveryCheckoutItemInput[] {
  return cart.map((item) => ({
    menu_item_name: item.menu_item_name,
    menu_item_id: item.menu_item_id,
    quantity: item.quantity,
    price: asDollars(item.unit_price + item.options.reduce((s, o) => s + o.price_change, 0)),
    options: item.options.map((o) => ({ name: o.name, price_change: asDollars(o.price_change) })),
    special_requests: item.special_requests,
    item_type: item.item_type,
    bundle_id: item.bundle_id,
  }));
}

function cartSubtotal(cart: CartItem[]): number {
  return cart.reduce((sum, item) => {
    const itemTotal =
      (item.unit_price + item.options.reduce((s, o) => s + o.price_change, 0)) *
      item.quantity;
    return sum + itemTotal;
  }, 0);
}

const EMAIL_REGEX = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

type ContactField = "name" | "phone" | "email";

/** The currency's symbol (e.g. "$", "€", "¥") for use as an input prefix. */
function currencySymbol(currencyCode: string): string {
  try {
    const parts = new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
      currencyDisplay: "narrowSymbol",
    }).formatToParts(0);
    return parts.find((p) => p.type === "currency")?.value || currencyCode;
  } catch (e) {
    console.error("Failed to resolve currency symbol", e);
    return currencyCode;
  }
}

/** Zero-decimal currencies (JPY, KRW, …) have no fractional minor unit. */
function isZeroDecimalCurrency(currencyCode: string): boolean {
  try {
    return (
      new Intl.NumberFormat(undefined, {
        style: "currency",
        currency: currencyCode,
      }).resolvedOptions().maximumFractionDigits === 0
    );
  } catch (e) {
    console.error("Failed to check zero-decimal currency", e);
    return false;
  }
}

export default function GuestDeliveryCheckout({
  isOpen,
  onClose,
  businessId,
  businessName,
  fulfillmentContext,
  cart,
  onSuccess,
  currency = "USD",
  taxRate = 0,
  serviceFeeRate = 0,
}: GuestDeliveryCheckoutProps) {
  const router = useRouter();
  const { t, currentLanguage } = useGuestTranslation();
  const fmtCurrency = (amount: number, code: string) =>
    formatGuestCurrency(amount, code, currentLanguage);
  const [customerName, setCustomerName] = useState("");
  const [customerPhone, setCustomerPhone] = useState("");
  const [customerEmail, setCustomerEmail] = useState("");
  const [driverTip, setDriverTip] = useState("");
  const [notes, setNotes] = useState("");
  const [placing, setPlacing] = useState(false);
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<ContactField, string>>>({});
  const nameRef = useRef<HTMLInputElement>(null);
  const phoneRef = useRef<HTMLInputElement>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  // Payment-mode disclosure (DLV-GUEST-5): tell the guest BEFORE they place
  // the order whether they'll pay online after acceptance or cash on delivery.
  // Resolved from the public delivery-settings endpoint; if it can't be
  // resolved we show nothing rather than guessing.
  const [paymentMode, setPaymentMode] = useState<"online" | "cash_on_delivery" | null>(null);
  // Distinguishes "fetch failed" from "still loading" so the disclosure's
  // absence is never silent — the guest gets an inline retry instead. (Wave 5)
  const [settingsFailed, setSettingsFailed] = useState(false);

  const loadPaymentMode = useCallback(async () => {
    if (!businessId) return;
    setSettingsFailed(false);
    try {
      const settings = await guestDeliveryApi.getSettings(businessId);
      const mode = settings?.payment_mode;
      if (mode === "online" || mode === "cash_on_delivery") {
        setPaymentMode(mode);
      }
    } catch {
      setSettingsFailed(true);
    }
  }, [businessId]);

  useEffect(() => {
    if (!isOpen) return;
    void loadPaymentMode();
  }, [isOpen, loadPaymentMode]);

  const subtotal = cartSubtotal(cart);
  const [confirmedFee, setConfirmedFee] = useState<number | null>(null);
  // feeJustChanged: tracks that we showed the fee-changed notice; the NEXT
  // submit should bypass the block and proceed to checkout.
  const feeJustChangedRef = useRef(false);
  const [feeChanged, setFeeChanged] = useState(false);

  // Apply free_delivery_minimum client-side so the displayed fee reflects the
  // real cart immediately — without waiting for the submit-time re-quote.
  // The fulfillmentContext quote was obtained at address-entry time (subtotal=0)
  // so its delivery_fee is always the base fee.  We recompute: if the cart
  // subtotal now meets the free-delivery threshold, show $0.
  const { free_delivery_minimum, delivery_fee: quotedFee } = fulfillmentContext.quote;
  const effectiveBaseFee =
    free_delivery_minimum > 0 && subtotal >= free_delivery_minimum ? 0 : quotedFee;
  const deliveryFee = confirmedFee ?? effectiveBaseFee;
  // Locale-safe parse so comma-decimal guests ("2,50") aren't truncated.
  // Clamped to >= 0 so a typed "-5" can never understate the displayed total
  // the guest commits to. (DLV-GUEST-4)
  const tipAmount = Math.max(0, parseLocaleDecimal(driverTip) || 0);
  // Mirror backend ComputeBillTotals: normalize the subtotal to the nearest cent
  // first (same rounding the backend applies before computing tax/fee), then
  // multiply by the percent rate and round to nearest cent.
  const normalizedSubtotal = Math.round(subtotal * 100) / 100;
  const taxAmount = taxRate > 0 ? Math.round(normalizedSubtotal * taxRate) / 100 : 0;
  const serviceFeeAmount = serviceFeeRate > 0 ? Math.round(normalizedSubtotal * serviceFeeRate) / 100 : 0;
  const total = normalizedSubtotal + taxAmount + serviceFeeAmount + deliveryFee + tipAmount;
  const zeroDecimal = isZeroDecimalCurrency(currency);

  const meetsMinimum = subtotal >= fulfillmentContext.quote.minimum_order_amount;

  const clearFieldError = (field: ContactField) => {
    setFieldErrors((prev) => {
      if (!prev[field]) return prev;
      const next = { ...prev };
      delete next[field];
      return next;
    });
  };

  const handlePlaceOrder = async () => {
    const nextErrors: Partial<Record<ContactField, string>> = {};
    if (!customerName.trim()) {
      nextErrors.name = t("businessPage.checkout.nameRequired");
    }
    if (!customerPhone.trim()) {
      nextErrors.phone = t("businessPage.checkout.phoneRequired");
    }
    if (!customerEmail.trim()) {
      nextErrors.email = t("businessPage.guestDelivery.emailRequired");
    } else if (!EMAIL_REGEX.test(customerEmail.trim())) {
      nextErrors.email = t("businessPage.checkout.emailInvalid");
    }
    if (Object.keys(nextErrors).length > 0) {
      setFieldErrors(nextErrors);
      setError("");
      const firstInvalid = nextErrors.name
        ? nameRef.current
        : nextErrors.phone
          ? phoneRef.current
          : emailRef.current;
      window.setTimeout(() => firstInvalid?.focus(), 0);
      return;
    }
    setFieldErrors({});
    if (!meetsMinimum) {
      setError(
        (t("businessPage.checkout.belowMinimum", {
          subtotal: fmtCurrency(subtotal, currency),
          minimum: fmtCurrency(fulfillmentContext.quote.minimum_order_amount, currency),
        }) as string) ||
          `Order subtotal (${fmtCurrency(subtotal, currency)}) is below the delivery minimum (${fmtCurrency(fulfillmentContext.quote.minimum_order_amount, currency)}).`,
      );
      return;
    }
    if (cart.length === 0) {
      setError(t("businessPage.checkout.cartEmpty"));
      return;
    }

    setPlacing(true);
    setError("");

    // Re-quote with current subtotal to catch stale fees, zone changes, or
    // free-above-minimum eligibility that may have changed since the initial quote.
    // If the fee changed, show a one-time notice and block; the NEXT submit (flagged
    // via feeJustChangedRef) bypasses the re-quote block and proceeds.
    if (!feeJustChangedRef.current) {
      try {
        const freshQuote = await guestDeliveryApi.quote(businessId, {
          order_subtotal: asDollars(subtotal),
          delivery_address: fulfillmentContext.delivery_address,
        });

        const currentDisplayedFee = confirmedFee ?? fulfillmentContext.quote.delivery_fee;
        const freshFee = freshQuote.delivery_fee;

        if (!freshQuote.eligible || !["quote_available", "below_minimum"].includes(freshQuote.reason_code)) {
          setError(localizeBackendError(freshQuote.message || "", t));
          setPlacing(false);
          return;
        }

        if (Math.abs(freshFee - currentDisplayedFee) > 0.001) {
          // Fee changed — update the confirmed fee so the display is accurate.
          setConfirmedFee(freshFee);

          if (freshFee < currentDisplayedFee) {
            // Fee dropped (e.g. free-delivery threshold now met): update silently
            // and continue to checkout.  No re-confirmation needed when the
            // guest pays less than expected.
            setFeeChanged(false);
          } else {
            // Fee increased — show notice and block; the NEXT submit will proceed.
            setFeeChanged(true);
            feeJustChangedRef.current = true;
            setError(
              (t("businessPage.guestDelivery.feeChangedNotice", {
                fee: fmtCurrency(freshFee, currency),
              }) as string) ||
                (t("businessPage.checkout.feeChanged", {
                  fee: fmtCurrency(freshFee, currency),
                }) as string) ||
                `Your order has changed; please review the updated delivery fee (${fmtCurrency(freshFee, currency)}) and click Place Order again to confirm.`,
            );
            setPlacing(false);
            return;
          }
        }

        // Fee matches — clear any previous fee-changed notice and proceed.
        setFeeChanged(false);
      } catch (e) {
        console.error("Delivery quote re-fetch failed", e);
        // Re-quote network failure is non-fatal — proceed with the original fee.
        // This prevents a failed quote endpoint from blocking all checkouts.
      }
    } else {
      // Second submit after fee-changed notice — reset flag and clear notice, then proceed.
      feeJustChangedRef.current = false;
      setFeeChanged(false);
    }

    try {
      const payload = {
        customer_name: customerName.trim(),
        customer_phone: customerPhone.trim(),
        customer_email: customerEmail.trim(),
        customer_locale: currentLanguage || undefined,
        delivery_address: fulfillmentContext.delivery_address,
        delivery_instructions: fulfillmentContext.delivery_instructions,
        contactless_delivery: fulfillmentContext.contactless_delivery,
        leave_at_door: fulfillmentContext.leave_at_door,
        notes: notes.trim() || undefined,
        driver_tip: tipAmount > 0 ? tipAmount : undefined,
        items: cartToCheckoutItems(cart),
      };
      // Retry-safe idempotency key: reused across a network-failure retry / reload
      // mid-submit so the backend replays the original order (HTTP 200 + duplicate)
      // instead of creating a second one. Bound to a fingerprint of the request
      // content so an EDITED cart mints a fresh id (an edited order must not be
      // deduped against the old one). Cleared only after a confirmed success.
      const requestId = getOrCreateCheckoutRequestId(
        businessId,
        computeCheckoutFingerprint(payload),
      );
      const result = await guestDeliveryApi.checkout(businessId, payload, {
        idempotencyKey: requestId,
      });

      clearCheckoutRequestId(businessId);
      onSuccess?.();
      onClose();
      // Online-payment orders must land on the pay page immediately: the
      // payment window is 15 minutes and the sweeper cancels unpaid orders,
      // so "notice the Pay Now card on the track page" is not a safe path.
      // The pay page self-redirects back to /track when the order is not
      // payable, and the track page keeps its Pay Now card as the fallback.
      const deliveryNumber = result.delivery_order.delivery_number;
      const awaitingOnlinePayment =
        result.delivery_order.payment_mode_stored === "online";
      router.push(
        awaitingOnlinePayment
          ? `/delivery/${deliveryNumber}/pay`
          : `/delivery/${deliveryNumber}/track`,
      );
    } catch (err) {
      const raw = getSafeApiErrorMessage(err, "");
      setError(raw ? localizeBackendError(raw, t) : t("businessPage.checkout.errorFallback"));
      setPlacing(false);
    }
  };

  const handleClose = () => {
    if (placing) return;
    setError("");
    setFieldErrors({});
    setFeeChanged(false);
    setConfirmedFee(null);
    feeJustChangedRef.current = false;
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="2xl"
      scrollBehavior="inside"
      classNames={{
        base: "bg-white",
        header: "border-b border-warm-200",
        footer: "border-t border-warm-200",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <ShoppingCart className="w-5 h-5 text-brand" />
            <span className="text-xl font-semibold text-ink-900">
              {(t("businessPage.checkout.title") as string) || "Complete your order"}
            </span>
          </div>
          <p className="text-sm font-normal text-ink-500">{businessName}</p>
        </ModalHeader>

        <ModalBody className="py-6 space-y-6">
          {/* Delivery summary */}
          <Card className="border border-emerald-200 bg-emerald-50">
            <CardBody className="space-y-2 py-4">
              <div className="flex items-center gap-2 text-sm text-emerald-800 font-medium">
                <MapPin className="w-4 h-4" />
                <span>{fulfillmentContext.delivery_address.formatted_address}</span>
              </div>
              {fulfillmentContext.quote.external_partner_links?.length ? (
                <div className="text-sm text-ink-600" data-testid="checkout-couriers">
                  <p>
                    {(t("businessPage.checkout.couriersNamed", {
                      names: fulfillmentContext.quote.external_partner_links
                        .map((link) => link.name)
                        .filter(Boolean)
                        .join(", "),
                    }) as string) || "Also available via {names}"}
                  </p>
                  <ul className="mt-1 flex flex-wrap gap-2">
                    {fulfillmentContext.quote.external_partner_links.map((link) => (
                      <li key={`${link.name}-${link.url}`}>
                        {/* Partner URLs are operator-configured data rendered on an
                            unauthenticated guest surface: only real web URLs are ever
                            linked, everything else degrades to plain text (#897). */}
                        {link.url && isSafeExternalTrackingUrl(link.url) ? (
                          <a
                            href={link.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="font-medium text-brand underline-offset-2 hover:underline"
                          >
                            {link.name}
                          </a>
                        ) : (
                          <span className="font-medium text-ink-800">{link.name}</span>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              <div className="flex flex-wrap gap-3 text-sm text-ink-600">
                <span className="inline-flex items-center gap-1">
                  <Truck className="w-4 h-4 text-ink-400" />
                  {t("businessPage.checkout.feeLabel") || "Fee"}: {fmtCurrency(deliveryFee, currency)}
                </span>
                <span className="inline-flex items-center gap-1">
                  <Clock className="w-4 h-4 text-ink-400" />
                  ~{fulfillmentContext.quote.estimated_total_minutes} min
                </span>
                {fulfillmentContext.quote.zone?.name ? (
                  <Chip size="sm" variant="flat" color="success">
                    {fulfillmentContext.quote.zone.name}
                  </Chip>
                ) : null}
              </div>
            </CardBody>
          </Card>

          {/* Order items */}
          <div>
            <h3 className="text-sm font-semibold text-ink-700 mb-3 uppercase tracking-wide">
              {(t("businessPage.checkout.cartLabel", {
                count: cart.reduce((s, i) => s + i.quantity, 0),
              }) as string) ||
                `Order items (${cart.reduce((s, i) => s + i.quantity, 0)})`}
            </h3>
            <div className="space-y-2">
              {cart.map((item) => {
                const lineTotal =
                  (item.unit_price + item.options.reduce((s, o) => s + o.price_change, 0)) *
                  item.quantity;
                return (
                  <div
                    key={item.key}
                    className="flex items-start justify-between gap-3 rounded-xl border border-warm-100 bg-warm-50 px-4 py-3"
                  >
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-ink-900 text-sm">{item.menu_item_name}</p>
                      {item.options.length > 0 && (
                        <p className="text-xs text-ink-500 mt-0.5">
                          {item.options.map((o) => o.name).join(", ")}
                        </p>
                      )}
                      {item.special_requests && (
                        <p className="text-xs text-ink-400 mt-0.5 italic">
                          {(t("businessPage.checkout.noteLabel") as string) || "Note"}:{" "}
                          {item.special_requests}
                        </p>
                      )}
                    </div>
                    <div className="text-right shrink-0">
                      <p className="text-sm font-semibold text-ink-900">
                        {fmtCurrency(lineTotal, currency)}
                      </p>
                      <p className="text-xs text-ink-500">x{item.quantity}</p>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          <Divider />

          {/* Contact information */}
          <form
            id="guest-delivery-checkout-form"
            className="space-y-4"
            noValidate
            onSubmit={(e) => {
              e.preventDefault();
              void handlePlaceOrder();
            }}
          >
            <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
            <h3 className="text-sm font-semibold text-ink-700 mb-3 uppercase tracking-wide">
              {(t("businessPage.checkout.contactInfo") as string) || "Contact information"}
            </h3>
            <GuestDemoPersonalDataNotice className="mb-3" />
            <div className="grid grid-cols-1 gap-4">
              <AccessibleInput
                ref={nameRef}
                id="guest-delivery-checkout-name"
                label={(t("businessPage.checkout.fullName") as string) || "Full name"}
                placeholder={
                  (t("businessPage.checkout.fullNamePlaceholder") as string) || "John Doe"
                }
                value={customerName}
                onChange={(e) => {
                  setCustomerName(e.target.value);
                  clearFieldError("name");
                }}
                isRequired
                isInvalid={Boolean(fieldErrors.name)}
                errorMessage={fieldErrors.name}
                aria-invalid={Boolean(fieldErrors.name) || undefined}
                autoComplete="name"
              />
              <AccessibleInput
                ref={phoneRef}
                id="guest-delivery-checkout-phone"
                label={
                  (t("businessPage.checkout.phoneNumber") as string) || "Phone number"
                }
                placeholder={
                  (t("businessPage.checkout.phonePlaceholder") as string) ||
                  "+1 (555) 123-4567"
                }
                value={customerPhone}
                onChange={(e) => {
                  setCustomerPhone(e.target.value);
                  clearFieldError("phone");
                }}
                isRequired
                isInvalid={Boolean(fieldErrors.phone)}
                errorMessage={fieldErrors.phone}
                aria-invalid={Boolean(fieldErrors.phone) || undefined}
                autoComplete="tel"
                type="tel"
              />
              <AccessibleInput
                ref={emailRef}
                id="guest-delivery-checkout-email"
                label={
                  (t("businessPage.checkout.emailLabel") as string) || "Email"
                }
                placeholder={
                  (t("businessPage.checkout.emailPlaceholder") as string) || "john@example.com"
                }
                value={customerEmail}
                onChange={(e) => {
                  setCustomerEmail(e.target.value);
                  clearFieldError("email");
                }}
                isRequired
                isInvalid={Boolean(fieldErrors.email)}
                errorMessage={fieldErrors.email}
                aria-invalid={Boolean(fieldErrors.email) || undefined}
                autoComplete="email"
                type="email"
              />
            </div>
          </form>

          {/* Driver tip + order notes */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <AccessibleInput
              label={
                (t("businessPage.checkout.driverTip") as string) || "Driver tip (optional)"
              }
              placeholder={
                (t("businessPage.checkout.tipPlaceholder") as string) || "0.00"
              }
              value={driverTip}
              // Tips can't be negative — strip minus signs at the input so the
              // field can never display an understated total. (DLV-GUEST-4)
              onChange={(e) => setDriverTip(e.target.value.replace(/-/g, ""))}
              type="text"
              inputMode={zeroDecimal ? "numeric" : "decimal"}
              startContent={
                <span className="text-sm text-ink-400">
                  {currencySymbol(currency)}
                </span>
              }
            />
            <AccessibleTextarea
              label={
                (t("businessPage.checkout.orderNotes") as string) || "Order notes (optional)"
              }
              placeholder={
                (t("businessPage.checkout.orderNotesPlaceholder") as string) ||
                "Any special instructions for the kitchen..."
              }
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              minRows={2}
            />
          </div>

          {/* Price breakdown */}
          <Card className="border border-warm-200 bg-warm-50">
            <CardBody className="space-y-2 py-4">
              <div className="flex justify-between text-sm text-ink-700">
                <span>{t("businessPage.checkout.subtotal") || "Subtotal"}</span>
                <span>{fmtCurrency(subtotal, currency)}</span>
              </div>
              {taxAmount > 0 && (
                <div className="flex justify-between text-sm text-ink-700">
                  <span>{t("businessPage.checkout.tax") || "Tax"}</span>
                  <span>{fmtCurrency(taxAmount, currency)}</span>
                </div>
              )}
              {serviceFeeAmount > 0 && (
                <div className="flex justify-between text-sm text-ink-700">
                  <span>{t("businessPage.checkout.serviceFee") || "Service fee"}</span>
                  <span>{fmtCurrency(serviceFeeAmount, currency)}</span>
                </div>
              )}
              <div className={`flex justify-between text-sm ${feeChanged ? "text-warning-700 font-medium" : "text-ink-700"}`}>
                <span>
                  {t("businessPage.checkout.deliveryFee") || "Delivery fee"}
                  {feeChanged ? ` (${t("businessPage.checkout.updated") || "updated"})` : ""}
                </span>
                <span>{fmtCurrency(deliveryFee, currency)}</span>
              </div>
              {tipAmount > 0 && (
                <div className="flex justify-between text-sm text-ink-700">
                  <span>{t("businessPage.checkout.driverTip") || "Driver tip"}</span>
                  <span>{fmtCurrency(tipAmount, currency)}</span>
                </div>
              )}
              <Divider />
              <div className="flex justify-between font-semibold text-ink-900">
                <span>{t("businessPage.checkout.total") || "Total"}</span>
                <span>{fmtCurrency(total, currency)}</span>
              </div>
              {!meetsMinimum && (
                <p className="text-xs text-warning-700 mt-1">
                  {(t("businessPage.checkout.addMoreForMinimum", {
                    amount: fmtCurrency(
                      fulfillmentContext.quote.minimum_order_amount - subtotal,
                      currency,
                    ),
                  }) as string) ||
                    `Add ${fmtCurrency(
                      fulfillmentContext.quote.minimum_order_amount - subtotal,
                      currency,
                    )} more to meet the delivery minimum.`}
                </p>
              )}
            </CardBody>
          </Card>

          {/* Payment-mode disclosure — the guest learns how payment works
              before committing the order, not on the tracking page. (DLV-GUEST-5) */}
          {paymentMode ? (
            <div
              className="flex items-center gap-2 rounded-xl border border-warm-200 bg-warm-50 px-4 py-3 text-sm text-ink-700"
              data-testid="payment-mode-disclosure"
            >
              {paymentMode === "online" ? (
                <CreditCard className="w-4 h-4 shrink-0 text-ink-500" />
              ) : (
                <Banknote className="w-4 h-4 shrink-0 text-ink-500" />
              )}
              <p>
                {paymentMode === "online"
                  ? (t("businessPage.checkout.payOnlineNotice") as string) ||
                    "You'll pay securely online once the restaurant accepts your order."
                  : (t("businessPage.checkout.payCashNotice") as string) ||
                    "You'll pay the driver in cash when your order arrives."}
              </p>
            </div>
          ) : null}
          {!paymentMode && settingsFailed ? (
            <div
              className="flex items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
              data-testid="payment-mode-unavailable"
            >
              <p>{t("businessPage.checkout.paymentModeUnavailable") as string}</p>
              <button
                type="button"
                onClick={() => void loadPaymentMode()}
                className="shrink-0 rounded-lg border border-amber-300 bg-white px-3 py-1.5 text-xs font-semibold text-amber-800"
              >
                {t("businessPage.checkout.paymentModeRetry") as string}
              </button>
            </div>
          ) : null}

          {error ? (
            <Card
              className={
                feeChanged
                  ? "border border-warning-200 bg-warning-50"
                  : "border border-danger-200 bg-danger-50"
              }
              role={feeChanged ? "status" : "alert"}
              aria-live={feeChanged ? "polite" : "assertive"}
              data-testid="delivery-checkout-error"
            >
              <CardBody
                className={`flex flex-row items-center gap-3 py-3 ${
                  feeChanged ? "text-warning-700" : "text-danger-700"
                }`}
              >
                <AlertCircle className="w-4 h-4 shrink-0" />
                <p className="text-sm">{error}</p>
              </CardBody>
            </Card>
          ) : null}
        </ModalBody>

        <ModalFooter>
          <Button variant="light" onPress={handleClose} isDisabled={placing}>
            {(t("businessPage.checkout.cancel") as string) || "Cancel"}
          </Button>
          <Button
            color="primary"
            onPress={handlePlaceOrder}
            isLoading={placing}
            isDisabled={!meetsMinimum || cart.length === 0}
            data-testid="place-order-btn"
          >
            {placing
              ? (t("businessPage.checkout.placingOrder") as string) || "Placing order..."
              : `${(t("businessPage.checkout.placeOrder") as string) || "Place order"} · ${fmtCurrency(total, currency)}`}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}

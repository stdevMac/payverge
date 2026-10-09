"use client";

import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import { ShoppingCart, Plus, Minus, Trash2, Tag, X } from "lucide-react";
import { CurrencyPrice } from "../../../../../components/common/CurrencyConverter";
import type { BusinessCurrencies, CartItem, PromotionPreview } from "../_types";
import type { PromoOffer } from "../../../../../api/promo";
import { validatePromoCode } from "../../../../../api/promo";
import { useGuestTranslation } from "../../../../../i18n/GuestTranslationProvider";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import {
  FiscalIdentityFields,
  emptyFiscalIdentity,
  type FiscalIdentityValue,
} from "../../../../../components/common/FiscalIdentityFields";
import { shouldShowFiscalIdentityFields } from "@/lib/fiscalIdentityAvailability";
import { useSuppressCookieBanner } from "@/contexts/CookieConsentContext";

interface CartModalProps {
  isOpen: boolean;
  onClose: () => void;
  cart: CartItem[];
  businessCurrencies: BusinessCurrencies;
  promotionPreview: PromotionPreview;
  orderLoading: boolean;
  quotePending: boolean;
  quoteError: unknown | null;
  quoteValid: boolean;
  quoteBlocked: boolean;
  /** Reason-mapped blocked copy (#822); falls back to the generic key. */
  quoteBlockedMessage?: string;
  onUpdateQuantity: (index: number, quantity: number) => void;
  onSubmitOrder: () => void;
  onClearCart: () => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  tableCode: string;
  appliedPromo: PromoOffer | null;
  onPromoApplied: (offer: PromoOffer) => void;
  onPromoRemoved: () => void;
  // Fiscal identity (AFIP/ARCA, T22). The parent persists the captured value to
  // the bill after it is created (the bill number isn't known until then).
  onFiscalIdentityChange?: (
    value: FiscalIdentityValue,
    isValid: boolean,
  ) => void;
  /** Business address ISO country. Form is hidden unless this is AR. */
  country?: string | null;
  /** Fiscal-settings country when the client already loaded settings. */
  fiscalCountry?: string | null;
}

function QuotePendingAmount({
  label,
  className = "font-mono tabular-nums",
}: {
  label: string;
  className?: string;
}) {
  return (
    <dd
      data-testid="quote-pending-amount"
      aria-busy="true"
      aria-label={label}
      className={className}
    >
      <span
        className="inline-block h-4 w-12 animate-pulse rounded bg-warm-200 align-middle"
        aria-hidden
      />
    </dd>
  );
}

export default function CartModal({
  isOpen,
  onClose,
  cart,
  businessCurrencies,
  promotionPreview,
  orderLoading,
  quotePending,
  quoteError,
  quoteValid,
  quoteBlocked,
  quoteBlockedMessage,
  onUpdateQuantity,
  onSubmitOrder,
  onClearCart,
  t,
  tableCode,
  appliedPromo,
  onPromoApplied,
  onPromoRemoved,
  onFiscalIdentityChange,
  country,
  fiscalCountry,
}: CartModalProps) {
  const showFiscalIdentity = shouldShowFiscalIdentityFields({
    country,
    fiscalCountry,
  });
  const { currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  // On a phone the cart is a bottom sheet whose Place Order button sits where
  // the first-visit cookie banner is pinned. Keep the banner out of the way
  // while the cart is open; it returns, still unanswered, when the cart closes.
  useSuppressCookieBanner(isOpen);

  const [promoInput, setPromoInput] = useState("");
  const [promoError, setPromoError] = useState("");
  const [promoLoading, setPromoLoading] = useState(false);
  const [showPromoInput, setShowPromoInput] = useState(false);
  const [fiscalIdentity, setFiscalIdentity] = useState<FiscalIdentityValue>(
    emptyFiscalIdentity(),
  );

  // Clear-cart needs a deliberate confirm — it sits right beside Place Order and
  // wiped the whole cart on a single accidental tap with no undo. First tap arms
  // a 3s "tap again to clear" state; second tap within the window clears.
  const [confirmClear, setConfirmClear] = useState(false);
  const confirmClearTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const headingRef = useRef<HTMLHeadingElement | null>(null);
  const pendingFocusRef = useRef<{ kind: "remove"; index: number } | null>(
    null,
  );
  const [liveMessage, setLiveMessage] = useState("");
  const announce = (message: string) => {
    setLiveMessage(message);
  };

  useEffect(
    () => () => {
      if (confirmClearTimerRef.current)
        clearTimeout(confirmClearTimerRef.current);
    },
    [],
  );
  useEffect(() => {
    if (!isOpen) {
      setConfirmClear(false);
      setLiveMessage("");
      pendingFocusRef.current = null;
    }
  }, [isOpen]);
  useLayoutEffect(() => {
    const pending = pendingFocusRef.current;
    if (!pending) return;
    pendingFocusRef.current = null;
    if (pending.kind === "remove") {
      if (cart.length === 0) {
        headingRef.current?.focus();
        return;
      }
      const nextIndex = Math.min(pending.index, cart.length - 1);
      const next = document.getElementById(`guest-cart-remove-${nextIndex}`);
      if (next instanceof HTMLElement) {
        next.focus();
        return;
      }
      headingRef.current?.focus();
    }
  }, [cart]);
  const handleClearPress = () => {
    if (confirmClear) {
      if (confirmClearTimerRef.current)
        clearTimeout(confirmClearTimerRef.current);
      setConfirmClear(false);
      announce(t("menu.cartCleared"));
      onClearCart();
      queueMicrotask(() => headingRef.current?.focus());
      return;
    }
    setConfirmClear(true);
    confirmClearTimerRef.current = setTimeout(
      () => setConfirmClear(false),
      3000,
    );
  };

  const handleApplyPromo = async () => {
    const code = promoInput.trim();
    if (!code) return;
    setPromoError("");
    setPromoLoading(true);
    try {
      const result = await validatePromoCode(tableCode, code);
      onPromoApplied({ ...result.offer, code });
      setShowPromoInput(false);
      setPromoInput("");
    } catch {
      setPromoError(t("menu.promoInvalid"));
    } finally {
      setPromoLoading(false);
    }
  };

  const handleRemovePromo = () => {
    onPromoRemoved();
    setPromoInput("");
    setPromoError("");
    setShowPromoInput(false);
  };
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      // Bottom-sheet on mobile, centered card on desktop.
      placement="auto"
      scrollBehavior="inside"
      classNames={{
        base: "sm:max-w-lg !m-0 sm:mx-4 sm:my-8 max-h-[85vh] sm:max-h-[80vh] rounded-t-3xl sm:rounded-2xl border border-warm-200",
        wrapper: "items-end sm:items-center",
        body: "p-0 bg-warm-50",
        header: "px-5 py-4 bg-white border-b border-warm-200",
        footer: "px-5 py-4 bg-white border-t border-warm-200",
      }}
      motionProps={{
        variants: {
          enter: {
            y: 0,
            transition: { duration: 0.25, ease: "easeOut" },
          },
          exit: {
            y: 24,
            transition: { duration: 0.18, ease: "easeIn" },
          },
        },
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-3">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
              <ShoppingCart className="h-5 w-5" strokeWidth={1.75} />
            </div>
            <div className="min-w-0">
              <h2
                ref={headingRef}
                id="guest-cart-heading"
                tabIndex={-1}
                className="font-title text-heading-md text-ink-950 outline-none"
              >
                {t("menu.yourOrder")}
              </h2>
              <p className="text-xs text-ink-600">
                {(() => {
                  const itemCount = cart.reduce(
                    (sum, item) => sum + item.quantity,
                    0,
                  );
                  return `${itemCount} ${itemCount === 1 ? t("menu.item") : t("menu.items")}`;
                })()}
              </p>
            </div>
          </div>
        </ModalHeader>
        <ModalBody>
          <div
            role="status"
            aria-live="polite"
            aria-atomic="true"
            className="sr-only"
            data-testid="guest-cart-live"
          >
            {liveMessage}
          </div>
          {cart.length === 0 ? (
            <div className="flex flex-col items-center justify-center px-6 py-16 text-center">
              <div className="mb-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-white text-ink-400 shadow-inner ring-1 ring-warm-200">
                <ShoppingCart className="h-8 w-8" strokeWidth={1.5} />
              </div>
              <h3 className="font-title text-heading-md text-ink-900 mb-2">
                {t("menu.cartEmptyM")}
              </h3>
              <p className="max-w-xs text-sm text-ink-500">
                {t("menu.cartEmptyDescriptionM")}
              </p>
            </div>
          ) : (
            <div className="space-y-4 p-5">
              <ul className="overflow-hidden rounded-2xl border border-warm-200 bg-white divide-y divide-warm-200/70">
                {cart.map((item, index) => {
                  const lineTotal =
                    (item.price +
                      (item.itemType === "menu_item"
                        ? (item.addOns?.reduce(
                            (sum, addon) => sum + addon.price,
                            0,
                          ) ?? 0)
                        : 0)) *
                    item.quantity;
                  return (
                    <li
                      key={`${item.itemType}-${
                        "menuItemId" in item
                          ? item.menuItemId
                          : "bundleId" in item
                            ? item.bundleId
                            : "sourceOfferId" in item
                              ? item.sourceOfferId
                              : "cart"
                      }-${index}`}
                      className="px-4 py-3"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-start gap-2">
                            <p className="text-sm font-semibold text-ink-900">
                              {item.name}
                            </p>
                            {item.itemType === "bundle" && (
                              <span className="rounded-full bg-brand/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-brand">
                                {t("menu.bundles.badge")}
                              </span>
                            )}
                          </div>
                          {item.itemType === "menu_item" &&
                            item.addOns &&
                            item.addOns.length > 0 && (
                              <p className="mt-0.5 text-xs text-ink-500">
                                {item.addOns.map((a) => a.name).join(" · ")}
                              </p>
                            )}
                          {item.specialRequests && (
                            <p className="mt-1 text-xs text-ink-500">
                              <span className="font-medium text-ink-700">
                                {t("menu.specialRequestsLabel")}
                              </span>{" "}
                              {item.specialRequests}
                            </p>
                          )}
                        </div>
                        <p className="flex-shrink-0 font-mono text-sm font-semibold tabular-nums text-ink-900">
                          <CurrencyPrice
                            amount={lineTotal}
                            fromCurrency={businessCurrencies.default_currency}
                            displayCurrency={
                              businessCurrencies.display_currency
                            }
                            locale={moneyLocale}
                          />
                        </p>
                      </div>

                      <div className="mt-2 flex items-center justify-between">
                        <div className="inline-flex items-center gap-1 rounded-full border border-warm-200 bg-warm-50 p-1">
                          <Button
                            isIconOnly
                            size="sm"
                            variant="light"
                            aria-label={t(
                              "accessibility.decreaseItemQuantity",
                              { name: item.name },
                            )}
                            className="h-9 min-w-9 rounded-full text-ink-700 hover:bg-white"
                            onPress={() => {
                              const nextQty = item.quantity - 1;
                              if (nextQty <= 0) {
                                pendingFocusRef.current = {
                                  kind: "remove",
                                  index,
                                };
                                announce(
                                  t("menu.cartItemRemoved", {
                                    name: item.name,
                                  }),
                                );
                              } else {
                                announce(
                                  t("menu.cartQuantityIs", {
                                    name: item.name,
                                    quantity: nextQty,
                                  }),
                                );
                              }
                              onUpdateQuantity(index, nextQty);
                            }}
                          >
                            <Minus className="h-3.5 w-3.5" strokeWidth={2} />
                          </Button>
                          <span className="min-w-[1.75rem] text-center font-mono text-sm font-semibold tabular-nums text-ink-900">
                            {item.quantity}
                          </span>
                          <Button
                            isIconOnly
                            size="sm"
                            variant="solid"
                            aria-label={t(
                              "accessibility.increaseItemQuantity",
                              { name: item.name },
                            )}
                            className="h-9 min-w-9 rounded-full bg-brand text-white hover:bg-brand-dark"
                            onPress={() => {
                              const nextQty = item.quantity + 1;
                              announce(
                                t("menu.cartQuantityIs", {
                                  name: item.name,
                                  quantity: nextQty,
                                }),
                              );
                              onUpdateQuantity(index, nextQty);
                            }}
                          >
                            <Plus className="h-3.5 w-3.5" strokeWidth={2} />
                          </Button>
                        </div>
                        <Button
                          size="sm"
                          variant="light"
                          id={`guest-cart-remove-${index}`}
                          aria-label={t("accessibility.removeNamedItem", {
                            name: item.name,
                          })}
                          className="text-xs text-rose-700 hover:bg-rose-50 hover:text-rose-800"
                          startContent={
                            <Trash2
                              className="h-3.5 w-3.5"
                              strokeWidth={1.75}
                            />
                          }
                          onPress={() => {
                            pendingFocusRef.current = {
                              kind: "remove",
                              index,
                            };
                            announce(
                              cart.length === 1
                                ? t("menu.cartEmptyAnnouncement")
                                : t("menu.cartItemRemoved", {
                                    name: item.name,
                                  }),
                            );
                            onUpdateQuantity(index, 0);
                          }}
                        >
                          {t("menu.remove") || "Remove"}
                        </Button>
                      </div>
                    </li>
                  );
                })}
              </ul>

              {/* Total */}
              <div className="rounded-2xl border border-warm-200 bg-white p-5">
                <dl className="space-y-2 text-sm">
                  <div className="flex justify-between text-ink-700">
                    <dt>{t("menu.summary.subtotal")}</dt>
                    <dd className="font-mono tabular-nums">
                      <CurrencyPrice
                        amount={promotionPreview.baseSubtotal}
                        fromCurrency={businessCurrencies.default_currency}
                        displayCurrency={businessCurrencies.display_currency}
                        locale={moneyLocale}
                      />
                    </dd>
                  </div>
                  {promotionPreview.autoDiscountTotal > 0 && (
                    <div className="flex justify-between text-emerald-700">
                      <dt>{t("menu.summary.autoDiscounts")}</dt>
                      <dd className="font-mono tabular-nums">
                        −
                        <CurrencyPrice
                          amount={promotionPreview.autoDiscountTotal}
                          fromCurrency={businessCurrencies.default_currency}
                          displayCurrency={businessCurrencies.display_currency}
                          locale={moneyLocale}
                        />
                      </dd>
                    </div>
                  )}
                  {promotionPreview.promoDiscount > 0 && (
                    <div className="flex justify-between text-emerald-700">
                      <dt>{t("menu.promoCode")}</dt>
                      <dd className="font-mono tabular-nums">
                        −
                        <CurrencyPrice
                          amount={promotionPreview.promoDiscount}
                          fromCurrency={businessCurrencies.default_currency}
                          displayCurrency={businessCurrencies.display_currency}
                          locale={moneyLocale}
                        />
                      </dd>
                    </div>
                  )}
                  {promotionPreview.applied.length > 0 && (
                    <ul className="space-y-1 rounded-xl border border-emerald-100 bg-emerald-50/60 p-2">
                      {promotionPreview.applied.map((entry) => (
                        <li
                          key={entry.offer.id ?? entry.offer.name}
                          className="flex justify-between text-xs text-emerald-800"
                        >
                          <span>{entry.offer.name}</span>
                          <span className="font-mono tabular-nums">
                            −
                            <CurrencyPrice
                              amount={entry.amount}
                              fromCurrency={businessCurrencies.default_currency}
                              displayCurrency={
                                businessCurrencies.display_currency
                              }
                              locale={moneyLocale}
                            />
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                  <div className="flex justify-between text-ink-700">
                    <dt>{t("common.tax")}</dt>
                    {quotePending ? (
                      <QuotePendingAmount label={t("menu.updatingTotal")} />
                    ) : (
                      <dd className="font-mono tabular-nums">
                        <CurrencyPrice
                          amount={promotionPreview.tax}
                          fromCurrency={businessCurrencies.default_currency}
                          displayCurrency={businessCurrencies.display_currency}
                          locale={moneyLocale}
                        />
                      </dd>
                    )}
                  </div>
                  <div className="flex justify-between text-ink-700">
                    <dt>{t("common.serviceFee")}</dt>
                    {quotePending ? (
                      <QuotePendingAmount label={t("menu.updatingTotal")} />
                    ) : (
                      <dd className="font-mono tabular-nums">
                        <CurrencyPrice
                          amount={promotionPreview.serviceFee}
                          fromCurrency={businessCurrencies.default_currency}
                          displayCurrency={businessCurrencies.display_currency}
                          locale={moneyLocale}
                        />
                      </dd>
                    )}
                  </div>
                  {promotionPreview.tip > 0 && (
                    <div className="flex justify-between text-ink-700">
                      <dt>{t("common.tip")}</dt>
                      <dd className="font-mono tabular-nums">
                        <CurrencyPrice
                          amount={promotionPreview.tip}
                          fromCurrency={businessCurrencies.default_currency}
                          displayCurrency={businessCurrencies.display_currency}
                          locale={moneyLocale}
                        />
                      </dd>
                    </div>
                  )}
                  <div className="flex items-center justify-between border-t border-warm-200 pt-3">
                    <dt className="text-base font-semibold text-ink-900">
                      {t("common.total")}
                    </dt>
                    {quotePending ? (
                      <QuotePendingAmount
                        label={t("menu.updatingTotal")}
                        className="font-mono text-xl font-semibold tabular-nums text-ink-950"
                      />
                    ) : (
                      <dd className="font-mono text-xl font-semibold tabular-nums text-ink-950">
                        <CurrencyPrice
                          amount={promotionPreview.finalTotal}
                          fromCurrency={businessCurrencies.default_currency}
                          displayCurrency={businessCurrencies.display_currency}
                          locale={moneyLocale}
                        />
                      </dd>
                    )}
                  </div>
                </dl>
              </div>

              {/* Fiscal invoice (AFIP/ARCA). Only AR venues have this flow;
                  fail closed when country/settings are unknown (#548). */}
              {showFiscalIdentity ? (
                <div data-testid="guest-fiscal-identity">
                  <FiscalIdentityFields
                    value={fiscalIdentity}
                    onChange={(next, valid) => {
                      setFiscalIdentity(next);
                      onFiscalIdentityChange?.(next, valid);
                    }}
                    // Guest fiscal strings live under menu.fiscalCustomer.*;
                    // the component emits keys relative to fiscalCustomer.*,
                    // so prefix the namespace here.
                    t={(key) => t(`menu.${key}`)}
                    variant="guest"
                  />
                </div>
              ) : null}

              {/* Promo code section */}
              <div className="rounded-2xl border border-warm-200 bg-white p-4">
                {appliedPromo ? (
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2 text-sm text-emerald-700">
                      <Tag
                        className="h-4 w-4 flex-shrink-0"
                        strokeWidth={1.75}
                      />
                      <span className="font-medium">
                        {appliedPromo.discount_type === "percentage"
                          ? t("menu.promoApplied", {
                              // Show the code the guest entered, not the offer
                              // name (PROMO-001). Fall back to the name only if
                              // no code is present on the offer.
                              code: appliedPromo.code || appliedPromo.name,
                              discount: `${appliedPromo.discount_value}%`,
                            })
                          : // The authoritative (clamped) discount amount is shown
                            // in the summary line above; here we just confirm the
                            // code so the raw value can't contradict the total.
                            appliedPromo.code || appliedPromo.name}
                      </span>
                    </div>
                    <Button
                      size="sm"
                      variant="light"
                      isIconOnly
                      aria-label={t("menu.removePromo")}
                      onPress={handleRemovePromo}
                      className="h-7 min-w-7 rounded-full text-ink-400 hover:bg-rose-50 hover:text-rose-600"
                    >
                      <X className="h-3.5 w-3.5" strokeWidth={2} />
                    </Button>
                  </div>
                ) : showPromoInput ? (
                  <div className="space-y-2">
                    <div className="flex gap-2">
                      <Input
                        size="sm"
                        placeholder={t("menu.promoCodePlaceholder")}
                        value={promoInput}
                        onChange={(e) => {
                          setPromoInput(e.target.value);
                          setPromoError("");
                        }}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") handleApplyPromo();
                        }}
                        classNames={{
                          input: "text-sm",
                          inputWrapper:
                            "h-9 bg-warm-50 border-warm-200 data-[hover=true]:bg-warm-50 data-[hover=true]:border-ink-300",
                        }}
                        aria-label={t("menu.promoCodePlaceholder")}
                        isInvalid={Boolean(promoError)}
                        aria-describedby={
                          promoError ? "cart-promo-error" : undefined
                        }
                      />
                      <Button
                        size="sm"
                        onPress={handleApplyPromo}
                        isLoading={promoLoading}
                        className="h-9 shrink-0 rounded-xl bg-brand px-4 text-sm font-semibold text-white hover:bg-brand-dark"
                      >
                        {t("menu.applyPromo")}
                      </Button>
                    </div>
                    {promoError && (
                      <p
                        id="cart-promo-error"
                        role="alert"
                        className="text-xs text-rose-700"
                      >
                        {promoError}
                      </p>
                    )}
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => setShowPromoInput(true)}
                    className="flex items-center gap-2 text-sm font-medium text-brand-dark hover:text-brand-800"
                  >
                    <Tag className="h-4 w-4" strokeWidth={1.75} />
                    {t("menu.promoCode")}
                  </button>
                )}
              </div>
            </div>
          )}
        </ModalBody>
        <ModalFooter
          className="flex flex-col gap-2"
          style={{
            paddingBottom: "max(env(safe-area-inset-bottom), 1rem)",
          }}
        >
          {cart.length > 0 && (
            <div className="w-full space-y-2">
              {quoteError || quoteBlocked ? (
                <p
                  role="alert"
                  className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
                >
                  {quoteBlocked
                    ? quoteBlockedMessage ||
                      t("menu.orderErrorItemUnavailableGeneric")
                    : t("errors.menuUnavailableDescription")}
                </p>
              ) : null}
              <div className="flex w-full gap-2">
                <Button
                  type="button"
                  size="lg"
                  data-testid="cart-place-order"
                  onPress={onSubmitOrder}
                  isLoading={orderLoading || quotePending}
                  isDisabled={
                    orderLoading ||
                    quotePending ||
                    Boolean(quoteError) ||
                    !quoteValid ||
                    confirmClear
                  }
                  className="flex-1 h-12 rounded-xl bg-ink-950 font-semibold text-white shadow-sm hover:bg-ink-900"
                >
                  {orderLoading
                    ? t("menu.orderProcessing")
                    : t("menu.placeOrder")}
                </Button>
                <Button
                  type="button"
                  isIconOnly
                  size="lg"
                  variant="flat"
                  data-testid="cart-clear"
                  aria-pressed={confirmClear}
                  aria-label={
                    confirmClear
                      ? t("menu.clearCartConfirmAria") ||
                        t("menu.clearCartConfirm")
                      : t("menu.clearCart") || "Clear cart"
                  }
                  title={
                    confirmClear
                      ? t("menu.clearCartConfirm")
                      : t("menu.clearCart")
                  }
                  onPress={handleClearPress}
                  className={
                    confirmClear
                      ? "h-12 w-12 min-w-12 shrink-0 rounded-xl bg-rose-600 text-white hover:bg-rose-700"
                      : "h-12 w-12 min-w-12 shrink-0 rounded-xl bg-rose-50 text-rose-600 hover:bg-rose-100"
                  }
                >
                  <Trash2 className="h-5 w-5" strokeWidth={1.75} />
                </Button>
              </div>
            </div>
          )}
          <Button
            variant="light"
            onPress={onClose}
            className="w-full text-ink-500 hover:text-ink-900"
          >
            {t("menu.continueShopping")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}

"use client";

import React, { useContext, useEffect, useMemo, useState } from "react";
import { Card, CardBody, Button, Chip, Skeleton } from "@nextui-org/react";
import { Truck, Clock, Receipt, MapPin, ChevronRight } from "lucide-react";
import GuestDeliveryOrder, {
  GuestDeliveryEditorContext,
  GuestDeliveryQuoteContext,
} from "./GuestDeliveryOrder";
import { DeliveryQuoteDto, DeliverySettingsDto } from "@/api/delivery";
import { formatGuestCurrency } from "@/utils/guestCurrencyFormatter";
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";

const ADDRESS_QUOTE_REASONS = new Set(["quote_available", "below_minimum"]);

function isAddressSpecificQuote(quote?: DeliveryQuoteDto | null): boolean {
  return Boolean(quote && ADDRESS_QUOTE_REASONS.has(quote.reason_code));
}

interface DesignSettings {
  primary_color: string;
  secondary_color: string;
  background_pattern?: string;
  pattern_opacity?: number;
  corner_radius?: string;
  shadow_intensity?: string;
  font_family?: string;
}

interface DeliveryAvailableCardProps {
  businessId: number;
  businessName: string;
  customerId?: number;
  onQuoteReady?: (context: GuestDeliveryQuoteContext) => void;
  deliverySettings?: DeliverySettingsDto | null;
  loading?: boolean;
  designSettings?: DesignSettings;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /** Business address country, pre-filled into the address form. */
  defaultCountry?: string;
  /**
   * When delivery hours match business hours (the common default), the card
   * must not claim "Delivery available" while the restaurant is closed.
   * Omitted means "unknown", so we do not invent a closed state.
   */
  isBusinessOpen?: boolean;
  /**
   * Saved fulfillment context. Used to prefill the editor and to distinguish
   * starting estimates from an address-specific live quote.
   */
  fulfillmentContext?:
    | (NonNullable<GuestDeliveryEditorContext> & {
        quote?: DeliveryQuoteDto;
        saved_at?: string;
      })
    | null;
  /**
   * Increment from the parent Edit action so the address form opens in one
   * step instead of requiring a second CTA click.
   */
  openAddressEditorToken?: number;
}

export default function DeliveryAvailableCard({
  businessId,
  businessName,
  customerId,
  onQuoteReady,
  deliverySettings,
  loading = false,
  // eslint-disable-next-line no-restricted-syntax -- API-stored design defaults
  designSettings = { primary_color: "#1a6b6a", secondary_color: "#2a8b8a" },
  currency = "USD",
  defaultCountry = "",
  isBusinessOpen,
  fulfillmentContext = null,
  openAddressEditorToken = 0,
}: DeliveryAvailableCardProps) {
  const ctx = useContext(GuestTranslationContext);
  const t = (k: string, params?: Record<string, string | number>) =>
    (ctx?.t?.(k, params) as string) || "";
  const fmtCurrency = (amount: number, code: string) =>
    formatGuestCurrency(amount, code, ctx?.currentLanguage);
  const [showOrderModal, setShowOrderModal] = useState(false);

  useEffect(() => {
    if (openAddressEditorToken > 0) {
      setShowOrderModal(true);
    }
  }, [openAddressEditorToken]);

  const radiusClass =
    designSettings.corner_radius === "none"
      ? "rounded-none"
      : designSettings.corner_radius === "small"
        ? "rounded-sm"
        : designSettings.corner_radius === "large"
          ? "rounded-xl"
          : "rounded-lg";

  const shadowClass =
    designSettings.shadow_intensity === "none"
      ? "shadow-none"
      : designSettings.shadow_intensity === "medium"
        ? "shadow-md"
        : designSettings.shadow_intensity === "strong"
          ? "shadow-xl"
          : "shadow-sm";

  const hasLiveQuote = isAddressSpecificQuote(fulfillmentContext?.quote);

  // Delivery hours (DLV-GUEST-6): when the business publishes a dedicated
  // delivery window, surface it on the card and flag the outside-hours state
  // instead of implying 24/7 availability. When hours match the business
  // schedule, fall through so we can honor isBusinessOpen (Closed Mode honesty).
  const deliveryHours = useMemo(() => {
    if (
      !deliverySettings ||
      deliverySettings.delivery_hours_same_as_business ||
      !deliverySettings.delivery_start_time ||
      !deliverySettings.delivery_end_time
    ) {
      return null;
    }
    const start = deliverySettings.delivery_start_time.slice(0, 5);
    const end = deliverySettings.delivery_end_time.slice(0, 5);
    const toMinutes = (value: string): number | null => {
      const match = /^(\d{1,2}):(\d{2})/.exec(value);
      if (!match) return null;
      return parseInt(match[1], 10) * 60 + parseInt(match[2], 10);
    };
    const startMin = toMinutes(start);
    const endMin = toMinutes(end);
    if (startMin === null || endMin === null || startMin === endMin)
      return null;
    const now = new Date();
    const nowMin = now.getHours() * 60 + now.getMinutes();
    // Overnight windows (e.g. 18:00–02:00) wrap past midnight.
    const withinWindow =
      startMin < endMin
        ? nowMin >= startMin && nowMin < endMin
        : nowMin >= startMin || nowMin < endMin;
    return { start, end, withinWindow };
  }, [deliverySettings]);

  // Accepting orders right now? Prefer custom delivery window; otherwise use
  // business open status when hours match the restaurant schedule.
  const acceptingOrders = useMemo(() => {
    if (deliveryHours) return deliveryHours.withinWindow;
    if (typeof isBusinessOpen === "boolean") return isBusinessOpen;
    // Unknown open state (caller didn't wire it) — don't invent a closed banner.
    return true;
  }, [deliveryHours, isBusinessOpen]);

  if (loading) {
    return (
      <Card className={`border-2 ${shadowClass} ${radiusClass} bg-white`}>
        <CardBody className="p-6 space-y-4">
          <Skeleton className="h-6 w-32 rounded-lg" />
          <Skeleton className="h-4 w-full rounded-lg" />
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <Skeleton className="h-20 rounded-2xl" />
            <Skeleton className="h-20 rounded-2xl" />
            <Skeleton className="h-20 rounded-2xl" />
          </div>
          <Skeleton className="h-12 rounded-2xl" />
        </CardBody>
      </Card>
    );
  }

  if (
    !deliverySettings?.delivery_enabled ||
    !deliverySettings?.in_house_delivery_enabled
  ) {
    return null;
  }

  const liveQuote = hasLiveQuote ? fulfillmentContext?.quote : null;
  const deliveryFee = liveQuote
    ? liveQuote.delivery_fee
    : deliverySettings.flat_delivery_fee;
  const minTravelMinutes = liveQuote
    ? liveQuote.estimated_total_minutes
    : deliverySettings.estimated_delivery_minutes ||
      deliverySettings.estimated_prep_time + 30;
  const freeAbove = liveQuote
    ? liveQuote.free_delivery_minimum
    : deliverySettings.free_delivery_minimum;
  const minimumOrder = liveQuote
    ? liveQuote.minimum_order_amount
    : deliverySettings.minimum_order_amount;
  const deliveryAreaLabel = liveQuote
    ? liveQuote.zone?.name ||
      fulfillmentContext?.delivery_address?.formatted_address ||
      t("businessPage.delivery.localArea") ||
      "Local area"
    : t("businessPage.delivery.areaUnconfirmed") ||
      "We'll confirm your area after you enter an address.";

  return (
    <>
      <Card
        className={`border-2 ${shadowClass} transition-all duration-300 ${radiusClass} bg-white`}
        style={{ borderColor: `${designSettings.primary_color}33` }}
      >
        <CardBody className="p-6">
          <div className="flex items-start justify-between mb-4">
            <div className="flex items-center gap-3">
              <div
                className={`w-12 h-12 ${radiusClass} flex items-center justify-center`}
                style={{ backgroundColor: `${designSettings.primary_color}15` }}
              >
                <Truck
                  className="w-6 h-6"
                  style={{ color: designSettings.primary_color }}
                />
              </div>
              <div>
                <h3 className="text-xl font-semibold text-gray-900">
                  {acceptingOrders
                    ? t("businessPage.delivery.availableTitle") ||
                      "Delivery available"
                    : t("businessPage.deliveryQuote.outsideHours") ||
                      "Delivery is not available at this time."}
                </h3>
                <p className="text-sm text-gray-600">
                  {acceptingOrders
                    ? t("businessPage.delivery.availableSubtitle") ||
                      "Enter your address to see delivery fees and timing for your area."
                    : t("menu.businessClosedDescription") ||
                      "You can still check fees and browse the menu — ordering resumes when we open."}
                </p>
              </div>
            </div>
            {acceptingOrders ? (
              <Chip
                variant="flat"
                size="sm"
                data-testid="delivery-quote-chip"
                style={{
                  backgroundColor: `${designSettings.primary_color}15`,
                  color: designSettings.primary_color,
                }}
              >
                {hasLiveQuote
                  ? t("businessPage.delivery.liveQuote") || "Live quote"
                  : t("businessPage.delivery.startingEstimate") ||
                    "Starting estimate"}
              </Chip>
            ) : (
              <Chip
                variant="flat"
                size="sm"
                className="bg-amber-100 text-amber-800"
              >
                {t("businessPage.closed") || "Closed"}
              </Chip>
            )}
          </div>

          <div className="space-y-3 mb-6">
            <div className="flex items-center gap-3 text-gray-700">
              <div
                className={`w-8 h-8 bg-white ${radiusClass} flex items-center justify-center shadow-sm`}
              >
                <Clock
                  className="w-4 h-4"
                  style={{ color: designSettings.primary_color }}
                />
              </div>
              <div>
                <p className="text-sm font-medium">
                  {hasLiveQuote
                    ? t("businessPage.delivery.estimatedTime") ||
                      "Estimated delivery time"
                    : t("businessPage.delivery.startingTimeLabel") ||
                      "Starting estimated time"}
                </p>
                <p className="text-xs text-gray-600">
                  {t("businessPage.delivery.estimatedTimeValue", {
                    minutes: minTravelMinutes,
                  }) || `About ${minTravelMinutes} minutes including prep`}
                </p>
                {!hasLiveQuote ? (
                  <p className="text-xs text-gray-600">
                    {t("businessPage.delivery.startingTimeHelp") ||
                      "Typical starting time. We'll confirm timing for your address."}
                  </p>
                ) : null}
                {deliveryHours ? (
                  <p
                    className="text-xs text-gray-600"
                    data-testid="delivery-hours"
                  >
                    {t("businessPage.delivery.hoursToday", {
                      start: deliveryHours.start,
                      end: deliveryHours.end,
                    }) ||
                      `Delivery hours: ${deliveryHours.start}–${deliveryHours.end}`}
                  </p>
                ) : null}
              </div>
            </div>

            <div className="flex items-center gap-3 text-gray-700">
              <div
                className={`w-8 h-8 bg-white ${radiusClass} flex items-center justify-center shadow-sm`}
              >
                <Receipt
                  className="w-4 h-4"
                  style={{ color: designSettings.primary_color }}
                />
              </div>
              <div>
                <p className="text-sm font-medium">
                  {deliveryFee === 0
                    ? t("businessPage.delivery.freeDelivery") || "Free delivery"
                    : hasLiveQuote
                      ? t("businessPage.delivery.feeWithAmount", {
                          amount: fmtCurrency(deliveryFee, currency),
                        }) ||
                        `${fmtCurrency(deliveryFee, currency)} delivery fee`
                      : t("businessPage.delivery.startingFeeLabel", {
                          amount: fmtCurrency(deliveryFee, currency),
                        }) ||
                        `Starting at ${fmtCurrency(deliveryFee, currency)} delivery fee`}
                </p>
                {!hasLiveQuote ? (
                  <p className="text-xs text-gray-600">
                    {t("businessPage.delivery.startingFeeHelp") ||
                      "Typical starting fee. We'll confirm the fee for your address."}
                  </p>
                ) : null}
                {/* Only mention thresholds that are actually configured —
                    never "Free above $0.00. Minimum order $0.00." (DLV-GUEST-8) */}
                {(freeAbove > 0 || minimumOrder > 0) && (
                  <p className="text-xs text-gray-600">
                    {[
                      freeAbove > 0
                        ? t("businessPage.delivery.freeAboveAmount", {
                            amount: fmtCurrency(freeAbove, currency),
                          }) ||
                          `Free above ${fmtCurrency(freeAbove, currency)}.`
                        : null,
                      minimumOrder > 0
                        ? t("businessPage.delivery.minimumOrderAmount", {
                            amount: fmtCurrency(minimumOrder, currency),
                          }) ||
                          `Minimum order ${fmtCurrency(minimumOrder, currency)}.`
                        : null,
                    ]
                      .filter(Boolean)
                      .join(" ")}
                  </p>
                )}
              </div>
            </div>

            <div className="flex items-center gap-3 text-gray-700">
              <div
                className={`w-8 h-8 bg-white ${radiusClass} flex items-center justify-center shadow-sm`}
              >
                <MapPin
                  className="w-4 h-4"
                  style={{ color: designSettings.primary_color }}
                />
              </div>
              <div>
                <p className="text-sm font-medium">
                  {t("businessPage.delivery.areaLabel") || "Delivery area"}
                </p>
                <p
                  className="text-xs text-gray-600"
                  data-testid="delivery-area-value"
                >
                  {deliveryAreaLabel}
                </p>
              </div>
            </div>
          </div>

          {!acceptingOrders ? (
            <p
              className="text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2 mb-3"
              data-testid="outside-hours-notice"
            >
              {deliveryHours
                ? t("businessPage.delivery.outsideHoursNotice", {
                    time: deliveryHours.start,
                  }) ||
                  `Delivery is closed right now — it opens at ${deliveryHours.start}.`
                : t("businessPage.deliveryQuote.outsideHours") ||
                  "Delivery is not available at this time."}
            </p>
          ) : null}

          {hasLiveQuote ? (
            <p
              className="text-xs text-gray-600 mb-3"
              data-testid="delivery-quote-meta"
            >
              {[
                fulfillmentContext?.delivery_address?.formatted_address
                  ? t("businessPage.delivery.quotedForAddress", {
                      address:
                        fulfillmentContext.delivery_address.formatted_address,
                    }) ||
                    `Quoted for ${fulfillmentContext.delivery_address.formatted_address}.`
                  : t("businessPage.delivery.addressQuote") ||
                    "Quote for your address",
                t("businessPage.delivery.refreshQuoteHint") ||
                  "Recheck your address if it changed.",
              ].join(" ")}
            </p>
          ) : null}

          <Button
            size="lg"
            data-testid="delivery-start-cta"
            className="h-auto min-h-12 w-full whitespace-normal overflow-visible py-3 font-semibold text-white [&>span]:h-auto [&>span]:whitespace-normal [&>span]:overflow-visible"
            style={{
              background: `linear-gradient(135deg, ${designSettings.primary_color}, ${designSettings.secondary_color})`,
            }}
            endContent={<ChevronRight className="w-5 h-5 shrink-0" />}
            onPress={() => setShowOrderModal(true)}
          >
            <span className="max-w-full whitespace-normal break-words text-center leading-snug line-clamp-2">
              {acceptingOrders
                ? hasLiveQuote
                  ? t("businessPage.delivery.updateAddressCta") ||
                    "Update address"
                  : t("businessPage.delivery.checkAddressCta") ||
                    "Check address and start order"
                : t("businessPage.guestDelivery.checkAddress") ||
                  "Check address"}
            </span>
          </Button>

          <p className="text-xs text-center text-gray-500 mt-3">
            {t("businessPage.delivery.orderAfterCheckout") ||
              "You'll confirm everything at checkout — nothing is ordered yet."}
          </p>
        </CardBody>
      </Card>

      <GuestDeliveryOrder
        isOpen={showOrderModal}
        onClose={() => setShowOrderModal(false)}
        businessId={businessId}
        businessName={businessName}
        customerId={customerId}
        currency={currency}
        defaultCountry={defaultCountry}
        initialContext={fulfillmentContext}
        onQuoteReady={(context) => {
          setShowOrderModal(false);
          onQuoteReady?.(context);
        }}
      />
    </>
  );
}

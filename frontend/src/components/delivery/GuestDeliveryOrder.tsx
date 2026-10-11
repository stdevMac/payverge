"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Switch,
  Card,
  CardBody,
  Chip,
  Divider,
} from "@nextui-org/react";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { AccessibleTextarea } from "@/components/ui/AccessibleTextarea";
import {
  MapPin,
  Clock,
  Receipt,
  Truck,
  ExternalLink,
  AlertCircle,
  CheckCircle,
} from "lucide-react";
import { guestDeliveryApi, DeliveryQuoteDto } from "@/api/delivery";
import { asDollars } from "@/types/money";
import { formatGuestCurrency } from "@/utils/guestCurrencyFormatter";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { isSafeExternalTrackingUrl } from "@/lib/externalUrl";
import {
  localizeBackendError,
  localizeQuoteReason,
} from "./deliveryGuestLocalization";

interface GuestDeliveryAddress {
  street: string;
  apartment?: string;
  city: string;
  state?: string;
  postal_code?: string;
  country: string;
  formatted_address: string;
}

export interface GuestDeliveryQuoteContext {
  delivery_address: GuestDeliveryAddress;
  delivery_instructions?: string;
  contactless_delivery: boolean;
  leave_at_door: boolean;
  quote: DeliveryQuoteDto;
}

/** Saved fulfillment / quote context used to prefill the address editor. */
export type GuestDeliveryEditorContext = {
  delivery_address?: Partial<GuestDeliveryAddress> | null;
  delivery_instructions?: string;
  contactless_delivery?: boolean;
  leave_at_door?: boolean;
} | null;

interface GuestDeliveryOrderProps {
  isOpen: boolean;
  onClose: () => void;
  businessId: number;
  businessName: string;
  customerId?: number;
  onQuoteReady?: (context: GuestDeliveryQuoteContext) => void;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  /**
   * Pre-fill the country field from the business's own address rather than a
   * hardcoded US literal. Left blank when the business country is unknown.
   */
  defaultCountry?: string;
  /**
   * Existing delivery context. When the guest edits a saved address the
   * structured fields, instructions, and preferences are restored. The saved
   * quote is not reused — they must re-check before replacing the context.
   */
  initialContext?: GuestDeliveryEditorContext;
}

type AddressField = "street" | "city" | "country";

const quotableReasonCodes = new Set(["quote_available", "below_minimum"]);

export default function GuestDeliveryOrder({
  isOpen,
  onClose,
  businessId,
  businessName,
  onQuoteReady,
  currency = "USD",
  defaultCountry = "",
  initialContext = null,
}: GuestDeliveryOrderProps) {
  const { t, currentLanguage } = useGuestTranslation();
  const fmtCurrency = (amount: number, code: string) =>
    formatGuestCurrency(amount, code, currentLanguage);
  const [loading, setLoading] = useState(false);
  const [quote, setQuote] = useState<DeliveryQuoteDto | null>(null);

  const [street, setStreet] = useState("");
  const [apartment, setApartment] = useState("");
  const [city, setCity] = useState("");
  const [state, setState] = useState("");
  const [postalCode, setPostalCode] = useState("");
  const [country, setCountry] = useState(defaultCountry);
  const [deliveryInstructions, setDeliveryInstructions] = useState("");
  const [contactlessDelivery, setContactlessDelivery] = useState(false);
  const [leaveAtDoor, setLeaveAtDoor] = useState(false);
  const [formError, setFormError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<AddressField, string>>>({});

  const streetRef = useRef<HTMLInputElement>(null);
  const cityRef = useRef<HTMLInputElement>(null);
  const countryRef = useRef<HTMLInputElement>(null);
  const initialContextRef = useRef(initialContext);
  const defaultCountryRef = useRef(defaultCountry);
  initialContextRef.current = initialContext;
  defaultCountryRef.current = defaultCountry;

  const formattedAddress = useMemo(() => {
    return [street, apartment, city, state, postalCode, country]
      .map((value) => value?.trim())
      .filter(Boolean)
      .join(", ");
  }, [street, apartment, city, state, postalCode, country]);

  const canContinue = quote ? quotableReasonCodes.has(quote.reason_code) : false;
  const minimumDelta = quote
    ? Math.max(quote.minimum_order_amount - quote.order_subtotal, 0)
    : 0;

  useEffect(() => {
    setQuote(null);
  }, [street, apartment, city, state, postalCode, country]);

  const initialStreet = initialContext?.delivery_address?.street ?? "";
  const initialCity = initialContext?.delivery_address?.city ?? "";
  useEffect(() => {
    if (!isOpen) return;
    const ctx = initialContextRef.current;
    const address = ctx?.delivery_address;
    setStreet(address?.street ?? "");
    setApartment(address?.apartment ?? "");
    setCity(address?.city ?? "");
    setState(address?.state ?? "");
    setPostalCode(address?.postal_code ?? "");
    setCountry(address?.country ?? defaultCountryRef.current ?? "");
    setDeliveryInstructions(ctx?.delivery_instructions ?? "");
    setContactlessDelivery(Boolean(ctx?.contactless_delivery));
    setLeaveAtDoor(Boolean(ctx?.leave_at_door));
    setQuote(null);
    setFormError("");
    setFieldErrors({});
    setLoading(false);
  }, [isOpen, initialStreet, initialCity]);

  const clearFieldError = (field: AddressField) => {
    setFieldErrors((prev) => {
      if (!prev[field]) return prev;
      const next = { ...prev };
      delete next[field];
      return next;
    });
  };

  const resetState = () => {
    setLoading(false);
    setQuote(null);
    setFormError("");
    setFieldErrors({});
    setStreet("");
    setApartment("");
    setCity("");
    setState("");
    setPostalCode("");
    setCountry(defaultCountry);
    setDeliveryInstructions("");
    setContactlessDelivery(false);
    setLeaveAtDoor(false);
  };

  const handleClose = () => {
    resetState();
    onClose();
  };

  const handleQuote = async () => {
    const nextErrors: Partial<Record<AddressField, string>> = {};
    if (!street.trim()) {
      nextErrors.street =
        (t("businessPage.guestDelivery.streetRequired") as string) ||
        "Enter a street address.";
    }
    if (!city.trim()) {
      nextErrors.city =
        (t("businessPage.guestDelivery.cityRequired") as string) ||
        "Enter a city.";
    }
    if (!country.trim()) {
      nextErrors.country =
        (t("businessPage.guestDelivery.countryRequired") as string) ||
        "Enter a country.";
    }
    if (Object.keys(nextErrors).length > 0) {
      setQuote(null);
      setFieldErrors(nextErrors);
      setFormError(t("businessPage.deliveryQuote.addressRequired"));
      const firstId = nextErrors.street
        ? "guest-delivery-street"
        : nextErrors.city
          ? "guest-delivery-city"
          : "guest-delivery-country";
      window.setTimeout(() => {
        const native =
          document.getElementById(firstId) ||
          document.querySelector<HTMLElement>(`#${firstId} input`);
        native?.focus();
      }, 0);
      return;
    }

    setFieldErrors({});
    setLoading(true);
    setQuote(null);
    setFormError("");
    try {
      const response = await guestDeliveryApi.quote(businessId, {
        order_subtotal: asDollars(0),
        delivery_address: {
          street,
          apartment,
          city,
          state,
          postal_code: postalCode,
          country,
          formatted_address: formattedAddress,
        },
      });
      setQuote(response);
    } catch (error) {
      // Backend errors arrive in English — map them to the guest's locale and
      // never render the raw message on this 21-locale surface. (DLV-GUEST-2)
      setFormError(
        error instanceof Error && error.message
          ? localizeBackendError(error.message, t, "businessPage.deliveryQuote.quoteFailed")
          : (t("businessPage.deliveryQuote.quoteFailed") as string),
      );
    } finally {
      setLoading(false);
    }
  };

  const handleContinue = () => {
    if (!quote || !canContinue) {
      return;
    }

    onQuoteReady?.({
      delivery_address: {
        street: street.trim(),
        apartment: apartment.trim() || undefined,
        city: city.trim(),
        state: state.trim() || undefined,
        postal_code: postalCode.trim() || undefined,
        country: country.trim(),
        formatted_address: formattedAddress,
      },
      delivery_instructions: deliveryInstructions.trim() || undefined,
      contactless_delivery: contactlessDelivery,
      leave_at_door: leaveAtDoor,
      quote,
    });
    handleClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="3xl"
      scrollBehavior="inside"
      classNames={{
        base: "bg-white",
        header: "border-b border-warm-200",
        footer: "border-t border-warm-200",
      }}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Chip color="primary" variant="flat">
              {(t("businessPage.guestDelivery.stepOne") as string) || "Step 1"}
            </Chip>
            <span className="text-sm text-ink-500">
              {(t("businessPage.guestDelivery.checkAddressIntro") as string) ||
                "Check your address before you browse the menu."}
            </span>
          </div>
          <h2 className="text-2xl font-semibold text-ink-900">
            {((t("businessPage.guestDelivery.title", { businessName }) as string) || `Delivery for ${businessName}`)}
          </h2>
          <p className="text-sm text-ink-500">
            {(t("businessPage.guestDelivery.subtitle") as string) ||
              "Your address and delivery quote stay with you while you browse the menu."}
          </p>
        </ModalHeader>

        <ModalBody className="py-6">
          {/* Real form semantics so pressing Enter in any address field runs
              the quote check. (DLV-GUEST-10) */}
          <form
            id="guest-delivery-address-form"
            className="space-y-6"
            onSubmit={(e) => {
              e.preventDefault();
              void handleQuote();
            }}
          >
          {/* Hidden submit button enables implicit form submission on Enter. */}
          <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <AccessibleInput
              ref={streetRef}
              id="guest-delivery-street"
              label={(t("businessPage.guestDelivery.streetAddress") as string) || "Street address"}
              value={street}
              onChange={(e) => {
                setStreet(e.target.value);
                clearFieldError("street");
              }}
              startContent={<MapPin className="w-4 h-4 text-ink-400" />}
              autoComplete="address-line1"
              isRequired
              isInvalid={Boolean(fieldErrors.street)}
              errorMessage={fieldErrors.street}
              aria-invalid={Boolean(fieldErrors.street) || undefined}
            />
            <AccessibleInput
              label={(t("businessPage.guestDelivery.apartment") as string) || "Apartment, suite, unit"}
              value={apartment}
              onChange={(e) => setApartment(e.target.value)}
              autoComplete="address-line2"
            />
            <AccessibleInput
              ref={cityRef}
              id="guest-delivery-city"
              label={(t("businessPage.guestDelivery.city") as string) || "City"}
              value={city}
              onChange={(e) => {
                setCity(e.target.value);
                clearFieldError("city");
              }}
              autoComplete="address-level2"
              isRequired
              isInvalid={Boolean(fieldErrors.city)}
              errorMessage={fieldErrors.city}
              aria-invalid={Boolean(fieldErrors.city) || undefined}
            />
            <AccessibleInput
              label={(t("businessPage.guestDelivery.state") as string) || "State / province"}
              value={state}
              onChange={(e) => setState(e.target.value)}
              autoComplete="address-level1"
            />
            <AccessibleInput
              label={(t("businessPage.guestDelivery.postalCode") as string) || "Postal code"}
              value={postalCode}
              onChange={(e) => setPostalCode(e.target.value)}
              autoComplete="postal-code"
              // Postcodes are alphanumeric in many countries (UK, CA, NL…) —
              // never force the numeric keyboard. (DLV-GUEST-9)
              inputMode="text"
            />
            <AccessibleInput
              ref={countryRef}
              id="guest-delivery-country"
              label={(t("businessPage.guestDelivery.country") as string) || "Country"}
              value={country}
              onChange={(e) => {
                setCountry(e.target.value);
                clearFieldError("country");
              }}
              autoComplete="country-name"
              isRequired
              isInvalid={Boolean(fieldErrors.country)}
              errorMessage={fieldErrors.country}
              aria-invalid={Boolean(fieldErrors.country) || undefined}
            />
          </div>

          <AccessibleTextarea
            label={(t("businessPage.guestDelivery.instructionsLabel") as string) || "Delivery instructions for checkout"}
            value={deliveryInstructions}
            onChange={(e) => setDeliveryInstructions(e.target.value)}
            minRows={2}
            description={
              (t("businessPage.guestDelivery.instructionsHelp") as string) ||
              "You can keep these now, then confirm contact details later during checkout."
            }
          />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="flex items-center justify-between rounded-xl border border-warm-200 px-4 py-3">
              <div>
                <p className="font-medium text-ink-900">
                  {(t("businessPage.guestDelivery.contactless") as string) || "Contactless delivery"}
                </p>
                <p className="text-xs text-ink-500">
                  {(t("businessPage.guestDelivery.contactlessHelp") as string) || "Driver can leave the order without a handoff."}
                </p>
              </div>
              <Switch
                isSelected={contactlessDelivery}
                onValueChange={setContactlessDelivery}
                aria-label={(t("businessPage.guestDelivery.contactless") as string) || "Contactless delivery"}
              />
            </div>
            <div className="flex items-center justify-between rounded-xl border border-warm-200 px-4 py-3">
              <div>
                <p className="font-medium text-ink-900">
                  {(t("businessPage.guestDelivery.leaveAtDoor") as string) || "Leave at door"}
                </p>
                <p className="text-xs text-ink-500">
                  {(t("businessPage.guestDelivery.leaveAtDoorHelp") as string) || "Useful for apartments or office drop-off instructions."}
                </p>
              </div>
              <Switch
                isSelected={leaveAtDoor}
                onValueChange={setLeaveAtDoor}
                aria-label={(t("businessPage.guestDelivery.leaveAtDoor") as string) || "Leave at door"}
              />
            </div>
          </div>

          {formError ? (
            <Card
              className="border border-danger-200 bg-danger-50"
              role="alert"
              aria-live="assertive"
              data-testid="delivery-form-error"
            >
              <CardBody className="flex flex-row items-center gap-3 text-danger-700">
                <AlertCircle className="w-5 h-5" />
                <p className="text-sm">{formError}</p>
              </CardBody>
            </Card>
          ) : null}

          {quote ? (
            <>
              <Divider />
              <Card
                className={`border ${canContinue ? "border-success-200 bg-success-50" : "border-warning-200 bg-warning-50"}`}
              >
                <CardBody className="space-y-4">
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-3">
                      {canContinue ? (
                        <CheckCircle className="w-5 h-5 text-success-600" />
                      ) : (
                        <AlertCircle className="w-5 h-5 text-warning-600" />
                      )}
                      <div>
                        {/* Localized headline from reason_code — never the raw
                            English backend message. (DLV-GUEST-2) */}
                        <p className="font-semibold text-ink-900">
                          {localizeQuoteReason(quote.reason_code, t)}
                        </p>
                      </div>
                    </div>
                    <Chip color={canContinue ? "success" : "warning"} variant="flat">
                      {canContinue
                        ? ((t("businessPage.guestDelivery.readyForMenu") as string) || "Ready for menu")
                        : ((t("businessPage.guestDelivery.needsAttention") as string) || "Needs attention")}
                    </Chip>
                  </div>

                  {/* Fee / minimum / ETA are only meaningful for a quotable
                      result (quote_available, below_minimum). For
                      zone_unavailable etc. the backend zeroes them, so hide the
                      cards rather than show a contradictory "AED 0.00 / 0 min"
                      next to the out-of-area warning. (L-2) */}
                  {canContinue && (
                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
                    <div className="rounded-xl bg-white/80 border border-white px-4 py-3">
                      <div className="flex items-center gap-2 text-ink-500 mb-1">
                        <Receipt className="w-4 h-4" />
                        {(t("businessPage.guestDelivery.deliveryFee") as string) || "Delivery fee"}
                      </div>
                      <p className="font-semibold text-ink-900">{fmtCurrency(quote.delivery_fee, currency)}</p>
                      {quote.free_delivery_minimum > 0 && (
                        <p className="text-xs text-ink-500">
                          {(t("businessPage.deliveryQuote.freeAbove", {
                            amount: fmtCurrency(quote.free_delivery_minimum, currency),
                          }) as string) || `Free above ${fmtCurrency(quote.free_delivery_minimum, currency)}`}
                        </p>
                      )}
                    </div>
                    <div className="rounded-xl bg-white/80 border border-white px-4 py-3">
                      <div className="flex items-center gap-2 text-ink-500 mb-1">
                        <Truck className="w-4 h-4" />
                        {(t("businessPage.guestDelivery.minimumToContinue") as string) || "Minimum to continue"}
                      </div>
                      <p className="font-semibold text-ink-900">{fmtCurrency(quote.minimum_order_amount, currency)}</p>
                      <p className="text-xs text-ink-500">
                        {minimumDelta > 0
                          ? ((t("businessPage.deliveryQuote.addMoreForMinimum", {
                              amount: fmtCurrency(minimumDelta, currency),
                            }) as string) ||
                              `Add ${fmtCurrency(minimumDelta, currency)} more to reach the delivery minimum.`)
                          : ((t("businessPage.deliveryQuote.meetsMinimum") as string) ||
                              "Your quote already meets the delivery minimum.")}
                      </p>
                    </div>
                    <div className="rounded-xl bg-white/80 border border-white px-4 py-3">
                      <div className="flex items-center gap-2 text-ink-500 mb-1">
                        <Clock className="w-4 h-4" />
                        {(t("businessPage.guestDelivery.estimatedTime") as string) || "Estimated time"}
                      </div>
                      <p className="font-semibold text-ink-900">
                        {(t("businessPage.guestDelivery.minutesValue", {
                          minutes: quote.estimated_total_minutes,
                        }) as string) || `${quote.estimated_total_minutes} min`}
                      </p>
                      <p className="text-xs text-ink-500">
                        {(t("businessPage.guestDelivery.prepPlusTravel", {
                          prep: quote.estimated_prep_time,
                          travel: quote.estimated_delivery_minutes,
                        }) as string) ||
                          `Prep ${quote.estimated_prep_time} min + travel ${quote.estimated_delivery_minutes} min`}
                      </p>
                    </div>
                  </div>
                  )}

                  {quote.zone ? (
                    <div className="rounded-xl border border-white bg-white/80 px-4 py-3 text-sm text-ink-700">
                      <p className="font-medium text-ink-900">
                        {((t("businessPage.guestDelivery.matchedZone", { name: quote.zone.name }) as string) || `Matched delivery zone: ${quote.zone.name}`)}
                      </p>
                      {quote.zone.description ? <p className="mt-1">{quote.zone.description}</p> : null}
                    </div>
                  ) : null}

                  {!canContinue && quote.partner_fallback_available && quote.external_partner_links?.length > 0 ? (
                    <div className="space-y-2">
                      <p className="text-sm font-medium text-ink-900">
                        {(t("businessPage.guestDelivery.fallbackPartners") as string) || "Fallback partners"}
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {quote.external_partner_links
                          .filter((link) => isSafeExternalTrackingUrl(link.url))
                          .map((link) => (
                          <Button
                            key={`${link.name}-${link.url}`}
                            as="a"
                            href={link.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            variant="flat"
                            endContent={<ExternalLink className="w-4 h-4" />}
                          >
                            {link.name}
                          </Button>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </CardBody>
              </Card>
            </>
          ) : null}
          </form>
        </ModalBody>

        <ModalFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-end">
          {quote && !canContinue ? (
            <p
              className="text-xs text-warning-700 sm:mr-auto"
              data-testid="continue-disabled-help"
            >
              {(t("businessPage.guestDelivery.continueDisabledHelp") as string) ||
                "We can't deliver to this address yet, so the menu can't open in delivery mode. Try another address."}
            </p>
          ) : null}
          <Button variant="light" onPress={handleClose}>
            {(t("businessPage.guestDelivery.close") as string) || "Close"}
          </Button>
          <Button color="primary" variant="flat" onPress={handleQuote} isLoading={loading}>
            {(t("businessPage.guestDelivery.checkAddress") as string) || "Check address"}
          </Button>
          <Button
            color="primary"
            onPress={handleContinue}
            isDisabled={!canContinue}
            aria-disabled={!canContinue}
            className={!canContinue ? "opacity-50" : undefined}
          >
            {(t("businessPage.guestDelivery.browseMenu") as string) || "Browse menu with delivery"}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}

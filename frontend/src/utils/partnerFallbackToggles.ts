export const buildDeliveryToggleSettings = <
  TSettings extends Record<string, unknown>,
>(
  currentSettings: TSettings,
  enabled: boolean,
): TSettings & {
  delivery_enabled: boolean;
  third_party_enabled?: boolean;
  in_house_delivery_enabled?: boolean;
} => {
  // Never round-trip payment_mode from the GET DTO — the resolved mode can
  // clobber the operator's stored preference (DEL-OP-2). Match
  // DeliverySettings.buildDeliverySettingsPayload which strips it when the
  // field is not intentionally edited.
  const { payment_mode: _paymentMode, ...rest } = currentSettings as TSettings & {
    payment_mode?: unknown;
  };

  // Disable: defensively clear sub-flags client-side so the public page hides
  // delivery immediately, even before the server's normalization round-trip.
  if (!enabled) {
    return {
      ...(rest as TSettings),
      delivery_enabled: false,
      third_party_enabled: false,
      in_house_delivery_enabled: false,
    };
  }
  // Enable: only flip the master flag. Sub-flags (third_party_enabled,
  // in_house_delivery_enabled) reflect what the merchant has actually
  // configured and should not be forced on by the toggle. Server allows
  // delivery_enabled=true with no fulfillment route as a setup-in-progress
  // state; partners or zones can be added through the editor afterward.
  return {
    ...(rest as TSettings),
    delivery_enabled: true,
  };
};

export interface ReservationTogglePayload {
  enabled: boolean;
}

export const buildReservationTogglePayload = (
  enabled: boolean,
): ReservationTogglePayload => ({
  enabled,
});

/**
 * Closed-hours signal for guest surfaces.
 *
 * Authoritative source is the backend orderability projection
 * (`menu.item_orderability`). When the dining room is closed, BE should emit
 * `business_closed` for guest items (never while open). Treat **any**
 * `business_closed` entry as closed so mixed maps (e.g. inventory honesty
 * states left over from older BE) still show the closed banner.
 * Empty / missing maps are "not closed" so we never flash a false banner
 * before data loads.
 */

type GuestOrderabilityEntry = {
  orderable?: boolean;
  state?: string;
};

export type GuestOrderabilityMap = Record<
  string,
  GuestOrderabilityEntry | undefined
>;

/**
 * Returns true when the orderability map indicates the business is closed
 * for ordering. Any `business_closed` state is enough (BE does not emit that
 * state while open).
 */
export function isBusinessClosedFromOrderability(
  map: GuestOrderabilityMap | null | undefined,
): boolean {
  if (!map) return false;
  const vals = Object.values(map).filter(
    (v): v is GuestOrderabilityEntry => v != null && typeof v === "object",
  );
  if (vals.length === 0) return false;
  return vals.some((v) => v.state === "business_closed");
}

/**
 * Compose kitchen/orders toggles with closed hours.
 * Open bills remain payable even when this returns false.
 */
export function isGuestOrderingEnabled(opts: {
  kitchenEnabled?: boolean | null;
  ordersEnabled?: boolean | null;
  businessClosed?: boolean;
}): boolean {
  return Boolean(
    opts.kitchenEnabled && opts.ordersEnabled && !opts.businessClosed,
  );
}

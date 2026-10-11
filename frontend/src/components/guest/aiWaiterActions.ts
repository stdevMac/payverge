import type { Bundle, MenuCategory, MenuItem } from "@/api/business";
import type { AssistantResponse } from "@/types/assistant";
import { isMenuItemOrderable } from "./menuItemAvailability";
import type {
  AiWaiterCartActionResult,
  AiWaiterCartRequest,
  LegacyAiWaiterCartCall,
} from "./aiWaiterTypes";

const nonOrderableStates = new Set([
  "manual_disabled",
  "inventory_out",
  "business_closed",
  "ordering_disabled",
]);

const flattenMenuItems = (menuData: readonly MenuCategory[]): MenuItem[] =>
  menuData.flatMap((category) => category.items ?? []);

const validQuantity = (quantity: unknown): quantity is number =>
  typeof quantity === "number" &&
  Number.isInteger(quantity) &&
  quantity >= 1 &&
  quantity <= 20;

const containsUnsafeUnicode = (value: string): boolean =>
  /[\p{Cc}\p{Cf}]/u.test(value);

const validStableID = (value: unknown): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value === value.trim() &&
  Array.from(value).length <= 200 &&
  !containsUnsafeUnicode(value);

const validNotes = (notes: unknown): notes is string | undefined =>
  notes === undefined ||
  (typeof notes === "string" &&
    Array.from(notes).length <= 200 &&
    !containsUnsafeUnicode(notes));

const validLiveBundleID = (id: unknown): id is number =>
  typeof id === "number" && Number.isSafeInteger(id) && id > 0;

const validCanonicalPrice = (price: unknown): price is number =>
  typeof price === "number" && Number.isFinite(price) && price >= 0;

const canonicalPresentation = (
  value: unknown,
  maxCodePoints: number,
): string | null => {
  if (typeof value !== "string" || containsUnsafeUnicode(value)) return null;
  const trimmed = value.trim();
  if (!trimmed || Array.from(trimmed).length > maxCodePoints) return null;
  return trimmed;
};

const menuItemIsOrderable = (item: MenuItem): boolean =>
  isMenuItemOrderable(item) &&
  !nonOrderableStates.has(item.orderability_state ?? "available");

const itemRequest = (
  item: MenuItem,
  quantity: number,
  notes?: string,
): AiWaiterCartRequest | null => {
  if (!validStableID(item.id) || !validCanonicalPrice(item.price)) return null;
  const itemName = canonicalPresentation(item.name, 300);
  const currency =
    item.currency === undefined
      ? undefined
      : canonicalPresentation(item.currency, 100);
  if (itemName === null || currency === null) return null;
  return {
    itemType: "menu_item",
    menuItemId: item.id,
    itemName,
    price: item.price,
    ...(currency === undefined ? {} : { currency }),
    quantity,
    ...(notes === undefined ? {} : { notes }),
  };
};

const bundleRequest = (
  bundle: Bundle,
  quantity: number,
  notes?: string,
): AiWaiterCartRequest | null => {
  if (!validLiveBundleID(bundle.id) || !validCanonicalPrice(bundle.price)) {
    return null;
  }
  const itemName = canonicalPresentation(bundle.name, 300);
  const currency =
    bundle.currency === undefined
      ? undefined
      : canonicalPresentation(bundle.currency, 100);
  if (itemName === null || currency === null) return null;
  return {
    itemType: "bundle",
    bundleId: bundle.id,
    itemName,
    price: bundle.price,
    ...(currency === undefined ? {} : { currency }),
    quantity,
    ...(notes === undefined ? {} : { notes }),
  };
};

/**
 * Resolves a server-validated V2 cart action against the live guest catalog.
 * Display names and labels are deliberately ignored as identity inputs.
 */
export function resolveCartAction(
  response: AssistantResponse,
  menuData: readonly MenuCategory[],
  bundles: readonly Bundle[],
): AiWaiterCartActionResult {
  const cartActions = response.actions.filter(
    (action) => action.type === "add_cart_item",
  );
  if (cartActions.length === 0) {
    return { ok: false, error: "cart_action_missing" };
  }
  const actions = cartActions.filter(
    (action) =>
      action.state === "ready" &&
      action.confirmation === "none" &&
      action.disabled_reason === null &&
      action.expires_at === null &&
      action.target.href === "",
  );
  if (actions.length === 0) {
    return { ok: false, error: "cart_action_not_ready" };
  }
  if (actions.length > 1) {
    return { ok: false, error: "cart_action_ambiguous" };
  }

  const action = actions[0];
  const { target } = action;
  if (
    !validStableID(target.id) ||
    !validQuantity(target.quantity) ||
    !validNotes(target.notes)
  ) {
    return { ok: false, error: "invalid_target" };
  }

  if (target.kind === "menu_item") {
    const matches = flattenMenuItems(menuData).filter(
      (item) => item.id === target.id,
    );
    if (matches.length !== 1) {
      return { ok: false, error: "menu_item_not_found" };
    }
    if (!menuItemIsOrderable(matches[0])) {
      return { ok: false, error: "menu_item_unavailable" };
    }
    const request = itemRequest(matches[0], target.quantity, target.notes);
    if (request === null) return { ok: false, error: "invalid_target" };
    return { ok: true, source: "v2_stable_id", request };
  }

  if (target.kind === "bundle") {
    if (!/^[1-9]\d*$/.test(target.id)) {
      return { ok: false, error: "invalid_target" };
    }
    const targetBundleID = Number(target.id);
    if (
      !Number.isSafeInteger(targetBundleID) ||
      String(targetBundleID) !== target.id
    ) {
      return { ok: false, error: "invalid_target" };
    }
    const matches = bundles.filter(
      (bundle) =>
        validLiveBundleID(bundle.id) && target.id === String(bundle.id),
    );
    if (matches.length !== 1) {
      return { ok: false, error: "bundle_not_found" };
    }
    if (!matches[0].is_active) {
      return { ok: false, error: "bundle_unavailable" };
    }
    const request = bundleRequest(matches[0], target.quantity, target.notes);
    if (request === null) return { ok: false, error: "invalid_target" };
    return { ok: true, source: "v2_stable_id", request };
  }

  return { ok: false, error: "invalid_target" };
}

const normalizeLegacyName = (name: string): string => name.trim().toLowerCase();

/**
 * Temporary V1-only adapter. Callers must opt into name identity explicitly;
 * resolveCartAction never invokes this fallback.
 */
export function resolveLegacyCartAction(
  call: LegacyAiWaiterCartCall,
  menuData: readonly MenuCategory[],
  bundles: readonly Bundle[],
): AiWaiterCartActionResult {
  if (typeof call !== "object" || call === null || Array.isArray(call)) {
    return { ok: false, error: "legacy_args_invalid" };
  }
  const raw = call as unknown as Record<string, unknown>;
  if (
    typeof raw.item_name !== "string" ||
    Array.from(raw.item_name).length > 300 ||
    containsUnsafeUnicode(raw.item_name) ||
    (raw.item_type !== undefined && typeof raw.item_type !== "string") ||
    (raw.quantity !== undefined && !validQuantity(raw.quantity)) ||
    !validNotes(raw.notes)
  ) {
    return { ok: false, error: "legacy_args_invalid" };
  }

  const requestedName = normalizeLegacyName(raw.item_name);
  const quantity = raw.quantity === undefined ? 1 : raw.quantity;
  const notes = raw.notes;
  if (!requestedName) return { ok: false, error: "legacy_args_invalid" };

  const hintedType = raw.item_type?.trim().toLowerCase();
  if (
    hintedType !== undefined &&
    hintedType !== "" &&
    hintedType !== "menu_item" &&
    hintedType !== "bundle"
  ) {
    return { ok: false, error: "legacy_item_not_found" };
  }

  const itemMatches =
    hintedType === "bundle"
      ? []
      : flattenMenuItems(menuData).filter((item) => {
          const name = canonicalPresentation(item.name, 300);
          return name !== null && normalizeLegacyName(name) === requestedName;
        });
  const bundleMatches =
    hintedType === "menu_item"
      ? []
      : bundles.filter((bundle) => {
          const name = canonicalPresentation(bundle.name, 300);
          return name !== null && normalizeLegacyName(name) === requestedName;
        });

  if (itemMatches.length + bundleMatches.length === 0) {
    return { ok: false, error: "legacy_item_not_found" };
  }
  if (itemMatches.length + bundleMatches.length !== 1) {
    return { ok: false, error: "legacy_item_ambiguous" };
  }

  if (itemMatches.length === 1) {
    if (!menuItemIsOrderable(itemMatches[0])) {
      return { ok: false, error: "menu_item_unavailable" };
    }
    const request = itemRequest(itemMatches[0], quantity, notes);
    if (request === null) return { ok: false, error: "legacy_item_not_found" };
    return { ok: true, source: "v1_name_compatibility", request };
  }

  if (!bundleMatches[0].is_active) {
    return { ok: false, error: "bundle_unavailable" };
  }
  const request = bundleRequest(bundleMatches[0], quantity, notes);
  if (request === null) return { ok: false, error: "legacy_item_not_found" };
  return { ok: true, source: "v1_name_compatibility", request };
}

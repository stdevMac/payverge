/**
 * Offer edit form — Applies-to / Target commits.
 *
 * NextUI Select `onChange` fires `""` when a listbox collapses. Treating
 * that as a real scope change wiped Applies-to and Target (#743).
 */

const OFFER_SCOPES = ["all", "category", "item", "bundle"] as const;

export type OfferScope = (typeof OFFER_SCOPES)[number];

export function isOfferScope(value: string): value is OfferScope {
  return (OFFER_SCOPES as readonly string[]).includes(value);
}

export function applyOfferScopeChange(
  current: { applicable_to: OfferScope; target_id: string },
  nextScope: string,
): { applicable_to: OfferScope; target_id: string } | null {
  if (!isOfferScope(nextScope)) return null;
  if (nextScope === current.applicable_to) {
    return {
      applicable_to: current.applicable_to,
      target_id: current.target_id,
    };
  }
  return { applicable_to: nextScope, target_id: "" };
}

export type OfferTargetDecision =
  | { kind: "commit"; target_id: string }
  | { kind: "clear" }
  | { kind: "ignore" };

/**
 * Autocomplete `onSelectionChange(null)` is either a listbox dismiss or a
 * real clear. Honor a clear only when the operator emptied the field.
 */
export function applyOfferTargetSelection(
  key: unknown,
  typedInput: string,
): OfferTargetDecision {
  if (key != null && String(key) !== "") {
    return { kind: "commit", target_id: String(key) };
  }
  if (typedInput.trim() === "") return { kind: "clear" };
  return { kind: "ignore" };
}

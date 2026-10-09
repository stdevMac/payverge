import type { KitId } from "../artDirection/kits";
import type { FormatDef } from "../formats/formats";
import type { SlotKey } from "../templates/types";
import {
  CHOOSABLE_COMPOSITIONS,
  COMPOSITIONS,
  COMPOSITION_ORDER,
  type CompositionId,
} from "./compositions";

/**
 * Content-derived signals. `play`, `hasPrice` and `hasHandle` are populated by
 * `contentSignalsFrom` but no rule below reads them yet; Wave 2's richer rule
 * set consults them. They are staged, not dead — keep them populated.
 */
export interface ContentSignals {
  play: string;
  dishNameLength: number;
  hasPrice: boolean;
  hasDiscount: boolean;
  hasHandle: boolean;
  hasPhoto: boolean;
}

/**
 * Photo-derived signals. Wave 1 always passes null; Wave 2's photoAnalysis
 * pre-pass fills it and the photo branches below activate.
 */
export interface PhotoSignals {
  negativeSpace: "top" | "bottom" | "left" | "right" | "center" | "none";
  busy: boolean;
}

/**
 * Dish-name length, in characters, at or above which the chooser prefers
 * `splitPanel` over `photoBottomStack`.
 *
 * 28 is hand-tuned and has no recorded derivation — neither the Wave 1 plan
 * nor the commit that introduced it explains the number, and it cannot be
 * recovered from the band geometry: the solver measures text through an
 * injected canvas `measure` callback, so no static glyph-width model exists
 * anywhere in this module tree to convert a band's `sizePct` into a character
 * budget. It is pinned on both sides by chooser.test.ts (28 must split, 27
 * must not), so a change fails a test rather than drifting silently. Retune it
 * against the `dishName` bands of `splitPanel` and `photoBottomStack` in
 * compositions.ts (sizePct, maxLines) and a look at rendered output — never in
 * isolation.
 */
const LONG_DISH_NAME_CHARS = 28;

/**
 * The composition chosen when the post has no photo. This is a designed pick,
 * not a filter: `nextComposition` derives its photo-free candidates from
 * `CompositionDef.requiresPhoto` so a new photo-free family joins the rotation
 * automatically, but adding one must NOT change what gets chosen here. The
 * poster is the intended no-photo look, not merely an eligible one.
 */
const NO_PHOTO_COMPOSITION: CompositionId = "posterStack";

/**
 * Where a pick lands when the chosen family refuses this canvas.
 *
 * Unreachable today — every CHOOSABLE_COMPOSITIONS entry accepts every registry
 * format, and only the legacy families refuse anything. It exists so that a
 * future family declaring `supportsFormat: (f) => f.medium === "screen"` degrades
 * to a designed layout rather than to `COMPOSITIONS[undefined]` and a blank post.
 */
const FALLBACK_COMPOSITION: CompositionId = "photoBottomStack";

/**
 * Narrow a value of unknown provenance — a `creative_snapshot.composition` off
 * the wire — to a `CompositionId` that is guaranteed to have a definition in
 * `COMPOSITIONS`. Exact match only: no trimming, no case folding.
 *
 * The backend deliberately does not whitelist this field (the id set grows
 * every wave, and pinning it there would reject posts the moment the frontend
 * ships a new family), so this guard is the only thing standing between a
 * stored id from a newer build and an undefined registry lookup. Use it
 * instead of `as CompositionId`, and fall back to `chooseComposition` when it
 * says no — a cast renders a blank post, a fall back renders a designed one.
 *
 * It lives here rather than beside `COMPOSITION_ORDER` to keep every path that
 * resolves an id — validate, choose, rotate — in one module.
 */
export function isCompositionId(value: unknown): value is CompositionId {
  return (
    typeof value === "string" &&
    (COMPOSITION_ORDER as readonly string[]).includes(value)
  );
}

/**
 * Narrow a wire value to a composition the kit path may hold.
 *
 * `isCompositionId` accepts the full registry — including the three `legacy*`
 * families that exist only to reproduce pre-rewrite templates without a kit.
 * The composer always carries a kit (stored or derived), so a stored
 * `legacyEditorial` beside `kit: "ticket"` would re-seed the toxic pairing
 * that paints a full-bleed texture over a layout declaring `motifs: []`.
 *
 * Use this on every read-back that lands in `PostCreative` / kit-path state.
 * Fall back to `chooseComposition` when it says no — same as `isCompositionId`
 * for unknown ids, but also for every legacy family.
 *
 * Render-time still has its own refuse in `resolveArtDirection`; this closes
 * the storage-side hole so the working creative never holds the pairing.
 */
export function isChoosableCompositionId(
  value: unknown,
): value is CompositionId {
  return (
    typeof value === "string" &&
    (CHOOSABLE_COMPOSITIONS as readonly string[]).includes(value)
  );
}

/** Build content signals from the resolved slot values. */
export function contentSignalsFrom(input: {
  play: string;
  slots: Partial<Record<SlotKey, string>>;
  hasPhoto: boolean;
  hasDiscount: boolean;
}): ContentSignals {
  const slotText = (key: SlotKey) => (input.slots[key] ?? "").trim();
  return {
    play: input.play,
    dishNameLength: slotText("dishName").length,
    hasPrice: slotText("price").length > 0,
    hasDiscount: input.hasDiscount,
    hasHandle: slotText("handle").length > 0,
    hasPhoto: input.hasPhoto,
  };
}

/**
 * Pick a composition from the content. Rules are ordered by strength: the
 * absence of a photo overrides everything, then a discount, then photo
 * geometry, then text length. Source order is priority order — first match
 * wins. Pure and deterministic — the same inputs always produce the same
 * composition, which is what makes Reshuffle an explicit rotation rather than
 * a random reroll.
 */
export function chooseComposition(
  content: ContentSignals,
  photo: PhotoSignals | null,
  _kit: KitId,
  format: FormatDef,
): CompositionId {
  const pick = (id: CompositionId): CompositionId =>
    COMPOSITIONS[id].supportsFormat(format) ? id : FALLBACK_COMPOSITION;
  if (!content.hasPhoto) return pick(NO_PHOTO_COMPOSITION);
  if (content.hasDiscount) return pick("badgeHero");
  if (photo?.busy) return pick("cornerCard");
  if (photo?.negativeSpace === "top") return pick("photoTopStack");
  if (photo?.negativeSpace === "bottom") return pick("photoBottomStack"); // Not redundant with the default below: it beats the long-name rule.
  if (content.dishNameLength >= LONG_DISH_NAME_CHARS) return pick("splitPanel");
  return pick("photoBottomStack");
}

/**
 * The candidates that can actually render this content, in rotation order.
 *
 * Exported because Reshuffle is a no-op whenever fewer than two survive (see
 * `nextComposition`), so the editor has to disable the control rather than
 * offer a button that does nothing — and it can only decide that honestly by
 * asking the same filter the rotation uses, not a second copy of the condition
 * that can drift away from this one.
 */
export function usableCompositions(
  content: ContentSignals,
  candidates: readonly CompositionId[],
  format: FormatDef,
): CompositionId[] {
  return candidates.filter(
    (id) =>
      COMPOSITIONS[id].supportsFormat(format) &&
      (content.hasPhoto || !COMPOSITIONS[id].requiresPhoto),
  );
}

/**
 * The next composition in a stable rotation, for the editor's Reshuffle
 * control. With no photo, only candidates whose definition says they do not
 * require one stay in the rotation — `CompositionDef.requiresPhoto` is the
 * source of truth, so a photo-free family added in a later wave joins the
 * rotation automatically. A `current` that is not among the usable candidates
 * (an unknown id, or one the photo filter just dropped) restarts the rotation
 * at the first usable candidate. If nothing is usable, `current` is returned
 * unchanged.
 *
 * Today the no-photo rotation is a fixed point: `posterStack` is the only
 * CHOOSABLE_COMPOSITIONS entry with `requiresPhoto: false` (the other
 * photo-free definitions are the legacy families, which the chooser never
 * offers), so `usable.length === 1` and this returns `current` unchanged.
 * That is the correct answer for a one-element rotation, so callers must
 * disable Reshuffle when fewer than two candidates are usable rather than
 * expect a different composition back.
 */
export function nextComposition(
  current: CompositionId,
  content: ContentSignals,
  candidates: readonly CompositionId[],
  format: FormatDef,
): CompositionId {
  const usable = usableCompositions(content, candidates, format);
  if (!usable.length) return current;
  const index = usable.indexOf(current);
  return usable[(index + 1) % usable.length];
}

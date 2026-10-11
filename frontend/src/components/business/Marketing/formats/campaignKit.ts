import { COMPOSITIONS, type CompositionId } from "../composition/compositions";
import { isCompositionId } from "../composition/chooser";
import { buildPostFilename } from "../postContent";
import type { RenderPostInput } from "../templates/renderPost";
import {
  DEFAULT_FORMAT_ID,
  FORMATS,
  FORMAT_ORDER,
  isFormatId,
  type FormatDef,
  type FormatId,
} from "./formats";

/**
 * How one approved concept becomes a coordinated set.
 *
 * Pure and deterministic, and that is a contract rather than a nicety. The
 * `creative_snapshot` stores the CONCEPT — a hero `aspect` plus a `kit_formats`
 * list — and every format is recomputed from those two values whenever the
 * Library reuses the row. If this module were not a pure function of its inputs,
 * reuse would have to persist one row per format, and a six-format campaign
 * would be six Library entries the operator has to recognise as one idea.
 *
 * The word "kit" is overloaded in this tree and the two meanings must not be
 * confused. A `CreativeKit` (artDirection/kits.ts) is an ART DIRECTION — faces,
 * grade, motifs, scrim. A CAMPAIGN kit, here, is a SET OF FORMATS sharing one of
 * those art directions. The i18n namespaces follow the same split: `kits.*` for
 * art direction, `campaignKit.*` for this.
 */

/** The set an operator gets when they have not chosen one. The whole registry. */
export const DEFAULT_KIT_FORMATS: readonly FormatId[] = FORMAT_ORDER;

/**
 * The formats a concept renders to, hero first.
 *
 * Hero-first is not cosmetic: it is the order the archive is written in and the
 * order the preview grid shows, and the hero is the one the operator approved on
 * screen. A stored list that puts the hero elsewhere — or omits it, which a
 * hand-edited row can — is reordered rather than rejected.
 *
 * Unknown ids are DROPPED rather than rejected, because a backend newer than
 * this build can legitimately return a format this build has no definition for,
 * and refusing the whole campaign over one unrenderable member would break reuse
 * of a row this build can mostly honour.
 */
export function resolveKitFormats(
  hero: string,
  stored: readonly string[] | undefined,
): FormatId[] {
  const heroId: FormatId = isFormatId(hero) ? hero : DEFAULT_FORMAT_ID;
  const source = stored?.length ? stored : DEFAULT_KIT_FORMATS;
  const out: FormatId[] = [heroId];
  const seen = new Set<FormatId>([heroId]);
  source.forEach((entry) => {
    if (!isFormatId(entry) || seen.has(entry)) return;
    seen.add(entry);
    out.push(entry);
  });
  return out;
}

/**
 * What a stored snapshot means.
 *
 * An ABSENT `kit_formats` is the legacy signal and resolves to the hero alone —
 * one format, the way every pre-Wave-4 post was exported. It deliberately does
 * NOT resolve to the default set: silently turning every stored post into a
 * six-format campaign on reuse would rewrite history the operator never approved.
 */
export function kitFormatsFromSnapshot(snapshot: {
  aspect: string;
  kit_formats?: readonly string[];
}): FormatId[] {
  if (!snapshot.kit_formats || snapshot.kit_formats.length === 0) {
    return [isFormatId(snapshot.aspect) ? snapshot.aspect : DEFAULT_FORMAT_ID];
  }
  return resolveKitFormats(snapshot.aspect, snapshot.kit_formats);
}

export interface KitRenderEntry {
  format: FormatDef;
  /** The concept, with only `aspect` changed. */
  input: RenderPostInput;
  /** Archive member name, including the extension the medium implies. */
  filename: string;
}

/**
 * The concept's composition, if this build knows it and it accepts this format.
 *
 * A legacy composition on a Wave 4 format is the one case this has to catch:
 * `supportsFormat` refuses it, and passing it through anyway would hand
 * `boundsFor` a format with no `legacyAspect` and lay the post out as 4:5 on a
 * canvas that is not one.
 */
function compositionAccepts(
  composition: CompositionId | undefined,
  format: FormatDef,
): boolean {
  if (!composition || !isCompositionId(composition)) return true;
  return COMPOSITIONS[composition].supportsFormat(format);
}

/**
 * One render input per format, varying only the format.
 *
 * Everything else — kit, composition, photo, crop, slots, palette, logo — is the
 * concept, passed through untouched. That is what makes the set coordinated
 * rather than six posts that happen to share a photo.
 */
export function kitRenderPlan(
  base: RenderPostInput,
  formats: readonly FormatId[],
  targetName?: string,
): KitRenderEntry[] {
  return formats
    .filter((id) => compositionAccepts(base.composition, FORMATS[id]))
    .map((id) => {
      const format = FORMATS[id];
      return {
        format,
        input: { ...base, aspect: id },
        filename: buildPostFilename(
          targetName ?? base.slots.dishName,
          id,
          format.medium === "print" ? "html" : "png",
        ),
      };
    });
}

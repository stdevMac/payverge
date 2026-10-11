import type {
  CompositionStrategyId,
  FeaturedImageSelection,
  ImageRole,
  MenuArtDirection,
  MenuPrintDiagnostic,
  PlannedMenuBlock,
  PlannedRect,
  PrintMenuItem,
  PrintMenuSection,
  RegisteredMenuDesignFamily,
} from "../types";
import { PRINT_PRICE_TREATMENTS } from "../types";
import { PT_TO_MM, estimateTextBox } from "./metrics";

export interface SectionPlanInput {
  contentRect: PlannedRect;
  section: PrintMenuSection;
  direction: MenuArtDirection;
  family: RegisteredMenuDesignFamily;
  includeHeading: boolean;
}

export interface SemanticMarker {
  mark: string;
  label: string;
  source: "dietary" | "allergen";
}

export interface SectionPlan {
  blocks: PlannedMenuBlock[];
  diagnostics: MenuPrintDiagnostic[];
  markersByItemId: Record<string, SemanticMarker[]>;
  consumedHeightMm: number;
}

export const SEMANTIC_MARKERS = Object.freeze({
  vegetarian: "V",
  vegan: "VG",
  "gluten-free": "GF",
  spicy: "S",
} as const);

export function validateImageSelectionRoles(
  family: RegisteredMenuDesignFamily,
  selections: readonly FeaturedImageSelection[],
): void {
  for (const selection of selections) {
    if (!family.photography.roles.includes(selection.role)) {
      throw new Error(
        `Menu family ${family.id} does not support image role ${selection.role}`,
      );
    }
  }
}

const roundMm = (value: number): number => Number(value.toFixed(2));

function allergenMark(label: string): string {
  return label
    .trim()
    .split(/[\s-]+/)
    .map((part) => part.match(/[\p{L}\p{N}]/u)?.[0]?.toUpperCase() ?? "")
    .filter(Boolean)
    .join("");
}

export function normalizeSemanticMarkers(
  item: PrintMenuItem,
): SemanticMarker[] {
  const markers: SemanticMarker[] = [];
  const seen = new Set<string>();
  for (const rawLabel of item.dietaryTags) {
    const label = rawLabel.trim();
    const mark =
      SEMANTIC_MARKERS[label.toLowerCase() as keyof typeof SEMANTIC_MARKERS];
    if (mark && !seen.has(mark)) {
      seen.add(mark);
      markers.push({ mark, label, source: "dietary" });
    }
  }
  for (const rawLabel of item.allergens) {
    const label = rawLabel.trim();
    const mark = allergenMark(label);
    if (mark && !seen.has(mark)) {
      seen.add(mark);
      markers.push({ mark, label, source: "allergen" });
    }
  }
  return markers;
}

interface CompositionContext extends SectionPlanInput {
  treatment: MenuArtDirection["treatment"];
  selectedImage?: FeaturedImageSelection;
}

interface CompositionResult {
  blocks: PlannedMenuBlock[];
  diagnostics?: MenuPrintDiagnostic[];
}

function makeRect(
  bounds: PlannedRect,
  xMm: number,
  yMm: number,
  widthMm: number,
  heightMm: number,
): PlannedRect {
  const x = Math.min(Math.max(xMm, bounds.xMm), bounds.xMm + bounds.widthMm);
  const y = Math.min(Math.max(yMm, bounds.yMm), bounds.yMm + bounds.heightMm);
  return {
    xMm: roundMm(x),
    yMm: roundMm(y),
    widthMm: roundMm(
      Math.max(0, Math.min(widthMm, bounds.xMm + bounds.widthMm - x)),
    ),
    heightMm: roundMm(
      Math.max(0, Math.min(heightMm, bounds.yMm + bounds.heightMm - y)),
    ),
  };
}

function headingBlock(
  context: CompositionContext,
  rect: PlannedRect,
): PlannedMenuBlock[] {
  return context.includeHeading
    ? [
        {
          id: `section:${context.section.id}:heading`,
          kind: "category-heading",
          sectionId: context.section.id,
          rect,
        },
      ]
    : [];
}

function imageBlock(
  context: CompositionContext,
  rect: PlannedRect,
): PlannedMenuBlock[] {
  return context.selectedImage
    ? [
        {
          id: `section:${context.section.id}:image:${context.selectedImage.itemId}`,
          kind: "image",
          sectionId: context.section.id,
          itemIds: [context.selectedImage.itemId],
          image: { ...context.selectedImage },
          rect,
        },
      ]
    : [];
}

function itemBlock(
  sectionId: string,
  item: PrintMenuItem,
  rect: PlannedRect,
  feature = false,
): PlannedMenuBlock {
  return {
    id: `section:${sectionId}:item:${item.id}`,
    kind: feature ? "item-feature" : "item-list",
    sectionId,
    itemIds: [item.id],
    rect,
  };
}

export interface PlannedItemMeasureInput {
  item: PrintMenuItem;
  widthMm: number;
  family: RegisteredMenuDesignFamily;
  treatment: MenuArtDirection["treatment"];
}

export interface PlannedItemMeasure {
  heightMm: number;
  descriptionLines: number;
  sourceDescriptionLines: number;
}

export function resolveTextGlyphWidthFactor(
  font: RegisteredMenuDesignFamily["typography"]["body"],
  text: string,
): number {
  if (
    /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u.test(
      text,
    )
  ) {
    return 1;
  }
  if (/[\p{Script=Arabic}\p{Script=Hebrew}]/u.test(text)) return 0.68;
  return font === "Oswald" ? 0.46 : font === "EB Garamond" ? 0.5 : 0.52;
}

export interface PlannedItemWidthInput {
  family: RegisteredMenuDesignFamily;
  contentWidthMm: number;
  treatment: MenuArtDirection["treatment"];
  hasSelectedImage: boolean;
  selectedImageRole?: ImageRole;
  itemIndex: number;
  ornamentIntensity: MenuArtDirection["ornamentIntensity"];
}

export function resolvePlannedItemWidth(input: PlannedItemWidthInput): number {
  const composition = input.family.compositions[0];
  if (composition === "editorial-asymmetric") {
    if (input.hasSelectedImage && input.treatment !== "type-led") {
      if (input.selectedImageRole === "paired") {
        return roundMm(input.contentWidthMm * 0.52);
      }
      if (input.selectedImageRole !== "full-width") {
        return roundMm(input.contentWidthMm * 0.62);
      }
    }
    // Full-measure rows push the price to the far page edge with a chasm of
    // untracked space after the name; a capped editorial column keeps the
    // name-price relationship tight and the right margin reads as intent.
    return roundMm(Math.min(input.contentWidthMm, 132));
  }
  if (composition === "formal-course") {
    return roundMm(
      input.contentWidthMm - (input.ornamentIntensity === "none" ? 0 : 16),
    );
  }
  if (composition === "lively-columns") {
    const availableWidth = Math.max(1, input.contentWidthMm - 5);
    return roundMm(availableWidth * (input.itemIndex % 2 === 0 ? 0.54 : 0.46));
  }
  if (composition === "modular-counter") {
    if (input.itemIndex === 0) return roundMm(input.contentWidthMm);
    return roundMm(
      input.contentWidthMm >= 120
        ? (input.contentWidthMm - 5) / 2
        : input.contentWidthMm,
    );
  }
  if (composition === "ingredient-air") {
    return roundMm(Math.min(115, input.contentWidthMm));
  }
  if (composition === "image-gallery") {
    return roundMm(
      input.hasSelectedImage && input.treatment === "photo-led"
        ? input.contentWidthMm * 0.39
        : input.contentWidthMm,
    );
  }
  return roundMm(input.contentWidthMm);
}

/** Half-size sheets get proportionally reduced air so airy families still fit. */
export function isCompactPaper(
  paperFormat?: MenuArtDirection["paperFormat"],
): boolean {
  return paperFormat === "a5" || paperFormat === "half-letter";
}

export function resolvePlannedItemGap(
  family: RegisteredMenuDesignFamily,
  treatment: MenuArtDirection["treatment"],
  paperFormat?: MenuArtDirection["paperFormat"],
): number {
  const composition = family.compositions[0];
  if (composition === "formal-course") return 8;
  if (composition === "cinematic-sections") return 5;
  if (composition === "modular-counter") return 3;
  if (composition === "bold-scan") return treatment === "compact" ? 1 : 3;
  if (composition === "ingredient-air") {
    if (isCompactPaper(paperFormat)) return treatment === "compact" ? 4 : 7;
    return treatment === "compact" ? 5 : 9;
  }
  if (composition === "image-gallery") return 5;
  return treatment === "compact" ? 2 : 4;
}

/**
 * Rendered `.item-name` font-size per family (renderPlannedHtml family CSS).
 * The estimator must track the same em scale: display-forward families
 * (night-house 1.16em) render taller name rows than the body floor implies
 * and clip inside their fixed-height planner blocks if modeled at 1em.
 */
const ITEM_NAME_EM: Readonly<Record<string, number>> = {
  atelier: 1.12,
  maison: 0.95,
  osteria: 1.08,
  "night-house": 1.16,
  counter: 1,
  street: 0.98,
  field: 0.84,
  gallery: 1.12,
};
const DEFAULT_ITEM_NAME_EM = 1.06;

/**
 * Rendered `.item-price` font-size per family for column mode; the price
 * inherits the item's 1.35 leading, so on short names its line box — not the
 * name's — governs the baseline row height.
 */
const ITEM_PRICE_EM: Readonly<Record<string, number>> = {
  atelier: 0.9,
  street: 1.05,
  field: 0.9,
  gallery: 0.9,
};
const DEFAULT_ITEM_PRICE_EM = 0.94;

/**
 * Fixed per-family chrome around the copy: field pads the whole item
 * 1mm block-start + block-end; night-house underlines the price row with
 * 1.1mm padding-block-end plus the rule itself.
 */
const ITEM_EXTRA_MM: Readonly<Record<string, number>> = {
  field: 2,
  "night-house": 1.3,
};

export function estimatePlannedItemHeight(
  input: PlannedItemMeasureInput,
): PlannedItemMeasure {
  const fontPt = input.family.typography.bodyFloorPt;
  const textWidth = Math.max(1, input.widthMm - 26);
  const namePt =
    fontPt * (ITEM_NAME_EM[input.family.id] ?? DEFAULT_ITEM_NAME_EM);
  const name = estimateTextBox({
    text: input.item.name,
    widthMm: textWidth,
    fontPt: namePt,
    floorPt: namePt,
    lineHeight: 1.18,
    glyphWidthFactor: resolveTextGlyphWidthFactor(
      input.family.typography.body,
      input.item.name,
    ),
  });
  const nameLineMm = name.heightMm / name.lines;
  const priceMode = PRINT_PRICE_TREATMENTS[input.family.id].mode;
  // In column mode the name and price share a baseline flex row, so the row
  // is as tall as the larger of the two line boxes.
  const priceLineMm =
    priceMode === "nested"
      ? 0
      : fontPt *
        (ITEM_PRICE_EM[input.family.id] ?? DEFAULT_ITEM_PRICE_EM) *
        1.35 *
        PT_TO_MM;
  const nameRowMm =
    Math.max(nameLineMm, priceLineMm) + (name.lines - 1) * nameLineMm;
  const description = input.item.description
    ? estimateTextBox({
        text: input.item.description,
        widthMm: textWidth,
        fontPt,
        floorPt: fontPt,
        lineHeight: 1.25,
        glyphWidthFactor: resolveTextGlyphWidthFactor(
          input.family.typography.body,
          input.item.description,
        ),
      })
    : { lines: 0, heightMm: 0 };
  const descriptionLines = description.lines;
  const lineHeightMm =
    description.lines > 0 ? description.heightMm / description.lines : 0;
  const hasMarkerRow = normalizeSemanticMarkers(input.item).length > 0;
  // Nested price treatments render the price as its own 0.9em block under the
  // copy with a 1mm block-start margin; the 1.6mm reserve keeps headroom.
  const nestedPriceMm =
    priceMode === "nested" ? fontPt * 0.9 * 1.35 * PT_TO_MM + 1.6 : 0;
  const verticalReserveMm = hasMarkerRow
    ? 4.5 + (input.treatment === "compact" ? 0 : 1.5)
    : input.treatment === "compact"
      ? 1.8
      : 3;
  return {
    heightMm: roundMm(
      nameRowMm +
        lineHeightMm * descriptionLines +
        nestedPriceMm +
        verticalReserveMm +
        (ITEM_EXTRA_MM[input.family.id] ?? 0),
    ),
    descriptionLines,
    sourceDescriptionLines: description.lines,
  };
}

function stackItems(
  context: CompositionContext,
  startY: number,
  width: number,
  gap: number,
  x = context.contentRect.xMm,
  featureFirst = false,
): PlannedMenuBlock[] {
  let cursorY = startY;
  return context.section.items.map((item, index) => {
    const measure = estimatePlannedItemHeight({
      item,
      widthMm: width,
      family: context.family,
      treatment: context.treatment,
    });
    const block = itemBlock(
      context.section.id,
      item,
      measuredItemRect(
        context.contentRect,
        x,
        cursorY,
        width,
        measure.heightMm,
      ),
      featureFirst && index === 0,
    );
    cursorY += measure.heightMm + gap;
    return block;
  });
}

function measuredItemRect(
  bounds: PlannedRect,
  xMm: number,
  yMm: number,
  widthMm: number,
  heightMm: number,
): PlannedRect {
  const rect = makeRect(bounds, xMm, yMm, widthMm, heightMm);
  if (
    rect.widthMm !== roundMm(widthMm) ||
    rect.heightMm !== roundMm(heightMm)
  ) {
    throw new Error(
      "measured item block exceeds content rect " +
        `(item x:${roundMm(xMm)} y:${roundMm(yMm)} w:${roundMm(widthMm)} h:${roundMm(heightMm)} ` +
        `bounds x:${bounds.xMm} y:${bounds.yMm} w:${bounds.widthMm} h:${bounds.heightMm})`,
    );
  }
  return rect;
}

function selectedImageRect(
  context: CompositionContext,
  startY: number,
): PlannedRect {
  const b = context.contentRect;
  const role = context.selectedImage?.role;
  const photoScale = context.treatment === "photo-led" ? 1.35 : 1;
  if (role === "full-width") {
    // A fixed 36mm band across a full letter measure letterboxes the photo
    // into a 5:1 smear. Proportion the band to the measure (roughly 3:1,
    // wider when photography leads) so the crop still reads as food.
    const bandHeight = Math.min(
      context.treatment === "photo-led" ? 76 : 64,
      Math.max(
        30,
        b.widthMm * (context.treatment === "photo-led" ? 0.4 : 0.34),
      ),
    );
    return makeRect(b, b.xMm, startY, b.widthMm, bandHeight);
  }
  if (role === "half-page")
    return makeRect(b, b.xMm, startY, b.widthMm * 0.5, 48 * photoScale);
  if (role === "category-opener")
    return makeRect(b, b.xMm, startY, b.widthMm * 0.55, 34 * photoScale);
  if (role === "compact-tile")
    return makeRect(b, b.xMm, startY, b.widthMm * 0.32, 24);
  if (role === "paired")
    return makeRect(
      b,
      b.xMm + b.widthMm * 0.52,
      startY,
      b.widthMm * 0.48,
      36 * photoScale,
    );
  return makeRect(
    b,
    b.xMm + b.widthMm * 0.62,
    startY,
    b.widthMm * 0.38,
    48 * photoScale,
  );
}

function editorialAsymmetric(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const headingY = b.yMm + 4;
  const headingHeight = context.includeHeading ? 12 : 0;
  const blocks = headingBlock(
    context,
    makeRect(b, b.xMm, headingY, b.widthMm * 0.34, headingHeight),
  );
  let itemStartY = headingY + headingHeight + 5;
  let itemWidth = Math.min(b.widthMm, 132);
  if (context.selectedImage && context.treatment !== "type-led") {
    const fullWidth = context.selectedImage.role === "full-width";
    const imageRect = selectedImageRect(
      context,
      fullWidth ? itemStartY : b.yMm,
    );
    blocks.push(...imageBlock(context, imageRect));
    if (fullWidth) itemStartY = imageRect.yMm + imageRect.heightMm + 5;
    else {
      itemWidth =
        context.selectedImage.role === "paired"
          ? b.widthMm * 0.52
          : b.widthMm * 0.62;
    }
  }
  blocks.push(
    ...stackItems(
      context,
      itemStartY,
      itemWidth,
      resolvePlannedItemGap(context.family, context.treatment),
    ),
  );
  return { blocks };
}

function formalCourse(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const headingWidth = b.widthMm * 0.6;
  // Measured, not fixed: the double-rule box carries the course description
  // inside it, and a 12mm flat reserve let two-line descriptions collide with
  // the bottom rule. 7mm of chrome covers the rule borders plus padding.
  const headingHeight = context.includeHeading
    ? measuredCategoryHeadingHeight(context, headingWidth, 7, 12)
    : 0;
  const blocks = headingBlock(
    context,
    makeRect(
      b,
      b.xMm + (b.widthMm - headingWidth) / 2,
      b.yMm,
      headingWidth,
      headingHeight,
    ),
  );
  let startY = b.yMm + headingHeight + (context.includeHeading ? 8 : 0);
  if (context.selectedImage && context.treatment !== "type-led") {
    const sized = selectedImageRect(context, startY);
    // Everything on a formal course page hangs from the centre axis; a
    // left-flushed photograph reads as a paste-up error.
    const imageRect = makeRect(
      b,
      b.xMm + (b.widthMm - sized.widthMm) / 2,
      startY,
      sized.widthMm,
      sized.heightMm,
    );
    blocks.push(...imageBlock(context, imageRect));
    startY += imageRect.heightMm + 8;
  }
  blocks.push(
    ...stackItems(
      context,
      startY,
      b.widthMm - (context.direction.ornamentIntensity === "none" ? 0 : 16),
      context.direction.outputFormat === "two-page-spread" &&
        context.treatment === "photo-led"
        ? 10
        : resolvePlannedItemGap(context.family, context.treatment),
      b.xMm + (context.direction.ornamentIntensity === "none" ? 0 : 8),
    ),
  );
  return { blocks };
}

function livelyColumns(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const hasSideImage = Boolean(
    context.selectedImage && context.treatment !== "type-led",
  );
  // The heading box carries the category description too, so its height must
  // be measured — a name-only estimate clipped every description below it.
  const headingWidth = b.widthMm * (hasSideImage ? 0.5 : 0.42);
  const headingHeight = context.includeHeading
    ? measuredCategoryHeadingHeight(context, headingWidth, 4, 8)
    : 0;
  const blocks = headingBlock(
    context,
    makeRect(b, b.xMm, b.yMm, headingWidth, headingHeight),
  );
  let y = b.yMm + headingHeight + 4;
  if (hasSideImage) {
    // The opener photo sits beside the heading instead of banding the full
    // measure, so the intro row composes as heading + image and the columns
    // resume beneath whichever runs deeper — no dead air next to the photo.
    const sized = selectedImageRect(context, b.yMm);
    const imageWidth = Math.min(sized.widthMm, b.widthMm * 0.44);
    const imageRect = makeRect(
      b,
      b.xMm + b.widthMm - imageWidth,
      b.yMm,
      imageWidth,
      sized.heightMm,
    );
    blocks.push(...imageBlock(context, imageRect));
    y = Math.max(y, b.yMm + imageRect.heightMm + 4);
  }
  const gutterMm = 5;
  const availableWidth = b.widthMm - gutterMm;
  const leftWidth = availableWidth * 0.54;
  const rightWidth = availableWidth * 0.46;
  const gap = resolvePlannedItemGap(context.family, context.treatment);
  const columnLefts = [b.xMm, b.xMm + leftWidth + gutterMm];
  const columnWidths = [leftWidth, rightWidth];
  const { items } = context.section;
  // Dishes alternate left/right in menu order, and the columns are deliberately
  // unequal (0.54 / 0.46), so the same description wraps taller on the right.
  const heights = items.map(
    (item, index) =>
      estimatePlannedItemHeight({
        item,
        widthMm: columnWidths[index % 2],
        family: context.family,
        treatment: context.treatment,
      }).heightMm,
  );
  // Letting each column advance by its own content makes the taller one sink
  // further with every entry, until the two stacks end 20mm+ apart and nothing
  // lines up across the gutter. Advancing both columns by the deeper of the
  // pair fixes that, but it can only ever cost depth — so take it when the
  // deeper column already dictated the section height (the usual case, and
  // then the grid is free) and keep the independent flow when the section is
  // tight enough that the difference would push dishes off the sheet.
  const rowLockedHeight = items.reduce(
    (total, _item, index) =>
      index % 2 === 0
        ? total + Math.max(heights[index], heights[index + 1] ?? 0) + gap
        : total,
    0,
  );
  const independentHeight = Math.max(
    heights.reduce(
      (total, height, index) =>
        index % 2 === 0 ? total + height + gap : total,
      0,
    ),
    heights.reduce(
      (total, height, index) =>
        index % 2 === 1 ? total + height + gap : total,
      0,
    ),
  );
  const rowLocked = rowLockedHeight <= independentHeight + 0.01;
  const cursors = [y, y];
  let rowTop = y;
  items.forEach((item, index) => {
    const column = index % 2;
    const top = rowLocked ? rowTop : cursors[column];
    blocks.push(
      itemBlock(
        context.section.id,
        item,
        measuredItemRect(
          b,
          columnLefts[column],
          top,
          columnWidths[column],
          heights[index],
        ),
      ),
    );
    cursors[column] += heights[index] + gap;
    if (column === 1) {
      rowTop += Math.max(heights[index - 1], heights[index]) + gap;
    } else if (index === items.length - 1) {
      rowTop += heights[index] + gap;
    }
  });
  return { blocks };
}

function cinematicSections(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const transitionHeight = context.includeHeading ? 18 : 0;
  const blocks = headingBlock(
    context,
    makeRect(b, b.xMm, b.yMm, b.widthMm, transitionHeight),
  );
  let startY = b.yMm + transitionHeight;
  if (context.selectedImage && context.treatment !== "type-led") {
    const imageRect =
      context.selectedImage.role === "editorial-crop"
        ? makeRect(
            b,
            b.xMm,
            startY,
            Math.min(b.widthMm, 90),
            Math.min(b.widthMm, 90) / 1.5,
          )
        : context.selectedImage.role === "full-width" &&
            context.direction.outputFormat === "drinks-card"
          ? makeRect(b, b.xMm, startY, b.widthMm, 42)
          : selectedImageRect(context, startY);
    blocks.push(...imageBlock(context, imageRect));
    startY += imageRect.heightMm + 4;
  }
  blocks.push(
    ...stackItems(
      context,
      startY,
      b.widthMm,
      resolvePlannedItemGap(context.family, context.treatment),
      b.xMm,
    ),
  );
  return { blocks };
}

function measuredCategoryHeadingHeight(
  context: CompositionContext,
  widthMm: number,
  verticalPaddingMm: number,
  minimumMm: number,
): number {
  const displayPt = 12 * context.family.typography.displayScale;
  const name = estimateTextBox({
    text: context.section.name,
    widthMm: Math.max(1, widthMm - 4),
    fontPt: displayPt,
    floorPt: displayPt,
    lineHeight: 1,
    glyphWidthFactor: resolveTextGlyphWidthFactor(
      context.family.typography.display,
      context.section.name,
    ),
  });
  const description = context.section.description
    ? estimateTextBox({
        text: context.section.description,
        widthMm: Math.max(1, widthMm - 4),
        fontPt: context.family.typography.bodyFloorPt * 0.88,
        lineHeight: 1.25,
        glyphWidthFactor: resolveTextGlyphWidthFactor(
          context.family.typography.body,
          context.section.description,
        ),
      }).heightMm + 1.5
    : 0;
  return Math.max(
    minimumMm,
    roundMm(name.heightMm + description + verticalPaddingMm),
  );
}

function modularCounter(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const headingWidth = Math.min(b.widthMm, 58);
  const headingHeight = context.includeHeading
    ? measuredCategoryHeadingHeight(context, headingWidth, 4, 10)
    : 0;
  const blocks = headingBlock(
    context,
    makeRect(b, b.xMm, b.yMm, headingWidth, headingHeight),
  );
  let startY = b.yMm + headingHeight + 3;
  if (context.selectedImage && context.treatment !== "type-led") {
    if (context.selectedImage.role === "full-width") {
      const imageRect = selectedImageRect(context, startY);
      blocks.push(...imageBlock(context, imageRect));
      startY += imageRect.heightMm + 4;
    } else {
      // Modular surfaces read best when the photo sits as a tile beside the
      // heading instead of a band that pushes every item down the page.
      const sized = selectedImageRect(context, b.yMm);
      const imageRect = makeRect(
        b,
        b.xMm + b.widthMm - sized.widthMm,
        b.yMm,
        sized.widthMm,
        sized.heightMm,
      );
      blocks.push(...imageBlock(context, imageRect));
      startY = Math.max(startY, b.yMm + imageRect.heightMm + 4);
    }
  }
  const gap = resolvePlannedItemGap(context.family, context.treatment);
  const [dominant, ...rest] = context.section.items;
  let cursorY = startY;
  if (dominant) {
    const measure = estimatePlannedItemHeight({
      item: dominant,
      widthMm: b.widthMm,
      family: context.family,
      treatment: context.treatment,
    });
    const tileHeightMm = measure.heightMm + 7;
    blocks.push(
      itemBlock(
        context.section.id,
        dominant,
        measuredItemRect(b, b.xMm, cursorY, b.widthMm, tileHeightMm),
        true,
      ),
    );
    cursorY += tileHeightMm + gap;
  }
  const twoColumns = rest.length > 1 && b.widthMm >= 120;
  const gutterMm = 5;
  const columnWidth = twoColumns ? (b.widthMm - gutterMm) / 2 : b.widthMm;
  const columnX = [b.xMm, b.xMm + columnWidth + gutterMm];
  const columnY = [cursorY, cursorY];
  rest.forEach((item, index) => {
    const column = twoColumns ? index % 2 : 0;
    const measure = estimatePlannedItemHeight({
      item,
      widthMm: columnWidth,
      family: context.family,
      treatment: context.treatment,
    });
    const tileHeightMm = measure.heightMm + 3;
    blocks.push(
      itemBlock(
        context.section.id,
        item,
        measuredItemRect(
          b,
          columnX[column],
          columnY[column],
          columnWidth,
          tileHeightMm,
        ),
      ),
    );
    columnY[column] += tileHeightMm + gap;
  });
  return { blocks };
}

function boldScan(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  // The scan bar is a filled band with the description inside it; a flat 8mm
  // reserve clipped any description. 4.5mm of chrome mirrors the rendered
  // 1.4mm block padding plus the name-to-description margin.
  const headingHeight = context.includeHeading
    ? measuredCategoryHeadingHeight(context, b.widthMm, 4.5, 8)
    : 0;
  const blocks = headingBlock(
    context,
    makeRect(b, b.xMm, b.yMm, b.widthMm, headingHeight),
  );
  let startY = b.yMm + headingHeight + 2;
  if (context.selectedImage && context.treatment !== "type-led") {
    const imageRect = selectedImageRect(context, startY);
    blocks.push(...imageBlock(context, imageRect));
    startY += imageRect.heightMm + 3;
  }
  const gap = resolvePlannedItemGap(context.family, context.treatment);
  if (
    context.treatment === "compact" &&
    b.widthMm >= 100 &&
    context.section.items.length > 3
  ) {
    // Dense scan menus column up on wide surfaces so the page reads as a
    // board, not a single towering list.
    const gutterMm = 6;
    const columnWidth = (b.widthMm - gutterMm) / 2;
    const columnX = [b.xMm, b.xMm + columnWidth + gutterMm];
    const columnY = [startY, startY];
    context.section.items.forEach((item, index) => {
      const column = index % 2;
      const measure = estimatePlannedItemHeight({
        item,
        widthMm: columnWidth,
        family: context.family,
        treatment: context.treatment,
      });
      blocks.push(
        itemBlock(
          context.section.id,
          item,
          measuredItemRect(
            b,
            columnX[column],
            columnY[column],
            columnWidth,
            measure.heightMm,
          ),
        ),
      );
      columnY[column] += measure.heightMm + gap;
    });
    return { blocks };
  }
  const items = stackItems(context, startY, b.widthMm, gap);
  blocks.push(...items);
  return { blocks };
}

function ingredientAir(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const compactPaper = isCompactPaper(context.direction.paperFormat);
  // Field's headings are short uppercase phrases set with wide tracking, and
  // its category descriptions ride in the same box. A 42mm measure broke both
  // across three or four ragged lines; 78mm keeps a two-to-four word heading on
  // one line and lets the description wrap at a comfortable reading measure.
  const headingWidth = Math.min(78, b.widthMm);
  const headingHeight = context.includeHeading
    ? measuredCategoryHeadingHeight(context, headingWidth, 0, 10)
    : 0;
  const blocks = headingBlock(
    context,
    makeRect(b, b.xMm, b.yMm, headingWidth, headingHeight),
  );
  let startY =
    b.yMm +
    headingHeight +
    (context.includeHeading ? (compactPaper ? 9 : 12) : 0);
  if (context.selectedImage && context.treatment !== "type-led") {
    const imageRect = selectedImageRect(context, startY);
    blocks.push(...imageBlock(context, imageRect));
    startY += imageRect.heightMm + 8;
  }
  blocks.push(
    ...stackItems(
      context,
      startY,
      Math.min(115, b.widthMm),
      resolvePlannedItemGap(
        context.family,
        context.treatment,
        context.direction.paperFormat,
      ),
    ),
  );
  return { blocks };
}

function imageGallery(context: CompositionContext): CompositionResult {
  const b = context.contentRect;
  const blocks: PlannedMenuBlock[] = [];
  if (context.selectedImage && context.treatment === "photo-led") {
    const imageWidth = b.widthMm * 0.56;
    // Width first, anchored to the right edge: rounding 0.61x and 0.39x
    // independently can overshoot the rect by a hundredth of a millimetre,
    // which measuredItemRect treats as overflow. The gutter absorbs the
    // rounding instead, and the width matches resolvePlannedItemWidth's
    // estimate exactly.
    const copyWidth = roundMm(b.widthMm * 0.39);
    const copyX = roundMm(b.xMm + b.widthMm - copyWidth);
    const headingHeight = context.includeHeading ? 12 : 0;
    blocks.push(
      ...imageBlock(context, makeRect(b, b.xMm, b.yMm, imageWidth, b.heightMm)),
      ...headingBlock(
        context,
        makeRect(b, copyX, b.yMm, copyWidth, headingHeight),
      ),
      ...stackItems(
        context,
        b.yMm + headingHeight + 6,
        copyWidth,
        resolvePlannedItemGap(context.family, context.treatment),
        copyX,
      ),
    );
    return { blocks };
  }
  let startY = b.yMm;
  if (context.selectedImage && context.treatment !== "type-led") {
    // Proportion the band to the measure instead of to whatever depth the
    // planner handed this section, so the crop stays photographic and the
    // section only consumes the height it actually composes.
    const imageHeight = Math.min(64, Math.max(30, b.widthMm * 0.34));
    blocks.push(
      ...imageBlock(context, makeRect(b, b.xMm, b.yMm, b.widthMm, imageHeight)),
    );
    startY += imageHeight + 8;
  }
  const headingHeight = context.includeHeading ? 12 : 0;
  blocks.push(
    ...headingBlock(
      context,
      makeRect(b, b.xMm, startY, b.widthMm, headingHeight),
    ),
  );
  blocks.push(
    ...stackItems(
      context,
      startY + headingHeight + 6,
      b.widthMm,
      resolvePlannedItemGap(context.family, context.treatment),
      b.xMm,
    ),
  );
  return { blocks };
}

const COMPOSITIONS: Record<
  CompositionStrategyId,
  (context: CompositionContext) => CompositionResult
> = {
  "editorial-asymmetric": editorialAsymmetric,
  "formal-course": formalCourse,
  "lively-columns": livelyColumns,
  "cinematic-sections": cinematicSections,
  "modular-counter": modularCounter,
  "bold-scan": boldScan,
  "ingredient-air": ingredientAir,
  "image-gallery": imageGallery,
};

function planSection(
  input: SectionPlanInput,
  treatment: MenuArtDirection["treatment"],
): SectionPlan {
  validateImageSelectionRoles(input.family, input.direction.imageSelections);
  const itemIds = new Set(input.section.items.map(({ id }) => id));
  const selectedImage = input.direction.imageSelections.find(({ itemId }) =>
    itemIds.has(itemId),
  );
  const composition = input.family.compositions[0];
  const result = COMPOSITIONS[composition]({
    ...input,
    treatment,
    selectedImage: selectedImage ? { ...selectedImage } : undefined,
  });
  const bottom = result.blocks.reduce(
    (maximum, block) => Math.max(maximum, block.rect.yMm + block.rect.heightMm),
    input.contentRect.yMm,
  );
  return {
    blocks: result.blocks,
    diagnostics: result.diagnostics ?? [],
    markersByItemId: Object.fromEntries(
      input.section.items.map((item) => [
        item.id,
        normalizeSemanticMarkers(item),
      ]),
    ),
    consumedHeightMm: roundMm(bottom - input.contentRect.yMm),
  };
}

export const planTypeLedSection = (input: SectionPlanInput): SectionPlan =>
  planSection(input, "type-led");

export const planBalancedSection = (input: SectionPlanInput): SectionPlan =>
  planSection(input, "balanced");

export const planPhotoLedSection = (input: SectionPlanInput): SectionPlan =>
  planSection(input, "photo-led");

export const planCompactSection = (input: SectionPlanInput): SectionPlan =>
  planSection(input, "compact");

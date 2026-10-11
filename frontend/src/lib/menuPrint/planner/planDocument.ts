import { effectivePrintDpi, type MenuPrintAudit } from "../audit";
import { getMenuDesignFamily } from "../families";
import { resolvePrintGeometry } from "../geometry";
import type {
  DiagnosticSeverity,
  MenuArtDirection,
  MenuPrintDiagnostic,
  PlannedMenuBlock,
  PlannedMenuDocument,
  PlannedMenuPage,
  PlannedRect,
  PrintMenuItem,
  PrintMenuModel,
  PrintMenuSection,
  RegisteredMenuDesignFamily,
} from "../types";
import { PRINT_CONTACT_PLACEMENTS, PRINT_TYPOGRAPHY_BY_FAMILY } from "../types";
import {
  estimatePlannedItemHeight,
  isCompactPaper,
  planBalancedSection,
  planCompactSection,
  planPhotoLedSection,
  planTypeLedSection,
  resolvePlannedItemGap,
  resolvePlannedItemWidth,
  validateImageSelectionRoles,
  resolveTextGlyphWidthFactor,
  type SectionPlan,
  type SectionPlanInput,
} from "./strategies";
import { estimateTextBox } from "./metrics";

export interface PlanMenuDocumentInput {
  model: PrintMenuModel;
  audit: MenuPrintAudit;
  direction: MenuArtDirection;
}

const FIXED_SURFACES = new Set([
  "single-sheet",
  "folded-booklet",
  "takeaway-trifold",
  "drinks-card",
  "counter-menu",
]);

const roundMm = (value: number): number => Number(value.toFixed(2));

function validateDirection(
  direction: MenuArtDirection,
): RegisteredMenuDesignFamily {
  const family = getMenuDesignFamily(direction.familyId);
  if (!family.supportedTreatments.includes(direction.treatment)) {
    throw new Error(
      `Menu family ${family.id} does not support treatment ${direction.treatment}`,
    );
  }
  if (!family.supportedFormats.includes(direction.outputFormat)) {
    throw new Error(
      `Menu family ${family.id} does not support format ${direction.outputFormat}`,
    );
  }
  if (
    !PRINT_TYPOGRAPHY_BY_FAMILY[family.id].includes(
      direction.typographyPersonality,
    )
  ) {
    throw new Error(
      `Menu family ${family.id} does not support typography ${direction.typographyPersonality}`,
    );
  }
  if (!PRINT_CONTACT_PLACEMENTS.includes(direction.contactPlacement)) {
    throw new Error(`Invalid contact placement ${direction.contactPlacement}`);
  }
  return family;
}

function validateModelAndSelections(
  model: PrintMenuModel,
  direction: MenuArtDirection,
): void {
  const sectionIds = new Set<string>();
  const itemsById = new Map<string, PrintMenuSection["items"][number]>();
  for (const section of model.sections) {
    if (sectionIds.has(section.id)) {
      throw new Error(`Duplicate menu section id: ${section.id}`);
    }
    sectionIds.add(section.id);
    for (const item of section.items) {
      if (itemsById.has(item.id)) {
        throw new Error(`Duplicate menu item id: ${item.id}`);
      }
      itemsById.set(item.id, item);
    }
  }
  const selectedItems = new Set<string>();
  const selectedUrls = new Set<string>();
  for (const selection of direction.imageSelections) {
    if (selectedItems.has(selection.itemId)) {
      throw new Error(`Duplicate image selection itemId: ${selection.itemId}`);
    }
    if (selectedUrls.has(selection.url)) {
      throw new Error(`Duplicate image selection URL: ${selection.url}`);
    }
    const item = itemsById.get(selection.itemId);
    if (!item) {
      throw new Error(`Image selection item not found: ${selection.itemId}`);
    }
    const validUrls = new Set([
      ...(item.imageCandidates ?? []),
      ...(item.imageUrl ? [item.imageUrl] : []),
    ]);
    if (!validUrls.has(selection.url)) {
      throw new Error(
        `Image selection URL does not belong to item ${item.id}: ${selection.url}`,
      );
    }
    selectedItems.add(selection.itemId);
    selectedUrls.add(selection.url);
  }
}

function severityRank(severity: DiagnosticSeverity): number {
  return severity === "blocked" ? 2 : severity === "warning" ? 1 : 0;
}

function auditDiagnostics(audit: MenuPrintAudit): MenuPrintDiagnostic[] {
  const diagnostics: MenuPrintDiagnostic[] = [];
  const append = (
    urls: readonly string[],
    suffix: string,
    severity: DiagnosticSeverity,
    messageKey: string,
  ): void => {
    [...new Set(urls)].sort().forEach((url) => {
      diagnostics.push({
        id: `image:${url}:${suffix}`,
        severity,
        messageKey,
        target: { kind: "image", id: url },
      });
    });
  };
  append(
    audit.failedUrls,
    "failed",
    "warning",
    "print.diagnostics.imageFailed",
  );
  append(
    audit.lowResolutionUrls,
    "low-resolution",
    "warning",
    "print.diagnostics.imageLowResolution",
  );
  append(
    audit.duplicateUrls,
    "duplicate",
    "warning",
    "print.diagnostics.imageDuplicate",
  );
  return diagnostics;
}

function requiredSectionHeight(
  section: PrintMenuSection,
  family: RegisteredMenuDesignFamily,
  direction: MenuArtDirection,
  widthMm: number,
  includeHeading: boolean,
): number {
  if (family.compositions[0] !== "image-gallery") {
    return sectionPlanner(direction.treatment)({
      contentRect: {
        xMm: 0,
        yMm: 0,
        widthMm,
        heightMm: 100_000,
      },
      section,
      direction,
      family,
      includeHeading,
    }).consumedHeightMm;
  }
  const selectedIds = new Set(
    direction.imageSelections.map(({ itemId }) => itemId),
  );
  const hasSelectedImage = section.items.some(({ id }) => selectedIds.has(id));
  const selectedImageRole = direction.imageSelections.find(({ itemId }) =>
    section.items.some(({ id }) => id === itemId),
  )?.role;
  const gap = resolvePlannedItemGap(family, direction.treatment);
  const itemHeights = section.items.map((item, itemIndex) => {
    const itemWidth = resolvePlannedItemWidth({
      family,
      contentWidthMm: widthMm,
      treatment: direction.treatment,
      hasSelectedImage,
      selectedImageRole,
      itemIndex,
      ornamentIntensity: direction.ornamentIntensity,
    });
    return estimatePlannedItemHeight({
      item,
      widthMm: itemWidth,
      family,
      treatment: direction.treatment,
    }).heightMm;
  });
  const itemStackHeight =
    itemHeights.reduce((sum, height) => sum + height, 0) +
    Math.max(0, itemHeights.length - 1) * gap;
  if (
    family.compositions[0] === "image-gallery" &&
    direction.treatment !== "type-led" &&
    hasSelectedImage
  ) {
    const headingHeight = includeHeading ? 12 : 0;
    if (direction.treatment === "photo-led") {
      // Side image column beside the copy: the section is copy-driven with a
      // 70mm floor so the photograph keeps real presence without the section
      // swallowing the whole page.
      return roundMm(Math.max(70, headingHeight + 6 + itemStackHeight));
    }
    // Full-width band above the copy; mirrors imageGallery's band sizing.
    const bandHeight = Math.min(64, Math.max(30, widthMm * 0.34));
    return roundMm(bandHeight + 8 + headingHeight + 6 + itemStackHeight);
  }
  return roundMm((includeHeading ? 18 : 0) + itemStackHeight);
}

function sectionPlanner(
  treatment: MenuArtDirection["treatment"],
): (input: SectionPlanInput) => SectionPlan {
  return {
    "type-led": planTypeLedSection,
    balanced: planBalancedSection,
    "photo-led": planPhotoLedSection,
    compact: planCompactSection,
  }[treatment];
}

interface PageState {
  page: PlannedMenuPage;
  regions: PlannedRect[];
  regionIndex: number;
  regionHasContent: boolean[];
  cursorY: number;
  featureImageCount: number;
  lastSectionId?: string;
}

function interSectionGapMm(
  family: RegisteredMenuDesignFamily,
  paperFormat?: MenuArtDirection["paperFormat"],
): number {
  if (family.compositions[0] === "formal-course") return 8;
  if (family.compositions[0] === "ingredient-air") {
    return isCompactPaper(paperFormat) ? 9 : 12;
  }
  return 5;
}

function mastheadWidthMm(
  family: RegisteredMenuDesignFamily,
  contentWidthMm: number,
): number {
  const fraction: Record<
    RegisteredMenuDesignFamily["compositions"][number],
    number
  > = {
    "editorial-asymmetric": 0.58,
    "formal-course": 1,
    "lively-columns": 0.72,
    "cinematic-sections": 1,
    "modular-counter": 0.68,
    "bold-scan": 1,
    "ingredient-air": 0.65,
    "image-gallery": 0.58,
  };
  return roundMm(contentWidthMm * fraction[family.compositions[0]]);
}

/** Tracking applied to the rendered restaurant name per typography personality. */
const MASTHEAD_TRACKING_EM: Partial<
  Record<MenuArtDirection["typographyPersonality"], number>
> = {
  editorial: -0.025,
  refined: 0.015,
  classic: 0.035,
  warm: 0.01,
  dramatic: 0.2, // .12em tracking + uppercase glyph widening
  bold: -0.02,
  modern: 0.16, // .08em tracking + uppercase glyph widening
  natural: -0.01,
};

function mastheadHeightMm(
  model: PrintMenuModel,
  direction: MenuArtDirection,
  family: RegisteredMenuDesignFamily,
  widthMm: number,
): number {
  const composition = family.compositions[0];
  const horizontalInset =
    composition === "formal-course"
      ? 16
      : composition === "cinematic-sections"
        ? 6
        : composition === "modular-counter"
          ? 4
          : composition === "editorial-asymmetric"
            ? 5
            : 0;
  const copyWidth = Math.max(1, widthMm - horizontalInset);
  // Mirrors the rendered .restaurant-name metrics (22pt × scale, 1.04 leading)
  // so long names that wrap to a second line are never clipped.
  const displayPt = 22 * family.typography.displayScale;
  const name = estimateTextBox({
    text: model.business.name,
    widthMm: copyWidth,
    fontPt: displayPt,
    floorPt: displayPt,
    lineHeight: 1.04,
    glyphWidthFactor: Math.max(
      0.2,
      resolveTextGlyphWidthFactor(
        family.typography.display,
        model.business.name,
      ) + (MASTHEAD_TRACKING_EM[direction.typographyPersonality] ?? 0),
    ),
  });
  // Rendered .restaurant-tagline is 7.5pt uppercase with .18em tracking at
  // the base, but maison (9.5pt) and osteria (9pt) restyle it as an italic
  // display line; mirror those or their mastheads plan a line short and the
  // tagline's descenders clip. Both carry the 2mm lockup gap + 2mm margin.
  const taglinePt =
    family.id === "maison" ? 9.5 : family.id === "osteria" ? 9 : 7.5;
  const tagline = model.business.tagline
    ? estimateTextBox({
        text: model.business.tagline,
        widthMm: copyWidth,
        fontPt: taglinePt,
        lineHeight: 1.35,
        glyphWidthFactor:
          resolveTextGlyphWidthFactor(
            family.typography.body,
            model.business.tagline,
          ) + 0.24,
      }).heightMm + 4
    : 0;
  // Rendered logo heights: contained 11mm, mark-only 8mm, wordmark 7mm,
  // plus the 2mm identity-lockup gap below the logo.
  const logo =
    direction.logoTreatment !== "hidden" && model.business.logoUrl
      ? (direction.logoTreatment === "mark-only"
          ? 8
          : direction.logoTreatment === "wordmark"
            ? 7
            : 11) + 2
      : 0;
  // cinematic: 3mm padding-block each side + .35mm borders; modular-counter:
  // 1.6mm accent rule + 3mm padding-block-start.
  const verticalInset =
    composition === "cinematic-sections"
      ? 7
      : composition === "bold-scan"
        ? 4
        : composition === "ingredient-air"
          ? 5
          : composition === "modular-counter"
            ? 5
            : 0;
  return roundMm(
    Math.max(24, logo + name.heightMm + tagline + verticalInset + 5),
  );
}

function decorateBlock(
  block: PlannedMenuBlock,
  pageIndex: number,
  regionIndex: number,
  continuation: boolean,
): PlannedMenuBlock {
  return {
    ...block,
    id: `page:${pageIndex}:region:${regionIndex}:${block.id}`,
    rect: { ...block.rect },
    itemIds: block.itemIds ? [...block.itemIds] : undefined,
    image: block.image ? { ...block.image } : undefined,
    continuation: continuation || block.continuation || undefined,
  };
}

export function planMenuDocument(
  input: PlanMenuDocumentInput,
): PlannedMenuDocument {
  const planned = planMenuDocumentOnce(input);
  if (
    planned.readiness !== "blocked" ||
    input.direction.imageSelections.length === 0
  ) {
    return planned;
  }
  // Photography is an enhancement; the items are the menu. When the document
  // cannot fit with its full photo set, shed photos from the end of the
  // selection one at a time and keep the richest plan that fits — a complete
  // menu with fewer photographs always beats a blocked plan that silently
  // drops dishes. Every shed photo surfaces as a plain warning.
  for (
    let keep = input.direction.imageSelections.length - 1;
    keep >= 0;
    keep -= 1
  ) {
    const reduced = planMenuDocumentOnce({
      ...input,
      direction: {
        ...input.direction,
        imageSelections: input.direction.imageSelections.slice(0, keep),
      },
    });
    if (reduced.readiness === "blocked") continue;
    const diagnostics = [
      ...new Map(
        [
          ...reduced.diagnostics,
          ...input.direction.imageSelections.slice(keep).map(
            (selection): MenuPrintDiagnostic => ({
              id: `image:${selection.url}:unplaced`,
              severity: "warning",
              messageKey: "print.diagnostics.imageUnplaced",
              target: { kind: "image", id: selection.url },
            }),
          ),
        ].map((item) => [item.id, item]),
      ).values(),
    ];
    return {
      ...reduced,
      diagnostics,
      readiness: diagnostics.reduce<DiagnosticSeverity>(
        (highest, diagnostic) =>
          severityRank(diagnostic.severity) > severityRank(highest)
            ? diagnostic.severity
            : highest,
        "ready",
      ),
    };
  }
  return planned;
}

function planMenuDocumentOnce(
  input: PlanMenuDocumentInput,
): PlannedMenuDocument {
  const failedImageUrls = new Set(input.audit.failedUrls);
  const direction: MenuArtDirection = {
    ...input.direction,
    palette: { ...input.direction.palette },
    imageSelections: input.direction.imageSelections
      .filter(({ url }) => !failedImageUrls.has(url))
      .map((image) => ({ ...image })),
    imageFocalPoints: Object.fromEntries(
      Object.entries(input.direction.imageFocalPoints).map(([url, point]) => [
        url,
        { ...point },
      ]),
    ),
  };
  const family = validateDirection(direction);
  validateImageSelectionRoles(family, input.direction.imageSelections);
  validateModelAndSelections(input.model, input.direction);
  const geometry = resolvePrintGeometry(
    direction.outputFormat,
    direction.paperFormat,
  );
  const contentLeft = geometry.safeArea.left;
  const contentWidth =
    geometry.widthMm - geometry.safeArea.left - geometry.safeArea.right;
  const footerHeight = 12;
  const foldedSurface =
    direction.outputFormat === "folded-booklet" ||
    direction.outputFormat === "takeaway-trifold";
  const maxPages = FIXED_SURFACES.has(direction.outputFormat)
    ? geometry.minimumPages
    : Number.POSITIVE_INFINITY;
  const pages: PageState[] = [];
  let identityOverflowed = false;
  const explicitCoverSelection = direction.imageSelections.find(
    ({ role }) => role === "cover",
  );
  const coverSelection =
    explicitCoverSelection ??
    (direction.coverMode === "photographic"
      ? direction.imageSelections[0]
      : undefined);

  const createRegions = (): PlannedRect[] => {
    const heightMm = roundMm(
      geometry.heightMm -
        geometry.safeArea.top -
        geometry.safeArea.bottom -
        footerHeight,
    );
    if (!foldedSurface) {
      return [
        {
          xMm: contentLeft,
          yMm: geometry.safeArea.top,
          widthMm: contentWidth,
          heightMm,
        },
      ];
    }
    const folds = geometry.foldsMm;
    const gutterMm = 3;
    const boundaries = [0, ...folds, geometry.widthMm];
    const edges = boundaries
      .slice(0, -1)
      .map((left, index) => [
        left + (index === 0 ? geometry.safeArea.left : gutterMm),
        boundaries[index + 1] -
          (index === boundaries.length - 2
            ? geometry.safeArea.right
            : gutterMm),
      ]);
    return edges.map(([left, right]) => ({
      xMm: roundMm(left),
      yMm: geometry.safeArea.top,
      widthMm: roundMm(right - left),
      heightMm,
    }));
  };

  const addPage = (): PageState | undefined => {
    if (pages.length >= maxPages) return undefined;
    const index = pages.length;
    const blocks: PlannedMenuBlock[] = [];
    const regions = createRegions();
    const firstRegion = regions[0];
    const state = {
      page: { index, blocks },
      regions,
      regionIndex: 0,
      regionHasContent: regions.map(() => false),
      cursorY: roundMm(firstRegion.yMm),
      featureImageCount: blocks.filter((block) => block.image).length,
    };
    pages.push(state);
    return state;
  };

  const firstPage = addPage()!;
  if (foldedSurface) {
    while (pages.length < geometry.minimumPages) addPage();
  }

  const identityRegionIndex = foldedSurface ? geometry.panelCount - 1 : 0;
  const identityRegion = firstPage.regions[identityRegionIndex];
  const hasDedicatedCover =
    direction.coverMode !== "none" || coverSelection !== undefined;
  const requiredMastheadHeight = mastheadHeightMm(
    input.model,
    direction,
    family,
    mastheadWidthMm(family, identityRegion.widthMm),
  );
  if (hasDedicatedCover) {
    const coverOwnsSurface = foldedSurface;
    const coverHeight = coverOwnsSurface
      ? identityRegion.heightMm
      : roundMm(
          Math.min(
            72,
            identityRegion.heightMm * 0.34,
            Math.max(60, requiredMastheadHeight + 36),
          ),
        );
    firstPage.page.blocks.push({
      id: "page:0:cover",
      kind: "cover",
      itemIds: coverSelection ? [coverSelection.itemId] : undefined,
      image: coverSelection ? { ...coverSelection } : undefined,
      rect: {
        xMm: identityRegion.xMm,
        yMm: identityRegion.yMm,
        widthMm: identityRegion.widthMm,
        heightMm: coverHeight,
      },
    });
    firstPage.regionHasContent[identityRegionIndex] = true;
    if (!foldedSurface) {
      firstPage.cursorY = roundMm(
        identityRegion.yMm + coverHeight + (coverOwnsSurface ? 0 : 6),
      );
    }
  } else {
    const mastheadWidth = mastheadWidthMm(family, identityRegion.widthMm);
    const mastheadHeight = roundMm(
      foldedSurface
        ? identityRegion.heightMm + footerHeight
        : Math.min(requiredMastheadHeight, identityRegion.heightMm * 0.42),
    );
    identityOverflowed = requiredMastheadHeight > mastheadHeight;
    firstPage.page.blocks.push({
      id: "page:0:masthead",
      kind: "masthead",
      rect: {
        xMm: roundMm(
          identityRegion.xMm + (identityRegion.widthMm - mastheadWidth) / 2,
        ),
        yMm: identityRegion.yMm,
        widthMm: mastheadWidth,
        heightMm: mastheadHeight,
      },
    });
    firstPage.regionHasContent[identityRegionIndex] = true;
    if (!foldedSurface) {
      firstPage.cursorY = roundMm(identityRegion.yMm + mastheadHeight + 5);
    }
  }

  const foldedTraversal = foldedSurface
    ? [
        ...pages[1].regions.map((_, regionIndex) => ({
          page: pages[1],
          regionIndex,
        })),
        ...(direction.outputFormat === "takeaway-trifold"
          ? []
          : firstPage.regions
              .map((_, regionIndex) => ({ page: firstPage, regionIndex }))
              .filter(({ regionIndex }) => regionIndex !== identityRegionIndex)
              .reverse()),
      ]
    : [];
  let foldedTraversalIndex = 0;
  let current = foldedSurface ? foldedTraversal[0].page : firstPage;
  if (foldedSurface) {
    current.regionIndex = foldedTraversal[0].regionIndex;
    current.cursorY = current.regions[current.regionIndex].yMm;
  }
  const diagnostics = auditDiagnostics(input.audit);
  if (!input.model.sections.some(({ items }) => items.length > 0)) {
    diagnostics.push({
      id: "menu:empty",
      severity: "blocked",
      messageKey: "print.emptyMenu",
      target: { kind: "setting", id: "categories" },
    });
  }
  if (direction.coverMode === "photographic" && !coverSelection) {
    diagnostics.push({
      id: "setting:cover:photograph-required",
      severity: "blocked",
      messageKey: "print.diagnostics.photographicCoverNeedsImage",
      target: { kind: "setting", id: "cover" },
    });
  }
  if (identityOverflowed) {
    diagnostics.push({
      id: "identity:overflow",
      severity: "blocked",
      messageKey: "print.diagnostics.chooseLargerFormat",
      target: { kind: "setting", id: direction.paperFormat },
      values: {
        requiredHeightMm: mastheadHeightMm(
          input.model,
          direction,
          family,
          mastheadWidthMm(family, contentWidth),
        ),
      },
    });
  }
  let overflowed = false;

  const advanceRegion = (): PageState | undefined => {
    if (foldedSurface) {
      foldedTraversalIndex += 1;
      const next = foldedTraversal[foldedTraversalIndex];
      if (!next) return undefined;
      next.page.regionIndex = next.regionIndex;
      next.page.cursorY = next.page.regions[next.regionIndex].yMm;
      next.page.lastSectionId = undefined;
      return next.page;
    }
    if (current.regionIndex + 1 < current.regions.length) {
      current.regionIndex += 1;
      current.cursorY = current.regions[current.regionIndex].yMm;
      current.lastSectionId = undefined;
      return current;
    }
    return addPage();
  };

  // Read-only twin of advanceRegion: which region would the next chunk land in?
  // Used to decide whether a category can be kept whole instead of leaving a
  // one- or two-item widow behind. Returns undefined when the surface is out of
  // panels/pages so fixed formats keep their existing overflow behaviour.
  const peekNextRegion = (): PlannedRect | undefined => {
    if (foldedSurface) {
      const next = foldedTraversal[foldedTraversalIndex + 1];
      return next ? next.page.regions[next.regionIndex] : undefined;
    }
    if (current.regionIndex + 1 < current.regions.length) {
      return current.regions[current.regionIndex + 1];
    }
    if (pages.length < maxPages) return createRegions()[0];
    return undefined;
  };

  const directionForPage = (
    page: PageState,
    section: PrintMenuSection,
  ): MenuArtDirection => {
    const remainingImages = Math.max(
      0,
      family.photography.maxFeatureImagesPerPage - page.featureImageCount,
    );
    const itemIds = new Set(section.items.map(({ id }) => id));
    return {
      ...direction,
      palette: { ...direction.palette },
      imageSelections: direction.imageSelections
        .filter(
          ({ itemId, role, url }) =>
            itemIds.has(itemId) &&
            role !== "cover" &&
            !(coverSelection?.itemId === itemId && coverSelection.url === url),
        )
        .slice(0, remainingImages)
        .map((selection) => ({ ...selection })),
      imageFocalPoints: Object.fromEntries(
        Object.entries(direction.imageFocalPoints).map(([url, point]) => [
          url,
          { ...point },
        ]),
      ),
    };
  };

  const diagnoseOverflow = (sectionId: string): void => {
    overflowed = true;
    diagnostics.push({
      id: `section:${sectionId}:overflow`,
      severity: "blocked",
      messageKey: "print.diagnostics.sectionOverflow",
      target: { kind: "category", id: sectionId },
    });
  };

  // A spread prints as a physical pair of pages; filling page one to the edge
  // while page two sits empty reads as a mistake. Track how much of the menu
  // has been placed so page breaks can happen at category boundaries once a
  // page holds its share.
  const spreadSurface = !foldedSurface && geometry.minimumPages > 1;
  const totalRequiredHeight = spreadSurface
    ? input.model.sections.reduce(
        (sum, section) =>
          sum +
          requiredSectionHeight(section, family, direction, contentWidth, true),
        0,
      )
    : 0;
  let placedContentHeight = 0;

  for (const [
    sectionIndex,
    originalSection,
  ] of input.model.sections.entries()) {
    let itemIndex = 0;
    if (
      spreadSurface &&
      sectionIndex > 0 &&
      current.page.index < geometry.minimumPages - 1 &&
      current.regionHasContent[current.regionIndex] &&
      placedContentHeight >=
        (totalRequiredHeight * (current.page.index + 1)) / geometry.minimumPages
    ) {
      const next = advanceRegion();
      if (next) current = next;
    }
    // A fold is a real editorial boundary. Start each category on a fresh
    // panel when doing so still leaves one panel for every remaining category.
    // Compact menus with more categories than panels may share a panel instead
    // of being rejected despite fitting physically.
    const remainingSections = input.model.sections.length - sectionIndex;
    const panelsAvailableAfterAdvance =
      foldedTraversal.length - foldedTraversalIndex - 1;
    if (
      foldedSurface &&
      current.regionHasContent[current.regionIndex] &&
      current.lastSectionId !== originalSection.id &&
      panelsAvailableAfterAdvance >= remainingSections
    ) {
      const next = advanceRegion();
      if (!next) {
        diagnoseOverflow(originalSection.id);
        break;
      }
      current = next;
    }
    while (itemIndex < originalSection.items.length) {
      const region = current.regions[current.regionIndex];
      const regionBottom = region.yMm + region.heightMm;
      const sectionGap =
        current.lastSectionId && current.lastSectionId !== originalSection.id
          ? interSectionGapMm(family, direction.paperFormat)
          : 0;
      const sectionStartY = roundMm(current.cursorY + sectionGap);
      const availableHeight = roundMm(regionBottom - sectionStartY);
      const countFit = (stripImages: boolean): number => {
        let fit = 0;
        for (
          let count = 1;
          count <= originalSection.items.length - itemIndex;
          count += 1
        ) {
          const candidate: PrintMenuSection = {
            ...originalSection,
            items: originalSection.items.slice(itemIndex, itemIndex + count),
          };
          const candidateDirection = directionForPage(current, candidate);
          if (stripImages) candidateDirection.imageSelections = [];
          const required = requiredSectionHeight(
            candidate,
            family,
            candidateDirection,
            region.widthMm,
            true,
          );
          if (required <= availableHeight) fit = count;
          else break;
        }
        return fit;
      };
      let fitCount = countFit(false);
      let stripImagesForChunk = false;
      if (fitCount === 0) {
        const freshRegion = !current.regionHasContent[current.regionIndex];
        const canAdvance = foldedSurface
          ? foldedTraversalIndex + 1 < foldedTraversal.length
          : current.regionIndex + 1 < current.regions.length ||
            pages.length < maxPages;
        // Photography is an enhancement; the items are the menu. Before
        // declaring a surface too dense, retry the fit without the photo — a
        // fresh region is the largest space this section will ever get, so a
        // failure there (or on the last region) can only be recovered by
        // giving the copy the photo's space.
        if (freshRegion || !canAdvance) {
          const stripped = countFit(true);
          if (stripped > 0) {
            fitCount = stripped;
            stripImagesForChunk = true;
          }
        }
        if (fitCount === 0) {
          if (freshRegion || !canAdvance) {
            diagnoseOverflow(originalSection.id);
            break;
          }
          const next = advanceRegion();
          if (!next) {
            diagnoseOverflow(originalSection.id);
            break;
          }
          current = next;
          continue;
        }
      }

      // Keep categories whole. Splitting a course so that one or two dishes
      // trail onto the next panel reads as a printing accident, so prefer
      // moving the whole remainder over when the next region can hold it and
      // this region already carries content. When deferring is impossible,
      // pull the break earlier so the continuation is a readable group.
      const remainingItems = originalSection.items.length - itemIndex;
      const widowCount = remainingItems - fitCount;
      const nextRegion = widowCount > 0 ? peekNextRegion() : undefined;
      // With nowhere left to continue, the tail is not a widow — it is the part
      // of the menu that does not fit at all. Placing fewer dishes there only
      // shrinks an already-blocked plan, so leave that path untouched.
      if (widowCount > 0 && widowCount <= 2 && nextRegion !== undefined) {
        // Measure against a fresh photography budget: the next region may sit
        // on a new page, and more images only make the block taller.
        const fitsNextRegion = (items: PrintMenuItem[]): boolean => {
          const candidate: PrintMenuSection = { ...originalSection, items };
          return (
            requiredSectionHeight(
              candidate,
              family,
              directionForPage({ ...current, featureImageCount: 0 }, candidate),
              nextRegion.widthMm,
              true,
            ) <= nextRegion.heightMm
          );
        };
        const remaining = originalSection.items.slice(itemIndex);
        if (
          current.regionHasContent[current.regionIndex] &&
          fitsNextRegion(remaining)
        ) {
          const next = advanceRegion();
          if (next) {
            current = next;
            continue;
          }
        }
        // Deferring is impossible, so move the break earlier instead — but only
        // when the larger continuation actually lands whole in the next region.
        // Otherwise the break just moves and the page loses dishes for nothing.
        const balanced = remainingItems - 3;
        if (
          balanced >= 1 &&
          balanced < fitCount &&
          fitsNextRegion(remaining.slice(balanced))
        ) {
          fitCount = balanced;
        }
      }

      const chunk: PrintMenuSection = {
        ...originalSection,
        items: originalSection.items.slice(itemIndex, itemIndex + fitCount),
      };
      const chunkDirection = directionForPage(current, chunk);
      if (stripImagesForChunk) chunkDirection.imageSelections = [];
      const requiredHeight = requiredSectionHeight(
        chunk,
        family,
        chunkDirection,
        region.widthMm,
        true,
      );
      const height = Math.min(availableHeight, requiredHeight);
      const contentRect: PlannedRect = {
        xMm: region.xMm,
        yMm: sectionStartY,
        widthMm: region.widthMm,
        heightMm: height,
      };
      const planned = sectionPlanner(direction.treatment)({
        contentRect,
        section: chunk,
        direction: chunkDirection,
        family,
        includeHeading: true,
      });
      current.page.blocks.push(
        ...planned.blocks.map((block) =>
          decorateBlock(
            block,
            current.page.index,
            current.regionIndex,
            itemIndex > 0,
          ),
        ),
      );
      diagnostics.push(...planned.diagnostics);
      current.featureImageCount += planned.blocks.filter(
        (block) => block.kind === "image",
      ).length;
      current.lastSectionId = originalSection.id;
      current.regionHasContent[current.regionIndex] = true;
      current.cursorY = roundMm(sectionStartY + height);
      placedContentHeight += height;
      itemIndex += fitCount;
      if (itemIndex < originalSection.items.length) {
        const next = advanceRegion();
        if (!next) {
          diagnoseOverflow(originalSection.id);
          break;
        }
        current = next;
      }
    }
    if (overflowed) break;
  }

  if (overflowed) {
    const totalItems = input.model.sections.reduce(
      (sum, section) => sum + section.items.length,
      0,
    );
    const placedItems = new Set(
      pages.flatMap(({ page }) =>
        page.blocks.flatMap((block) =>
          block.kind === "item-list" || block.kind === "item-feature"
            ? (block.itemIds ?? [])
            : [],
        ),
      ),
    ).size;
    diagnostics.push({
      id: `format:${direction.outputFormat}:too-dense`,
      severity: "blocked",
      messageKey: "print.diagnostics.chooseLargerFormat",
      target: { kind: "setting", id: direction.outputFormat },
      values: {
        omittedItems: Math.max(0, totalItems - placedItems),
        placedItems,
        totalItems,
      },
    });
  }

  const placedImageUrls = new Set(
    pages.flatMap(({ page }) =>
      page.blocks.flatMap((block) => (block.image ? [block.image.url] : [])),
    ),
  );
  for (const block of pages.flatMap(({ page }) => page.blocks)) {
    if (!block.image) continue;
    const measurement = input.audit.measurements?.[block.image.url];
    if (!measurement) continue;
    const effectiveDpi = effectivePrintDpi(measurement, {
      widthMm: block.rect.widthMm,
      heightMm: block.rect.heightMm,
    });
    const minimumDpi = block.image.role === "compact-tile" ? 150 : 220;
    if (effectiveDpi !== undefined && effectiveDpi < minimumDpi) {
      diagnostics.push({
        id: `image:${block.image.url}:effective-dpi`,
        severity: "warning",
        messageKey: "print.diagnostics.imageEffectiveDpi",
        target: { kind: "image", id: block.image.url },
        values: { effectiveDpi, minimumDpi },
      });
    }
  }
  for (const selection of direction.imageSelections) {
    if (!placedImageUrls.has(selection.url)) {
      diagnostics.push({
        id: `image:${selection.url}:unplaced`,
        severity: "warning",
        messageKey: "print.diagnostics.imageUnplaced",
        target: { kind: "image", id: selection.url },
      });
    }
  }

  while (pages.length < geometry.minimumPages) addPage();

  // Sparse continuation pages should look intentionally composed, not as if
  // the planner abandoned the content at the top edge. Preserve the exact
  // block rhythm and seat the small course group in the upper third of the
  // safe region: dead-centre makes the group float as an island between two
  // equal voids, while an upper bias still reads as a deliberate opening.
  if (!foldedSurface) {
    pages.slice(1).forEach((state) => {
      const contentBlocks = state.page.blocks.filter(
        (block) => block.kind !== "footer" && block.kind !== "folio",
      );
      const itemCount = contentBlocks.reduce(
        (total, block) =>
          total +
          (block.kind === "item-list" || block.kind === "item-feature"
            ? (block.itemIds?.length ?? 0)
            : 0),
        0,
      );
      if (itemCount === 0 || itemCount > 2 || contentBlocks.length === 0)
        return;
      const region = state.regions[0];
      const top = Math.min(...contentBlocks.map((block) => block.rect.yMm));
      const bottom = Math.max(
        ...contentBlocks.map((block) => block.rect.yMm + block.rect.heightMm),
      );
      // Never push the cluster past the safe region: a tall block (a gallery
      // image column, say) seated at the upper third would otherwise land on
      // the footer.
      const delta = Math.max(
        0,
        Math.min(
          roundMm(region.yMm + region.heightMm * 0.16 - top),
          roundMm(region.yMm + region.heightMm - bottom),
        ),
      );
      contentBlocks.forEach((block) => {
        block.rect.yMm = roundMm(block.rect.yMm + delta);
      });
    });
  }

  // Distribute leftover depth so a page never ends in a dead band. Block
  // heights and intra-section rhythm stay untouched: the whole content cluster
  // (identity lockup included) slides down as one unit so the masthead keeps
  // its fixed relationship to the first course, and the gaps *between* courses
  // grow modestly on top of that.
  if (!foldedSurface) {
    for (const state of pages) {
      const region = state.regions[0];
      const contentBlocks = state.page.blocks.filter(
        (block) => block.kind !== "footer" && block.kind !== "folio",
      );
      if (!contentBlocks.length) continue;
      const itemCount = contentBlocks.reduce(
        (total, block) =>
          total +
          (block.kind === "item-list" || block.kind === "item-feature"
            ? (block.itemIds?.length ?? 0)
            : 0),
        0,
      );
      // Sparse continuation pages were already centered above.
      if (state.page.index > 0 && itemCount > 0 && itemCount <= 2) continue;
      const sectionIds = [
        ...new Set(
          contentBlocks
            .filter((block) => block.sectionId)
            .sort((first, second) => first.rect.yMm - second.rect.yMm)
            .map((block) => block.sectionId as string),
        ),
      ];
      if (!sectionIds.length) continue;
      const bottom = Math.max(
        ...contentBlocks.map((block) => block.rect.yMm + block.rect.heightMm),
      );
      const leftover = region.yMm + region.heightMm - bottom;
      if (leftover <= 22) continue;
      // Only a hairline stays unallocated; anything larger reappears as the
      // dead band this pass exists to remove.
      const spendable = leftover - 4;
      // Gaps between courses grow modestly, then the whole cluster drops by a
      // little under half of what is left so the residual air lands roughly
      // 42:58 above/below the content — optically centred, since a mass reads
      // as sunken when the space above and below it is mathematically equal.
      // Continuation pages are the exception: they open without a masthead, so
      // a large drop reads as the printer skipping — they absorb more of the
      // leftover into course gaps and stay anchored near the top edge.
      const continuationPage = state.page.index > 0;
      const gapShift =
        sectionIds.length > 1
          ? Math.min(
              (spendable * (continuationPage ? 0.3 : 0.2)) /
                (sectionIds.length - 1),
              continuationPage ? 12 : 9,
            )
          : 0;
      const gapTotal = gapShift * (sectionIds.length - 1);
      const clusterShift = roundMm(
        Math.min(
          (spendable - gapTotal) * 0.42,
          continuationPage ? region.heightMm * 0.06 : Number.POSITIVE_INFINITY,
        ),
      );
      contentBlocks.forEach((block) => {
        block.rect.yMm = roundMm(block.rect.yMm + clusterShift);
      });
      sectionIds.forEach((sectionId, order) => {
        if (order === 0) return;
        const delta = roundMm(gapShift * order);
        contentBlocks
          .filter((block) => block.sectionId === sectionId)
          .forEach((block) => {
            block.rect.yMm = roundMm(block.rect.yMm + delta);
          });
      });
    }
  }

  if (foldedSurface) {
    for (const state of pages) {
      state.regions.forEach((region, regionIndex) => {
        if (state.regionHasContent[regionIndex]) return;
        const hasContact = Boolean(
          input.model.business.address || input.model.business.customUrl,
        );
        const hasStory = Boolean(input.model.business.tagline);
        if (!hasContact && !hasStory) return;
        const contactPanel = state.page.index === 0 && regionIndex === 0;
        state.page.blocks.push({
          id: `page:${state.page.index}:panel:${regionIndex}:note:${contactPanel ? "contact" : "story"}`,
          kind: "panel-note",
          rect: {
            xMm: region.xMm,
            yMm: roundMm(region.yMm + region.heightMm * 0.3),
            widthMm: region.widthMm,
            heightMm: roundMm(Math.min(48, region.heightMm * 0.34)),
          },
        });
        state.regionHasContent[regionIndex] = true;
      });
    }
  }

  if (
    FIXED_SURFACES.has(direction.outputFormat) ||
    direction.outputFormat === "two-page-spread"
  ) {
    pages.forEach(({ page }, pageIndex) => {
      if (pageIndex === 0) return;
      const itemCount = page.blocks.reduce(
        (total, block) =>
          total +
          (block.kind === "item-list" || block.kind === "item-feature"
            ? (block.itemIds?.length ?? 0)
            : 0),
        0,
      );
      if (itemCount <= 1) {
        diagnostics.push({
          id: `page:${pageIndex}:underfilled`,
          severity: "warning",
          messageKey: "print.diagnostics.underfilledPage",
          target: { kind: "page", id: String(pageIndex) },
          values: { page: pageIndex + 1, itemCount },
        });
      }
    });
  }

  for (const state of pages) {
    const footerRegions = foldedSurface
      ? state.page.index === 0
        ? [state.regions[0]]
        : []
      : state.regions;
    footerRegions.forEach((region, panelIndex) => {
      state.page.blocks.push({
        id:
          state.regions.length === 1
            ? `page:${state.page.index}:footer`
            : `page:${state.page.index}:panel:${panelIndex}:footer`,
        kind: "footer",
        rect: {
          xMm: region.xMm,
          yMm: roundMm(region.yMm + region.heightMm),
          widthMm: region.widthMm,
          heightMm: footerHeight,
        },
      });
    });
  }

  const deduped = [
    ...new Map(diagnostics.map((item) => [item.id, item])).values(),
  ];
  const readiness = deduped.reduce<DiagnosticSeverity>(
    (highest, diagnostic) =>
      severityRank(diagnostic.severity) > severityRank(highest)
        ? diagnostic.severity
        : highest,
    "ready",
  );

  return {
    direction,
    geometry: {
      ...geometry,
      safeArea: { ...geometry.safeArea },
      foldsMm: [...geometry.foldsMm],
    },
    pages: pages.map(({ page }) => ({
      index: page.index,
      blocks: page.blocks.map((block) => ({
        ...block,
        rect: { ...block.rect },
        itemIds: block.itemIds ? [...block.itemIds] : undefined,
        image: block.image ? { ...block.image } : undefined,
      })),
    })),
    diagnostics: deduped.map((diagnostic) => ({
      ...diagnostic,
      target: { ...diagnostic.target },
      values: diagnostic.values ? { ...diagnostic.values } : undefined,
    })),
    readiness,
  };
}

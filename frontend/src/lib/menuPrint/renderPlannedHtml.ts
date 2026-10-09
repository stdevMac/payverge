import { getMenuDesignFamily } from "./families";
import {
  buildFontFaceCss,
  getPrintLanguageDirection,
  getPrintScriptFontFallback,
  type PrintFontFamilyName,
} from "./fonts";
import { formatMenuPrice } from "./formatPrice";
import { normalizeSemanticMarkers } from "./planner/strategies";
import type {
  MenuDesignFamilyId,
  PlannedMenuBlock,
  PlannedMenuDocument,
  PlannedMenuPage,
  PrintMenuItem,
  PrintMenuModel,
  PrintMenuSection,
  RegisteredMenuDesignFamily,
} from "./types";
import {
  MENU_OUTPUT_FORMATS,
  MENU_TREATMENTS,
  PRINT_CONTACT_PLACEMENTS,
  PRINT_PRICE_TREATMENTS,
  PRINT_TYPOGRAPHY_BY_FAMILY,
} from "./types";

export interface RenderPlannedMenuOptions {
  origin: string;
  qrDataUrl?: string;
  qrCaption?: string;
}

interface RenderContext {
  family: RegisteredMenuDesignFamily;
  primaryFooter?: PlannedMenuBlock;
  /** Number of non-contact panel-notes already rendered (document order). */
  storyPanelsSeen: number;
}

const SERIF_FAMILIES = new Set<PrintFontFamilyName>([
  "DM Serif Display",
  "EB Garamond",
  "Cormorant Garamond",
]);
const COLOR_RE = /^#(?:[0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$/i;
const SAFE_QR_RE =
  /^data:image\/(?:png|jpeg|gif|webp);base64,[a-z0-9+/]+={0,2}$/i;
const CAPTIONED_QR_MIN_FOOTER_WIDTH_MM = 64;
const BLOCK_KINDS = new Set([
  "masthead",
  "cover",
  "category-heading",
  "item-list",
  "item-feature",
  "image",
  "panel-note",
  "footer",
  "folio",
]);
const IMAGE_ROLES = new Set([
  "cover",
  "full-width",
  "half-page",
  "editorial-crop",
  "category-opener",
  "paired",
  "compact-tile",
]);

/** Escape data for HTML text and quoted attribute contexts. */
export function escapeHtml(value: string): string {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export function renderPlannedMenuHtml(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  options: RenderPlannedMenuOptions,
): string {
  const context = validateRenderInput(model, document, options);
  const language = model.language.trim() || "en";
  const direction =
    getPrintLanguageDirection(language) === "rtl" ? ' dir="rtl"' : "";
  const scriptFonts = getPrintScriptFontFallback(language);
  const displayStack = printFontStack(
    context.family.typography.display,
    scriptFonts?.displayFamily,
  );
  const bodyStack = printFontStack(
    context.family.typography.body,
    scriptFonts?.bodyFamily,
  );
  const structuralClasses =
    FAMILY_STRUCTURE_CLASSES[document.direction.familyId].join(" ");
  const pages = document.pages
    .map((page) => renderPage(model, document, page, options, context))
    .join("\n");

  return `<!doctype html>
<html lang="${escapeHtml(language)}"${direction}>
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>${escapeHtml(model.business.name)}</title>
<style>
${buildDocumentCss(document, context.family, displayStack, bodyStack, options.origin, scriptFonts?.families ?? [])}
</style>
</head>
<body class="menu-document family-${document.direction.familyId} treatment-${document.direction.treatment} format-${document.direction.outputFormat} ornament-${document.direction.ornamentIntensity} native-ornament-${context.family.ornament} logo-${document.direction.logoTreatment} typography-${document.direction.typographyPersonality} contact-${document.direction.contactPlacement} ${structuralClasses}">
${pages}
</body>
</html>`;
}

export function renderPlannedPageHtml(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  page: PlannedMenuPage,
  options: RenderPlannedMenuOptions,
): string {
  const context = validateRenderInput(model, document, options);
  if (!document.pages.includes(page)) {
    throw new Error(`Planned page does not belong to document: ${page.index}`);
  }
  return renderPage(model, document, page, options, context);
}

function renderPage(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  page: PlannedMenuPage,
  options: RenderPlannedMenuOptions,
  context: RenderContext,
): string {
  const pageNumber = page.index + 1;
  const blocks = page.blocks
    .map((block) =>
      renderBlock(model, document, pageNumber, block, options, context),
    )
    .join("\n");
  const family = document.direction.familyId;

  return `<section class="print-page" data-page="${pageNumber}" data-family="${family}" data-treatment="${document.direction.treatment}" data-format="${document.direction.outputFormat}" data-composition="${context.family.compositions[0]}" data-ornament="${document.direction.ornamentIntensity}">
${blocks}
</section>`;
}

function renderBlock(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  pageNumber: number,
  block: PlannedMenuBlock,
  options: RenderPlannedMenuOptions,
  context: RenderContext,
): string {
  validateRect(block);
  const role = block.image?.role ?? block.kind;
  const familyClass = `family-${document.direction.familyId}`;
  const structureClass = structureClassForBlock(
    document.direction.familyId,
    block.kind,
  );
  const classes = [
    "planned-block",
    `block-${block.kind}`,
    `kind-${block.kind}`,
    `role-${role}`,
    familyClass,
    structureClass,
    block.kind === "cover" && block.image ? "has-cover-image" : "",
    block.continuation ? "is-continuation" : "",
  ]
    .filter(Boolean)
    .join(" ");
  const style = rectStyle(block);
  const content = renderBlockContent(
    model,
    document,
    pageNumber,
    block,
    options,
    context,
  );

  return `<div class="${classes}" data-block-id="${escapeHtml(block.id)}" data-kind="${block.kind}" data-role="${role}" style="${style}">${content}</div>`;
}

function renderBlockContent(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  pageNumber: number,
  block: PlannedMenuBlock,
  options: RenderPlannedMenuOptions,
  context: RenderContext,
): string {
  switch (block.kind) {
    case "masthead":
      return renderMasthead(model, document);
    case "cover":
      return renderCover(model, document, block);
    case "category-heading":
      return renderCategoryHeading(requireSection(model, block));
    case "item-list":
    case "item-feature":
      return renderItems(model, block, context.family);
    case "image":
      return renderImage(model, document, block);
    case "panel-note":
      return renderPanelNote(model, document, block, context);
    case "footer":
      return renderFooter(
        model,
        document,
        block,
        block === context.primaryFooter ? options : undefined,
      );
    case "folio":
      return `<span class="folio-number">${pageNumber}</span>`;
  }
}

function renderPanelNote(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  block: PlannedMenuBlock,
  context: RenderContext,
): string {
  const contactPanel = block.id.endsWith(":contact");
  // Never print customUrl (internal slug) — it is not guest-facing copy.
  if (contactPanel) {
    const primary = model.business.address || model.business.tagline;
    return `<div class="panel-note-copy">${primary ? `<p>${escapeHtml(primary)}</p>` : ""}</div>`;
  }

  // Story panels: tagline once, then decorative empties. Address only when
  // contact is not already printed on cover/footer.
  const storyIndex = context.storyPanelsSeen;
  context.storyPanelsSeen += 1;
  if (storyIndex > 0) {
    return `<div class="panel-note-copy panel-note-ornament" aria-hidden="true"><span class="panel-note-rule"></span></div>`;
  }
  const primary = model.business.tagline;
  const secondary =
    document.direction.contactPlacement === "none"
      ? model.business.address
      : undefined;
  return `<div class="panel-note-copy">${primary ? `<p>${escapeHtml(primary)}</p>` : ""}${secondary ? `<span>${escapeHtml(secondary)}</span>` : ""}</div>`;
}

const FOLDED_COVER_FORMATS: readonly string[] = [
  "folded-booklet",
  "takeaway-trifold",
];

const COVER_MONOGRAM_STOP_WORDS = new Set([
  "the",
  "of",
  "and",
  "at",
  "a",
  "an",
  "de",
  "del",
  "la",
  "el",
  "los",
  "las",
  "y",
  "al",
  "le",
  "les",
  "lo",
  "du",
  "des",
  "et",
  "chez",
  "il",
  "della",
  "di",
  "e",
  "da",
  "do",
  "dos",
  "das",
  "o",
  "der",
  "die",
  "das",
  "und",
  "van",
  "het",
  "en",
]);

function coverMonogram(name: string): string {
  const words = name.split(/\s+/).filter((word) => /\p{L}|\p{N}/u.test(word));
  const significant = words.filter(
    (word) => !COVER_MONOGRAM_STOP_WORDS.has(word.toLocaleLowerCase()),
  );
  return (significant.length > 0 ? significant : words)
    .slice(0, 2)
    .map((word) => [...word][0]?.toLocaleUpperCase() ?? "")
    .join("");
}

function renderMasthead(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
): string {
  const treatment = document.direction.logoTreatment;
  const logoUrl = safeImageUrl(model.business.logoUrl);
  // Empty alt on purpose: the name always prints as an h1 in the same lockup,
  // so alt text would duplicate it and leak as visible copy on a broken logo.
  const logo =
    treatment !== "hidden" && logoUrl
      ? `<img class="restaurant-logo logo-${treatment}" src="${escapeHtml(logoUrl)}" alt=""/>`
      : "";
  const tagline = model.business.tagline
    ? `<p class="restaurant-tagline">${escapeHtml(model.business.tagline)}</p>`
    : "";
  if (!FOLDED_COVER_FORMATS.includes(document.direction.outputFormat)) {
    const contact =
      document.direction.contactPlacement === "cover" && model.business.address
        ? `<p class="cover-contact">${escapeHtml(model.business.address)}</p>`
        : "";
    return `<div class="identity-lockup">${logo}<h1 class="restaurant-name">${escapeHtml(model.business.name)}</h1>${tagline}${contact}</div>`;
  }
  // On folded surfaces the masthead owns the whole cover panel, so a bare
  // name lockup floats in a void. Compose three tiers instead: a mark on
  // top, the title centered, and a place line (or ornament) at the foot.
  const monogram = coverMonogram(model.business.name);
  const mark =
    logo ||
    (monogram
      ? `<span class="cover-monogram" aria-hidden="true">${escapeHtml(monogram)}</span>`
      : "");
  const anchor =
    document.direction.contactPlacement !== "none" && model.business.address
      ? `<p class="cover-contact">${escapeHtml(model.business.address)}</p>`
      : `<span class="cover-foot-ornament" aria-hidden="true"><span class="cover-foot-gem"></span></span>`;
  return `<div class="identity-lockup cover-lockup"><div class="cover-tier cover-tier-mark">${mark}</div><div class="cover-tier cover-tier-title"><h1 class="restaurant-name">${escapeHtml(model.business.name)}</h1>${tagline}</div><div class="cover-tier cover-tier-anchor">${anchor}</div></div>`;
}

function renderCover(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  block: PlannedMenuBlock,
): string {
  const logoUrl = safeImageUrl(model.business.logoUrl);
  const coverLogo =
    document.direction.logoTreatment !== "hidden" && logoUrl
      ? `<img class="restaurant-logo cover-logo logo-${document.direction.logoTreatment}" src="${escapeHtml(logoUrl)}" alt=""/>`
      : "";
  const title = `<div class="cover-type">${coverLogo}<p class="cover-restaurant">${escapeHtml(model.business.name)}</p>${
    model.business.tagline
      ? `<p class="cover-tagline">${escapeHtml(model.business.tagline)}</p>`
      : ""
  }${
    document.direction.contactPlacement === "cover" && model.business.address
      ? `<p class="cover-contact">${escapeHtml(model.business.address)}</p>`
      : ""
  }</div>`;
  if (!block.image) return title;
  const image = requireItem(model, block.image.itemId);
  return `${renderPlannedImage(image, document, block.image.url, block.image.role, "cover-image")}${title}`;
}

function renderCategoryHeading(section: PrintMenuSection): string {
  return `<h2 class="category-name">${escapeHtml(section.name)}</h2>${
    section.description
      ? `<p class="category-description">${escapeHtml(section.description)}</p>`
      : ""
  }`;
}

function renderItems(
  model: PrintMenuModel,
  block: PlannedMenuBlock,
  family: RegisteredMenuDesignFamily,
): string {
  requireSection(model, block);
  const itemIds = block.itemIds ?? [];
  if (!itemIds.length) {
    throw new Error(`Planned ${block.kind} block has no itemIds: ${block.id}`);
  }
  return itemIds
    .map((itemId) => renderItem(requireItem(model, itemId), model, family))
    .join("");
}

function renderItem(
  item: PrintMenuItem,
  model: PrintMenuModel,
  family: RegisteredMenuDesignFamily,
): string {
  const spec = PRINT_PRICE_TREATMENTS[family.id];
  const price = formatMenuPrice(
    item.price,
    model.currency,
    spec,
    model.language || "en",
  );
  const description = item.description
    ? `<p class="item-description">${escapeHtml(item.description)}</p>`
    : "";
  const markers = normalizeSemanticMarkers(item)
    .map(
      ({ mark, label, source }) =>
        `<span class="semantic-mark ${source}-mark" aria-label="${escapeHtml(label)}">${escapeHtml(mark)}</span>`,
    )
    .join("");
  const markerGroup = markers
    ? `<span class="semantic-markers">${markers}</span>`
    : "";
  const name = `<h3 class="item-name">${escapeHtml(item.name)}</h3>`;
  const priceTag = `<span class="item-price">${escapeHtml(price)}</span>`;
  if (spec.mode === "nested") {
    return `<article class="menu-item" data-item-id="${escapeHtml(item.id)}">${name}${description}${priceTag}${markerGroup}</article>`;
  }
  const leaders = spec.leaders
    ? '<span class="price-leaders" aria-hidden="true"></span>'
    : "";
  return `<article class="menu-item" data-item-id="${escapeHtml(item.id)}"><div class="item-line">${name}${leaders}${priceTag}</div>${description}${markerGroup}</article>`;
}

function renderImage(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  block: PlannedMenuBlock,
): string {
  if (!block.image) {
    throw new Error(`Planned image block has no image: ${block.id}`);
  }
  const item = requireItem(model, block.image.itemId);
  return renderPlannedImage(
    item,
    document,
    block.image.url,
    block.image.role,
    "planned-image",
  );
}

function renderPlannedImage(
  item: PrintMenuItem,
  document: PlannedMenuDocument,
  rawUrl: string,
  role: PlannedMenuBlock["image"] extends infer _Image
    ? NonNullable<PlannedMenuBlock["image"]>["role"]
    : never,
  baseClass: string,
): string {
  const url = safeImageUrl(rawUrl);
  if (!url) return "";
  const focal = document.direction.imageFocalPoints[rawUrl] ?? {
    x: 0.5,
    y: 0.5,
  };
  const x = formatPercent(clampFocal(focal.x));
  const y = formatPercent(clampFocal(focal.y));
  const cropClass = role === "compact-tile" ? "crop-contain" : "crop-cover";
  return `<img class="${baseClass} image-role-${role} ${cropClass}" src="${escapeHtml(url)}" alt="${escapeHtml(item.name)}" style="object-position:${x}% ${y}%"/>`;
}

function renderFooter(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  block: PlannedMenuBlock,
  primaryOptions?: RenderPlannedMenuOptions,
): string {
  // Contact prints once, on the primary (first) footer next to the QR;
  // repeating the address on every continuation page reads as boilerplate.
  const showContact =
    document.direction.contactPlacement === "footer" &&
    primaryOptions !== undefined;
  const address =
    showContact && model.business.address
      ? `<span class="footer-address">${escapeHtml(model.business.address)}</span>`
      : "";
  const legend = renderMarkerLegend(model);
  const qrDataUrl = safeQrDataUrl(primaryOptions?.qrDataUrl);
  const qrLayout =
    block.rect.widthMm >= CAPTIONED_QR_MIN_FOOTER_WIDTH_MM
      ? "captioned"
      : "compact";
  const qr = qrDataUrl
    ? `<figure class="footer-qr footer-qr-${qrLayout}"><img class="qr-image" src="${escapeHtml(qrDataUrl)}" alt="${escapeHtml(primaryOptions?.qrCaption ?? "Menu QR code")}"/>${
        qrLayout === "captioned" && primaryOptions?.qrCaption
          ? `<figcaption>${escapeHtml(primaryOptions.qrCaption)}</figcaption>`
          : ""
      }</figure>`
    : "";
  const copyClass = qrDataUrl
    ? `footer-copy has-qr-clearance qr-clearance-${qrLayout}`
    : "footer-copy";
  return `<div class="${copyClass}">${legend}${address}</div>${qr}`;
}

/** Unique semantic markers in first-appearance order for the printed legend. */
function collectDocumentMarkers(
  model: PrintMenuModel,
): Array<{ mark: string; label: string }> {
  const seen = new Set<string>();
  const markers: Array<{ mark: string; label: string }> = [];
  for (const section of model.sections) {
    for (const item of section.items) {
      for (const marker of normalizeSemanticMarkers(item)) {
        if (seen.has(marker.mark)) continue;
        seen.add(marker.mark);
        markers.push({
          mark: marker.mark,
          label: capitalizeMarkerLabel(marker.label),
        });
      }
    }
  }
  return markers;
}

function capitalizeMarkerLabel(label: string): string {
  const trimmed = label.trim();
  if (!trimmed) return trimmed;
  return trimmed.charAt(0).toLocaleUpperCase() + trimmed.slice(1);
}

function renderMarkerLegend(model: PrintMenuModel): string {
  const markers = collectDocumentMarkers(model);
  if (!markers.length) return "";
  const pairs = markers
    .map(
      ({ mark, label }) =>
        `${escapeHtml(mark)} ${escapeHtml(label)}`,
    )
    .join(" · ");
  return `<span class="footer-marker-legend">${pairs}</span>`;
}

function requireSection(
  model: PrintMenuModel,
  block: PlannedMenuBlock,
): PrintMenuSection {
  const sectionId = block.sectionId ?? "";
  const section = model.sections.find(
    (candidate) => candidate.id === sectionId,
  );
  if (!section) {
    throw new Error(`Planned block references missing section: ${sectionId}`);
  }
  return section;
}

function requireItem(model: PrintMenuModel, itemId: string): PrintMenuItem {
  const item = model.sections
    .flatMap((section) => section.items)
    .find((candidate) => candidate.id === itemId);
  if (!item) {
    throw new Error(`Planned block references missing item: ${itemId}`);
  }
  return item;
}

function validateRenderInput(
  model: PrintMenuModel,
  document: PlannedMenuDocument,
  options: RenderPlannedMenuOptions,
): RenderContext {
  validateOrigin(options.origin);
  const family = getMenuDesignFamily(document.direction.familyId);
  for (const role of ["ground", "ink", "accent", "muted"] as const) {
    const value = document.direction.palette[role];
    if (!value || !COLOR_RE.test(value)) {
      throw new Error(`Invalid print palette color: ${role}`);
    }
  }
  validateDirectionTokens(document, family);
  if (
    !Number.isFinite(document.geometry.widthMm) ||
    document.geometry.widthMm <= 0 ||
    !Number.isFinite(document.geometry.heightMm) ||
    document.geometry.heightMm <= 0
  ) {
    throw new Error("Invalid print page geometry");
  }
  for (const [position, page] of document.pages.entries()) {
    if (
      typeof page.index !== "number" ||
      !Number.isFinite(page.index) ||
      !Number.isInteger(page.index) ||
      page.index < 0 ||
      page.index !== position
    ) {
      throw new Error(`Invalid planned page index at position ${position}`);
    }
  }
  const sectionIds = new Set<string>();
  const itemIds = new Set<string>();
  for (const section of model.sections) {
    sectionIds.add(section.id);
    section.items.forEach((item) => itemIds.add(item.id));
  }
  for (const page of document.pages) {
    for (const block of page.blocks) {
      if (!BLOCK_KINDS.has(block.kind)) {
        throw new Error("Invalid print document token: blockKind");
      }
      if (block.image && !IMAGE_ROLES.has(block.image.role)) {
        throw new Error("Invalid print document token: imageRole");
      }
      validateRect(block);
      if (
        (block.kind === "category-heading" ||
          block.kind === "item-list" ||
          block.kind === "item-feature" ||
          block.kind === "image") &&
        !sectionIds.has(block.sectionId ?? "")
      ) {
        throw new Error(
          `Planned block references missing section: ${block.sectionId ?? ""}`,
        );
      }
      for (const itemId of block.itemIds ?? []) {
        if (!itemIds.has(itemId)) {
          throw new Error(`Planned block references missing item: ${itemId}`);
        }
      }
      if (block.image && !itemIds.has(block.image.itemId)) {
        throw new Error(
          `Planned block references missing item: ${block.image.itemId}`,
        );
      }
    }
  }
  return {
    family,
    primaryFooter: document.pages
      .flatMap((page) => page.blocks)
      .find((block) => block.kind === "footer"),
    storyPanelsSeen: 0,
  };
}

function validateDirectionTokens(
  document: PlannedMenuDocument,
  family: RegisteredMenuDesignFamily,
): void {
  const { direction } = document;
  if (
    !MENU_TREATMENTS.includes(direction.treatment) ||
    !family.supportedTreatments.includes(direction.treatment)
  ) {
    throw new Error("Invalid print document token: treatment");
  }
  if (
    !MENU_OUTPUT_FORMATS.includes(direction.outputFormat) ||
    !family.supportedFormats.includes(direction.outputFormat)
  ) {
    throw new Error("Invalid print document token: outputFormat");
  }
  if (
    !["wordmark", "contained", "mark-only", "hidden"].includes(
      direction.logoTreatment,
    )
  ) {
    throw new Error("Invalid print document token: logoTreatment");
  }
  if (
    !["none", "restrained", "expressive"].includes(direction.ornamentIntensity)
  ) {
    throw new Error("Invalid print document token: ornamentIntensity");
  }
  if (
    !PRINT_TYPOGRAPHY_BY_FAMILY[family.id].includes(
      direction.typographyPersonality,
    )
  ) {
    throw new Error("Invalid print document token: typographyPersonality");
  }
  if (!PRINT_CONTACT_PLACEMENTS.includes(direction.contactPlacement)) {
    throw new Error("Invalid print document token: contactPlacement");
  }
}

function validateOrigin(origin: string): void {
  if (origin === "") return;
  try {
    const parsed = new URL(origin);
    if (
      (parsed.protocol !== "https:" && parsed.protocol !== "http:") ||
      parsed.origin !== origin ||
      parsed.username ||
      parsed.password
    ) {
      throw new Error("unsafe");
    }
  } catch {
    throw new Error("Invalid print origin");
  }
}

function safeImageUrl(rawUrl?: string): string | undefined {
  const value = rawUrl?.trim();
  if (!value || /[\u0000-\u001f\u007f]/.test(value)) return undefined;
  try {
    const parsed = new URL(value, "https://relative.invalid");
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return undefined;
    }
    if (value.startsWith("//")) return undefined;
    return value;
  } catch {
    return undefined;
  }
}

function safeQrDataUrl(rawUrl?: string): string | undefined {
  const value = rawUrl?.trim();
  return value && SAFE_QR_RE.test(value) ? value : undefined;
}

function clampFocal(value: number): number {
  if (!Number.isFinite(value)) return 0.5;
  return Math.min(1, Math.max(0, value));
}

function formatPercent(value: number): string {
  return String(Number((value * 100).toFixed(3)));
}

function validateRect(block: PlannedMenuBlock): void {
  const values = Object.values(block.rect);
  if (
    values.some((value) => !Number.isFinite(value)) ||
    block.rect.widthMm < 0 ||
    block.rect.heightMm < 0
  ) {
    throw new Error(`Invalid planned block rectangle: ${block.id}`);
  }
}

function rectStyle(block: PlannedMenuBlock): string {
  const { xMm, yMm, widthMm, heightMm } = block.rect;
  // Logical inline start mirrors under dir="rtl"; equals left in LTR.
  return `inset-inline-start:${xMm}mm;top:${yMm}mm;width:${widthMm}mm;height:${heightMm}mm`;
}

function printFontStack(
  family: PrintFontFamilyName,
  scriptFamily?: PrintFontFamilyName,
): string {
  const active = scriptFamily ? `"${family}","${scriptFamily}"` : `"${family}"`;
  return SERIF_FAMILIES.has(family)
    ? `${active},"Noto Serif",Georgia,serif`
    : `${active},"Noto Sans",system-ui,sans-serif`;
}

const TYPOGRAPHY_PERSONALITY_CSS = `
.typography-editorial .restaurant-name,.typography-editorial .category-name{letter-spacing:-.025em}
.typography-refined .restaurant-name,.typography-refined .category-name{font-style:italic;letter-spacing:.015em}
.typography-classic .restaurant-name,.typography-classic .category-name{text-align:center;letter-spacing:.035em}
.typography-warm .restaurant-name,.typography-warm .category-name{letter-spacing:.01em;text-transform:none}
.typography-dramatic .restaurant-name,.typography-dramatic .category-name{text-transform:uppercase;letter-spacing:.12em}
.typography-bold .restaurant-name,.typography-bold .category-name{font-weight:600;letter-spacing:-.02em}
.typography-modern .restaurant-name,.typography-modern .category-name{font-weight:600;letter-spacing:.08em;text-transform:uppercase}
.typography-graphic .restaurant-name,.typography-graphic .category-name{font-weight:600;text-transform:uppercase;letter-spacing:.06em}
.typography-natural .restaurant-name,.typography-natural .category-name{font-style:italic;letter-spacing:-.01em}`;

const FAMILY_STRUCTURE_CLASSES: Record<MenuDesignFamilyId, readonly string[]> =
  {
    atelier: [
      "atelier-asymmetric-masthead",
      "atelier-offset-heading",
      "atelier-editorial-side-crop",
    ],
    maison: ["maison-course-axis", "maison-double-rules", "maison-inset-frame"],
    osteria: [
      "osteria-unequal-columns",
      "osteria-category-opener",
      "osteria-lively-rhythm",
    ],
    "night-house": [
      "night-house-cinematic-surface",
      "night-house-frame",
      "night-house-wide-band",
    ],
    counter: [
      "counter-modular-tile",
      "counter-filled-label",
      "counter-dominant-special",
    ],
    street: [
      "street-condensed-scan",
      "street-scan-bar",
      "street-price-rail",
      "street-full-description",
    ],
    field: [
      "field-ingredient-spacing",
      "field-narrow-measure",
      "field-organic-restraint",
    ],
    gallery: [
      "gallery-image-dominant",
      "gallery-negative-space",
      "gallery-photo-field",
    ],
  };

function structureClassForBlock(
  familyId: MenuDesignFamilyId,
  kind: PlannedMenuBlock["kind"],
): string {
  const classes = FAMILY_STRUCTURE_CLASSES[familyId];
  if (kind === "masthead") return classes[0];
  if (kind === "category-heading") return classes[1] ?? classes[0];
  if (kind === "image" || kind === "cover") {
    return classes[2] ?? classes[0];
  }
  return "";
}

function buildDocumentCss(
  document: PlannedMenuDocument,
  family: RegisteredMenuDesignFamily,
  displayStack: string,
  bodyStack: string,
  origin: string,
  scriptFamilies: readonly PrintFontFamilyName[],
): string {
  const palette = document.direction.palette;
  return `${buildFontFaceCss(
    [
      ...new Set([
        family.typography.display,
        family.typography.body,
        ...scriptFamilies,
      ]),
    ],
    origin,
  )}
:root{--page-width:${document.geometry.widthMm}mm;--page-height:${document.geometry.heightMm}mm;--ground:${palette.ground};--ink:${palette.ink};--accent:${palette.accent};--muted:${palette.muted};--display-font:${displayStack};--body-font:${bodyStack};--inline-shift:3mm;--inline-tilt:-1deg}
[dir="rtl"]{--inline-shift:-3mm;--inline-tilt:1deg}
@page { size: var(--page-width) var(--page-height); margin:0; }
*{box-sizing:border-box}
html,body{margin:0;padding:0}
body{font-family:var(--body-font);font-size:${family.typography.bodyFloorPt}pt;line-height:1.35;background:var(--ground);color:var(--ink)}
.print-page{position:relative;width:var(--page-width);height:var(--page-height);break-after:page;overflow:hidden;background:var(--ground);color:var(--ink);print-color-adjust:exact;-webkit-print-color-adjust:exact}
.print-page:last-child{break-after:auto}
.planned-block{position:absolute}
.block-masthead{overflow:hidden;overflow-wrap:anywhere}
.restaurant-logo,.planned-image,.cover-image{display:block;max-width:100%;max-height:100%}
.identity-lockup{display:flex;flex-direction:column;align-items:center;justify-content:flex-start;gap:2mm;width:100%;height:100%;text-align:center}
.identity-lockup.cover-lockup{display:grid;grid-template-rows:1fr auto 1fr;grid-template-columns:minmax(0,1fr);row-gap:4mm;padding:8mm}
.cover-lockup>.cover-tier{display:flex;flex-direction:column;align-items:inherit;gap:2mm;min-block-size:0}
.cover-lockup>.cover-tier-mark{align-self:start}
.cover-lockup>.cover-tier-anchor{align-self:end;justify-content:flex-end}
.cover-monogram{display:inline-grid;place-items:center;min-inline-size:11mm;block-size:11mm;padding-inline:1.6mm;border:.4mm solid var(--accent);font-family:var(--display-font);font-size:12.5pt;line-height:1;letter-spacing:.14em;text-indent:.14em}
.cover-foot-ornament{display:flex;align-items:center;gap:2mm}
.cover-foot-ornament::before,.cover-foot-ornament::after{content:"";inline-size:9mm;border-block-start:.35mm solid var(--accent)}
.cover-foot-gem{inline-size:1.7mm;block-size:1.7mm;border:.35mm solid var(--accent);transform:rotate(45deg)}
.cover-tier-anchor .cover-contact{margin:0;max-inline-size:100%}
.restaurant-logo{flex:none;object-position:center}
.logo-contained{object-fit:contain;width:auto;max-width:38mm;height:11mm}
.logo-mark-only{object-fit:contain;width:8mm;height:8mm}
.logo-wordmark{object-fit:contain;width:auto;max-width:48mm;height:7mm}
[dir="rtl"] .logo-wordmark{object-position:center}
.restaurant-name,.category-name{font-family:var(--display-font);margin:0}
.restaurant-name{font-size:${roundCssNumber(22 * family.typography.displayScale)}pt;line-height:1.04}
.restaurant-tagline{margin:2mm 0 0;font-size:7.5pt;letter-spacing:.18em;text-transform:uppercase;color:var(--muted)}
.cover-tagline,.category-description,.item-description{color:var(--muted)}
.category-description{margin:1.4mm 0 0;font-size:8pt;line-height:1.3}
.cover-contact{margin:2.5mm 0 0;color:var(--muted);font-size:7.5pt;letter-spacing:.08em}
.category-name{font-size:${roundCssNumber(12 * family.typography.displayScale)}pt;line-height:1.05}
.menu-item{overflow:hidden}
.block-item-list{overflow:hidden}
.block-item-list .menu-item{height:auto;overflow:visible}
.block-item-list .menu-item+.menu-item{margin-block-start:2mm}
.block-item-feature>.menu-item:only-child{height:100%;overflow:hidden}
.item-line{display:flex;align-items:baseline;justify-content:space-between;gap:2.5mm}
.item-name{min-width:0;overflow-wrap:anywhere;font-family:var(--display-font);font-weight:500;font-size:1.06em;line-height:1.18;margin:0}
.item-price{flex:none;font-variant-numeric:tabular-nums;white-space:nowrap;font-size:.94em}
.price-leaders{display:none}
.item-description{font-size:.86em;line-height:1.32;margin:.8mm 0 0}
.semantic-markers{display:flex;flex-wrap:wrap;gap:0 .7em;margin-top:1.1mm;font-size:6.3pt;font-weight:600;letter-spacing:.14em;text-transform:uppercase;color:var(--muted)}
.semantic-mark{display:inline-flex;color:inherit}
.semantic-mark+.semantic-mark::before{content:"\\00B7";margin-inline-end:.7em;opacity:.6}
.planned-image,.cover-image{width:100%;height:100%}
.block-cover{overflow:hidden;display:grid;place-items:center}
.block-cover .cover-type{position:relative;z-index:1;display:flex;height:100%;width:100%;flex-direction:column;align-items:center;justify-content:flex-end;padding:8%;text-align:center}
.block-cover:not(.has-cover-image) .cover-type{justify-content:center}
.block-cover .cover-logo{margin-block-end:3mm}
.block-cover .cover-restaurant{margin:0;font-family:var(--display-font);font-size:${roundCssNumber(25 * family.typography.displayScale)}pt;line-height:1.02}
.block-cover:not(.has-cover-image) .cover-restaurant::after{content:"";display:block;inline-size:20mm;border-block-start:.45mm solid var(--accent);margin:4mm auto 0}
.block-cover .cover-tagline{margin:3mm 0 0;font-size:7.5pt;letter-spacing:.18em;text-transform:uppercase}
.block-cover.has-cover-image .cover-image{position:absolute;inset:0}
.block-cover.has-cover-image::after{content:"";position:absolute;inset:0;background:linear-gradient(180deg,transparent 28%,color-mix(in srgb,var(--ink) 78%,transparent) 100%)}
.block-cover.has-cover-image .cover-type{color:var(--ground);text-shadow:0 .3mm .8mm color-mix(in srgb,var(--ink) 55%,transparent)}
.block-cover.has-cover-image .cover-logo{padding:1mm 1.5mm;background:color-mix(in srgb,var(--ground) 92%,transparent);border-radius:.8mm;filter:drop-shadow(0 .3mm .8mm color-mix(in srgb,var(--ink) 30%,transparent))}
.block-cover.has-cover-image .cover-tagline,.block-cover.has-cover-image .cover-contact{color:inherit}
.crop-cover{object-fit:cover}
.crop-contain{object-fit:contain}
.block-footer{max-block-size:12mm;overflow:hidden}
.block-panel-note{display:grid;place-items:center;text-align:center;padding:6mm;color:var(--muted)}
.panel-note-copy{max-width:54mm}.panel-note-copy p{margin:0;font-family:var(--display-font);font-size:13pt;line-height:1.15}.panel-note-copy span{display:block;margin-top:3mm;font-size:7pt;letter-spacing:.14em;text-transform:uppercase}
.panel-note-ornament{display:grid;place-items:center;width:100%;height:100%}
.panel-note-rule{display:block;inline-size:14mm;border-block-start:.35mm solid color-mix(in srgb,var(--accent) 70%,transparent)}
.footer-copy{block-size:12mm;max-block-size:12mm;overflow:hidden;display:flex;flex-direction:column;justify-content:flex-start;align-items:stretch;gap:.6mm;padding-block-start:1.4mm;border-block-start:.2mm solid color-mix(in srgb,var(--muted) 32%,transparent);font-size:6.5pt;letter-spacing:.12em;text-transform:uppercase;color:var(--muted)}
.footer-copy>*{min-width:0;overflow:hidden;overflow-wrap:anywhere;line-height:1.4}
.footer-marker-legend{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-clamp:2;font-size:6.3pt;font-weight:600;letter-spacing:.14em;text-transform:uppercase;color:var(--muted);line-height:1.35;max-block-size:calc(1.35em * 2)}
.footer-copy.qr-clearance-compact{padding-inline-end:14mm}
.footer-copy.qr-clearance-captioned{padding-inline-end:32mm}
.footer-qr{position:absolute;inset-inline-end:0;bottom:0;block-size:12mm;overflow:hidden;display:grid;align-items:center;gap:1.5mm;margin:0;font-size:6pt}
.footer-qr-compact{inline-size:14mm;grid-template-columns:10mm;justify-content:end}
.footer-qr-captioned{inline-size:30mm;grid-template-columns:10mm 1fr}
.footer-qr figcaption{min-width:0;overflow:hidden;line-height:1.1}
.qr-image{width:10mm;height:10mm;object-fit:contain}
.folio-number{font-variant-numeric:tabular-nums}
.treatment-compact .item-description{font-size:.8em}
.treatment-photo-led .block-image{isolation:isolate}
${TYPOGRAPHY_PERSONALITY_CSS}
${FAMILY_CSS[document.direction.familyId]}
.ornament-none .block-category-heading{border:0;background:none;color:inherit;box-shadow:none}
.ornament-none .print-page{outline:0}
.ornament-restrained .block-category-heading,.ornament-expressive .block-category-heading{border:0;background:none;color:inherit;box-shadow:none}
.ornament-restrained .print-page,.ornament-expressive .print-page{outline:0}
.ornament-restrained.native-ornament-none .block-category-heading{border:0;background:none}
.ornament-expressive.native-ornament-none .block-category-heading{border:0;background:none}
.ornament-restrained.native-ornament-rule .category-name::after{content:"";display:block;inline-size:11mm;border-block-start:.5mm solid var(--accent);margin-block-start:1.8mm}
.typography-classic.ornament-restrained.native-ornament-rule .category-name::after{margin-inline:auto}
.ornament-expressive.native-ornament-rule .block-category-heading{border-block-start:.75mm double var(--accent);padding-block-start:2mm}
.ornament-restrained.native-ornament-double-rule .block-category-heading{border-block:.45mm double var(--accent);padding-block:1.8mm}
.ornament-expressive.native-ornament-double-rule .block-category-heading{border-block:1mm double var(--accent);padding-block:2mm}
.ornament-restrained.native-ornament-frame .print-page{outline:.3mm solid var(--accent);outline-offset:-4mm}
.ornament-expressive.native-ornament-frame .print-page{outline:.8mm double var(--accent);outline-offset:-5mm}
.family-street.ornament-restrained .block-category-heading{background:var(--ink);color:var(--ground);--muted:color-mix(in srgb,currentColor 75%,transparent)}
.family-counter.ornament-restrained .category-name::after{content:"";display:block;inline-size:100%;border-block-start:1.1mm solid var(--accent);margin-block-start:1.2mm}
.ornament-expressive.native-ornament-block .block-category-heading{background:var(--ink);color:var(--ground);--muted:color-mix(in srgb,currentColor 75%,transparent);box-shadow:inset 0 -.8mm 0 var(--accent)}
.block-category-heading.is-continuation{border:0;border-block:0;border-block-start:0;background:none;box-shadow:none;padding:0;padding-block:0;padding-block-start:0;color:inherit}
.block-category-heading.is-continuation .category-name{font-size:${roundCssNumber(10 * family.typography.displayScale)}pt;color:var(--muted)}
.block-category-heading.is-continuation .category-name::after{content:none;display:none;border:0;margin:0;inline-size:0}
.family-street.ornament-restrained .block-category-heading.is-continuation,.ornament-expressive.native-ornament-block .block-category-heading.is-continuation,.ornament-expressive.native-ornament-rule .block-category-heading.is-continuation,.ornament-restrained.native-ornament-double-rule .block-category-heading.is-continuation,.ornament-expressive.native-ornament-double-rule .block-category-heading.is-continuation{border:0;border-block:0;border-block-start:0;background:none;box-shadow:none;padding:0;padding-block:0;color:inherit}`;
}

function roundCssNumber(value: number): string {
  return String(Number(value.toFixed(2)));
}

const FAMILY_CSS: Record<MenuDesignFamilyId, string> = {
  atelier: `
.atelier-asymmetric-masthead.block-masthead{border-block-start:.55mm solid var(--ink);padding-block-start:3.5mm}
.family-atelier .identity-lockup{align-items:flex-start;text-align:start}
.family-atelier .restaurant-tagline{letter-spacing:.22em}
.atelier-editorial-side-crop.block-image{border-inline-end:2mm solid var(--ground)}
.family-atelier .item-name{font-weight:400;font-size:1.12em}
.family-atelier .item-price{font-family:var(--body-font);font-weight:500;font-size:.9em}
.family-atelier .item-line{justify-content:flex-start}
.family-atelier .price-leaders{display:block;flex:1 1 4mm;min-inline-size:4mm;block-size:.72em;border-block-end:.28mm dotted color-mix(in srgb,var(--muted) 55%,transparent)}
.family-atelier .block-item-feature{padding-inline-start:5mm;border-inline-start:.3mm solid var(--accent)}`,
  maison: `
.maison-course-axis.block-masthead{text-align:center;padding-inline:8mm}
.maison-course-axis .identity-lockup:not(.cover-lockup)::after,.maison-course-axis .cover-tier-title::after{content:"";inline-size:26mm;border-block-start:.8mm double var(--accent);margin-block-start:2mm}
.family-maison .restaurant-tagline{font-family:var(--display-font);font-style:italic;text-transform:none;letter-spacing:.03em;font-size:9.5pt}
.maison-double-rules.block-category-heading{text-align:center}
.maison-inset-frame.block-cover,.family-maison .print-page{outline:.3mm solid var(--accent);outline-offset:-5mm}
.family-maison .menu-item{text-align:center}
.family-maison .item-name{font-weight:600;letter-spacing:.12em;text-transform:uppercase;font-size:.95em}
.family-maison .item-description{font-style:italic;font-size:.94em;max-width:84%;margin:1mm auto 0}
.family-maison .item-price{display:block;margin-block-start:1mm;font-size:.9em;letter-spacing:.14em;color:var(--ink)}
.family-maison .semantic-markers{justify-content:center}`,
  osteria: `
.osteria-unequal-columns.block-masthead{border-block-start:.6mm solid var(--accent);padding-block-start:2.5mm;text-align:center}
.format-folded-booklet .osteria-unequal-columns.block-masthead,.format-takeaway-trifold .osteria-unequal-columns.block-masthead{border:0;padding:0;outline:.35mm solid var(--accent);outline-offset:-4mm}
.osteria-unequal-columns .cover-tier-title::before,.osteria-unequal-columns .cover-tier-title::after{content:"";display:block;width:22mm;border-block-start:.45mm solid var(--accent);margin-block:1.5mm}
.family-osteria .restaurant-tagline{font-family:var(--display-font);font-style:italic;text-transform:none;letter-spacing:.02em;font-size:9pt}
.osteria-category-opener.block-category-heading{min-inline-size:0;overflow:visible}
.osteria-category-opener .category-name{text-wrap:balance;overflow-wrap:normal;word-break:normal;hyphens:manual;line-height:1.12}
.family-osteria .item-name{font-weight:600;font-size:1.08em}
.family-osteria .item-line{justify-content:flex-start}
.family-osteria .price-leaders{display:block;flex:1 1 4mm;min-inline-size:4mm;block-size:.72em;border-block-end:.32mm dotted color-mix(in srgb,var(--muted) 65%,transparent)}`,
  "night-house": `
.night-house-cinematic-surface.block-masthead{border-block:.35mm solid var(--accent);padding-block:3mm;text-align:center}
.night-house-frame.block-category-heading{border:.3mm solid var(--accent);padding:2.2mm 3mm;text-align:center}
.night-house-wide-band.block-image{box-shadow:0 0 0 .4mm var(--accent)}
.family-night-house .item-line{border-block-end:.18mm solid color-mix(in srgb,var(--muted) 32%,transparent);padding-block-end:1.1mm}
.family-night-house .item-name{font-weight:600;font-size:1.16em;letter-spacing:.02em}
.family-night-house .item-price{color:var(--accent);font-weight:600}
.family-night-house .item-description{font-size:.84em}`,
  counter: `
.counter-modular-tile.block-masthead{border-block-start:1.6mm solid var(--accent);padding-block-start:3mm}
.family-counter .identity-lockup{align-items:flex-start;text-align:start}
.counter-filled-label.block-category-heading{display:flex;flex-direction:column;align-items:flex-start;justify-content:center}
.counter-dominant-special.block-image{border:.35mm solid var(--ink);padding:1mm;background:var(--ground)}
.family-counter .block-item-feature{border-block-start:.6mm solid var(--accent);padding-block-start:2mm}
.family-counter .item-name{font-family:var(--body-font);font-weight:700;font-size:1em}
.family-counter .item-price{font-weight:700}
.family-counter .item-description{font-size:.84em}`,
  street: `
.street-condensed-scan.block-masthead{text-transform:uppercase;border-block:1.1mm solid var(--ink);padding-block:2.5mm}
.family-street .identity-lockup{align-items:flex-start;text-align:start}
.format-takeaway-trifold .street-condensed-scan.block-masthead{background:var(--ink);color:var(--ground);border:0;outline:.7mm solid var(--accent);outline-offset:-4mm;padding:5mm;--muted:color-mix(in srgb,currentColor 75%,transparent)}
.format-takeaway-trifold .street-condensed-scan .cover-lockup{padding:4mm}
.format-takeaway-trifold.family-street .block-panel-note{border-block:.6mm solid var(--accent);color:var(--ink);text-transform:uppercase;letter-spacing:.04em}
.street-scan-bar.block-category-heading{padding:1.4mm 2.5mm;text-transform:uppercase}
.family-street .item-name{font-family:var(--body-font);font-weight:600;text-transform:uppercase;letter-spacing:.04em;font-size:.98em}
.family-street .item-price{font-family:var(--display-font);font-weight:600;font-size:1.05em}
.street-full-description .item-description,.family-street .item-description{max-width:92%;line-height:1.25;overflow:visible}`,
  field: `
.field-ingredient-spacing.block-masthead{padding-block-end:4mm}
.field-ingredient-spacing .identity-lockup:not(.cover-lockup)::after,.field-ingredient-spacing .cover-tier-title::after{content:"";inline-size:14mm;border-block-start:.3mm solid var(--accent);margin-block-start:2.5mm}
.field-narrow-measure .category-name{font-size:10.5pt;letter-spacing:.22em;text-transform:uppercase;font-style:normal}
.field-organic-restraint.block-image{border-radius:1.5mm}
.family-field .item-name{font-family:var(--body-font);font-weight:600;text-transform:uppercase;letter-spacing:.12em;font-size:.84em}
.family-field .item-price{color:var(--muted);font-weight:500;font-size:.9em}
.family-field .item-description{max-width:88%;letter-spacing:.015em}
.family-field .menu-item{padding-block:1mm}`,
  gallery: `
.family-gallery .identity-lockup{align-items:flex-start;text-align:start}
.gallery-negative-space.block-category-heading{padding-block-start:3mm}
.gallery-negative-space .category-name{letter-spacing:.01em}
.gallery-photo-field.block-image,.gallery-photo-field.block-cover{isolation:isolate}
.family-gallery .item-name{font-weight:600;font-size:1.12em}
.family-gallery .item-price{color:var(--muted);font-size:.9em}
.family-gallery .block-item-feature{margin-inline-start:8mm}`,
};

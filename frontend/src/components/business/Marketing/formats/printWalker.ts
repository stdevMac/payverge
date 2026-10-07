import { buildFontFaceCss } from "@/lib/menuPrint/fonts";
import { escapeHtml } from "@/lib/menuPrint/renderHtml";
import type { MotifId } from "../artDirection/kits";
import { cssFontFamily } from "../artDirection/typeface";
import type {
  LogoNode,
  MotifNode,
  PhotoNode,
  Scene,
  SceneNode,
  ScrimNode,
  ShapeNode,
  TextNode,
} from "../scene/types";
import type { FormatDef } from "./formats";
import {
  HAIRLINE_MM,
  bleedBoxMm,
  cropMarkRects,
  pageBoxMm,
  pxToMm,
  pxToPt,
  rectToMm,
  type MmRect,
} from "./printGeometry";

/**
 * The print walker: the second consumer of the Wave 1 `Scene` IR, alongside the
 * canvas walker in ../scene/renderScene.ts.
 *
 * It takes the SAME node list the canvas walker paints and emits an HTML
 * document with a fixed `@page`, so a table tent and a feed post are the same
 * art direction on two media rather than two designs that happen to look alike.
 *
 * Pure and DOM-free by construction, like `../scene/buildScene.ts` and
 * `@/lib/menuPrint/renderHtml.ts` before it: no `window`, no `document`, no
 * `"use client"`. The transport — an iframe and `window.print()` — belongs to the
 * hook in ../hooks/useCampaignKitExport.ts, exactly as `useQrSheetPrint` owns the
 * transport for `qrSheetHtml`.
 *
 * Two things are deliberately imported from `@/lib/menuPrint/` rather than
 * reimplemented:
 *
 *  - `buildFontFaceCss`, so a print font path fixed there reaches this walker too;
 *  - `escapeHtml`, the same escape `qrSheetHtml.ts` and `renderHtml.ts` use, so
 *    there is one answer in the repo to "what is safe to interpolate".
 *
 * What is NOT imported is `renderMenuPrintHtml`. That function's parameters are a
 * menu model, a menu template spec and menu options; it walks sections and items
 * and has no concept of a scene. There is nothing there to call.
 *
 * Two gotchas are copied on purpose rather than rediscovered, both already paid
 * for by Menu Print Studio:
 *
 *  1. `@page` needs an `@media screen` mirror. Page margins and page size do not
 *     apply on screen, so a preview without the mirror is a different document
 *     from the print.
 *  2. Print fonts want FULL builds. The DM faces this walker uses are Google
 *     CSS-API subsets, which strip `smcp`/`onum`/`tnum` feature tables. That is
 *     safe HERE, and only here, because the marketing scene IR never asks for an
 *     OpenType feature — no small caps, no oldstyle figures. `printWalker.test.ts`
 *     asserts the emitted CSS contains no `font-variant-*` at all, so the day a
 *     kit wants small caps the test fails and the face has to be swapped for a
 *     full build (EB Garamond is already in `public/fonts/print/`) rather than
 *     silently synthesizing them.
 */

export interface PrintWalkerOptions {
  /** Absolute origin for @font-face URLs, e.g. "https://pos.example.com" or "". */
  origin: string;
  /** Document title; shows in the print dialog and the saved-PDF filename. */
  title: string;
  /** BCP 47 tag for the `lang` attribute. */
  lang: string;
  /**
   * Optional public guest deep link (S2-D). When set with `qrDataUrl`, a small
   * QR corner is rendered on print formats. When only the URL is set, a short
   * text link is shown instead. Never pass private/signed asset URLs.
   */
  guestUrl?: string;
  /** Pre-rendered QR PNG/SVG data URL for guestUrl. Omit when URL unknown. */
  qrDataUrl?: string;
  /**
   * Optional promo / correlation code (S3-Reach). Shown under the QR when set.
   * Prefer short codes like PV-HH-A3F2 — never signed URLs or PII.
   */
  promoCode?: string;
}

/**
 * The two faces `artDirection/typeface.ts` resolves to. Both are registered in
 * `@/lib/menuPrint/fonts.ts` PRINT_FONT_FAMILIES, which is why this walker can
 * reuse that builder instead of hand-writing @font-face blocks.
 */
const PRINT_FAMILIES = ["DM Serif Display", "DM Sans"];

/** Crop-mark ink. Registration black would be better; this is what prints. */
/* eslint-disable-next-line no-restricted-syntax -- print CSS token, not Tailwind UI */
const MARK_INK = "#000000";

/** Round a millimetre length for CSS. Sub-micron precision is noise in a string. */
function mm(value: number): string {
  return `${Number(value.toFixed(4))}mm`;
}

/** Absolute-position declarations for a page-space rectangle. */
function mmRectStyle(rect: MmRect): string {
  return [
    `left:${mm(rect.leftMm)}`,
    `top:${mm(rect.topMm)}`,
    `width:${mm(rect.wMm)}`,
    `height:${mm(rect.hMm)}`,
  ].join(";");
}

function cropMarkHtml(format: FormatDef): string {
  const spec = format.print;
  if (!spec) return "";
  return cropMarkRects(spec)
    .map((rect) => `<div class="pv-mark" style="${mmRectStyle(rect)}"></div>`)
    .join("\n");
}

function buildCss(format: FormatDef, fontFaceCss: string, background: string): string {
  const spec = format.print!;
  const page = pageBoxMm(spec);
  const bleed = bleedBoxMm(spec);

  return `${fontFaceCss}

@page {
  size: ${mm(page.wMm)} ${mm(page.hMm)};
  margin: 0;
}

* { box-sizing: border-box; }

html, body {
  margin: 0;
  padding: 0;
  background: #ffffff;
  -webkit-print-color-adjust: exact;
  print-color-adjust: exact;
}

/* Preview parity: @page size and margins only apply when printing, so the page
   box is mirrored here for the on-screen iframe. Without it the preview is a
   viewport-width document and the print is a 143mm one — two different
   layouts, which is the trap menuPrint/renderHtml.ts already fell into once. */
@media screen {
  body {
    width: ${mm(page.wMm)};
    height: ${mm(page.hMm)};
    margin: 0 auto;
    box-shadow: 0 0 0 1px rgba(0,0,0,0.08);
  }
}

.pv-page {
  position: relative;
  width: ${mm(page.wMm)};
  height: ${mm(page.hMm)};
  overflow: hidden;
}

/* The artwork covers the BLEED box, not the trim box, so a cut that drifts by
   less than the bleed still lands on ink. */
.pv-art {
  position: absolute;
  left: ${mm(bleed.leftMm)};
  top: ${mm(bleed.topMm)};
  width: ${mm(bleed.wMm)};
  height: ${mm(bleed.hMm)};
  overflow: hidden;
  background: ${background};
}

.pv-node {
  position: absolute;
  margin: 0;
}

.pv-mark {
  position: absolute;
  background: ${MARK_INK};
}
`;
}

/**
 * Turn a `Scene` into a printable HTML document for a print format.
 *
 * Throws for a screen format rather than degrading: a caller that hands a 4:5
 * post to the print walker has a routing bug, and emitting a page-less document
 * would hide it behind a plausible-looking preview.
 */
export function renderScenePrintHtml(
  scene: Scene,
  format: FormatDef,
  options: PrintWalkerOptions,
): string {
  if (format.medium !== "print" || !format.print) {
    throw new Error("not_a_print_format");
  }

  const fontFaceCss = buildFontFaceCss(PRINT_FAMILIES, options.origin);
  const css = buildCss(format, fontFaceCss, scene.background);
  const nodesHtml = scene.nodes
    .map((node) => nodeHtml(node, format))
    .filter((html) => html.length > 0)
    .join("\n");
  const guestBadge = guestDeepLinkHtml(
    options.guestUrl,
    options.qrDataUrl,
    options.promoCode,
  );

  return `<!doctype html>
<html lang="${escapeHtml(options.lang || "en")}">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>${escapeHtml(options.title)}</title>
<style>
${css}
.pv-guest {
  position: absolute;
  right: 4mm;
  bottom: 4mm;
  z-index: 5;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1mm;
  max-width: 28mm;
  font-family: "DM Sans", system-ui, sans-serif;
  font-size: 6pt;
  line-height: 1.2;
  color: #1c1917;
  background: rgba(255,255,255,0.92);
  padding: 1.5mm;
  border-radius: 1mm;
}
.pv-guest img {
  width: 18mm;
  height: 18mm;
  display: block;
}
.pv-guest a {
  color: inherit;
  text-decoration: none;
  word-break: break-all;
  text-align: center;
}
.pv-guest .pv-promo {
  font-weight: 700;
  font-size: 7pt;
  letter-spacing: 0.04em;
  text-align: center;
}
</style>
</head>
<body>
<div class="pv-page">
<div class="pv-art">
${nodesHtml}
${guestBadge}
</div>
${cropMarkHtml(format)}
</div>
</body>
</html>`;
}

/**
 * Optional QR / deep-link badge for print formats. Pure HTML; empty when no
 * public guest URL is known (S2-D.6). Promo code is S3-Reach correlation only.
 */
export function guestDeepLinkHtml(
  guestUrl?: string,
  qrDataUrl?: string,
  promoCode?: string,
): string {
  const url = (guestUrl ?? "").trim();
  const code = (promoCode ?? "").trim().toUpperCase();
  // Allow promo-only badge when URL missing (rare — print packs without storefront).
  if (!url && !code) return "";
  if (url) {
    // Only http(s) absolute URLs — never javascript: or relative private paths.
    if (!/^https?:\/\//i.test(url)) return "";
    if (/[?&]X-Amz-/i.test(url)) return "";
  }

  const safeUrl = url ? escapeHtml(url) : "";
  const qr = (qrDataUrl ?? "").trim();
  // data:image only — refuse remote scripts masquerading as images.
  const qrImg =
    url && qr.startsWith("data:image/")
      ? `<img alt="" src="${escapeHtml(qr)}" width="72" height="72"/>`
      : "";
  const promoHtml = code
    ? `<span class="pv-promo">${escapeHtml(code)}</span>`
    : "";
  const linkHtml = safeUrl
    ? `<a href="${safeUrl}">${safeUrl}</a>`
    : "";
  return `<div class="pv-guest">${qrImg}${promoHtml}${linkHtml}</div>`;
}

/**
 * Where a photo's focal point lands, as a CSS `object-position`.
 *
 * The canvas walker crops the SOURCE with `fitCover` and draws the result; CSS
 * cannot express a source crop, so `object-fit: cover` plus `object-position`
 * expresses the same intent declaratively. The two agree on the common case (a
 * focal point, cover-fitted) and diverge on `crop.zoom`, which has no CSS
 * equivalent short of a transform that would then need its own overflow box.
 * Zoom is therefore dropped rather than approximated — a tent that is slightly
 * wider than the operator's zoom asked for is a far smaller error than a photo
 * scaled out of its frame.
 */
function objectPosition(node: PhotoNode): string {
  const x = Math.min(100, Math.max(0, (node.crop?.x ?? 0.5) * 100));
  const y = Math.min(100, Math.max(0, (node.crop?.y ?? 0.5) * 100));
  return `${Number(x.toFixed(4))}% ${Number(y.toFixed(4))}%`;
}

function photoHtml(node: PhotoNode, format: FormatDef): string {
  // Photo sits in an absolutely positioned box; the img fills it with cover.
  // White balance is a second layer with mix-blend-mode:multiply — the same
  // per-channel scale the canvas walker paints after drawImage (renderScene
  // PAINTERS.photo). A filter-only path would drop grey-world WB that Wave 2
  // stills and motion both apply.
  const box = [
    mmRectStyle(rectToMm(node.rect, format)),
    "overflow:hidden",
  ].join(";");
  const imgStyle = [
    "position:absolute",
    "left:0",
    "top:0",
    "width:100%",
    "height:100%",
    "object-fit:cover",
    `object-position:${objectPosition(node)}`,
    // Kit grade (contrast/saturate/brightness) via CSS filter.
    node.filter ? `filter:${node.filter}` : "",
  ]
    .filter(Boolean)
    .join(";");
  const wb = node.whiteBalance
    ? `<div aria-hidden="true" style="position:absolute;left:0;top:0;width:100%;height:100%;background:${escapeHtml(node.whiteBalance)};mix-blend-mode:multiply"></div>`
    : "";
  return `<div class="pv-node" style="${box}"><img alt="" src="${escapeHtml(node.url)}" style="${imgStyle}"/>${wb}</div>`;
}

function scrimHtml(node: ScrimNode, format: FormatDef): string {
  // `adaptive` is deliberately ignored. It means "measure what is already
  // painted underneath and strengthen the scrim accordingly", which is a raster
  // readback; a print walker has no bitmap to read. `ScrimNode.adaptive`'s own
  // documentation names this case and says a non-raster walker must paint
  // `from`->`to` as authored, which is what happens here.
  // Prefer `boost` when present — adaptive already resolved upstream.
  const from = node.boost?.from ?? node.from;
  const to = node.boost?.to ?? node.to;
  const style = [
    mmRectStyle(rectToMm(node.rect, format)),
    `background:linear-gradient(to bottom, ${from}, ${to})`,
  ].join(";");
  return `<div class="pv-node" style="${style}"></div>`;
}

function textHtml(node: TextNode, format: FormatDef): string {
  const spec = format.print!;
  const fontPx = node.sizePct * format.px.w;
  const style = [
    mmRectStyle(rectToMm(node.rect, format)),
    `font-family:${cssFontFamily(node.font)}`,
    // DM Serif Display ships one weight and synthetic bold looks wrong, so the
    // serif face is pinned to 400 here exactly as `cssFontString` pins it for
    // the canvas walker.
    `font-weight:${node.font === "serif" ? 400 : node.weight}`,
    `font-size:${Number(pxToPt(fontPx, spec.dpi).toFixed(4))}pt`,
    `line-height:${node.lineHeight}`,
    `color:${node.color}`,
    `text-align:${node.align}`,
    node.uppercase ? "text-transform:uppercase" : "",
    node.tracking ? `letter-spacing:${node.tracking}em` : "",
    // The canvas walker wraps with `wrapText` and clips by line count. CSS does
    // the same job declaratively; `-webkit-line-clamp` is the only cross-engine
    // way to cap lines and is supported unprefixed in every print target here.
    "display:-webkit-box",
    "-webkit-box-orient:vertical",
    `-webkit-line-clamp:${Math.max(1, node.maxLines)}`,
    "overflow:hidden",
  ]
    .filter(Boolean)
    .join(";");

  const pill = node.pill
    ? `background:${node.pill.fill};border-radius:999px;padding:${mm(
        node.pill.padY * format.px.w * (25.4 / spec.dpi),
      )} ${mm(node.pill.padX * format.px.w * (25.4 / spec.dpi))};`
    : "";

  return `<p class="pv-node" style="${style};${pill}">${escapeHtml(node.text)}</p>`;
}

/**
 * Motifs this walker can express in CSS, and motifs it deliberately cannot.
 *
 * The canvas drawers in ../scene/motifs.ts are imperative 2D paint routines;
 * there is no way to call them from a string builder. Two of the seven are
 * simple enough to restate as boxes:
 *
 *  - `rule` is a hairline above the stack;
 *  - `cornerBrackets` is four L-shaped borders at the envelope's corners.
 *
 * The other five are dropped, each for a reason:
 *
 *  - `seal` and `ticketNotch` are arc geometry, and a CSS approximation would
 *    be a different mark rather than the same one at lower fidelity;
 *  - `halftone` is an 8.6px disc on a 32.4px pitch — as a CSS repeating gradient
 *    it moirés against a 300 dpi raster grid, which is worse than absent;
 *  - `grain` is per-pixel noise with no CSS expression at all;
 *  - `tape` is a rotated translucent strip whose whole effect is the canvas
 *    drawer's specific alpha and rotation.
 *
 * Dropping is the same contract `paintNode` states in ../scene/renderScene.ts: a
 * walker that cannot express a node skips it rather than approximating it. The
 * parity test in printWalker.test.ts holds these two lists to `MOTIF_DRAWERS`, so
 * a motif added later has to be classified rather than silently forgotten.
 */
export const PRINTABLE_MOTIFS: readonly MotifId[] = ["rule", "cornerBrackets"];

export const DROPPED_MOTIFS: readonly MotifId[] = [
  "seal",
  "ticketNotch",
  "halftone",
  "grain",
  "tape",
];

/** Hairline for the printed rule, in the same 0.25pt convention as the marks. */
const RULE_THICKNESS_MM = HAIRLINE_MM * 2;

/** How long each bracket arm is, as a fraction of the envelope's shorter side. */
const BRACKET_ARM_FRACTION = 0.12;

function shapeHtml(node: ShapeNode, format: FormatDef): string {
  const spec = format.print!;
  const style = [
    mmRectStyle(rectToMm(node.rect, format)),
    `background:${node.fill}`,
    // Radius is a fraction of canvas WIDTH on both axes, matching
    // `ShapeNode.radiusPct` — a height-relative radius would round the same node
    // differently on a tent and a feed post.
    node.radiusPct
      ? `border-radius:${mm(pxToMm(node.radiusPct * format.px.w, spec.dpi))}`
      : "",
  ]
    .filter(Boolean)
    .join(";");
  return `<div class="pv-node" style="${style}"></div>`;
}

function logoHtml(node: LogoNode, format: FormatDef): string {
  const spec = format.print!;
  // LogoRect has no h — only an origin and a WIDTH-relative diameter. Map origin
  // through rectToMm with a synthetic zero height for left/top only.
  const box = rectToMm(
    { x: node.rect.x, y: node.rect.y, w: node.rect.w, h: 0 },
    format,
  );
  // Square, sized off canvas WIDTH. Matches how the canvas walker calls
  // `drawLogo` with `size: node.rect.w`.
  const sideMm = pxToMm(node.rect.w * format.px.w, spec.dpi);
  const style = [
    `left:${mm(box.leftMm)}`,
    `top:${mm(box.topMm)}`,
    `width:${mm(sideMm)}`,
    `height:${mm(sideMm)}`,
    "object-fit:cover",
    "border-radius:50%",
    // The canvas walker paints a translucent white ring behind the disc; a
    // box-shadow of the same colour is the CSS equivalent and prints identically.
    `box-shadow:0 0 0 ${mm(pxToMm(format.px.w * 0.008, spec.dpi))} rgba(255,255,255,0.94)`,
  ].join(";");
  return `<img class="pv-node" alt="" src="${escapeHtml(node.url)}" style="${style}"/>`;
}

function motifHtml(node: MotifNode, format: FormatDef): string {
  if (!PRINTABLE_MOTIFS.includes(node.motif)) return "";
  const box = rectToMm(node.rect, format);
  const alpha = node.opacity ?? 1;

  if (node.motif === "rule") {
    const style = [
      `left:${mm(box.leftMm)}`,
      `top:${mm(box.topMm)}`,
      `width:${mm(box.wMm)}`,
      `height:${mm(RULE_THICKNESS_MM)}`,
      `background:${node.color}`,
      `opacity:${alpha}`,
    ].join(";");
    return `<div class="pv-node pv-motif pv-motif-rule" style="${style}"></div>`;
  }

  const armMm = Math.min(box.wMm, box.hMm) * BRACKET_ARM_FRACTION;
  const corners: Array<[string, string]> = [
    [`left:${mm(box.leftMm)};top:${mm(box.topMm)}`, "border-left;border-top"],
    [
      `left:${mm(box.leftMm + box.wMm - armMm)};top:${mm(box.topMm)}`,
      "border-right;border-top",
    ],
    [
      `left:${mm(box.leftMm)};top:${mm(box.topMm + box.hMm - armMm)}`,
      "border-left;border-bottom",
    ],
    [
      `left:${mm(box.leftMm + box.wMm - armMm)};top:${mm(
        box.topMm + box.hMm - armMm,
      )}`,
      "border-right;border-bottom",
    ],
  ];
  return corners
    .map(([position, edges]) => {
      const borders = edges
        .split(";")
        .map((edge) => `${edge}:${mm(RULE_THICKNESS_MM)} solid ${node.color}`)
        .join(";");
      const style = [
        position,
        `width:${mm(armMm)}`,
        `height:${mm(armMm)}`,
        borders,
        `opacity:${alpha}`,
      ].join(";");
      return `<div class="pv-node pv-motif pv-motif-bracket" style="${style}"></div>`;
    })
    .join("\n");
}

/**
 * One node, as absolutely-positioned HTML. A kind this walker does not know is
 * skipped rather than thrown on, matching `paintNode`'s contract in
 * ../scene/renderScene.ts: the IR is shared, and one walker gaining a kind must
 * not break the others.
 */
function nodeHtml(node: SceneNode, format: FormatDef): string {
  switch (node.kind) {
    case "photo":
      return photoHtml(node, format);
    case "scrim":
      return scrimHtml(node, format);
    case "text":
      return textHtml(node, format);
    case "shape":
      return shapeHtml(node, format);
    case "motif":
      return motifHtml(node, format);
    case "logo":
      return logoHtml(node, format);
    default:
      return "";
  }
}

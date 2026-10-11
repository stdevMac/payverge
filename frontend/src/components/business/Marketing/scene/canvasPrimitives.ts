import type { MarketingCrop } from "@/api/marketing";
import type { NormalizedRect, ScrimTreatment } from "../templates/types";
import { perceptualLuma } from "../photo/luma";
import { adaptiveScrimBoost } from "./scrim";

/**
 * Canvas drawing primitives shared by the scene walker and the post renderer.
 *
 * This module is a LEAF on purpose, for the same reason `./geometry.ts` and
 * `../artDirection/typeface.ts` are leaves. `renderScene` paints with these
 * helpers and `renderPost` both re-exports them for callers and owns image
 * loading / scene assembly on top. If the helpers live in `renderPost`, the
 * walker has to import the post entry and the post entry has to import the
 * walker — a real cycle madge reports as
 * `templates/renderPost.ts > scene/renderScene.ts`.
 *
 * That is why nothing here may import from `./renderScene`, `./buildScene`,
 * `./motifs`, or `../templates/renderPost`. Allowed dependencies are only
 * importless type homes (`../templates/types`, `@/api/marketing` crop shape)
 * and other leaves (`../photo/luma`). Re-implementing a painter in either
 * caller to dodge the cycle is forbidden: scrim, logo, source-crop,
 * corner-radius and line-breaking behaviour must not drift between preview,
 * export and the walker.
 *
 * Pure canvas helpers: no scene IR, no kit, no composition.
 */

export const DEFAULT_CROP: MarketingCrop = { x: 0.5, y: 0.5, zoom: 1 };

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

/**
 * Pure source crop rectangle for cover-fit plus focal point and zoom.
 * Focal x/y are normalized 0..1 and zoom is bounded to 1..3.
 *
 * ## Aspect fidelity (S1-Photo)
 *
 * Cover-fit **never squashes** the source: it scales uniformly until the
 * destination rectangle is filled, then crops the overflow. Portrait phone
 * photos (e.g. 9:16) into a 4:5 feed frame crop top/bottom; landscape menu
 * shots (e.g. 3:2 or 16:9) into 4:5 crop the sides. Extreme ratios (panorama
 * strip, ultra-tall) keep more of the source discarded — readiness flags
 * `extremeAspect` so the operator can re-shoot or generate a native hero.
 *
 * AI generate/cleanup only produce 1:1 | 4:5 | 9:16 heroes (`heroGenAspectFor`).
 * Campaign formats wide / strip / 5:7 intentionally re-crop from that hero
 * through this same path — they are not separate generation jobs.
 */
export function fitCover(
  srcW: number,
  srcH: number,
  dstW: number,
  dstH: number,
  crop: MarketingCrop = DEFAULT_CROP,
): { sx: number; sy: number; sw: number; sh: number } {
  const srcRatio = srcW / srcH;
  const dstRatio = dstW / dstH;
  let coverW: number;
  let coverH: number;
  if (srcRatio > dstRatio) {
    coverH = srcH;
    coverW = srcH * dstRatio;
  } else {
    coverW = srcW;
    coverH = srcW / dstRatio;
  }

  const zoom = clamp(Number.isFinite(crop.zoom) ? crop.zoom : 1, 1, 3);
  const sw = coverW / zoom;
  const sh = coverH / zoom;
  const focalX = clamp(Number.isFinite(crop.x) ? crop.x : 0.5, 0, 1);
  const focalY = clamp(Number.isFinite(crop.y) ? crop.y : 0.5, 0, 1);
  const sx = clamp(focalX * srcW - sw / 2, 0, srcW - sw);
  const sy = clamp(focalY * srcH - sh / 2, 0, srcH - sh);

  return {
    sx: Math.round(sx),
    sy: Math.round(sy),
    sw: Math.round(sw),
    sh: Math.round(sh),
  };
}

function splitGraphemes(value: string): string[] {
  if (typeof Intl.Segmenter === "function") {
    return Array.from(
      new Intl.Segmenter(undefined, { granularity: "grapheme" }).segment(value),
      ({ segment }) => segment,
    );
  }

  const codePoints = Array.from(value);
  const graphemes: string[] = [];
  let current = "";
  let regionalIndicators = 0;
  const isRegionalIndicator = (character: string) =>
    /^\p{Regional_Indicator}$/u.test(character);
  const isExtension = (character: string) =>
    /^\p{Mark}$/u.test(character) ||
    /^[\uFE0E\uFE0F]$/u.test(character) ||
    /^[\u{1F3FB}-\u{1F3FF}]$/u.test(character);

  codePoints.forEach((character) => {
    if (!current) {
      current = character;
      regionalIndicators = isRegionalIndicator(character) ? 1 : 0;
      return;
    }

    if (
      character === "\u200D" ||
      current.endsWith("\u200D") ||
      isExtension(character) ||
      (isRegionalIndicator(character) && regionalIndicators === 1)
    ) {
      current += character;
      regionalIndicators = isRegionalIndicator(character)
        ? (regionalIndicators + 1) % 2
        : 0;
      return;
    }

    graphemes.push(current);
    current = character;
    regionalIndicators = isRegionalIndicator(character) ? 1 : 0;
  });

  if (current) graphemes.push(current);
  return graphemes;
}

/** Greedy localized wrap with grapheme fallback and deterministic ellipsis. */
export function wrapText(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxWidth: number,
  maxLines: number,
): string[] {
  if (maxLines <= 0 || maxWidth <= 0) return [];
  const graphemes = splitGraphemes(text.trim().replace(/\s+/g, " "));
  const lines: string[] = [];
  let start = 0;

  while (start < graphemes.length && lines.length < maxLines) {
    let fitEnd = start;
    while (fitEnd < graphemes.length) {
      const probe = graphemes.slice(start, fitEnd + 1).join("");
      if (ctx.measureText(probe).width > maxWidth) break;
      fitEnd += 1;
    }

    if (fitEnd === start) {
      if (ctx.measureText("…").width <= maxWidth) lines.push("…");
      break;
    }

    if (fitEnd === graphemes.length) {
      lines.push(graphemes.slice(start).join("").trimEnd());
      break;
    }

    let breakEnd = fitEnd;
    for (let index = fitEnd - 1; index > start; index -= 1) {
      if (/^\s+$/u.test(graphemes[index])) {
        breakEnd = index;
        break;
      }
    }

    let line = graphemes.slice(start, breakEnd).join("").trimEnd();
    if (lines.length === maxLines - 1) {
      let lineGraphemes = splitGraphemes(line);
      while (
        lineGraphemes.length > 0 &&
        ctx.measureText(`${lineGraphemes.join("")}…`).width > maxWidth
      ) {
        lineGraphemes = lineGraphemes.slice(0, -1);
      }
      line = `${lineGraphemes.join("").trimEnd()}…`;
    }
    if (line) lines.push(line);

    start = breakEnd;
    while (start < graphemes.length && /^\s+$/u.test(graphemes[start])) {
      start += 1;
    }
  }

  return lines;
}

export function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  radius: number,
) {
  const resolvedRadius = Math.min(radius, width / 2, height / 2);
  ctx.beginPath();
  ctx.moveTo(x + resolvedRadius, y);
  ctx.arcTo(x + width, y, x + width, y + height, resolvedRadius);
  ctx.arcTo(x + width, y + height, x, y + height, resolvedRadius);
  ctx.arcTo(x, y + height, x, y, resolvedRadius);
  ctx.arcTo(x, y, x + width, y, resolvedRadius);
  ctx.closePath();
}

export function sampleRegionLuminance(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  region: NormalizedRect,
): number {
  const x = Math.floor(region.x * width);
  const y = Math.floor(region.y * height);
  const sampleW = Math.max(1, Math.floor(region.w * width));
  const sampleH = Math.max(1, Math.floor(region.h * height));
  let data: ImageData;
  try {
    data = ctx.getImageData(x, y, sampleW, sampleH);
  } catch {
    return 128;
  }
  let sum = 0;
  const pixels = data.data.length / 4;
  for (let index = 0; index < data.data.length; index += 4) {
    sum += perceptualLuma(
      data.data[index],
      data.data[index + 1],
      data.data[index + 2],
    );
  }
  return pixels ? sum / pixels : 128;
}

/** Luminance of the bottom 35% band, where captions and CTAs sit. */
export function sampleBottomLuminance(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
): number {
  return sampleRegionLuminance(ctx, width, height, {
    x: 0,
    y: 0.65,
    w: 1,
    h: 0.35,
  });
}

function paintScrim(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  treatment: ScrimTreatment,
  from: string,
  to: string,
): void {
  const region = treatment.region;
  const x = region.x * width;
  const y = region.y * height;
  const regionH = region.h * height;
  const gradient = ctx.createLinearGradient(x, y, x, y + regionH);
  gradient.addColorStop(0, from);
  gradient.addColorStop(1, to);
  ctx.fillStyle = gradient;
  ctx.fillRect(x, y, region.w * width, regionH);
}

export function applyScrim(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  treatment: ScrimTreatment,
  luminance: number | null,
): void {
  paintScrim(ctx, width, height, treatment, treatment.from, treatment.to);
  if (!treatment.adaptive || luminance == null) return;
  const boost = adaptiveScrimBoost(luminance, treatment.luminanceThreshold);
  if (!boost) return;
  paintScrim(ctx, width, height, treatment, boost.from, boost.to);
}
export function drawLogo(
  ctx: CanvasRenderingContext2D,
  image: CanvasImageSource | null | undefined,
  width: number,
  height: number,
  anchor: { x: number; y: number; size: number },
): void {
  if (!image) return;
  const size = Math.round(anchor.size * width);
  const x = anchor.x * width;
  const y = anchor.y * height;
  const centerX = x + size / 2;
  const centerY = y + size / 2;
  const radius = size / 2 + Math.round(width * 0.008);
  ctx.save();
  ctx.beginPath();
  ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
  ctx.fillStyle = "rgba(255,255,255,0.94)";
  ctx.fill();
  ctx.beginPath();
  ctx.arc(centerX, centerY, size / 2, 0, Math.PI * 2);
  ctx.clip();
  ctx.drawImage(image, x, y, size, size);
  ctx.restore();
}

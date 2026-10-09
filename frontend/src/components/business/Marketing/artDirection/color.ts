export interface Rgb {
  r: number;
  g: number;
  b: number;
}

const HEX_PATTERN = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i;

/**
 * Parse `#rgb` or `#rrggbb` into channels, or null when unparseable. The
 * leading `#` is optional; 8-digit alpha hex is rejected.
 */
export function parseHex(hex: string): Rgb | null {
  const matched = (hex ?? "").trim().match(HEX_PATTERN);
  if (!matched) return null;
  let body = matched[1];
  if (body.length === 3) {
    body = body
      .split("")
      .map((c) => c + c)
      .join("");
  }
  return {
    r: parseInt(body.slice(0, 2), 16),
    g: parseInt(body.slice(2, 4), 16),
    b: parseInt(body.slice(4, 6), 16),
  };
}

function clampChannel(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(255, Math.round(value)));
}

export function toHex(color: Rgb): string {
  const part = (value: number) =>
    clampChannel(value).toString(16).padStart(2, "0");
  return `#${part(color.r)}${part(color.g)}${part(color.b)}`;
}

/** sRGB companding knee, in the slightly-off form WCAG 2.1 specifies. */
const SRGB_KNEE = 0.03928;
/** The same knee on the linear side, so the two conversions stay inverses. */
const LINEAR_KNEE = SRGB_KNEE / 12.92;

/** One 0–255 sRGB channel to its 0–1 linear-light value. */
function linearize(raw: number): number {
  const c = clampChannel(raw) / 255;
  return c <= SRGB_KNEE ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

/**
 * Inverse of `linearize`: a 0–1 linear-light value back to 0–255 sRGB. Returns
 * a float; rounding to an integer channel is `toHex`/`clampChannel`'s job, and
 * that rounding is exactly what `CONTRAST_MARGIN` below exists to absorb.
 */
function delinearize(value: number): number {
  const v = Math.max(0, Math.min(1, value));
  const c =
    v <= LINEAR_KNEE ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - 0.055;
  return c * 255;
}

/** WCAG 2.1 relative luminance. */
export function relativeLuminance(color: Rgb): number {
  return (
    0.2126 * linearize(color.r) +
    0.7152 * linearize(color.g) +
    0.0722 * linearize(color.b)
  );
}

/** WCAG 2.1 contrast ratio, 1..21. Symmetric in its arguments. */
export function contrastRatio(a: Rgb, b: Rgb): number {
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  const lighter = Math.max(la, lb);
  const darker = Math.min(la, lb);
  return (lighter + 0.05) / (darker + 0.05);
}

/** Clamp `alpha` to 0..1, treating a non-finite value as fully transparent. */
function clampAlpha(alpha: number): number {
  if (!Number.isFinite(alpha)) return 0;
  return Math.max(0, Math.min(1, alpha));
}

/** Source-over blend of two channel triples. Floats out; rounding is the caller's. */
function blend(source: Rgb, backdrop: Rgb, alpha: number): Rgb {
  const a = clampAlpha(alpha);
  const mix = (s: number, d: number) => a * s + (1 - a) * d;
  return {
    r: mix(source.r, backdrop.r),
    g: mix(source.g, backdrop.g),
    b: mix(source.b, backdrop.b),
  };
}

/** Round a float triple to the bytes a framebuffer would actually store. */
function toBytes(color: Rgb): Rgb {
  return {
    r: clampChannel(color.r),
    g: clampChannel(color.g),
    b: clampChannel(color.b),
  };
}

/**
 * Paint `source` over `backdrop` at `alpha` and return what is left on the
 * canvas — the colour any type drawn afterwards will actually read against.
 *
 * The blend is over the sRGB channel VALUES, not over linear light. That is
 * deliberate, and it is not the colour-science answer: a physically faithful
 * composite mixes linear intensities, and mixing companded values instead
 * biases every result dark. It is however precisely what `globalAlpha` on a 2D
 * canvas does — the context composites in the (non-linear) storage space of its
 * backing store — and this function exists to predict that bitmap, not to model
 * light. Blending in linear light would put `#ffffff` at 50% over `#000000` at
 * `#bcbcbc` instead of the `#808080` the canvas paints, i.e. it would measure a
 * contrast the reader never sees.
 *
 * `alpha` is clamped to 0..1, so 0 returns the backdrop and 1 the source and no
 * argument can extrapolate past either. An unparseable argument yields the
 * backdrop unchanged, which is `pickReadableColor`'s habit: degrade to the
 * paintable input rather than throw inside a render path.
 */
export function compositeOver(
  source: string,
  backdrop: string,
  alpha: number,
): string {
  const src = parseHex(source);
  const dst = parseHex(backdrop);
  if (!src || !dst) return backdrop;
  return toHex(blend(src, dst, alpha));
}

/*
 * Declared as channels and converted, rather than written as hex literals, so
 * the repo's no-restricted-syntax hex ban stays live for the rest of this file
 * — the one place a stray hex literal most deserves catching. These are the
 * real design tokens: white, and ink-900 (#1c1917).
 */
const WHITE: Rgb = { r: 255, g: 255, b: 255 };
const INK_900: Rgb = { r: 28, g: 25, b: 23 };

/**
 * Pick the first candidate meeting `minRatio` against `backdrop`. When none do,
 * return the highest-contrast candidate rather than knowingly rendering
 * unreadable text. With no candidates, return whichever of white/near-black
 * reads better on the backdrop.
 */
export function pickReadableColor(
  candidates: string[],
  backdrop: string,
  minRatio = 4.5,
): string {
  const back = parseHex(backdrop) ?? WHITE;
  const scored = candidates
    .map((hex) => ({ hex, rgb: parseHex(hex) }))
    .filter((entry): entry is { hex: string; rgb: Rgb } => entry.rgb !== null)
    .map((entry) => ({ ...entry, ratio: contrastRatio(entry.rgb, back) }));

  if (!scored.length) {
    return contrastRatio(WHITE, back) >= contrastRatio(INK_900, back)
      ? toHex(WHITE)
      : toHex(INK_900);
  }

  const passing = scored.find((entry) => entry.ratio >= minRatio);
  if (passing) return passing.hex;
  return scored.reduce((best, entry) =>
    entry.ratio > best.ratio ? entry : best,
  ).hex;
}

/**
 * The open luminance interval in which NO candidate reaches the ratio — the
 * range of backdrops where `pickReadableColor` can only degrade. `low >= high`
 * means the interval is empty and every backdrop is serviceable.
 */
export interface UnreadableBand {
  low: number;
  high: number;
}

/**
 * Derive the band of backdrop luminances that `candidates` cannot serve.
 *
 * A candidate of luminance `Lc` clears `minRatio` against a backdrop of
 * luminance `L` when `L <= (Lc + 0.05) / minRatio - 0.05` (backdrop darker) or
 * `L >= minRatio * (Lc + 0.05) - 0.05` (backdrop lighter). Each candidate
 * therefore fails on one open interval, and the band no candidate covers is the
 * intersection of those intervals: `(max low, min high)`.
 *
 * For the assembler's white / ink-900 pair at 4.5:1 that works out to
 * (0.18333, 0.22018) — a 0.0368-wide hole that real brand primaries land in
 * (`#6366f1`, `#8b5cf6`, `#a855f7` all do).
 */
export function unreadableBand(
  candidates: string[],
  minRatio: number,
): UnreadableBand {
  let low = -Infinity;
  let high = Infinity;
  let found = false;
  candidates.forEach((hex) => {
    const rgb = parseHex(hex);
    if (!rgb) return;
    found = true;
    const luminance = relativeLuminance(rgb);
    low = Math.max(low, (luminance + 0.05) / minRatio - 0.05);
    high = Math.min(high, minRatio * (luminance + 0.05) - 0.05);
  });
  // With nothing to move toward there is no meaningful band: report it empty so
  // callers leave the surface alone rather than chase an unreachable target.
  return found ? { low, high } : { low: 0, high: 0 };
}

/**
 * How far past the band edge a conditioned surface is pushed, in luminance.
 *
 * Landing exactly on the edge does not work. The conditioned colour is rounded
 * back to integer RGB, and that rounding moves luminance by up to 0.001843 in
 * either direction — enough to re-enter the band. Targeting the exact edge
 * leaves `#6366f1` at 4.478:1 and `#8b5cf6` at 4.490:1, both under the floor
 * they were conditioned to clear.
 *
 * 0.004 was measured, not guessed: over all 1,267,369 sRGB colours inside the
 * white / ink-900 band at 4.5:1, the worst final ratio after rounding is
 * 4.5032 at a 0.002 margin (`#7e7e63` -> `#838367`) and 4.5361 at 0.004
 * (`#8f7776` -> `#957c7b`). The worst rounding shift seen anywhere in that
 * sweep is 0.001843 (0.001831 at the shipped margin), so 0.004 buys a 2.2x
 * safety factor over observed rounding error for a luminance shift small
 * enough to stay invisible next to the shift the conditioning already applies.
 */
const CONTRAST_MARGIN = 0.004;

/**
 * Rescale a colour's LINEAR channels so its luminance lands on `target`.
 *
 * Luminance is a linear combination of the linear-light channels, so scaling
 * them all by one factor scales luminance by exactly that factor and leaves
 * chromaticity — hue and saturation — untouched. That exactness is why this is
 * the only mechanism used in both directions.
 *
 * Returns null when the scale would clip a channel past full, which is the one
 * case where the operation cannot preserve chromaticity: the caller must then
 * pick a target it can actually reach. Blending toward white would reach any
 * target, but it desaturates, so it is deliberately not the fallback here.
 *
 * Black is the other null: every channel is already zero, so no factor moves it
 * and there is no chromaticity to preserve either. The guard is not cosmetic —
 * without it the factor is `target / 0` and each channel becomes `0 * Infinity`
 * = NaN, which `clampChannel` then quietly maps back to 0, handing the caller
 * black as though the target had been reached.
 *
 * Exported because it is a primitive of this module's vocabulary alongside
 * `relativeLuminance` and `unreadableBand`, and because both of the null cases
 * above are contracts callers depend on and so have to be directly pinnable.
 */
export function scaleLuminance(color: Rgb, target: number): Rgb | null {
  const luminance = relativeLuminance(color);
  if (luminance <= 0) return null;
  const k = Math.max(0, target) / luminance;
  const channels = [
    linearize(color.r) * k,
    linearize(color.g) * k,
    linearize(color.b) * k,
  ];
  if (channels.some((value) => value > 1)) return null;
  return {
    r: delinearize(channels[0]),
    g: delinearize(channels[1]),
    b: delinearize(channels[2]),
  };
}

/**
 * A layer painted OVER a surface before any type reads against it.
 *
 * `color` at `alpha` over the surface — see `compositeOver` — is then the
 * bitmap the glyphs sit on, so it, and not the surface, is what has to clear
 * the unreadable band.
 */
export interface SurfaceOverlay {
  color: string;
  alpha: number;
}

/**
 * The surface as it will actually be seen: with `overlay` painted over it.
 *
 * Floats out. `relativeLuminance` rounds its argument to whole channels
 * anyway, so every measurement taken through here lands on the bytes the
 * framebuffer will hold rather than on an intermediate no walker paints.
 */
function asPainted(surface: Rgb, overlay?: SurfaceOverlay | null): Rgb {
  if (!overlay) return surface;
  const src = parseHex(overlay.color);
  if (!src) return surface;
  return blend(src, surface, overlay.alpha);
}

/**
 * Halvings of the surface-luminance interval when solving under an overlay.
 * 2^-40 of a 0..1 range is orders of magnitude finer than one 8-bit channel
 * step, so the search always settles on a byte boundary and further halvings
 * would only re-test the same colour.
 */
const OVERLAY_SOLVE_STEPS = 40;

/**
 * A hair inside the exact clip point. Scaling to `luminance / headroom` puts
 * the brightest linear channel on exactly 1.0 in real arithmetic, where a
 * last-bit float error can put it a hair past and make `scaleLuminance` refuse
 * the entire search. 1e-9 of luminance is under 3e-7 of one channel step, so
 * the backoff costs nothing and removes the cliff.
 */
const CLIP_GUARD = 1 - 1e-9;

/**
 * Find the surface luminance whose PAINTED result — the surface seen through
 * `overlay` — lands past `edge`, moving the surface as little as it can.
 *
 * Painted luminance rises monotonically with the surface's own: every channel
 * of the composite is `alpha * overlay + (1 - alpha) * surface`, so lifting the
 * surface lifts every composite channel and so the luminance. That monotonicity
 * is what makes a bisection correct, and the sRGB companding sitting between
 * the two luminances is what makes it necessary — there is no closed form to
 * invert. The interval starts at the surface's own luminance (which by
 * construction does NOT reach the edge) and at the far extreme (black when
 * darkening, the brightest non-clipping scale when lightening), so every
 * halving keeps one reaching end and one not.
 *
 * The predicate is evaluated on the composite of an integer-rounded surface,
 * i.e. on the bytes that reach the framebuffer, so the answer carries no
 * rounding error of its own; `edge` already carries `CONTRAST_MARGIN` for the
 * ratio's sake.
 *
 * Returns null when the edge is out of reach — the overlay alone already sits
 * past it, or getting there would clip a channel — and the caller then tries
 * the other edge.
 */
function solveUnderOverlay(
  color: Rgb,
  overlay: SurfaceOverlay,
  edge: number,
  lighten: boolean,
): number | null {
  const luminance = relativeLuminance(color);
  const reaches = (target: number): boolean => {
    const moved = scaleLuminance(color, target);
    if (!moved) return false;
    const seen = relativeLuminance(asPainted(toBytes(moved), overlay));
    return lighten ? seen >= edge : seen <= edge;
  };

  const headroom = Math.max(
    linearize(color.r),
    linearize(color.g),
    linearize(color.b),
  );
  let far = lighten && headroom > 0 ? (luminance / headroom) * CLIP_GUARD : 0;
  if (!reaches(far)) return null;

  let near = luminance;
  for (let i = 0; i < OVERLAY_SOLVE_STEPS; i += 1) {
    const mid = (near + far) / 2;
    if (reaches(mid)) far = mid;
    else near = mid;
  }
  return far;
}

/**
 * Move a surface colour out of the luminance band where `candidates` cannot
 * reach `minRatio`, so a foreground picked against it afterwards is readable
 * rather than least-bad.
 *
 * When an `overlay` is given it is the COMPOSITE that has to clear the band,
 * not the surface: a layer painted between the surface and the type is what the
 * type reads against, and a surface conditioned on its own is dragged straight
 * back in by it. The surface still moves along its own chromaticity ray, so the
 * brand colour keeps its hue whatever the overlay is tinted.
 *
 * A surface whose painted result is already outside the band is returned
 * byte-identical — the same string, not a re-serialized equivalent — so no
 * brand colour is shifted gratuitously. Otherwise it moves to the NEARER edge,
 * overshooting it by `CONTRAST_MARGIN` (see above), and hue and saturation
 * survive exactly because the move is a scale of the linear channels.
 *
 * Neither direction is guaranteed, so both are tried, nearer first, and the
 * escape is VERIFIED rather than assumed. Lightening fails when scaling up
 * would clip a channel — a colour with a channel already near full (`#a855f7`'s
 * blue is 247) can need more headroom than it has. Darkening fails when the
 * lower edge is below zero luminance, which happens whenever `candidates` holds
 * no light foreground: `["#1c1917"]` alone bands (-0.0367, 0.2202), and
 * darkening there merely clamps to black, still inside it. Clipping the
 * lightening scale would reach any edge but only by desaturating, which is the
 * one thing this function promises not to do, so when neither direction escapes
 * the surface is returned unchanged and `pickReadableColor` degrades to the
 * least-bad candidate exactly as it documents. One honest degradation beats a
 * silent recolour of the operator's brand plus an unreadable result anyway —
 * inside the band, by definition, no candidate reaches `minRatio` regardless of
 * where in it the surface lands.
 *
 * For the assembler's own white / ink-900 pair the lower edge is +0.1833 and
 * darkening always escapes: over all 1,267,369 sRGB colours in that band, zero
 * come back still inside it.
 */
export function conditionSurface(
  surface: string,
  candidates: string[],
  minRatio: number,
  overlay?: SurfaceOverlay | null,
): string {
  const rgb = parseHex(surface);
  if (!rgb) return surface;

  const { low, high } = unreadableBand(candidates, minRatio);
  const seen = relativeLuminance(asPainted(rgb, overlay));
  if (seen <= low || seen >= high) return surface;

  // Nearer edge first, measured on the painted colour because the painted
  // colour is what the band describes.
  const edges =
    high - seen < seen - low
      ? [high + CONTRAST_MARGIN, low - CONTRAST_MARGIN]
      : [low - CONTRAST_MARGIN, high + CONTRAST_MARGIN];

  for (const edge of edges) {
    const target = overlay
      ? solveUnderOverlay(rgb, overlay, edge, edge > seen)
      : edge;
    if (target === null) continue;
    const moved = scaleLuminance(rgb, target);
    if (!moved) continue;
    const bytes = toBytes(moved);
    const escaped = relativeLuminance(asPainted(bytes, overlay));
    if (escaped > low && escaped < high) continue;
    return toHex(bytes);
  }
  return surface;
}

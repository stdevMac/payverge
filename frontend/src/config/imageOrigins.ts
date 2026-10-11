import manifest from "./imageOrigins.json";

export type ImageOrigin = {
  protocol: "https";
  hostname: string;
  pathname: `/${string}`;
};

/**
 * next/image `remotePatterns` manifest (build-time: next.config.mjs reads the
 * JSON). Same-origin `/media/**` needs no entry; hosts listed here are also
 * allowed by the CSP and trusted for AI waiter entity images.
 */
export const imageOrigins = manifest as ImageOrigin[];

/** Lower-case hostnames next/image may optimize. */
export const optimizableImageHosts: ReadonlySet<string> = new Set(
  imageOrigins.map((origin) => origin.hostname.toLowerCase()),
);

/**
 * Whether next/image may route `src` through the optimizer. Same-origin paths
 * (including proxied uploads at /media/**), static imports and hosts in the
 * build-time manifest qualify. Any other absolute URL — e.g. an upload bucket
 * a deployment allows at runtime via MEDIA_ORIGINS — must render with
 * `unoptimized`, or the optimizer rejects it and the image never loads.
 */
export function canOptimizeImageSrc(src: unknown): boolean {
  if (src && typeof src === "object") return true; // static import
  if (typeof src !== "string" || !src) return false;
  if (src.startsWith("data:") || src.startsWith("blob:")) return true;
  try {
    const { hostname } = new URL(src, "https://placeholder.invalid");
    if (hostname === "placeholder.invalid") return true; // relative path
    return optimizableImageHosts.has(hostname.toLowerCase());
  } catch {
    return false;
  }
}

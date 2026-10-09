// A screen is "still loading" while either of these holds:
//   BOOT_GATE_SELECTOR — Payverge's boot/tab gates and every dashboard
//     skeleton set `aria-busy="true"` (DashboardLayout's auth gate,
//     DashboardTabLoadingSkeleton, per-tab *Skeleton components).
//   SKELETON_PULSE_SELECTOR + area threshold — inner-card placeholders that
//     use Tailwind `animate-pulse` without aria-busy. The area threshold
//     exists because some elements pulse FOREVER by design (e.g. LiveBills'
//     8x8 "Live" dot). Treating those as "loading" wedged every shot into the
//     full timeout, so only large pulsing blocks count as skeletons.
export const BOOT_GATE_SELECTOR = '[aria-busy="true"]';
export const SKELETON_PULSE_SELECTOR = '[class*="animate-pulse"]';
export const MIN_SKELETON_AREA_PX = 1200; // px^2; smallest real skeleton (h-5 w-28) is ~2200

// Pure predicate: is this screen visually settled enough to capture?
// `doc` is anything exposing querySelector/querySelectorAll (real document or
// a test fake).
export function evaluateReadiness(
  doc,
  { waitFor, minSkeletonArea = MIN_SKELETON_AREA_PX } = {},
) {
  if (doc.querySelector(BOOT_GATE_SELECTOR)) {
    return { ready: false, reason: "loading" };
  }
  const pulses = doc.querySelectorAll(SKELETON_PULSE_SELECTOR);
  for (const el of pulses) {
    const rect = el.getBoundingClientRect ? el.getBoundingClientRect() : null;
    if (rect && rect.width * rect.height >= minSkeletonArea) {
      return { ready: false, reason: "skeleton" };
    }
  }
  if (waitFor && !doc.querySelector(waitFor)) {
    return { ready: false, reason: "awaiting-selector" };
  }
  return { ready: true, reason: "ready" };
}

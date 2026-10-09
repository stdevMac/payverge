/**
 * Scroll behavior that honors prefers-reduced-motion.
 * SSR (no window) uses an instant jump. A browser without matchMedia keeps
 * the smooth default, since we cannot tell that the user asked to reduce motion.
 */
export function preferredScrollBehavior(): ScrollBehavior {
  if (typeof window === "undefined") return "auto";
  const media = window.matchMedia?.("(prefers-reduced-motion: reduce)");
  if (!media) return "smooth";
  return media.matches ? "auto" : "smooth";
}

import { useEffect, useState } from "react";

/**
 * Subscribe to a CSS media query. Returns `false` during SSR and in
 * environments without `window.matchMedia` (e.g. jsdom), so components that
 * render a mobile-only branch don't double-mount in tests — gate the mobile
 * branch on this and keep the desktop branch behind a CSS `hidden md:block`.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const mql = window.matchMedia(query);
    const update = () => setMatches(mql.matches);
    update();
    mql.addEventListener("change", update);
    return () => mql.removeEventListener("change", update);
  }, [query]);

  return matches;
}

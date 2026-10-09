"use client";

import { useState, useEffect, useLayoutEffect, useCallback, useRef } from "react";

const STOREFRONT_LAZY_TABS = ["delivery", "reservations"] as const;

interface UseHashTabsOptions {
  validTabs: string[];
  defaultTab?: string;
  /** Tabs that may appear after async feature discovery. */
  lazyTabs?: readonly string[];
  /** When true, unavailable tabs fall back to the default. */
  tabsReady?: boolean;
  /**
   * Read the initial tab from (and write tab changes back to) `window.location`.
   * The public storefront needs this — deep links like `/b/slug#menu` are part
   * of the contract. Embedded renders of the same tree (the Business Page
   * editor's live preview, #591) must NOT hijack the host document's URL, so
   * they pass `false` and get purely local tab state instead.
   */
  syncLocation?: boolean;
}

function resolveStorefrontTab({
  candidate,
  validTabs,
  defaultTab,
  lazyTabs = STOREFRONT_LAZY_TABS,
  tabsReady = true,
}: {
  candidate: string;
  validTabs: string[];
  defaultTab: string;
  lazyTabs?: readonly string[];
  tabsReady?: boolean;
}): string {
  if (!candidate) return defaultTab;
  if (validTabs.includes(candidate)) return candidate;
  if (!tabsReady && lazyTabs.includes(candidate)) return candidate;
  return defaultTab;
}

function readLocationCandidate(): { hash: string; queryTab: string | null } {
  if (typeof window === "undefined") return { hash: "", queryTab: null };
  return {
    hash: window.location.hash.replace(/^#/, ""),
    queryTab: new URLSearchParams(window.location.search).get("tab"),
  };
}

function replaceHash(tab: string) {
  try {
    window.history.replaceState(null, "", `#${tab}`);
  } catch {
    // SSR or restricted history — best effort only.
  }
}

export function useHashTabs({
  validTabs,
  defaultTab = "about",
  lazyTabs = STOREFRONT_LAZY_TABS,
  tabsReady = true,
  syncLocation = true,
}: UseHashTabsOptions) {
  const [activeTab, setActiveTab] = useState<string>(defaultTab);
  const [locationReady, setLocationReady] = useState(false);
  const validTabsRef = useRef(validTabs);
  const defaultTabRef = useRef(defaultTab);
  const lazyTabsRef = useRef(lazyTabs);
  const tabsReadyRef = useRef(tabsReady);
  const syncLocationRef = useRef(syncLocation);
  validTabsRef.current = validTabs;
  defaultTabRef.current = defaultTab;
  lazyTabsRef.current = lazyTabs;
  tabsReadyRef.current = tabsReady;
  syncLocationRef.current = syncLocation;

  const applyCandidate = useCallback((candidate: string, rewriteInvalid: boolean) => {
    const resolved = resolveStorefrontTab({
      candidate,
      validTabs: validTabsRef.current,
      defaultTab: defaultTabRef.current,
      lazyTabs: lazyTabsRef.current,
      tabsReady: tabsReadyRef.current,
    });
    setActiveTab(resolved);
    if (
      rewriteInvalid &&
      syncLocationRef.current &&
      candidate &&
      resolved === defaultTabRef.current &&
      candidate !== defaultTabRef.current &&
      !validTabsRef.current.includes(candidate) &&
      (tabsReadyRef.current || !lazyTabsRef.current.includes(candidate))
    ) {
      replaceHash(defaultTabRef.current);
    }
    return resolved;
  }, []);

  useLayoutEffect(() => {
    if (!syncLocationRef.current) {
      // Embedded/preview render: the host document's URL is not ours to read
      // or rewrite. Start on the default tab and mark ready immediately.
      applyCandidate("", false);
      setLocationReady(true);
      return;
    }
    const { hash, queryTab } = readLocationCandidate();
    const candidate = hash || queryTab || "";
    applyCandidate(candidate, true);
    if (!hash && queryTab && validTabsRef.current.includes(queryTab)) {
      normalizeQueryTabIntoHash(queryTab);
    } else if (
      !hash &&
      queryTab &&
      !tabsReadyRef.current &&
      lazyTabsRef.current.includes(queryTab)
    ) {
      normalizeQueryTabIntoHash(queryTab);
    }
    setLocationReady(true);
  }, [applyCandidate]);

  useEffect(() => {
    if (!syncLocation) return;
    const handleHashChange = () => {
      const { hash } = readLocationCandidate();
      applyCandidate(hash, true);
    };

    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, [applyCandidate, syncLocation]);

  useEffect(() => {
    if (!locationReady) return;
    if (!tabsReady && lazyTabs.includes(activeTab)) return;
    if (!validTabs.includes(activeTab)) {
      setActiveTab(defaultTab);
      if (!syncLocation) return;
      const current = window.location.hash.replace(/^#/, "");
      if (current && current !== defaultTab && !validTabs.includes(current)) {
        replaceHash(defaultTab);
      }
    }
  }, [activeTab, validTabs, defaultTab, tabsReady, lazyTabs, locationReady, syncLocation]);

  const changeTab = useCallback((tab: string) => {
    setActiveTab(tab);
    // Preview/embedded renders keep tab state local — never touch the host URL.
    if (!syncLocationRef.current) return;
    // pushState updates the URL without firing `hashchange`. Storefront AiWaiter
    // / Concierge close on hash-tab navigation (PG-15.3), so notify listeners
    // the same way a real hash navigation would.
    window.history.pushState(null, "", `#${tab}`);
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  }, []);

  return { activeTab, changeTab, locationReady };
}

// Normalize `?tab=foo` into `#foo` so the rest of this hook's hash-driven
// logic (hashchange listener, changeTab pushState) stays in sync.
function normalizeQueryTabIntoHash(tab: string) {
  try {
    const url = new URL(window.location.href);
    url.searchParams.delete("tab");
    url.hash = tab;
    window.history.replaceState(null, "", url.toString());
  } catch {
    // SSR or malformed URL — best effort only.
  }
}

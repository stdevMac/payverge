"use client";

import dynamic from "next/dynamic";

// Same rationale as SimpleLanguageSwitcherLazy: defer the NextUI-based
// floating pill out of the eager critical chunk so the marketing LCP path
// doesn't carry post-hydration UI weight. The skeleton is `null` because
// the pill is fixed-position and its absence on first paint does not shift
// any layout.
const FloatingLanguageSwitcher = dynamic(
  () => import("./FloatingLanguageSwitcher"),
  { ssr: false, loading: () => null },
);

export default function FloatingLanguageSwitcherLazy() {
  return <FloatingLanguageSwitcher />;
}

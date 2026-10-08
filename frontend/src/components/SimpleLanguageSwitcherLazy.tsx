"use client";

import dynamic from "next/dynamic";

// SimpleLanguageSwitcher wraps a NextUI <Dropdown>, which transitively pulls
// framer-motion + dropdown-overlay machinery into whatever chunk references
// it. Sitting eagerly in the (shop) layout pinned that to the marketing LCP
// critical path on every public page despite the switcher being a floating,
// post-hydration pill. Deferring with ssr:false drops framer-motion out of
// the eager bundle (PageSpeed flagged ~21 KiB unused framer-motion JS on
// first paint). The skeleton is `null` because the pill is fixed-position
// and its absence on first paint does not shift any layout.
const SimpleLanguageSwitcher = dynamic(
  () => import("./SimpleLanguageSwitcher"),
  { ssr: false, loading: () => null },
);

export default function SimpleLanguageSwitcherLazy({
  compact = false,
}: {
  compact?: boolean;
} = {}) {
  return <SimpleLanguageSwitcher compact={compact} />;
}

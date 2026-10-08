"use client";

import React from "react";
import { sanitizeCssColor, resolveCornerRadius } from "@/lib/storefront/theme";

interface StorefrontThemeStyleProps {
  designSettings: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
  };
}

/**
 * Scoped storefront theme (plan 2.3). The old implementation wrote --primary
 * and global .bg-primary/.text-primary utility classes onto :root via a
 * global stylesheet, leaking the merchant palette across client-side
 * navigation and colliding with NextUI's own `primary` tokens elsewhere.
 * Everything is scoped to the `.storefront-theme` wrapper class on the page
 * root <main>, so the merchant theme lives and dies with the storefront
 * subtree. Colors are sanitized before interpolation — they are
 * business-controlled strings reaching CSS.
 */
export default function StorefrontThemeStyle({
  designSettings,
}: StorefrontThemeStyleProps) {
  const primary = sanitizeCssColor(designSettings.primary_color);
  const secondary = sanitizeCssColor(designSettings.secondary_color);
  const radius = resolveCornerRadius(designSettings.corner_radius);

  return (
    <style jsx global>{`
      .scrollbar-hide::-webkit-scrollbar { display: none; }
      .scrollbar-hide { -ms-overflow-style: none; scrollbar-width: none; }
      .storefront-theme {
        --primary: ${primary};
        --secondary: ${secondary};
        --radius: ${radius};
      }
      .storefront-theme .bg-primary { background-color: var(--primary); }
      .storefront-theme .text-primary { color: var(--primary); }
      .storefront-theme .border-primary { border-color: var(--primary); }
      .storefront-theme .bg-secondary { background-color: var(--secondary); }
      .storefront-theme .text-secondary { color: var(--secondary); }
    `}</style>
  );
}

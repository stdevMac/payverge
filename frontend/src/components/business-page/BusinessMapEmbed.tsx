"use client";

import React from "react";
import { Navigation } from "lucide-react";

interface BusinessMapEmbedProps {
  /** Full human-readable address; encoded into the Google Maps URLs. */
  address: string;
  /** Merchant primary color for the directions CTA. */
  primaryColor: string;
  /** Merchant corner-radius class (getRadiusClass) shared with the tab. */
  radiusClass: string;
  t: (key: string) => string;
}

/**
 * Google Maps embed + "Get directions" CTA for the contact tab (plan 3.3).
 * Keyless embed — the `output=embed` endpoint needs no API key, so there is
 * no new secret or config surface. The iframe is lazy-loaded so it never
 * blocks first paint for guests who never open the contact tab.
 */
export default function BusinessMapEmbed({
  address,
  primaryColor,
  radiusClass,
  t,
}: BusinessMapEmbedProps) {
  const encoded = encodeURIComponent(address);
  return (
    <div className="space-y-3 pt-2">
      <div className={`overflow-hidden ${radiusClass} border border-gray-200`}>
        <iframe
          src={`https://www.google.com/maps?q=${encoded}&output=embed`}
          title={t("businessPage.mapTitle") || "Map"}
          loading="lazy"
          referrerPolicy="no-referrer-when-downgrade"
          allowFullScreen
          className="aspect-video w-full border-0 md:aspect-auto md:h-[360px]"
        />
      </div>
      <a
        href={`https://www.google.com/maps/search/?api=1&query=${encoded}`}
        target="_blank"
        rel="noopener noreferrer"
        className={`inline-flex items-center gap-2 px-5 py-2.5 ${radiusClass} text-sm font-semibold text-white transition-opacity hover:opacity-90`}
        style={{ backgroundColor: primaryColor }}
      >
        <Navigation className="h-4 w-4" aria-hidden />
        {t("businessPage.getDirections")}
      </a>
    </div>
  );
}

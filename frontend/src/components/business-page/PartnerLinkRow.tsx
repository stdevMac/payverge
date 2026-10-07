import React from "react";
import NextImage from "next/image";
import { ExternalLink } from "lucide-react";
import { isSafeExternalTrackingUrl } from "@/lib/externalUrl";

interface PartnerLink {
  name: string;
  url: string;
  provider_key?: string;
  icon_url?: string;
}

interface PartnerLinkRowProps {
  link: PartnerLink;
  designSettings: {
    primary_color: string;
    corner_radius?: string;
  };
  /** Localized open-partner label. */
  ctaLabel: string;
}

function getPartnerHost(rawUrl: string): string {
  try {
    return new URL(rawUrl).hostname.replace(/^www\./, "");
  } catch {
    return rawUrl;
  }
}

export default function PartnerLinkRow({
  link,
  designSettings,
  ctaLabel,
}: PartnerLinkRowProps) {
  // Operator-configured URL rendered as an href on the public business page:
  // anything that is not a real web URL (javascript:, data:, garbage) is not
  // rendered at all rather than linked (#897).
  if (!isSafeExternalTrackingUrl(link.url)) {
    return null;
  }
  const host = getPartnerHost(link.url);

  return (
    <li className="flex items-center gap-4 py-4 group">
      <div className="w-10 h-10 rounded-lg border border-gray-200 bg-white flex items-center justify-center overflow-hidden flex-shrink-0">
        {link.icon_url ? (
          <NextImage
            src={link.icon_url}
            alt={link.name}
            width={36}
            height={36}
            className="w-9 h-9 object-cover"
            unoptimized
          />
        ) : (
          <ExternalLink className="w-4 h-4 text-gray-400" />
        )}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-gray-900 truncate">
          {link.name}
        </p>
        <p className="text-xs text-gray-500 truncate">{host}</p>
      </div>
      <a
        href={link.url}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-1 text-sm font-semibold transition-colors hover:underline"
        style={{ color: designSettings.primary_color }}
      >
        {ctaLabel}
        <ExternalLink className="w-3 h-3" />
      </a>
    </li>
  );
}

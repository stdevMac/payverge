import type { PublicBusiness } from "@/api/publicBusiness";
import { guestPublicUrl } from "./guestUrls";
import { OG_IMAGE_URL } from "./openGraphImages";

const DAY_NAMES = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

function formatStreetAddress(business: PublicBusiness): string {
  const address = business.address || ({} as PublicBusiness["address"]);
  return [
    address?.street,
    address?.city,
    address?.state,
    address?.postal_code,
    address?.country,
  ]
    .filter(Boolean)
    .join(", ");
}

export interface BusinessJsonLdExtras {
  /**
   * Google rating for `aggregateRating`. Emitted only when both the rating
   * and a positive review count are known — schema.org validators flag a
   * count-less rating as incomplete.
   */
  googleRating?: { ratingValue: number; reviewCount: number } | null;
  /** Path locale for self-canonical storefront URLs (#864). */
  locale?: string;
  /**
   * Canonical storefront path. Defaults to `/b/{customUrl}`; `/` when the
   * venue is the one the instance serves at its root, so JSON-LD `url`
   * matches the canonical link and og:url.
   */
  pagePath?: string;
  /** Emits acceptsReservations "True". Only pass when actually known. */
  acceptsReservations?: boolean;
  /**
   * Entry-point URLs for potentialAction (OrderAction / ReserveAction).
   * Only pass targets for features known to be enabled — the server render
   * path cannot see the client-fetched delivery/reservation settings, so it
   * passes nothing and no actions are emitted (correctness over completeness).
   */
  orderActionTarget?: string | null;
  reserveActionTarget?: string | null;
}

function firstStorefrontImage(
  business: PublicBusiness,
  baseUrl: string,
): string {
  if (business.logo) return business.logo;
  const raw = business.banner_images;
  if (raw) {
    try {
      const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
      if (Array.isArray(parsed) && typeof parsed[0] === "string" && parsed[0]) {
        return parsed[0];
      }
    } catch {
      // fall through
    }
  }
  return `${baseUrl}${OG_IMAGE_URL}`;
}

export function buildBusinessJsonLd(
  business: PublicBusiness,
  customUrl: string,
  baseUrl: string,
  extras: BusinessJsonLdExtras = {},
): Record<string, unknown> {
  const openingHours = (business.operating_hours || [])
    .filter((h) => !h.is_closed && h.open_time && h.close_time)
    .map((h) => ({
      "@type": "OpeningHoursSpecification",
      dayOfWeek: DAY_NAMES[h.day_of_week],
      opens: h.open_time,
      closes: h.close_time,
    }))
    .filter((h) => Boolean(h.dayOfWeek));

  const pageUrl = guestPublicUrl(
    extras.locale ?? "en",
    extras.pagePath ?? `/b/${customUrl}`,
    baseUrl,
  );
  const menuUrl = `${pageUrl}${pageUrl.includes("?") ? "&" : "?"}tab=menu`;

  const potentialActions: Record<string, unknown>[] = [];
  const actionPlatforms = [
    "http://schema.org/DesktopWebPlatform",
    "http://schema.org/MobileWebPlatform",
  ];
  if (extras.orderActionTarget) {
    potentialActions.push({
      "@type": "OrderAction",
      target: {
        "@type": "EntryPoint",
        urlTemplate: extras.orderActionTarget,
        actionPlatform: actionPlatforms,
      },
    });
  }
  if (extras.reserveActionTarget) {
    potentialActions.push({
      "@type": "ReserveAction",
      target: {
        "@type": "EntryPoint",
        urlTemplate: extras.reserveActionTarget,
        actionPlatform: actionPlatforms,
      },
    });
  }

  const googleRating =
    extras.googleRating &&
    extras.googleRating.ratingValue > 0 &&
    extras.googleRating.reviewCount > 0
      ? extras.googleRating
      : null;

  return {
    "@context": "https://schema.org",
    "@type": "Restaurant",
    name: business.name,
    image: firstStorefrontImage(business, baseUrl),
    description:
      business.description ||
      `Visit ${business.name} - Powered by Payverge AI Restaurant Management`,
    url: pageUrl,
    telephone: business.phone || "",
    address: {
      "@type": "PostalAddress",
      streetAddress: formatStreetAddress(business),
    },
    hasMenu: `${pageUrl}#menu`,
    menu: menuUrl,
    ...(openingHours.length > 0
      ? { openingHoursSpecification: openingHours }
      : {}),
    ...(googleRating
      ? {
          aggregateRating: {
            "@type": "AggregateRating",
            ratingValue: googleRating.ratingValue,
            reviewCount: googleRating.reviewCount,
          },
        }
      : {}),
    ...(extras.acceptsReservations ? { acceptsReservations: "True" } : {}),
    ...(potentialActions.length > 0 ? { potentialAction: potentialActions } : {}),
  };
}

import { getPublicConfig } from "@/config/publicConfig";
import type { Metadata } from "next";
import { headers } from "next/headers";
import Maintenance from "@/components/Maintenance";
import NotFoundClient from "./NotFoundClient";
import { getTranslation } from "@/i18n/getTranslation";
import { resolveOperatorLocale } from "@/i18n/htmlLangDir";
import {
  defaultOpenGraphImages,
  siteTwitterDefaults,
  SITE_OG_SITE_NAME,
  SITE_OG_TYPE,
} from "@/lib/seo/openGraphImages";

// not-found.tsx must be a server component for route metadata to take effect;
// client components can't export `metadata` or `generateMetadata`. The page
// itself is otherwise static — only Maintenance is interactive and it brings
// its own "use client" boundary, so importing it here is safe.

// #721: the body is translated by NotFoundClient but the title and the social
// card were not, so a Spanish 404 rendered "Página no encontrada" under an
// English browser-tab title, and — because the route declared no
// openGraph/twitter of its own — shared the root layout's English homepage
// product card for a dead URL. Read the locale middleware already resolved and
// own both.
//
// (The English tab title is deliberately not quoted here: the hardcoded-string
// gate scans this file with a regex that cannot tell a comment from JSX.)
export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const locale = resolveOperatorLocale(requestHeaders.get("x-payverge-locale"));

  const title = getTranslation("notFound.metaTitle", locale) as string;
  const description = getTranslation("notFound.body", locale) as string;
  const ogLocale =
    locale === "es-AR" ? "es_AR" : locale === "es" ? "es_ES" : "en_US";

  return {
    title,
    description,
    robots: { index: false, follow: false },
    // Deliberately no canonical and no og:url: the requested URL does not
    // exist, and pointing either at the homepage is what made the 404 look
    // like the product page in the first place.
    openGraph: {
      type: SITE_OG_TYPE,
      siteName: SITE_OG_SITE_NAME,
      locale: ogLocale,
      title,
      description,
      images: [...defaultOpenGraphImages],
    },
    twitter: siteTwitterDefaults({ title, description }),
  };
}

export default function NotFound() {
  const isMaintenanceMode = getPublicConfig().maintenanceMode;

  if (isMaintenanceMode) {
    return <Maintenance />;
  }

  return <NotFoundClient />;
}

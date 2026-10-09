import type { Metadata } from "next";
import { headers } from "next/headers";
import { getTranslation } from "@/i18n/getTranslation";
import { resolveOperatorLocale } from "@/i18n/htmlLangDir";
import {
  pageOpenGraphFromCanonical,
  siteTwitterDefaults,
} from "@/lib/seo/openGraphImages";
import { publicPageAlternates } from "@/i18n/publicPageRoutes";

const REGISTER_PATH = "/business/register";

// SEO-3: `alternates.canonical` is mandatory here, not decorative. Without it
// this layout inherits the root layout's `canonical: "/"` (src/app/layout.tsx),
// which told Google the signup page was a duplicate of the homepage — while
// sitemap.xml simultaneously advertised it as `index, follow`. Canonical wins
// that contradiction, so the page was consolidated away. `openGraph.url` had
// the same defect and made every shared signup link preview as the homepage.
//
// Sitemap already lists /business/register plus /es and /es-ar siblings
// (LOCALIZED_PUBLIC_ROUTES). The page must self-canonicalize with the same
// hreflang cluster (#911) — otherwise Google consolidates the Spanish wizard
// onto the English URL.
//
// MIN-2: document title (and OG/Twitter titles) follow x-payverge-locale so an
// es UI does not keep the English browser tab title.
export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const locale = resolveOperatorLocale(
    requestHeaders.get("x-payverge-locale"),
  );

  const title = getTranslation("businessRegister.metaTitle", locale) as string;
  const description = getTranslation(
    "businessRegister.metaDescription",
    locale,
  ) as string;
  const alternates = publicPageAlternates(locale, REGISTER_PATH);

  return {
    title,
    description,
    alternates,
    // #809: shared helpers restore og:locale (and keep siteName/type/images)
    // that the hand-rolled openGraph block omitted — the other public pages
    // already emit og:locale via the same path (#687).
    openGraph: pageOpenGraphFromCanonical({
      title,
      description,
      url: String(alternates.canonical),
      locale,
    }),
    twitter: siteTwitterDefaults({ title, description }),
  };
}

export default function BusinessRegisterLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return children;
}

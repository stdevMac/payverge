import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { getSiteUrl } from "@/config/publicConfig";
import { getServerHome } from "@/lib/instance/serverHome";
import { getServerInstanceInfo } from "@/lib/instance/serverInstance";
import { siteNameOf } from "@/lib/instance/instanceInfo";
import { guestPublicUrl } from "@/lib/seo/guestUrls";
import { resolveGuestLocaleFromRequest } from "@/i18n/guestLocale.server";
import VenueDirectory, {
  directoryCopy,
} from "@/components/venue-directory/VenueDirectory";
import {
  storefrontMetadata,
  StorefrontView,
} from "@/app/b/[customUrl]/storefrontRender";

// The instance root is the venue's public page, not a product site:
//   PRIMARY_VENUE / the single published venue → that storefront, canonical "/"
//   several published venues                    → a directory of /b/<slug> links
//   nothing published                           → /dashboard (operator sign-in)
// Resolution lives in the backend (GET /api/v1/home) so it follows the same
// publish gate as /b/<slug>. Rendered per request: publishing or unpublishing
// a venue must reach "/" without a rebuild.
export const dynamic = "force-dynamic";
export const revalidate = 0;

type SearchParams = Record<string, string | string[] | undefined>;

interface HomePageProps {
  searchParams?: Promise<SearchParams>;
}

function firstParam(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

export async function generateMetadata({
  searchParams,
}: HomePageProps): Promise<Metadata> {
  const sp = searchParams ? await searchParams : {};
  const home = await getServerHome();
  if (home?.mode === "venue" && home.primary) {
    return storefrontMetadata({
      customUrl: home.primary.custom_url,
      searchParams: sp,
      pagePath: "/",
    });
  }
  const instance = await getServerInstanceInfo();
  const title = siteNameOf(instance);
  const { locale } = await resolveGuestLocaleFromRequest(firstParam(sp.lang));
  const copy = directoryCopy(locale);
  const canonical = guestPublicUrl(locale, "/", getSiteUrl());
  return {
    title,
    description: copy.lede,
    alternates: { canonical },
    // The directory's own social card: the instance name and the directory
    // lede, never the root layout's defaults.
    openGraph: {
      type: "website",
      url: canonical,
      title,
      description: copy.lede,
      siteName: title,
    },
    twitter: { card: "summary", title, description: copy.lede },
    // Nothing published yet: the page only redirects; keep it out of indexes.
    ...(home?.mode === "directory"
      ? {}
      : { robots: { index: false, follow: false } }),
  };
}

export default async function HomePage({ searchParams }: HomePageProps) {
  const sp = searchParams ? await searchParams : {};
  const home = await getServerHome();

  if (home?.mode === "venue" && home.primary) {
    return StorefrontView({
      customUrl: home.primary.custom_url,
      searchParams: sp,
      pagePath: "/",
    });
  }

  if (home?.mode === "directory") {
    const instance = await getServerInstanceInfo();
    const { locale } = await resolveGuestLocaleFromRequest(firstParam(sp.lang));
    return (
      <VenueDirectory
        venues={home.venues}
        locale={locale}
        title={siteNameOf(instance)}
      />
    );
  }

  // "empty" (no published venue) or the backend never answered: the
  // operator sign-in is the only useful destination. The query survives so
  // an invite link (/?invite_code=…) still reaches the sign-in with it.
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(sp)) {
    for (const v of Array.isArray(value) ? value : [value]) {
      if (typeof v === "string") query.append(key, v);
    }
  }
  const qs = query.toString();
  redirect(qs ? `/dashboard?${qs}` : "/dashboard");
}

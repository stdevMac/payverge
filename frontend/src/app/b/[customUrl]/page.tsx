import type { Metadata } from "next";
import { getServerHome, isPrimaryVenueSlug } from "@/lib/instance/serverHome";
import { storefrontMetadata, StorefrontView } from "./storefrontRender";

// Publish-gated storefront: never Data-Cache a published snapshot after the
// owner flips business_page_enabled off. ISR revalidate:60 left guests/crawlers
// with HTTP 200 + full Restaurant JSON-LD for 60–90s+ while the API already
// returned 404. Match /t/[tableCode] and /business/[businessId] no-store.
export const dynamic = "force-dynamic";
export const revalidate = 0;

interface BusinessPageProps {
  params: Promise<{ customUrl: string }>;
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}

// The venue the instance serves at "/" (PRIMARY_VENUE or the only published
// venue) stays reachable here, but canonicalizes to the site root so the two
// URLs are never indexed as duplicates.
async function canonicalPathFor(customUrl: string): Promise<string> {
  const home = await getServerHome();
  return isPrimaryVenueSlug(home, customUrl) ? "/" : `/b/${customUrl}`;
}

export async function generateMetadata({
  params,
  searchParams,
}: BusinessPageProps): Promise<Metadata> {
  const { customUrl } = await params;
  const sp = searchParams ? await searchParams : {};
  return storefrontMetadata({
    customUrl,
    searchParams: sp,
    pagePath: await canonicalPathFor(customUrl),
  });
}

export default async function BusinessPage({
  params,
  searchParams,
}: BusinessPageProps) {
  const { customUrl } = await params;
  const sp = searchParams ? await searchParams : {};
  return StorefrontView({
    customUrl,
    searchParams: sp,
    pagePath: await canonicalPathFor(customUrl),
  });
}

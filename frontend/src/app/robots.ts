import type { MetadataRoute } from "next";
import { getSiteUrl } from "@/config/publicConfig";
import { isSeoIndexingEnabled } from "@/config/serverConfig";

// Rendered per request: the canonical origin (PUBLIC_URL) and the indexing
// policy (SEO_INDEXING) are runtime settings, so one image serves any
// deployment. Without `force-dynamic` Next would freeze the build machine's
// values into a static robots.txt.
export const dynamic = "force-dynamic";

// HEADS UP for deployments behind Cloudflare: its "AI Crawl Control" /
// "managed robots.txt" feature replaces this response at the edge with its
// own template. Disable "Manage robots.txt" (Bots → AI Crawl Control) or carve
// a Bypass rule for /robots.txt so the policy below is what crawlers see.
export default function robots(): MetadataRoute.Robots {
  // A fresh self-hosted install is private by default: nothing is crawlable
  // until the operator opts in with SEO_INDEXING=true (the root layout emits
  // the matching noindex meta).
  if (!isSeoIndexingEnabled()) {
    return {
      rules: [{ userAgent: "*", disallow: "/" }],
    };
  }

  const siteUrl = getSiteUrl();
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: [
          // Prefix match: /admin and /dashboard cover the exact URLs and
          // every child. `/admin/*` does not match the bare /admin path.
          "/admin",
          "/dashboard",
          "/api/*",
          "/t/*", // Table codes - dynamic guest pages
          "/_next/*",
          "/*.json$",
        ],
      },
      {
        userAgent: "Googlebot",
        allow: "/",
        disallow: [
          "/admin",
          "/dashboard",
          "/api/*",
          "/t/*", // Table codes — guest transactional pages (parity with *)
        ],
      },
    ],
    sitemap: `${siteUrl}/sitemap.xml`,
    host: siteUrl,
  };
}

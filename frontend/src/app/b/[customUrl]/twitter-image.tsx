// Twitter card crawlers request /twitter-image; reuse the storefront OG card.
export { default, alt, size, contentType } from "./opengraph-image";

// Route segment config must be declared here as literals: Next.js reads it
// statically and ignores re-exported values (build warning "can't recognize
// the exported `runtime` field"). Keep in step with ./opengraph-image.
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

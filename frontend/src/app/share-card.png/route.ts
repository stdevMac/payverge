import { getServerInstanceInfo } from "@/lib/instance/serverInstance";
import { siteNameOf } from "@/lib/instance/instanceInfo";
import { renderShareCard } from "@/lib/seo/shareCard";

// The instance name, logo and brand color are runtime settings
// (GET /api/v1/instance), so the card renders per request.
export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(): Promise<Response> {
  const instance = await getServerInstanceInfo();
  return renderShareCard({
    name: siteNameOf(instance),
    logoUrl: instance?.logo_url || undefined,
    brandColor: instance?.brand_color,
  });
}

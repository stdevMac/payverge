import { getSiteUrl } from "@/config/publicConfig";
import { getSecurityEmail } from "@/config/serverConfig";
import { buildSecurityTxt } from "@/lib/security/securityTxt";

// Per request: contact (SECURITY_EMAIL, else SUPPORT_EMAIL) and canonical
// origin (PUBLIC_URL) are runtime settings, and Expires rolls forward.
export const dynamic = "force-dynamic";

export function GET(): Response {
  const contactEmail = getSecurityEmail();
  if (!contactEmail) {
    // An unconfigured instance publishes no contact rather than a wrong one.
    return new Response("Not Found\n", {
      status: 404,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
  return new Response(
    buildSecurityTxt({ contactEmail, siteUrl: getSiteUrl() }),
    {
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
        "Cache-Control": "public, max-age=3600",
      },
    },
  );
}

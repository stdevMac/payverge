import { legalNameOf, productNameOf, type InstanceInfo } from "./instanceInfo";

/**
 * Fill the contact tokens in legal copy (src/i18n/messages/<locale>/legal.json)
 * from GET /api/v1/instance. A missing address renders as `fallback` (the
 * caller passes a localized "not configured" label) rather than an upstream
 * address, so a self-host never routes its users to someone else's inbox.
 */
export function fillLegalTokens(
  text: string,
  info: InstanceInfo | null | undefined,
  fallback: string,
): string {
  const support = info?.support_email || "";
  const security = info?.security_email || support;
  const values: Record<string, string> = {
    supportEmail: support || fallback,
    securityEmail: security || fallback,
    publicUrl: info?.public_url || (typeof window !== "undefined" ? window.location.origin : "") || fallback,
    legalName: legalNameOf(info),
    productName: productNameOf(info),
  };
  // tArray() also yields structured entries (objects); leave those alone.
  if (typeof text !== "string") return text;
  return text.replace(/\{(supportEmail|securityEmail|publicUrl|legalName|productName)\}/g, (_, key: string) => values[key]);
}

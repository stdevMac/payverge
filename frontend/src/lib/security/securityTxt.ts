/**
 * RFC 9116 security.txt body for this deployment. Served by
 * app/.well-known/security.txt/route.ts from runtime env, so a self-hosted
 * instance names its own contact and canonical origin, never another
 * deployment's.
 */

/** RFC 9116 §2.5.5 recommends an Expires value less than a year ahead. */
export const SECURITY_TXT_TTL_DAYS = 180;

const DAY_MS = 24 * 60 * 60 * 1000;

export interface SecurityTxtInput {
  /** Bare email address; rendered as a mailto: Contact. */
  contactEmail: string;
  /** Canonical origin without a trailing slash, e.g. https://pos.example.com */
  siteUrl: string;
  now?: Date;
}

export function buildSecurityTxt({
  contactEmail,
  siteUrl,
  now = new Date(),
}: SecurityTxtInput): string {
  // Truncate to the UTC day so every response in a day is byte-identical
  // (cache friendly) and the date only moves forward.
  const today = Date.UTC(
    now.getUTCFullYear(),
    now.getUTCMonth(),
    now.getUTCDate(),
  );
  const expires = new Date(today + SECURITY_TXT_TTL_DAYS * DAY_MS);
  return [
    "# Vulnerability disclosure (RFC 9116)",
    `Contact: mailto:${contactEmail}`,
    "Preferred-Languages: en",
    `Canonical: ${siteUrl}/.well-known/security.txt`,
    `Expires: ${expires.toISOString()}`,
    "",
  ].join("\n");
}

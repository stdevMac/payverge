/**
 * Server-side decision for the legal pages (privacy, terms, refund).
 *
 * Every install renders the labelled generic template (or redirects to the
 * operator's own LEGAL_TERMS_URL / LEGAL_PRIVACY_URL); there is no upstream
 * product-site legal text to choose.
 */

import type { Metadata } from "next";
import { getTranslation } from "@/i18n/getTranslation";
import type { InstanceInfo } from "./instanceInfo";
import { productNameOf } from "./instanceInfo";

export type LegalPageKind = "privacy" | "terms" | "refund";

/**
 * The operator's own legal page for `kind` (LEGAL_TERMS_URL /
 * LEGAL_PRIVACY_URL via /instance), or null. When set, the page redirects
 * there instead of rendering the template.
 */
export function externalLegalUrl(
  kind: LegalPageKind,
  instance: InstanceInfo | null,
): string | null {
  if (kind === "terms") return instance?.legal_terms_url || null;
  if (kind === "privacy") return instance?.legal_privacy_url || null;
  return null;
}

/** Neutral, non-indexed metadata for the generic template. */
export function genericLegalMetadata(
  kind: LegalPageKind,
  locale: Parameters<typeof getTranslation>[1],
  instance: InstanceInfo | null,
): Metadata {
  const heading = getTranslation(`legal.template.${kind}.title`, locale) as string;
  return {
    title: `${heading} — ${productNameOf(instance)}`,
    robots: { index: false, follow: false },
  };
}

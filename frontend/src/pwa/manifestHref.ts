/** Locale-specific web app manifest hrefs used by the root layout (#914). */
export function pwaManifestHref(
  locale: "en" | "es" | "es-ar",
): string {
  if (locale === "es-ar") return "/site.es-ar.webmanifest";
  if (locale === "es") return "/site.es.webmanifest";
  return "/site.webmanifest";
}

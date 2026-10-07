import type { StorefrontLocale } from "./localeRegistry";

export async function loadGuestMessages(
  locale: StorefrontLocale,
): Promise<Record<string, unknown>> {
  return (await import(`./guest-messages/${locale}.json`)).default as Record<
    string,
    unknown
  >;
}

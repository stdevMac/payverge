import { Metadata } from "next";
import ScanClient from "./ScanClient";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import { resolveGuestLocaleFromRequest } from "@/i18n/guestLocale.server";
import { loadGuestMessages } from "@/i18n/guestMessages.server";

// /scan is a purely guest-facing entry surface (a diner landing here to access
// their table), so it runs under the 21-locale GuestTranslationProvider rather
// than the en/es-only operator tier. ScanClient + QRCodeScanner read scan.*
// keys from the guest message bundles.
export async function generateMetadata(): Promise<Metadata> {
  const guestLocale = await resolveGuestLocaleFromRequest(undefined);
  const messages = await loadGuestMessages(guestLocale.locale);
  const scan = messages.scan as {
    scanner?: { title?: string };
    header?: { title?: string };
  };
  const heading =
    (typeof scan?.scanner?.title === "string" && scan.scanner.title) ||
    (typeof scan?.header?.title === "string" && scan.header.title) ||
    "Scan table code";
  return {
    title: `${heading} — Payverge`,
    robots: { index: false, follow: true },
  };
}

interface ScanPageProps {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}

export default async function ScanPage({ searchParams }: ScanPageProps) {
  const sp = searchParams ? await searchParams : {};
  const rawLang = Array.isArray(sp.lang) ? sp.lang[0] : sp.lang;
  // PG-21 / #390: same resolver as /t and /b — ?lang= → guest cookie →
  // Accept-Language. A diner who saved Spanish on a table/storefront must not
  // get English first paint at QR-entry.
  const guestLocale = await resolveGuestLocaleFromRequest(rawLang);
  const initialMessages = await loadGuestMessages(guestLocale.locale);

  return (
    <GuestTranslationProvider
      businessId={undefined}
      initialLanguage={guestLocale.locale}
      initialMessages={initialMessages}
      preferInitialLanguage={guestLocale.isExplicit}
    >
      <ScanClient />
    </GuestTranslationProvider>
  );
}

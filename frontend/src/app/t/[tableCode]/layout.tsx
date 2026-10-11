import type { Metadata } from "next";
import { notFound } from "next/navigation";
import CustomerAuthShell from "@/components/customer/CustomerAuthShell";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import { loadGuestMessages } from "@/i18n/guestMessages.server";
import {
  defaultLocale,
  isGuestLocale,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";
import { lookupGuestTable } from "@/lib/guest/lookupGuestTable";
import { guestTablePageMetadata } from "@/lib/guest/guestTableMetadata";
import { headers } from "next/headers";

export const dynamic = "force-dynamic";
export const revalidate = 0;

interface TableLayoutProps {
  params: Promise<{ tableCode: string }>;
}

/**
 * Guest table micro-pages are transactional (QR codes, session state).
 * Never inherit the marketing homepage title or root index,follow.
 * Business-scoped when resolvable; never guest PII (table codes).
 */
export async function generateMetadata({
  params,
}: TableLayoutProps): Promise<Metadata> {
  const { tableCode } = await params;
  const lookup = await lookupGuestTable(tableCode, "business");
  const businessName = lookup.kind === "found" ? lookup.businessName : null;
  const headerStore = await headers();
  const raw = headerStore.get("x-payverge-locale") ?? defaultLocale;
  const locale: StorefrontLocale = isGuestLocale(raw) ? raw : defaultLocale;
  return guestTablePageMetadata({
    tableCode,
    surface: "table",
    businessName,
    locale,
  });
}

/**
 * PG-21: seed guest messages on the server so PersistentGuestNav labels
 * (Table / Menu / Bill) render in the diner locale on first paint — not English
 * SSR followed by a hydration flip. Locale comes from middleware's
 * x-payverge-locale (?lang= → guest cookie → Accept-Language).
 */
export default async function TableLayout({
  children,
  params,
}: Readonly<{
  children: React.ReactNode;
  params: Promise<{ tableCode: string }>;
}>) {
  const { tableCode } = await params;
  const lookup = await lookupGuestTable(tableCode, "business");
  if (lookup.kind === "not_found") {
    notFound();
  }

  const headerStore = await headers();
  const raw = headerStore.get("x-payverge-locale") ?? defaultLocale;
  const locale: StorefrontLocale = isGuestLocale(raw) ? raw : defaultLocale;
  const initialMessages = await loadGuestMessages(locale);

  return (
    <CustomerAuthShell>
      <GuestTranslationProvider
        businessId={undefined}
        initialLanguage={locale}
        initialMessages={initialMessages}
        preferInitialLanguage
      >
        {/* Route sits outside (shop); root SkipToMainLink needs this target. */}
        <main id="main-content" tabIndex={-1}>
          {children}
        </main>
      </GuestTranslationProvider>
    </CustomerAuthShell>
  );
}

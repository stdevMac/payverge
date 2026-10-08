import type { Metadata } from "next";
import { headers } from "next/headers";
import { lookupGuestTable } from "@/lib/guest/lookupGuestTable";
import { guestTablePageMetadata } from "@/lib/guest/guestTableMetadata";
import {
  defaultLocale,
  isGuestLocale,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";

interface BillLayoutProps {
  params: Promise<{ tableCode: string }>;
}

/**
 * PG-19: honest per-page <title> for guest bill pages.
 * Business-scoped when resolvable; never the marketing homepage title and
 * never guest PII (names, bill numbers, table codes as the only signal).
 */
export async function generateMetadata({
  params,
}: BillLayoutProps): Promise<Metadata> {
  const { tableCode } = await params;
  const lookup = await lookupGuestTable(tableCode, "business");
  const businessName = lookup.kind === "found" ? lookup.businessName : null;
  const headerStore = await headers();
  const raw = headerStore.get("x-payverge-locale") ?? defaultLocale;
  const locale: StorefrontLocale = isGuestLocale(raw) ? raw : defaultLocale;
  return guestTablePageMetadata({
    tableCode,
    surface: "bill",
    businessName,
    locale,
  });
}

export default function BillLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return <>{children}</>;
}

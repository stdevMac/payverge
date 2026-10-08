import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { headers } from "next/headers";
import { lookupGuestTable } from "@/lib/guest/lookupGuestTable";
import { fetchGuestTableCatalog } from "@/lib/guest/fetchGuestTableCatalog";
import { guestTablePageMetadata } from "@/lib/guest/guestTableMetadata";
import {
  defaultLocale,
  isGuestLocale,
  type StorefrontLocale,
} from "@/i18n/localeRegistry";

export const dynamic = "force-dynamic";
export const revalidate = 0;

interface MenuLayoutProps {
  params: Promise<{ tableCode: string }>;
  children: React.ReactNode;
}

export async function generateMetadata({
  params,
}: Pick<MenuLayoutProps, "params">): Promise<Metadata> {
  const { tableCode } = await params;
  const lookup = await lookupGuestTable(tableCode, "business");
  const businessName = lookup.kind === "found" ? lookup.businessName : null;
  const headerStore = await headers();
  const raw = headerStore.get("x-payverge-locale") ?? defaultLocale;
  const locale: StorefrontLocale = isGuestLocale(raw) ? raw : defaultLocale;
  return guestTablePageMetadata({
    tableCode,
    surface: "menu",
    businessName,
    locale,
  });
}

export default async function MenuLayout({
  children,
  params,
}: MenuLayoutProps) {
  const { tableCode } = await params;
  const lookup = await lookupGuestTable(tableCode, "business");
  if (lookup.kind === "not_found") {
    notFound();
  }
  const catalog = await fetchGuestTableCatalog(tableCode);
  return (
    <>
      {catalog.dishes.length > 0 ? (
        <section
          data-testid="guest-menu-ssr-catalog"
          className="sr-only"
          aria-hidden="true"
        >
          <h2>{catalog.businessName ? `${catalog.businessName} menu` : "Menu"}</h2>
          <ul>
            {catalog.dishes.map((name) => (
              <li key={name}>{name}</li>
            ))}
          </ul>
        </section>
      ) : null}
      {children}
    </>
  );
}

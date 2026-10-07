import type { Metadata } from "next";
import { getSiteUrl } from "@/config/publicConfig";
import {
  siteOpenGraphDefaults,
  siteTwitterDefaults,
} from "@/lib/seo/openGraphImages";
import { guestOgLocale, guestPublicPath, guestPublicUrl } from "@/lib/seo/guestUrls";
import { loadGuestMessages } from "@/i18n/guestMessages.server";
import { defaultLocale, normalizeGuestLangParam } from "@/i18n/localeRegistry";


export type GuestTableSurface = "table" | "menu" | "bill";

function guestTablePagePath(
  tableCode: string,
  surface: GuestTableSurface,
): string {
  const encoded = encodeURIComponent(tableCode);
  if (surface === "menu") return `/t/${encoded}/menu`;
  if (surface === "bill") return `/t/${encoded}/bill`;
  return `/t/${encoded}`;
}

function surfaceLabel(
  messages: Record<string, unknown>,
  surface: GuestTableSurface,
): string {
  const nav = messages.navigation as Record<string, unknown> | undefined;
  const key =
    surface === "menu" ? "menu" : surface === "bill" ? "bill" : "table";
  const value = nav?.[key];
  if (typeof value === "string" && value.trim()) return value;
  return surface === "menu" ? "Menu" : surface === "bill" ? "Bill" : "Table";
}

export async function guestTablePageMetadata(opts: {
  tableCode: string;
  surface: GuestTableSurface;
  businessName: string | null;
  locale?: string;
}): Promise<Metadata> {
  const locale = normalizeGuestLangParam(opts.locale) ?? defaultLocale;
  const messages = await loadGuestMessages(locale);
  const label = surfaceLabel(messages, opts.surface);
  const title = opts.businessName
    ? `${opts.businessName} – ${label} | Payverge`
    : `${label} | Payverge`;
  const description = opts.businessName
    ? opts.surface === "menu"
      ? `Menu at ${opts.businessName}. Order from your table QR.`
      : opts.surface === "bill"
        ? `Table bill at ${opts.businessName}.`
        : `Your table at ${opts.businessName}.`
    : opts.surface === "menu"
      ? "Guest table menu on Payverge."
      : opts.surface === "bill"
        ? "Guest table bill on Payverge."
        : "Guest table on Payverge.";
  const path = guestTablePagePath(opts.tableCode, opts.surface);
  const baseUrl = getSiteUrl();
  const url = guestPublicUrl(locale, path, baseUrl);
  return {
    title,
    description,
    robots: { index: false, follow: false },
    alternates: {
      canonical: url,
      languages: {
        en: `${baseUrl}${guestPublicPath("en", path)}`,
        es: `${baseUrl}${guestPublicPath("es", path)}`,
        "es-AR": `${baseUrl}${guestPublicPath("es-AR", path)}`,
        "x-default": `${baseUrl}${path}`,
      },
    },
    openGraph: siteOpenGraphDefaults({
      url,
      title,
      description,
      locale: guestOgLocale(locale),
    }),
    twitter: siteTwitterDefaults({ title, description }),
  };
}

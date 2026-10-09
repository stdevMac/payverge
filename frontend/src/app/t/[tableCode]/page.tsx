import React from "react";
import nextDynamic from "next/dynamic";
import { notFound } from "next/navigation";
import { Spinner } from "@nextui-org/react";
import { GuestTranslationProvider } from "../../../i18n/GuestTranslationProvider";
import { serializeJsonLd } from "@/lib/seo/jsonLd";
import { lookupGuestTable } from "@/lib/guest/lookupGuestTable";
import { guestTablePageMetadata } from "@/lib/guest/guestTableMetadata";
import { resolveGuestLocaleFromRequest } from "@/i18n/guestLocale.server";
import { loadGuestMessages } from "@/i18n/guestMessages.server";

export const dynamic = "force-dynamic";
export const revalidate = 0;

// Lazy load the heavy GuestTableView component
const GuestTableView = nextDynamic(
  () =>
    import("../../../components/guest/GuestTableView").then((mod) => ({
      default: mod.GuestTableView,
    })),
  {
    loading: () => (
      <div className="flex min-h-[100dvh] items-center justify-center bg-warm-50">
        <Spinner size="lg" color="primary" />
      </div>
    ),
  },
);

interface TablePageProps {
  params: Promise<{ tableCode: string }>;
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}

export default async function TablePage({ params, searchParams }: TablePageProps) {
  const { tableCode } = await params;
  const sp = searchParams ? await searchParams : {};
  const rawLang = Array.isArray(sp.lang) ? sp.lang[0] : sp.lang;
  // PG-21: cookie / Accept-Language so SSR chrome matches hydration.
  const guestLocale = await resolveGuestLocaleFromRequest(rawLang);
  const initialMessages = await loadGuestMessages(guestLocale.locale);
  const lookup = await lookupGuestTable(tableCode);
  if (lookup.kind === "not_found") {
    notFound();
  }
  const businessName =
    lookup.kind === "found" ? lookup.businessName || "" : "";

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "Restaurant",
    name: businessName || "Payverge Partner Restaurant",
    makesOffer: {
      "@type": "Offer",
      name: "Interactive Digital Menu & AI Waiter Ordering via Payverge",
    },
  };

  return (
    <GuestTranslationProvider
      businessId={undefined}
      initialLanguage={guestLocale.locale}
      initialMessages={initialMessages}
      preferInitialLanguage={guestLocale.isExplicit}
    >
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: serializeJsonLd(jsonLd) }}
      />
      <GuestTableView tableCode={tableCode} />
    </GuestTranslationProvider>
  );
}

export async function generateMetadata({ params }: TablePageProps) {
  const { tableCode } = await params;
  const lookup = await lookupGuestTable(tableCode);
  const businessName =
    lookup.kind === "found" ? lookup.businessName : null;
  const guestLocale = await resolveGuestLocaleFromRequest(undefined);
  return guestTablePageMetadata({
    tableCode,
    surface: "table",
    businessName,
    locale: guestLocale.locale,
  });
}

import { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { Suspense } from "react";
import SpaceScanClient from "./SpaceScanClient";
import {
  GUEST_LOCALE_COOKIE,
  resolveGuestRequestLocale,
} from "@/i18n/guestLocaleResolver";

const SPACE_SCAN_TITLES: Record<string, string> = {
  en: "Space scan — Payverge",
  es: "Escaneo de espacio — Payverge",
  "es-AR": "Escaneo de espacio — Payverge",
};

export async function generateMetadata(): Promise<Metadata> {
  const jar = await cookies();
  const hdrs = await headers();
  const resolved = resolveGuestRequestLocale({
    guestCookie: jar.get(GUEST_LOCALE_COOKIE)?.value,
    acceptLanguage: hdrs.get("accept-language"),
  });
  return {
    title: SPACE_SCAN_TITLES[resolved.locale] ?? SPACE_SCAN_TITLES.en,
    robots: { index: false, follow: false },
  };
}

export default async function SpaceScanPage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const resolved = await params;
  const token = decodeURIComponent(resolved.token || "");

  return (
    <Suspense
      fallback={
        <div className="flex min-h-[50vh] items-center justify-center text-sm text-ink-500">
          Loading scan…
        </div>
      }
    >
      <SpaceScanClient token={token} />
    </Suspense>
  );
}

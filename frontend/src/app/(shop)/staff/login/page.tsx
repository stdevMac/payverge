import { Metadata } from "next";
import { headers } from "next/headers";
import StaffLoginClient from "./StaffLoginClient";
import { getTranslation } from "@/i18n/getTranslation";
import { resolveOperatorLocale } from "@/i18n/htmlLangDir";
import {
  pageOpenGraphFromCanonical,
  siteTwitterDefaults,
} from "@/lib/seo/openGraphImages";
import { publicPageAlternates } from "@/i18n/publicPageRoutes";

export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const locale = resolveOperatorLocale(
    requestHeaders.get("x-payverge-locale"),
  );
  const title = getTranslation("staffLogin.metaTitle", locale) as string;
  const description = getTranslation("staffLogin.subtitle", locale) as string;
  const alternates = publicPageAlternates(locale, "/staff/login");
  return {
    title,
    description,
    alternates,
    robots: { index: false, follow: true },
    openGraph: pageOpenGraphFromCanonical({
      title,
      description,
      url: String(alternates.canonical),
      locale,
    }),
    twitter: siteTwitterDefaults({ title, description }),
  };
}

export default function StaffLoginPage() {
  return <StaffLoginClient />;
}

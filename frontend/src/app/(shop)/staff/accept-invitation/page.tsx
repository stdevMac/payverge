import { Metadata } from "next";
import { headers } from "next/headers";
import AcceptInvitationClient from "./AcceptInvitationClient";
import { getTranslation } from "@/i18n/getTranslation";
import { resolveOperatorLocale } from "@/i18n/htmlLangDir";

// #881: the tab title stayed "Accept invitation — Payverge" for a Spanish
// staffer even though the page body was translated. Same operator-locale
// header the sibling /staff/login page reads.
export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const locale = resolveOperatorLocale(requestHeaders.get("x-payverge-locale"));
  return {
    title: getTranslation("staffInvitation.metaTitle", locale) as string,
    description: getTranslation(
      "staffInvitation.metaDescription",
      locale,
    ) as string,
    robots: { index: false, follow: true },
  };
}

export default function AcceptInvitationPage() {
  return <AcceptInvitationClient />;
}

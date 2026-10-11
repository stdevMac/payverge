import type { Metadata } from "next";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import GenericLegalTemplate from "@/components/legal/GenericLegalTemplate";
import { resolveOperatorLocale } from "@/i18n/htmlLangDir";
import {
  externalLegalUrl,
  genericLegalMetadata,
} from "@/lib/instance/legalPage";
import { getServerInstanceInfo } from "@/lib/instance/serverInstance";

export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const locale = resolveOperatorLocale(requestHeaders.get("x-payverge-locale"));
  return genericLegalMetadata("terms", locale, await getServerInstanceInfo());
}

export default async function TermsPage() {
  const instance = await getServerInstanceInfo();
  const external = externalLegalUrl("terms", instance);
  if (external) redirect(external);
  return <GenericLegalTemplate kind="terms" serverInstance={instance} />;
}

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
  return genericLegalMetadata("privacy", locale, await getServerInstanceInfo());
}

export default async function PrivacyPolicyPage() {
  const instance = await getServerInstanceInfo();
  const external = externalLegalUrl("privacy", instance);
  if (external) redirect(external);
  return <GenericLegalTemplate kind="privacy" serverInstance={instance} />;
}

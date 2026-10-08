import { Metadata } from "next";
import { headers } from "next/headers";
import ForgotPasswordClient from "./ForgotPasswordClient";
import { getTranslation } from "@/i18n/getTranslation";
import { resolveOperatorLocale } from "@/i18n/htmlLangDir";

export async function generateMetadata(): Promise<Metadata> {
  const requestHeaders = await headers();
  const locale = resolveOperatorLocale(
    requestHeaders.get("x-payverge-locale"),
  );
  return {
    title: getTranslation("forgotPassword.metaTitle", locale) as string,
    robots: { index: false, follow: true },
  };
}

export default async function ForgotPasswordPage({
  searchParams,
}: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}) {
  const params = searchParams ? await searchParams : {};
  const redirectParam =
    typeof params.redirect === "string" ? params.redirect : undefined;
  return <ForgotPasswordClient redirectParam={redirectParam} />;
}

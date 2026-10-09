"use client";

import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useInstance } from "@/hooks/useInstance";
import { fillLegalTokens } from "@/lib/instance/legalTokens";
import type { InstanceInfo } from "@/lib/instance/instanceInfo";

export type LegalKind = "privacy" | "terms" | "refund";

/**
 * The legal page every install shows unless the operator links its own
 * (LEGAL_TERMS_URL / LEGAL_PRIVACY_URL): a short, clearly labelled generic
 * notice filled from LEGAL_ENTITY / COMPANY_NAME / SUPPORT_EMAIL / PUBLIC_URL. It is
 * not legal advice; the banner tells readers the operator has not published
 * its own document.
 */
export default function GenericLegalTemplate({
  kind,
  serverInstance = null,
}: {
  kind: LegalKind;
  serverInstance?: InstanceInfo | null;
}) {
  const { locale } = useSimpleLocale();
  const { instance: clientInstance } = useInstance();
  const instance = clientInstance ?? serverInstance;
  const notConfigured = getTranslation(
    "legal.template.notConfigured",
    locale,
  ) as string;
  const fill = (text: string) => fillLegalTokens(text, instance, notConfigured);
  const text = (key: string): string => {
    const result = getTranslation(`legal.template.${key}`, locale);
    return fill(Array.isArray(result) ? result.join(" ") : String(result));
  };
  const paragraphs = (() => {
    const result = getTranslation(`legal.template.${kind}.body`, locale);
    return (Array.isArray(result) ? result : [String(result)]).map(fill);
  })();

  return (
    <div className="min-h-screen bg-white">
      <div className="container mx-auto max-w-3xl px-4 py-16 sm:px-6">
        <p
          className="mb-6 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900"
          data-testid="legal-template-banner"
        >
          <strong className="font-semibold">{text("label")}</strong>{" "}
          {text("banner")}
        </p>
        <h1 className="mb-8 font-title text-4xl text-ink-950">
          {text(`${kind}.title`)}
        </h1>
        <div className="space-y-4 text-gray-700">
          {paragraphs.map((p, i) => (
            <p key={i}>{p}</p>
          ))}
        </div>
      </div>
    </div>
  );
}

"use client";

import Link from "next/link";
import Image from "next/image";
import InstanceLogo from "@/components/instance/InstanceLogo";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";
import { localizedPublicHref } from "@/i18n/publicPageRoutes";
import { brandLinks } from "@/config/brand";

export default function NotFoundClient() {
  const { locale } = useSimpleLocale();
  const t = (key: string): string => {
    const result = getChromeTranslation(`notFound.${key}`, locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  return (
    <main
      id="main-content"
      tabIndex={-1}
      className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6"
    >
      <div className="max-w-lg w-full text-center">
        <InstanceLogo
          imgClassName="mx-auto mb-10 opacity-90"
          style={{ width: "192px", height: "auto" }}
          fallback={(alt) => (
            <Image
              src="/images/PayvergeLogo.png"
              alt={alt}
              width={384}
              height={125}
              priority
              style={{ width: "192px", height: "auto" }}
              className="mx-auto mb-10 opacity-90"
            />
          )}
        />
        <p className="text-label uppercase text-ink-500 mb-3">404</p>
        <h1 className="font-title text-display-md text-ink-950 mb-4">
          {t("title")}
        </h1>
        <p className="text-body text-ink-600 mb-8">{t("body")}</p>
        <div className="flex flex-col sm:flex-row gap-3 justify-center">
          <Link
            href={localizedPublicHref(locale, "/")}
            className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
          >
            {t("backToHome")}
          </Link>
          {brandLinks.contactEmail ? (
            <a
              href={`mailto:${brandLinks.contactEmail}`}
              className="inline-flex items-center justify-center rounded-full border border-ink-200 px-6 py-3 text-sm font-semibold text-ink-800 transition-colors hover:bg-ink-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink-400 focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {t("contactSupport")}
            </a>
          ) : null}
        </div>
      </div>
    </main>
  );
}

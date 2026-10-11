"use client";

import React from "react";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";

interface SessionTimeoutWarningProps {
  isExpired: boolean;
  onLogin: () => void;
}

// Only renders when the silent refresh has actually failed — i.e. the
// refresh_token cookie is dead and the user has to sign back in. We no longer
// surface a proactive "session expires soon" toast because the background
// refresh handles renewal silently; a midday popup just confused operators.
export default function SessionTimeoutWarning({
  isExpired,
  onLogin,
}: SessionTimeoutWarningProps) {
  const { locale } = useSimpleLocale();
  const t = (key: string): string => {
    const result = getChromeTranslation(`authModal.sessionExpired.${key}`, locale);
    return typeof result === "string" && result.length > 0 ? result : key;
  };

  if (!isExpired) return null;

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="bg-white rounded-xl shadow-2xl p-6 max-w-sm mx-4 border border-warm-200">
        <h3 className="text-lg font-semibold text-ink-900 mb-2">
          {t("title")}
        </h3>
        <p className="text-sm text-ink-600 mb-4">{t("body")}</p>
        <button
          onClick={onLogin}
          className="w-full px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-lg font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
        >
          {t("loginCta")}
        </button>
      </div>
    </div>
  );
}

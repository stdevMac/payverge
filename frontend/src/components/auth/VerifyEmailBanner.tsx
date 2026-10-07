"use client";

import { useEffect, useState } from "react";
import { Button } from "@nextui-org/react";
import { MailWarning, X } from "lucide-react";
import toast from "react-hot-toast";
import { useAuth } from "@/providers/HybridAuthProvider";
import { authAPI } from "@/api/auth";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";

const DISMISS_KEY = "payverge_verify_banner_dismissed";

/**
 * Slim top banner nudging an authenticated email/password user to verify their
 * email. Signups now stay in the funnel (session issued at registration), so
 * verification is asynchronous — this banner is the standing reminder until it's
 * done. Renders only for an authenticated user session whose email is unverified
 * and that hasn't been dismissed this session.
 */
export default function VerifyEmailBanner() {
  const { isOAuthUser, emailVerified, oauthData } = useAuth();
  const { locale } = useSimpleLocale();
  const [dismissed, setDismissed] = useState(true);
  const [resending, setResending] = useState(false);

  const t = (key: string): string => {
    const result = getChromeTranslation(`authModal.${key}`, locale);
    if (typeof result === "string" && result.length > 0) return result;
    return key;
  };

  // Read the per-session dismissal on mount (sessionStorage is client-only).
  useEffect(() => {
    try {
      setDismissed(sessionStorage.getItem(DISMISS_KEY) === "1");
    } catch {
      setDismissed(false);
    }
  }, []);

  const shouldShow = isOAuthUser && !emailVerified && !dismissed;
  if (!shouldShow) return null;

  const handleDismiss = () => {
    try {
      sessionStorage.setItem(DISMISS_KEY, "1");
    } catch {
      // Non-fatal: banner just won't persist its dismissal.
    }
    setDismissed(true);
  };

  const handleResend = async () => {
    const email = oauthData?.email;
    if (!email) return;
    setResending(true);
    try {
      await authAPI.resendVerification(email);
      toast.success(t("verifyBannerResent"));
    } catch {
      toast.error(t("verifyBannerResendFailed"));
    } finally {
      setResending(false);
    }
  };

  return (
    <div
      role="status"
      className="flex items-center gap-3 border-b border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-900"
    >
      <MailWarning className="h-4 w-4 flex-shrink-0" />
      <span className="flex-1">{t("verifyBannerText")}</span>
      <Button
        type="button"
        size="sm"
        variant="flat"
        className="bg-amber-100 text-amber-900 hover:bg-amber-200"
        isLoading={resending}
        onPress={handleResend}
      >
        {t("verifyBannerResend")}
      </Button>
      <button
        type="button"
        aria-label={t("verifyBannerDismiss")}
        onClick={handleDismiss}
        className="rounded-sm p-1 text-amber-700 hover:bg-amber-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-400"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
}

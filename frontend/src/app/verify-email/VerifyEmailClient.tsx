"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Spinner, Input, Button } from "@nextui-org/react";
import { Mail } from "lucide-react";
import { authAPI } from "@/api/auth";
import { brandLinks } from "@/config/brand";
import { getSearchParam } from "@/utils/nextRouteParams";
import { getApiErrorCode } from "@/utils/apiError";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useAuth } from "@/providers/HybridAuthProvider";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";

function VerifyEmailPageInner() {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const { refreshSession } = useAuth();

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(`verifyEmail.${key}`, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const searchParams = useSearchParams();
  const token = getSearchParam(searchParams, "token") || "";
  const [status, setStatus] = useState<"loading" | "success" | "error">("loading");
  const [message, setMessage] = useState("");
  const [resendEmail, setResendEmail] = useState("");
  const [resendStatus, setResendStatus] = useState<"idle" | "submitting" | "sent">("idle");

  useEffect(() => {
    let cancelled = false;

    async function runVerification() {
      if (!token) {
        if (!cancelled) {
          setStatus("error");
          setMessage(t("messages.missingToken"));
        }
        return;
      }

      try {
        const response = await authAPI.verifyEmail(token);
        if (!cancelled) {
          setStatus("success");
          // Localized copy instead of the backend's English message — the flag
          // tells us which of the two success outcomes to render.
          setMessage(
            response.already_verified
              ? t("messages.alreadyVerified")
              : t("messages.successDefault"),
          );
          // The provider caches emailVerified from init — refresh it so the
          // dashboard's "verify your email" banner clears without a reload.
          void refreshSession();
        }
      } catch (error) {
        if (!cancelled) {
          setStatus("error");
          const code = getApiErrorCode(error);
          setMessage(
            code === "already_verified"
              ? t("messages.alreadyVerified")
              : t("messages.failed"),
          );
        }
      }
    }

    void runVerification();

    return () => {
      cancelled = true;
    };
  }, [token, t, refreshSession]);

  const handleResend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (resendStatus === "submitting") return;
    setResendStatus("submitting");
    try {
      await authAPI.resendVerification(resendEmail);
    } catch {
      // Swallow — server always returns 200; client treats failure as success too
    }
    setResendStatus("sent");
  };

  const label =
    status === "loading"
      ? t("labels.loading")
      : status === "success"
        ? t("labels.success")
        : t("labels.error");

  const heading =
    status === "loading"
      ? t("headings.loading")
      : status === "success"
        ? t("headings.success")
        : t("headings.error");

  const bodyMessage =
    status === "loading" && !message ? t("messages.verifying") : message;

  return (
    <main className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
      <div className="max-w-lg w-full text-center">
        <p className="text-label uppercase text-ink-500 mb-3">{label}</p>
        <h1 className="font-title text-display-md text-ink-950 mb-4">{heading}</h1>
        <p className="text-body text-ink-600 mb-8">{bodyMessage}</p>

        {status === "loading" ? (
          <div className="flex justify-center py-2">
            <Spinner color="current" className="text-brand" />
          </div>
        ) : (
          <div className="flex flex-col sm:flex-row gap-3 justify-center">
            {status === "success" && (
              <Link
                href="/dashboard"
                className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
              >
                {t("actions.continueToDashboard")}
              </Link>
            )}
            <Link
              href="/"
              className={
                status === "success"
                  ? "inline-flex items-center justify-center rounded-full border border-ink-200 px-6 py-3 text-sm font-semibold text-ink-800 transition-colors hover:bg-ink-50"
                  : "inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
              }
            >
              {t("actions.returnHome")}
            </Link>
            {status === "error" && brandLinks.contactEmail && (
              <a
                href={`mailto:${brandLinks.contactEmail}`}
                className="inline-flex items-center justify-center rounded-full border border-ink-200 px-6 py-3 text-sm font-semibold text-ink-800 transition-colors hover:bg-ink-50"
              >
                {t("actions.contactSupport")}
              </a>
            )}
          </div>
        )}

        {status === "error" && (
          <div className="mt-12 max-w-md mx-auto text-left">
            {resendStatus === "sent" ? (
              <p className="text-body text-ink-600 text-center">
                {t("resend.sent")}
              </p>
            ) : (
              <>
                <p className="text-sm text-ink-700 text-center mb-3">
                  {t("resend.prompt")}
                </p>
                <form onSubmit={handleResend} className="space-y-3">
                  <Input
                    type="email"
                    autoComplete="email"
                    label={t("resend.emailLabel")}
                    labelPlacement="outside"
                    placeholder={t("resend.emailPlaceholder")}
                    value={resendEmail}
                    onChange={(e) => setResendEmail(e.target.value)}
                    startContent={<Mail className="w-5 h-5 text-ink-400" />}
                    classNames={{
                      inputWrapper:
                        "border-2 border-ink-200 hover:border-ink-300 bg-white",
                      label: "text-sm font-medium text-ink-700",
                    }}
                    required
                  />
                  <Button
                    type="submit"
                    className="w-full rounded-full bg-brand text-white font-semibold hover:bg-brand-dark transition-colors"
                    size="lg"
                    isLoading={resendStatus === "submitting"}
                  >
                    {t("resend.submit")}
                  </Button>
                </form>
              </>
            )}
          </div>
        )}
      </div>
    </main>
  );
}

export default function VerifyEmailClient() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <VerifyEmailPageInner />
    </Suspense>
  );
}

"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Button } from "@nextui-org/react";
import { Mail, ArrowLeft } from "lucide-react";
import { authAPI } from "@/api/auth";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { buildLoginReturnHref } from "@/components/auth/authRedirect";
import { useInstance } from "@/hooks/useInstance";

export default function ForgotPasswordClient({
  redirectParam,
}: {
  redirectParam?: string;
}) {
  const { locale } = useSimpleLocale();
  // EMAIL_PROVIDER=log sends no email (the link reaches the server log only when EMAIL_LOG_CONTENT=true).
  const emailOff = useInstance().isOff("email");
  const [currentLocale, setCurrentLocale] = useState(locale);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(`forgotPassword.${key}`, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const [email, setEmail] = useState("");
  const [emailError, setEmailError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isSubmitted, setIsSubmitted] = useState(false);
  const loginHref = buildLoginReturnHref(redirectParam, currentLocale);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim()) {
      setEmailError(t("request.emailRequired"));
      return;
    }
    setEmailError("");
    setIsSubmitting(true);

    try {
      await authAPI.requestPasswordReset(email.trim());
      setIsSubmitted(true);
    } catch {
      // Do not reveal if email exists or not
      setIsSubmitted(true);
    } finally {
      setIsSubmitting(false);
    }
  };

  if (isSubmitted) {
    return (
      <div className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
        <div className="max-w-lg w-full text-center">
          <p className="text-label uppercase text-ink-500 mb-3">
            {t("success.label")}
          </p>
          <h1 className="font-title text-4xl md:text-5xl text-ink-950 mb-4">
            {t("success.headline")}
          </h1>
          <p className="text-body text-ink-600 mb-2">
            {t("success.bodyBeforeEmail")}
            <strong>{email}</strong>
            {t("success.bodyAfterEmail")}
          </p>
          <p className="text-sm text-ink-500 mb-8">{t(emailOff ? "success.noEmailHint" : "success.spamHint")}</p>
          <div className="flex flex-col sm:flex-row gap-3 justify-center">
            <Link
              href={loginHref}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
            >
              {t("success.returnToLogin")}
            </Link>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
      <div className="max-w-md w-full">
        <div className="mb-8 text-center">
          <p className="text-label uppercase text-ink-500 mb-3">
            {t("request.label")}
          </p>
          <h1 className="font-title text-4xl md:text-5xl text-ink-950 mb-4">
            {t("request.headline")}
          </h1>
          <p className="text-body text-ink-600">{t("request.subhead")}</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <AccessibleInput
            type="email"
            autoComplete="email"
            label={t("request.emailLabel")}
            placeholder={t("request.emailPlaceholder")}
            value={email}
            onChange={(e) => {
              setEmail(e.target.value);
              if (emailError) setEmailError("");
            }}
            startContent={<Mail className="w-5 h-5 text-ink-400" />}
            classNames={{
              inputWrapper: "border-2 border-ink-200 hover:border-ink-300 bg-white",
            }}
            required
          />
          {emailError ? (
            <p role="alert" className="text-sm text-rose-700">
              {emailError}
            </p>
          ) : null}

          <Button
            type="submit"
            className="w-full rounded-full bg-brand text-white font-semibold hover:bg-brand-dark transition-colors"
            size="lg"
            isLoading={isSubmitting}
          >
            {t("request.submit")}
          </Button>
        </form>

        <div className="mt-6 text-center">
          <Link
            href={loginHref}
            className="inline-flex items-center gap-2 text-sm text-ink-600 hover:text-ink-900 transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
            {t("request.backToLogin")}
          </Link>
        </div>
      </div>
    </div>
  );
}

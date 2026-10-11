"use client";

import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Button } from "@nextui-org/react";
import { ArrowLeft } from "lucide-react";
import { authAPI } from "@/api/auth";
import PasswordField from "@/components/auth/PasswordField";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";
import { getSearchParam } from "@/utils/nextRouteParams";
import { getLocalizedApiError } from "@/utils/apiError";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { buildLoginReturnHref } from "@/components/auth/authRedirect";

function ResetPasswordInner() {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const searchParams = useSearchParams();
  const token = getSearchParam(searchParams, "token") || "";

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(`resetPassword.${key}`, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );
  const loginHref = buildLoginReturnHref(null, currentLocale);

  const [newPassword, setNewPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isSubmitted, setIsSubmitted] = useState(false);
  const [error, setError] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const passwordRef = useRef<HTMLInputElement>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setPasswordError("");
    if (newPassword.length < 8) {
      const message = t("request.passwordTooShort");
      setError(message);
      setPasswordError(message);
      passwordRef.current?.focus();
      return;
    }
    setIsSubmitting(true);
    try {
      await authAPI.resetPassword(token, newPassword);
      setIsSubmitted(true);
    } catch (err) {
      setError(getLocalizedApiError(err, currentLocale));
    } finally {
      setIsSubmitting(false);
    }
  };

  if (!token) {
    return (
      <div className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
        <div className="max-w-lg w-full text-center">
          <p className="text-label uppercase text-ink-500 mb-3">
            {t("error.label")}
          </p>
          <h1 className="font-title text-4xl md:text-5xl text-ink-950 mb-4">
            {t("error.invalidToken")}
          </h1>
          <p className="text-body text-ink-600 mb-8">
            {t("error.invalidTokenHelp")}
          </p>
          <Link
            href="/forgot-password"
            className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
          >
            {t("error.requestNew")}
          </Link>
        </div>
      </div>
    );
  }

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
          <p className="text-body text-ink-600 mb-8">{t("success.body")}</p>
          <Link
            href={loginHref}
            className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark"
          >
            {t("success.signIn")}
          </Link>
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

        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          {error && (
            <div
              role="alert"
              className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800"
            >
              {error}
            </div>
          )}

          <PasswordField
            inputRef={passwordRef}
            autoComplete="new-password"
            label={t("request.newPasswordLabel")}
            value={newPassword}
            onChange={(event) => {
              setNewPassword(event.target.value);
              setPasswordError("");
              setError("");
            }}
            showLabel={t("request.showPassword")}
            hideLabel={t("request.hidePassword")}
            minimumLength={8}
            minimumLengthLabel={t("request.passwordTooShort")}
            isInvalid={Boolean(passwordError)}
            errorMessage={passwordError}
          />

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

export default function ResetPasswordClient() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <ResetPasswordInner />
    </Suspense>
  );
}

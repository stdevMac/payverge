"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";

export default function VerifyEmailError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "VerifyEmailErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Verification error"
      heading="We couldn't verify your email."
      body="Try again, or sign in and resend the verification link from your account."
      primaryCta={{ href: "/dashboard", label: "Go to sign in" }}
      onRetry={reset}
    />
  );
}

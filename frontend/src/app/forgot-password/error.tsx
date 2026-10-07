"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";

export default function ForgotPasswordError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "ForgotPasswordErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Reset error"
      heading="Something went wrong sending your reset link."
      body="Try again in a moment, or request a new reset link."
      primaryCta={{ href: "/forgot-password", label: "Request a new link" }}
      onRetry={reset}
    />
  );
}

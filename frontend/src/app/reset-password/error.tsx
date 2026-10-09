"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";

export default function ResetPasswordError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "ResetPasswordErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Reset error"
      heading="We couldn't reset your password."
      body="Your link may have expired. Try again, or request a new reset link."
      primaryCta={{ href: "/forgot-password", label: "Request a new link" }}
      onRetry={reset}
    />
  );
}

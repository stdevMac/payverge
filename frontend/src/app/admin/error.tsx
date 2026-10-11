"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";

export default function AdminError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "AdminErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Admin error"
      heading="Something went wrong."
      body="We've logged the issue. Try again or head back to the dashboard."
      primaryCta={{ href: "/admin", label: "Back to admin" }}
      secondaryCta={{ href: "/", label: "Home" }}
      onRetry={reset}
    />
  );
}

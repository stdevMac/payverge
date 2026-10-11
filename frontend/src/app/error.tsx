"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "GlobalErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Error"
      heading="Something went wrong."
      body="We've logged the error. Try again in a moment, or head back to home."
      primaryCta={{ href: "/", label: "Back to home" }}
      onRetry={reset}
    />
  );
}

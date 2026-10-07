"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";

export default function BusinessError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "BusinessErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Business error"
      heading="The dashboard hit a snag."
      body="Reload the page to try again. If this keeps happening, contact support."
      primaryCta={{ href: "/business", label: "Back to business" }}
      secondaryCta={{ href: "/", label: "Home" }}
      onRetry={reset}
    />
  );
}

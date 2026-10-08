"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { useErrorBoundaryCopy } from "@/i18n/useErrorBoundaryCopy";
import { logError } from "@/utils/errorLogger";

// Copy is resolved provider-free via useErrorBoundaryCopy() so this boundary
// still localizes when the GuestTranslationProvider tree has crashed.
export default function ScanError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "ScanErrorBoundary", "render", { digest: error.digest });
  }, [error]);

  const copy = useErrorBoundaryCopy();

  return (
    <ErrorBoundaryShell
      label={copy.scanLabel}
      heading={copy.scanTitle}
      body={copy.scanBody}
      primaryCta={{ href: "/scan", label: copy.scanAgain }}
      onRetry={reset}
      retryLabel={copy.retry}
    />
  );
}

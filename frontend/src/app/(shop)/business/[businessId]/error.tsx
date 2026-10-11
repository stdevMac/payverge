"use client";

import { useEffect } from "react";
import ErrorBoundaryShell from "@/components/shared/ErrorBoundaryShell";
import { logError } from "@/utils/errorLogger";
import { VENUES_OVERVIEW_PATH } from "@/utils/businessUrl";

// Operator-facing boundary for the dashboard subtree. Without this file the
// nearest boundary is (shop)/error.tsx, which tells a restaurant owner whose
// console broke that "your cart is safe" and offers "Browse menu".
export default function BusinessIdError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "BusinessIdErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  return (
    <ErrorBoundaryShell
      label="Dashboard error"
      heading="The dashboard hit a snag."
      body="Reload to try again. If this keeps happening, contact support — your data is safe."
      primaryCta={{ href: VENUES_OVERVIEW_PATH, label: "Back to dashboard" }}
      onRetry={reset}
    />
  );
}

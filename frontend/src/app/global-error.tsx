"use client";

import { useEffect } from "react";
import * as Sentry from "@sentry/nextjs";

// Inline styles are intentional — this component renders when the root layout
// (including Tailwind and all providers) is broken, so no CSS framework is
// available.
/* eslint-disable no-restricted-syntax */

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    Sentry.withScope((scope) => {
      scope.setTag("component", "GlobalErrorBoundary");
      scope.setTag("function", "render");

      if (error.digest) {
        scope.setContext("payverge", { digest: error.digest });
      }

      Sentry.captureException(error);
    });
  }, [error]);

  return (
    <html lang="en">
      <body
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: "100vh",
          fontFamily: "system-ui, -apple-system, sans-serif",
          backgroundColor: "#faf9f6",
          color: "#1c1917",
          margin: 0,
        }}
      >
        <div
          style={{ textAlign: "center", maxWidth: "400px", padding: "24px" }}
        >
          <h2 style={{ fontSize: "24px", marginBottom: "8px" }}>
            Something went wrong
          </h2>
          <p style={{ color: "#78716c", marginBottom: "24px" }}>
            We are working to fix this. Please try again.
          </p>
          <button
            onClick={reset}
            style={{
              padding: "10px 24px",
              backgroundColor: "#1a6b6a",
              color: "white",
              border: "none",
              borderRadius: "8px",
              cursor: "pointer",
              fontSize: "14px",
              fontWeight: 500,
            }}
          >
            Try again
          </button>
        </div>
      </body>
    </html>
  );
}

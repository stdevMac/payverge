"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useErrorBoundaryCopy } from "@/i18n/useErrorBoundaryCopy";
import { logError } from "@/utils/errorLogger";

// Copy is resolved provider-free via useErrorBoundaryCopy() so this boundary
// still localizes when the GuestTranslationProvider tree has crashed.
export default function TableError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void logError(error, "TableErrorBoundary", "render", {
      digest: error.digest,
    });
  }, [error]);

  const params = useParams<{ tableCode?: string }>();
  const tableCode = params?.tableCode;
  const copy = useErrorBoundaryCopy();

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-warm-50 px-6 text-center">
      <h1 className="font-serif text-2xl text-ink-900">{copy.title}</h1>
      <p className="max-w-sm text-sm text-ink-600">{copy.tableBody}</p>
      <div className="flex gap-3">
        <button
          type="button"
          onClick={reset}
          className="rounded-xl bg-brand px-5 py-2.5 text-sm font-semibold text-white"
        >
          {copy.retry}
        </button>
        {tableCode && (
          <Link
            href={`/t/${tableCode}`}
            className="rounded-xl border border-warm-200 bg-white px-5 py-2.5 text-sm font-semibold text-ink-700"
          >
            {copy.backToTable}
          </Link>
        )}
      </div>
      {error.digest && (
        <p className="text-xs text-ink-500">
          {copy.errorCode}: {error.digest}
        </p>
      )}
    </div>
  );
}

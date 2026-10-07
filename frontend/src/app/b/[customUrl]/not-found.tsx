"use client";

import Link from "next/link";
import { Compass } from "lucide-react";
import { useErrorBoundaryCopy } from "@/i18n/useErrorBoundaryCopy";

// Client component so useErrorBoundaryCopy can read ?lang= / document.lang
// (server-rendered not-found would always fall back to English). Does not use
// GuestTranslationProvider — same provider-free rule as error boundaries.
export default function StorefrontNotFound() {
  const copy = useErrorBoundaryCopy();

  return (
    <div className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
      <div className="max-w-lg w-full text-center flex flex-col items-center gap-4">
        <div
          aria-hidden
          className="w-14 h-14 rounded-2xl bg-warm-100 border border-warm-200 flex items-center justify-center"
        >
          <Compass className="w-6 h-6 text-ink-500" />
        </div>
        <p className="text-label uppercase text-ink-500">{copy.notFound}</p>
        <h1 className="font-title text-display-md text-ink-950">
          {copy.notFoundTitle}
        </h1>
        <p className="text-body text-ink-600 max-w-md">{copy.notFoundBody}</p>
        <Link
          href="/"
          className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark mt-2"
        >
          {copy.goHome}
        </Link>
      </div>
    </div>
  );
}

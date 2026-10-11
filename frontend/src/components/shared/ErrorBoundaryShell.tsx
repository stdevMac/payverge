"use client";

import Link from "next/link";

interface CtaLink {
  href: string;
  label: string;
}

interface ErrorBoundaryShellProps {
  label?: string;
  heading: string;
  body: string;
  primaryCta?: CtaLink;
  secondaryCta?: CtaLink;
  onRetry?: () => void;
  retryLabel?: string;
}

// NOTE: Error boundaries render when something has already broken, so copy is
// intentionally kept in English rather than depending on the i18n provider. If
// the surrounding tree fails, we still want this shell to render reliably.
export default function ErrorBoundaryShell({
  label = "Error",
  heading,
  body,
  primaryCta,
  secondaryCta,
  onRetry,
  retryLabel = "Try again",
}: ErrorBoundaryShellProps) {
  // Layout rules:
  // - If onRetry is provided, the retry button takes the primary (brand) slot
  //   and primaryCta (if any) is rendered as the secondary link. Explicit
  //   secondaryCta wins over primaryCta in the secondary slot when both exist.
  // - Without onRetry, primaryCta is the brand button and secondaryCta is the
  //   outlined link. If neither is supplied we fall back to a home link.
  const fallbackHomeCta: CtaLink = { href: "/", label: "Back to home" };
  const navPrimary = primaryCta ?? (onRetry ? undefined : fallbackHomeCta);
  const linkedSecondary = onRetry ? secondaryCta ?? primaryCta : secondaryCta;

  return (
    <div className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
      <div className="max-w-lg w-full text-center">
        <p className="text-label uppercase text-ink-500 mb-3">{label}</p>
        <h1 className="font-title text-display-md text-ink-950 mb-4">
          {heading}
        </h1>
        <p className="text-body text-ink-600 mb-8">{body}</p>
        <div className="flex flex-col sm:flex-row gap-3 justify-center">
          {onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {retryLabel}
            </button>
          ) : (
            navPrimary && (
              <Link
                href={navPrimary.href}
                className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
              >
                {navPrimary.label}
              </Link>
            )
          )}
          {linkedSecondary && (
            <Link
              href={linkedSecondary.href}
              className="inline-flex items-center justify-center rounded-full border border-ink-200 px-6 py-3 text-sm font-semibold text-ink-800 transition-colors hover:bg-ink-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ink-400 focus-visible:ring-offset-2 focus-visible:ring-offset-warm-50"
            >
              {linkedSecondary.label}
            </Link>
          )}
        </div>
      </div>
    </div>
  );
}

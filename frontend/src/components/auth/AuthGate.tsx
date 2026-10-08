"use client";

import { ReactNode, Suspense, useEffect, useRef } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Spinner } from "@nextui-org/react";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";
import type { Locale } from "@/i18n/localeRegistry";
import {
  CLIENT_AUTH_RENDER_THROUGH_PREFIXES,
  CLIENT_PROTECTED_PREFIXES,
  CLIENT_PUBLIC_PREFIXES,
  matchesPublicPrefix,
} from "@/utils/publicPaths";
import { buildAuthRedirectUrl } from "./authRedirect";
import { SESSION_HINT_KEY } from "@/utils/refreshAuth";

export function shouldWallSessionError({
  sessionInfoError,
  authenticated,
  hasSessionHint,
}: {
  sessionInfoError: boolean;
  authenticated: boolean;
  hasSessionHint: boolean;
}): boolean {
  if (!sessionInfoError || authenticated) return false;
  // A prior session hint means this is a transport blip, not a logout (#621).
  if (hasSessionHint) return false;
  return true;
}

function readSessionHint(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(SESSION_HINT_KEY) === "1";
  } catch {
    return false;
  }
}

function SessionLoadingSpinner({ locale }: { locale: Locale }) {
  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <Spinner
        size="lg"
        color="primary"
        aria-label={
          (getChromeTranslation("common.loadingSession", locale) as string) ||
          "Loading session"
        }
      />
    </div>
  );
}

/**
 * Gates protected-page rendering on HybridAuthProvider's `isInitialized`
 * signal. Prevents the flash-of-authenticated-shell that happens when a
 * page mounts before session-info returns.
 *
 * Public paths are always allowed through. If the pathname matches a
 * protected prefix and we're initialized but have no authenticated
 * principal, redirect to /dashboard with the current path as the redirect
 * param.
 *
 * Transport failure on session-info (`sessionInfoError`) is NOT treated as
 * anonymous — we show a retriable card so a transient network blip cannot
 * bounce an authenticated operator to /dashboard?redirect=….
 *
 * Inner component holds `useSearchParams` so the export can wrap it in
 * Suspense (Next.js static-render requirement).
 */
function AuthGateInner({ children }: { children: ReactNode }) {
  // Next.js 14 types usePathname() as string | null; coalesce to "/" since
  // root is public and this keeps downstream callers simple.
  const pathname = usePathname() ?? "/";
  const searchParams = useSearchParams();
  const router = useRouter();
  const { locale } = useSimpleLocale();
  const {
    isInitialized,
    isLoading,
    isWeb3User,
    isStaffUser,
    isOAuthUser,
    sessionInfoError,
    retrySessionBootstrap,
  } = useAuth();
  const autoRetriedRef = useRef(false);

  useEffect(() => {
    if (!sessionInfoError || autoRetriedRef.current) return;
    autoRetriedRef.current = true;
    void retrySessionBootstrap();
  }, [sessionInfoError, retrySessionBootstrap]);

  const pub = matchesPublicPrefix(pathname, CLIENT_PUBLIC_PREFIXES);
  const selfGated = matchesPublicPrefix(
    pathname,
    CLIENT_AUTH_RENDER_THROUGH_PREFIXES,
  );
  const isProtected = matchesPublicPrefix(pathname, CLIENT_PROTECTED_PREFIXES);
  const renderThrough = pub || selfGated || !isProtected;
  const authenticated = isWeb3User || isStaffUser || isOAuthUser;

  // Preserve query string so a bounce from e.g. /business/x/dashboard?tab=settings
  // can restore the full destination after login (encode once in buildAuthRedirectUrl).
  const search = searchParams?.toString();
  const redirectTarget = search ? `${pathname}?${search}` : pathname;

  useEffect(() => {
    if (renderThrough) return;
    if (!isInitialized) return;
    // Indeterminate session-info transport failure must not look like logout.
    if (sessionInfoError) return;
    if (!authenticated) {
      router.replace(buildAuthRedirectUrl(redirectTarget));
    }
  }, [
    renderThrough,
    isInitialized,
    sessionInfoError,
    authenticated,
    redirectTarget,
    router,
  ]);

  // Public and self-gated routes render immediately. Self-gated pages still
  // enforce auth in their own UI/data flow; AuthGate just avoids blanking
  // shared chrome before they can render.
  if (renderThrough) return <>{children}</>;

  const hasHint = readSessionHint();
  // Fail-open on a session-info blip when this browser already had a session
  // so CRM/Director never full-page-wall a valid operator (#621).
  if (
    sessionInfoError &&
    (authenticated || hasHint)
  ) {
    return <>{children}</>;
  }

  // Protected paths block until hydration resolves.
  if (!isInitialized || isLoading) {
    return <SessionLoadingSpinner locale={locale} />;
  }

  // Transport failure verifying session — retriable, not anonymous.
  if (shouldWallSessionError({
    sessionInfoError,
    authenticated,
    hasSessionHint: hasHint,
  })) {
    const title =
      (getChromeTranslation("common.sessionCheckFailed", locale) as string) ||
      "We couldn't verify your session";
    const hint =
      (getChromeTranslation("common.sessionCheckFailedHint", locale) as string) ||
      "Check your connection and try again.";
    const retryLabel =
      (getChromeTranslation("common.retry", locale) as string) || "Retry";
    return (
      <div className="flex min-h-[50vh] items-center justify-center px-4">
        <div className="w-full max-w-md rounded-2xl border border-ink-100 bg-white p-8 text-center shadow-sm">
          <h1 className="font-title text-xl text-ink-900 mb-2">{title}</h1>
          <p className="text-sm text-ink-600 mb-6">{hint}</p>
          <button
            type="button"
            onClick={() => {
              void retrySessionBootstrap();
            }}
            className="rounded-full bg-brand px-6 py-2.5 text-sm font-semibold text-white hover:bg-brand-dark transition-colors"
          >
            {retryLabel}
          </button>
        </div>
      </div>
    );
  }

  // Initialized but unauthenticated — the effect above is redirecting.
  // Render nothing to avoid a flash of the protected shell.
  if (!authenticated) return null;

  return <>{children}</>;
}

export default function AuthGate({ children }: { children: ReactNode }) {
  const { locale } = useSimpleLocale();
  return (
    <Suspense fallback={<SessionLoadingSpinner locale={locale} />}>
      <AuthGateInner>{children}</AuthGateInner>
    </Suspense>
  );
}

// app/providers.tsx
"use client";

import { NextUIProvider } from "@nextui-org/react";
import { ThemeProvider } from "@/context/ThemeContext";
import { HybridAuthProvider } from "@/providers/HybridAuthProvider";
import AuthGate from "@/components/auth/AuthGate";
import { ToastProvider } from "@/contexts/ToastContext";
import {
  PwaInstallProvider,
  usePwaInstall,
} from "@/providers/PwaInstallProvider";
import PwaInstallSurface from "@/components/pwa/PwaInstallSurface";
import { usePathname } from "next/navigation";
import { useChromeLocale } from "@/i18n/useChromeLocale";
import { toReactAriaLocale } from "@/components/ui/modalCloseLabel";
import { ModalCloseAriaLocalizer } from "@/components/ui/ModalCloseAriaLocalizer";

// ThemeTransitionEnhancer (a MutationObserver on <html class>) was removed:
// the app is light-only for now, so there's no theme toggle to smooth out.
// Worse, the observer's callback added a class to <html>, and any other code
// touching <html class> (e.g. ThemeProvider's render-time
// classList.remove("dark")) re-triggered the observer, which under prod
// builds combined with React re-renders into a microtask storm that pegged
// the JS thread to 100% on the first nav click — freezing the tab so deeply
// the DevTools console couldn't execute new code. Reintroduce only when
// dark mode actually ships and only as a useEffect-scoped observer that
// ignores its own additions.

const OWNER_DASHBOARD_ROUTE =
  /^\/business\/(?!register(?:\/|$))[^/]+\/dashboard(?:\/|$)/;
const STAFF_HOME_ROUTE = /^\/staff\/home(?:\/|$)/;

function PwaInstallSurfaceForRoute() {
  const pathname = usePathname();
  const { identity } = usePwaInstall();
  const allowFloatingPrompt = Boolean(
    identity &&
      pathname &&
      ((identity.roleType === "owner" && OWNER_DASHBOARD_ROUTE.test(pathname)) ||
        (identity.roleType === "staff" && STAFF_HOME_ROUTE.test(pathname))),
  );

  return <PwaInstallSurface allowFloatingPrompt={allowFloatingPrompt} />;
}

/**
 * F4 / #26: pass chrome locale into NextUI → @react-aria I18nProvider so
 * modal DismissButtons announce "Descartar" after an in-app guest es-AR
 * switch, not only on /es URL prefixes. Must sit under
 * SimpleTranslationProvider (root layout).
 */
function NextUIWithLocale({ children }: { children: React.ReactNode }) {
  const locale = useChromeLocale();
  return (
    <NextUIProvider locale={toReactAriaLocale(locale)}>{children}</NextUIProvider>
  );
}

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider>
      <NextUIWithLocale>
        {/* Single root-level HybridAuthProvider mount: session-info is
            fetched once, cache fingerprint is set once, and AuthGate
            handles the render-gate for all route groups. CustomerAuthProvider
            mounts only through components/customer/CustomerAuthShell on
            diner-facing routes so non-diner pages do not request
            /customer/session-info on every load. */}
        <HybridAuthProvider>
          <PwaInstallProvider>
            <ToastProvider>
              {/* F4: rewrite NextUI's hardcoded Close aria-label under es. */}
              <ModalCloseAriaLocalizer />
              <AuthGate>{children}</AuthGate>
              <PwaInstallSurfaceForRoute />
            </ToastProvider>
          </PwaInstallProvider>
        </HybridAuthProvider>
      </NextUIWithLocale>
    </ThemeProvider>
  );
}

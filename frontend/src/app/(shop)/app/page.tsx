"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { Spinner } from "@nextui-org/react";
import { getMyBusinesses } from "@/api/business";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useAuth } from "@/providers/HybridAuthProvider";
import { usePwaInstall } from "@/providers/PwaInstallProvider";
import type { PwaIdentity } from "@/pwa/installState";
import { resolvePwaLaunchPath } from "@/pwa/launch";
import { browserPlatformSnapshot, detectPwaPlatform } from "@/pwa/platform";
import { trackPwaEvent } from "@/utils/analytics";

interface PendingLaunch {
  key: string;
  identity: PwaIdentity | null;
  promise: Promise<string>;
  standalone: boolean;
  hasResolutionError(): boolean;
}

function isStandaloneLaunch(): boolean {
  try {
    return detectPwaPlatform(browserPlatformSnapshot()) === "installed";
  } catch {
    return false;
  }
}

function safeTrackLaunch(identity: PwaIdentity | null, path: string): void {
  try {
    trackPwaEvent("pwa_standalone_launched", {
      role_type: identity?.roleType ?? "unknown",
      destination: path === "/dashboard" ? "chooser" : "business",
    });
  } catch {
    // Analytics is best effort and must never delay the launch redirect.
  }
}

function safeTrackLaunchResolutionError(
  identity: PwaIdentity | null,
  path: string,
): void {
  try {
    trackPwaEvent("pwa_launch_resolution_error", {
      role_type: identity?.roleType ?? "unknown",
      destination: path === "/dashboard" ? "chooser" : "business",
    });
  } catch {
    // Analytics is best effort and must never delay fallback navigation.
  }
}

export default function AppLaunchPage() {
  const router = useRouter();
  const { isInitialized, isLoading, staffData } = useAuth();
  const { identity, getRememberedDashboardPath } = usePwaInstall();
  const { locale } = useSimpleLocale();
  const pendingLaunchRef = useRef<PendingLaunch | null>(null);
  const completedLaunchKeyRef = useRef<string | null>(null);

  const identityKey = identity?.key ?? null;
  const roleType = identity?.roleType ?? null;
  const staffBusinessId = staffData?.business_id ?? null;
  const staffBusinessSlug = staffData?.business_slug ?? null;

  useEffect(() => {
    if (!isInitialized || isLoading) return;

    const capturedIdentity: PwaIdentity | null =
      identityKey && roleType ? { key: identityKey, roleType } : null;
    const launchKey = capturedIdentity
      ? [
          capturedIdentity.key,
          capturedIdentity.roleType,
          staffBusinessId ?? "",
          staffBusinessSlug ?? "",
        ].join(":")
      : "missing-identity";

    if (completedLaunchKeyRef.current === launchKey) return;

    let pending = pendingLaunchRef.current;
    if (!pending || pending.key !== launchKey) {
      let rememberedPath: string | null = null;
      let resolutionError = capturedIdentity === null;
      if (capturedIdentity?.roleType === "owner") {
        try {
          rememberedPath = getRememberedDashboardPath();
        } catch {
          rememberedPath = null;
          resolutionError = true;
        }
      }

      const promise = capturedIdentity
        ? resolvePwaLaunchPath({
            identity: capturedIdentity,
            rememberedPath,
            staffData:
              staffBusinessId === null
                ? null
                : {
                    business_id: staffBusinessId,
                    business_slug: staffBusinessSlug ?? undefined,
                  },
            loadBusinesses: getMyBusinesses,
            onResolutionError: () => {
              resolutionError = true;
            },
          }).catch(() => {
            resolutionError = true;
            return "/dashboard";
          })
        : Promise.resolve("/dashboard");

      pending = {
        key: launchKey,
        identity: capturedIdentity,
        promise,
        standalone: isStandaloneLaunch(),
        hasResolutionError: () => resolutionError,
      };
      pendingLaunchRef.current = pending;
    }

    let active = true;
    const currentLaunch = pending;
    void currentLaunch.promise.then((path) => {
      if (
        !active ||
        pendingLaunchRef.current !== currentLaunch ||
        completedLaunchKeyRef.current === currentLaunch.key
      ) {
        return;
      }

      completedLaunchKeyRef.current = currentLaunch.key;
      if (currentLaunch.hasResolutionError()) {
        safeTrackLaunchResolutionError(currentLaunch.identity, path);
      }
      if (currentLaunch.standalone) {
        safeTrackLaunch(currentLaunch.identity, path);
      }
      router.replace(path);
    });

    return () => {
      active = false;
    };
  }, [
    getRememberedDashboardPath,
    identityKey,
    isInitialized,
    isLoading,
    roleType,
    router,
    staffBusinessId,
    staffBusinessSlug,
  ]);

  const opening = String(getTranslation("pwa.launch.opening", locale));

  return (
    <section
      role="status"
      aria-live="polite"
      className="flex min-h-[60vh] flex-col items-center justify-center gap-3 text-ink-700"
    >
      <Spinner color="primary" aria-label={opening} />
      <p className="text-sm">{opening}</p>
    </section>
  );
}

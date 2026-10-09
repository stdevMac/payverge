"use client";

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useSimpleLocale } from "@/i18n/OperatorLocaleProvider";
import {
  clearLastDashboardPath,
  derivePwaIdentity,
  getLastDashboardPath,
  isInstallComplete,
  isSnoozed,
  markCardShown,
  markInstallComplete,
  noteDashboardVisit,
  saveLastDashboardPath,
  snoozeForSevenDays,
  storageAvailable,
  wasCardShown,
  type PwaIdentity,
} from "@/pwa/installState";
import {
  browserPlatformSnapshot,
  detectPwaPlatform,
} from "@/pwa/platform";
import {
  trackPwaEvent,
  type PwaAnalyticsEventName,
  type PwaAnalyticsProps,
} from "@/utils/analytics";
import { useAuth } from "./HybridAuthProvider";

export type PwaInstallState =
  | "installed"
  | "native-installable"
  | "manual-install"
  | "unavailable";
export type InstallHelpMode = "manual-install" | "unsupported" | null;

interface DeferredInstallEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{
    outcome: "accepted" | "dismissed";
    platform: string;
  }>;
}

interface RetainedInstallEvent {
  event: DeferredInstallEvent;
  generation: number;
}

export interface PwaInstallContextValue {
  state: PwaInstallState;
  identity: PwaIdentity | null;
  shouldShowFloatingPrompt: boolean;
  helpMode: InstallHelpMode;
  claimFloatingPrompt(): () => void;
  requestInstall(): Promise<void>;
  dismissForSevenDays(): void;
  closeHelp(): void;
  confirmManualInstall(): void;
  recordDashboardVisit(path: string): void;
  getRememberedDashboardPath(): string | null;
}

const PwaInstallContext = createContext<PwaInstallContextValue | null>(null);

const serviceWorkerRegistrations = new WeakMap<
  ServiceWorkerContainer,
  Promise<ServiceWorkerRegistration>
>();

function registerServiceWorkerOnce(
  container: ServiceWorkerContainer,
): Promise<ServiceWorkerRegistration> {
  const existing = serviceWorkerRegistrations.get(container);
  if (existing) return existing;

  const registration = Promise.resolve().then(() =>
    container.register("/sw.js"),
  );
  serviceWorkerRegistrations.set(container, registration);
  return registration;
}

function postOfflineLocale(
  registration: ServiceWorkerRegistration,
  locale: string,
): void {
  const message = { type: "SET_OFFLINE_LOCALE", locale };
  const workers = new Set([
    registration.installing,
    registration.waiting,
    registration.active,
  ]);
  for (const worker of workers) {
    try {
      worker?.postMessage(message);
    } catch {
      // A worker can be replaced between reading the slot and postMessage.
    }
  }
}

function analyticsProps(
  state: PwaInstallState,
  identity: PwaIdentity | null,
): PwaAnalyticsProps {
  const width = typeof window === "undefined" ? 0 : window.innerWidth;
  return {
    platform_path: state,
    device_category:
      width < 768 ? "mobile" : width < 1280 ? "tablet" : "desktop",
    role_type: identity?.roleType ?? "unknown",
  };
}

function safeTrack(
  eventName: PwaAnalyticsEventName,
  props: PwaAnalyticsProps,
): void {
  try {
    trackPwaEvent(eventName, props);
  } catch {
    // Analytics is best effort and must never gate install behavior.
  }
}

export function PwaInstallProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const auth = useAuth();
  const { locale } = useSimpleLocale();
  const identity = useMemo(
    () =>
      auth.isInitialized
        ? derivePwaIdentity({
            isStaffUser: auth.isStaffUser,
            staffId: auth.staffData?.id ?? null,
            staffBusinessId: auth.staffData?.business_id ?? null,
            isOAuthUser: auth.isOAuthUser,
            oauthUserId: auth.oauthData?.userId ?? null,
            isWeb3User: auth.isWeb3User,
            walletAddress: auth.walletAddress,
          })
        : null,
    [
      auth.isInitialized,
      auth.isStaffUser,
      auth.staffData?.id,
      auth.staffData?.business_id,
      auth.isOAuthUser,
      auth.oauthData?.userId,
      auth.isWeb3User,
      auth.walletAddress,
    ],
  );

  const [state, setState] = useState<PwaInstallState>("unavailable");
  const [helpMode, setHelpMode] = useState<InstallHelpMode>(null);
  const [eligibleThisSession, setEligibleThisSession] = useState(false);
  const [cardVisible, setCardVisible] = useState(false);
  const [floatingPromptClaimed, setFloatingPromptClaimed] = useState(false);
  const stateRef = useRef<PwaInstallState>(state);
  const identityRef = useRef<PwaIdentity | null>(null);
  const serviceWorkerFailedRef = useRef(
    typeof navigator === "undefined" ||
      !("serviceWorker" in navigator) ||
      !navigator.serviceWorker,
  );
  const operationGenerationRef = useRef(0);
  const deferredRef = useRef<RetainedInstallEvent | null>(null);
  const previousIdentityKeyRef = useRef<string | null>(null);
  const floatingPromptClaimsRef = useRef(new Set<symbol>());
  const analyticsContextRef = useRef(analyticsProps("unavailable", null));
  stateRef.current = state;

  const setInstallState = useCallback((nextState: PwaInstallState) => {
    stateRef.current = nextState;
    setState(nextState);
  }, []);

  const markServiceWorkerUnavailable = useCallback(() => {
    if (!serviceWorkerFailedRef.current) {
      serviceWorkerFailedRef.current = true;
      operationGenerationRef.current += 1;
    }
    deferredRef.current = null;
    setEligibleThisSession(false);
    setCardVisible(false);
    setHelpMode(null);
    if (stateRef.current !== "installed") {
      setInstallState("unavailable");
    }
  }, [setInstallState]);

  useLayoutEffect(() => {
    identityRef.current = identity;
  }, [identity]);

  useEffect(() => {
    analyticsContextRef.current = analyticsProps(state, identity);
  }, [identity, state]);

  useEffect(() => {
    if (!("serviceWorker" in navigator) || !navigator.serviceWorker) {
      markServiceWorkerUnavailable();
      return;
    }

    const container = navigator.serviceWorker;
    let cancelled = false;
    registerServiceWorkerOnce(container).catch(() => {
      if (cancelled) return;
      markServiceWorkerUnavailable();
      safeTrack(
        "pwa_service_worker_registration_error",
        analyticsContextRef.current,
      );
    });

    return () => {
      cancelled = true;
    };
  }, [markServiceWorkerUnavailable]);

  useEffect(() => {
    if (!("serviceWorker" in navigator) || !navigator.serviceWorker) return;

    const container = navigator.serviceWorker;
    let cancelled = false;
    const sendLocale = () => {
      container.ready
        .then((registration) => {
          if (!cancelled) postOfflineLocale(registration, locale);
        })
        .catch(() => undefined);
    };
    const handleControllerChange = () => sendLocale();

    sendLocale();
    container.addEventListener("controllerchange", handleControllerChange);
    return () => {
      cancelled = true;
      container.removeEventListener("controllerchange", handleControllerChange);
    };
  }, [locale]);

  useEffect(() => {
    const previousIdentityKey = previousIdentityKeyRef.current;
    const currentIdentityKey = identity?.key ?? null;

    if (previousIdentityKey && previousIdentityKey !== currentIdentityKey) {
      clearLastDashboardPath(localStorage, previousIdentityKey);
      operationGenerationRef.current += 1;
      deferredRef.current = null;
      setEligibleThisSession(false);
      setCardVisible(false);
      setHelpMode(null);
    }

    previousIdentityKeyRef.current = currentIdentityKey;
  }, [identity?.key]);

  useEffect(() => {
    const platform = detectPwaPlatform(browserPlatformSnapshot());
    const completed = identity
      ? isInstallComplete(localStorage, identity.key)
      : false;

    if (platform === "installed" || completed) {
      operationGenerationRef.current += 1;
      deferredRef.current = null;
      setInstallState("installed");
      setEligibleThisSession(false);
      setCardVisible(false);
      return;
    }

    if (serviceWorkerFailedRef.current) {
      setInstallState("unavailable");
      return;
    }

    setInstallState(deferredRef.current ? "native-installable" : platform);
  }, [identity, setInstallState]);

  useEffect(() => {
    const beforeInstall = (raw: Event) => {
      if (serviceWorkerFailedRef.current || stateRef.current === "installed") {
        return;
      }
      raw.preventDefault();
      const generation = operationGenerationRef.current + 1;
      operationGenerationRef.current = generation;
      deferredRef.current = {
        event: raw as DeferredInstallEvent,
        generation,
      };
      setInstallState("native-installable");
    };
    const installed = () => {
      const currentIdentity = identityRef.current;
      if (currentIdentity) {
        markInstallComplete(localStorage, currentIdentity.key);
      }
      operationGenerationRef.current += 1;
      deferredRef.current = null;
      setEligibleThisSession(false);
      setCardVisible(false);
      setHelpMode(null);
      setInstallState("installed");
      safeTrack(
        "pwa_appinstalled_observed",
        analyticsProps("installed", currentIdentity),
      );
    };

    window.addEventListener("beforeinstallprompt", beforeInstall);
    window.addEventListener("appinstalled", installed);
    return () => {
      window.removeEventListener("beforeinstallprompt", beforeInstall);
      window.removeEventListener("appinstalled", installed);
    };
  }, [setInstallState]);

  const dismissForSevenDays = useCallback(() => {
    const currentIdentity = identityRef.current;
    const currentState = stateRef.current;
    if (currentIdentity) {
      snoozeForSevenDays(localStorage, currentIdentity.key);
    }
    setEligibleThisSession(false);
    setCardVisible(false);
    safeTrack(
      "pwa_install_card_dismissed",
      analyticsProps(currentState, currentIdentity),
    );
  }, []);

  const requestInstall = useCallback(async () => {
    const currentState = stateRef.current;
    const currentIdentity = identityRef.current;
    safeTrack(
      "pwa_install_action_clicked",
      analyticsProps(currentState, currentIdentity),
    );

    if (currentState === "manual-install") {
      setHelpMode("manual-install");
      safeTrack(
        "pwa_ios_instructions_viewed",
        analyticsProps(currentState, currentIdentity),
      );
      return;
    }

    const retained = deferredRef.current;
    if (currentState !== "native-installable" || !retained) {
      setHelpMode("unsupported");
      return;
    }

    deferredRef.current = null;
    const operationIdentityKey = currentIdentity?.key ?? null;
    const operationIsCurrent = () =>
      operationGenerationRef.current === retained.generation &&
      (identityRef.current?.key ?? null) === operationIdentityKey;

    try {
      await retained.event.prompt();
      if (!operationIsCurrent()) return;
      const choice = await retained.event.userChoice;
      if (!operationIsCurrent()) return;
      const outcome = choice.outcome === "accepted" ? "accepted" : "dismissed";
      safeTrack("pwa_native_prompt_outcome", {
        ...analyticsProps(currentState, currentIdentity),
        outcome,
      });
      setEligibleThisSession(false);
      setCardVisible(false);
      setInstallState("unavailable");
      if (outcome === "dismissed" && currentIdentity) {
        snoozeForSevenDays(localStorage, currentIdentity.key);
      }
    } catch {
      if (!operationIsCurrent()) return;
      setEligibleThisSession(false);
      setCardVisible(false);
      setInstallState("unavailable");
      setHelpMode("unsupported");
    }
  }, [setInstallState]);

  const closeHelp = useCallback(() => setHelpMode(null), []);

  const confirmManualInstall = useCallback(() => {
    const currentIdentity = identityRef.current;
    if (currentIdentity) {
      markInstallComplete(localStorage, currentIdentity.key);
    }
    operationGenerationRef.current += 1;
    deferredRef.current = null;
    setHelpMode(null);
    setEligibleThisSession(false);
    setCardVisible(false);
    setInstallState("installed");
  }, [setInstallState]);

  const recordDashboardVisit = useCallback(
    (path: string) => {
      if (!identity || !storageAvailable(localStorage)) return;
      const validVisit =
        identity.roleType === "staff"
          ? path === "/staff/home"
          : saveLastDashboardPath(localStorage, identity.key, path);
      if (!validVisit) return;

      const returning = noteDashboardVisit(
        localStorage,
        sessionStorage,
        identity.key,
      );
      if (
        returning &&
        !serviceWorkerFailedRef.current &&
        !wasCardShown(sessionStorage, identity.key) &&
        !isSnoozed(localStorage, identity.key)
      ) {
        setEligibleThisSession(true);
      }
    },
    [identity],
  );

  const claimFloatingPrompt = useCallback(() => {
    const claim = Symbol("floating-prompt-claim");
    const claims = floatingPromptClaimsRef.current;
    claims.add(claim);
    setFloatingPromptClaimed(true);

    let released = false;
    return () => {
      if (released) return;
      released = true;
      claims.delete(claim);
      if (claims.size === 0) {
        setFloatingPromptClaimed(false);
        setCardVisible(false);
      }
    };
  }, []);

  const actionable =
    state === "native-installable" || state === "manual-install";

  useEffect(() => {
    if (
      !identity ||
      !eligibleThisSession ||
      !actionable ||
      !floatingPromptClaimed ||
      cardVisible
    ) {
      return;
    }
    if (wasCardShown(sessionStorage, identity.key)) return;
    if (!markCardShown(sessionStorage, identity.key)) return;

    setCardVisible(true);
    safeTrack("pwa_install_card_viewed", analyticsProps(state, identity));
  }, [
    actionable,
    cardVisible,
    eligibleThisSession,
    floatingPromptClaimed,
    identity,
    state,
  ]);

  const shouldShowFloatingPrompt = Boolean(
    identity &&
    floatingPromptClaimed &&
    cardVisible &&
    actionable,
  );

  const getRememberedDashboardPath = useCallback(
    () => (identity ? getLastDashboardPath(localStorage, identity.key) : null),
    [identity],
  );

  const value = useMemo<PwaInstallContextValue>(
    () => ({
      state,
      identity,
      shouldShowFloatingPrompt,
      helpMode,
      claimFloatingPrompt,
      requestInstall,
      dismissForSevenDays,
      closeHelp,
      confirmManualInstall,
      recordDashboardVisit,
      getRememberedDashboardPath,
    }),
    [
      state,
      identity,
      shouldShowFloatingPrompt,
      helpMode,
      claimFloatingPrompt,
      requestInstall,
      dismissForSevenDays,
      closeHelp,
      confirmManualInstall,
      recordDashboardVisit,
      getRememberedDashboardPath,
    ],
  );

  return (
    <PwaInstallContext.Provider value={value}>
      {children}
    </PwaInstallContext.Provider>
  );
}

export function usePwaInstall(): PwaInstallContextValue {
  const value = useContext(PwaInstallContext);
  if (!value) {
    throw new Error("usePwaInstall must be used within PwaInstallProvider");
  }
  return value;
}

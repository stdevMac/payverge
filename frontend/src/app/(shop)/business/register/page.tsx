"use client";

import React, {
  Suspense,
  useState,
  useEffect,
  useRef,
  useCallback,
} from "react";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";
import {
  Card,
  CardBody,
  Button,
  Input,
  Progress,
  Select,
  SelectItem,
  Autocomplete,
  AutocompleteItem,
} from "@nextui-org/react";
import { isBusinessStepComplete, isValidEmail } from "./_stepValidation";
import {
  buildRegistrationDraft,
  splitRegistrationDraft,
  REGISTRATION_DRAFT_STORAGE_KEY,
} from "./_registrationDraft";
import { ArrowLeft, ChevronRight, ChevronLeft } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useAccount } from "wagmi";
import { useUserStore } from "@/store/useUserStore";
import { useAuth } from "@/providers/HybridAuthProvider";
import { getUserProfile } from "@/api/users/profile";
import {
  BUSINESS_TYPE_OPTIONS,
  getDefaultsForCountry,
  defaultRegisterCountryForLocale,
} from "@/lib/geoDefaults";
import { getAllCountryOptions } from "@/lib/isoCountries";
import {
  createBusiness,
  getMyBusinesses,
  type CreateBusinessRequest,
} from "../../../../api/business";
import { AuthModal } from "../../../../components/auth/AuthModal";
import { UserInterface as User } from "@/interface/users/users-interface";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  usePageTracking,
  useClickTracking,
} from "@/hooks/useAnalytics";
import { getSearchParam } from "@/utils/nextRouteParams";
import { localizedErrorMessage } from "@/utils/localizedError";
import {
  getBusinessDashboardPath,
  getBusinessFirstValuePath,
} from "@/utils/businessUrl";
import {
  safeActivationToken,
  trackOptionalActivationEvent,
} from "@/lib/analytics/activationEvents";

// Account/workspace setup: the venue basics, then sign-in when needed.
type FormStep = "business" | "auth";

const STORAGE_KEY = REGISTRATION_DRAFT_STORAGE_KEY;
const WORKSPACE_IDEMPOTENCY_STORAGE_KEY =
  "payverge_workspace_creation_idempotency";

function workspaceIdempotencyKeyForPayload(payload: CreateBusinessRequest): string {
  const canonicalPayload = JSON.stringify(payload);
  try {
    const saved = localStorage.getItem(WORKSPACE_IDEMPOTENCY_STORAGE_KEY);
    if (saved) {
      const parsed = JSON.parse(saved) as { key?: string; payload?: string };
      if (parsed.key && parsed.payload === canonicalPayload) return parsed.key;
    }
    const key =
      typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : `workspace-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    localStorage.setItem(
      WORKSPACE_IDEMPOTENCY_STORAGE_KEY,
      JSON.stringify({ key, payload: canonicalPayload }),
    );
    return key;
  } catch {
    return `workspace-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  }
}

function BusinessRegisterPageInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const registrationStartedAtRef = useRef(Date.now());
  const { address, isConnected } = useAccount();
  const { user } = useUserStore();
  const {
    isOAuthUser: providerOAuthUser,
    isWeb3User: providerWeb3User,
    isStaffUser: _providerStaffUser,
    oauthData,
    isLoading: authProviderLoading,
    isInitialized: authProviderInitialized,
  } = useAuth();

  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = (key: string): string => {
    const result = getTranslation(`businessRegister.${key}`, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  // The escape hatch leads to the operator dashboard ("/" is a venue page
  // now). #720: Spanish operators keep their locale via ?lang=, which the
  // middleware honours on operator routes.
  const homeHref =
    locale === "en" ? "/dashboard" : `/dashboard?lang=${locale.toLowerCase()}`;

  const trustMessages = ((): string[] => {
    const raw = getTranslation("businessRegister.trustMessages", currentLocale);
    return Array.isArray(raw) ? (raw as string[]) : [];
  })();

  // Full ISO country registry, localized + sorted for the current operator
  // locale. Memoized: Intl.DisplayNames construction is not free.
  const allCountryOptions = React.useMemo(
    () => getAllCountryOptions(currentLocale),
    [currentLocale],
  );

  // Analytics tracking
  usePageTracking();
  const trackClick = useClickTracking();

  const [currentStep, setCurrentStep] = useState<FormStep>("business");


  // Persist current funnel position into the URL so OAuth round-trips,
  // reloads, and shared links land the user back on the same step.
  const writeUrl = useCallback(
    (
      next: {
        step?: FormStep;
      },
      mode: "replace" | "push" = "replace",
    ) => {
      const params = new URLSearchParams(searchParams?.toString() ?? "");
      if (next.step !== undefined) params.set("step", next.step);
      const url = `/business/register?${params.toString()}`;
      if (mode === "push") {
        router.push(url);
      } else {
        router.replace(url);
      }
    },
    [router, searchParams],
  );

  // MIN-3: logged-in owners who already have a business should land in the
  // panel, not the register wizard. Opt out with ?new=1 to add another venue.
  useEffect(() => {
    if (!authProviderInitialized || authProviderLoading) return;
    if (!user && !isConnected && !providerOAuthUser) return;
    if (getSearchParam(searchParams, "new") === "1") return;
    let cancelled = false;
    void getMyBusinesses()
      .then((businesses) => {
        if (cancelled || !businesses?.length) return;
        router.replace(getBusinessDashboardPath(businesses[0]));
      })
      .catch(() => {
        // Leave the wizard if listing fails (network / auth edge).
      });
    return () => {
      cancelled = true;
    };
  }, [
    authProviderInitialized,
    authProviderLoading,
    user,
    isConnected,
    providerOAuthUser,
    router,
    searchParams,
  ]);

  // Initialize from URL params
  useEffect(() => {
    const step = getSearchParam(searchParams, "step");
    const validSteps: FormStep[] = ["business", "auth"];

    if (step && (validSteps as string[]).includes(step)) {
      setCurrentStep(step as FormStep);
    } else if (!step) {
      // Browser Back to the bare funnel URL: land on the first step instead
      // of freezing on whatever step state was last pushed.
      setCurrentStep("business");
    }
  }, [searchParams]);

  const stepContextRef = useRef<HTMLDivElement>(null);
  const stepHeadingRef = useRef<HTMLHeadingElement>(null);
  const workspaceCreationInFlightRef = useRef(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAuthModal, setShowAuthModal] = useState(false);
  // Surface the country-required error only after the user has interacted
  // with the Select (avoids error state on first paint).
  const [countryTouched, setCountryTouched] = useState(false);
  const [formData, setFormData] = useState<CreateBusinessRequest>({
    owner_name: "",
    name: "",
    logo: "",
    email: "",
    address: {
      street: "",
      city: "",
      state: "",
      postal_code: "",
      country: defaultRegisterCountryForLocale(locale),
    },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
    business_page_enabled: true,
    show_reviews: true,
    google_reviews_enabled: false,
    counter_enabled: false,
    business_type: "other",
  });

  // Keep server and client text stable during hydration.
  const trustMessageIndex = 0;
  const getTrustMessage = (stepIndex: number) => {
    if (trustMessages.length === 0) return "";
    return trustMessages[(trustMessageIndex + stepIndex) % trustMessages.length];
  };

  // Restore saved form data from localStorage on mount. Mount-only by
  // design so browser navigation cannot replay an old draft over later edits.
  useEffect(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (!saved) return;
      const { address: draftAddress, ...draftFields } = splitRegistrationDraft(
        JSON.parse(saved),
      );
      setFormData((prev) => ({
        ...prev,
        ...draftFields,
        address: { ...prev.address, ...draftAddress },
      }));
    } catch {
      // Private browsing or storage unavailable — ignore
    }
  }, []);

  // Locale-driven country preselect (#912). The useState initializer above is
  // not enough on its own: SimpleTranslationProvider resolves the locale inside
  // an effect, so first paint is often still `en`, and the draft-restore effect
  // right above spreads a stored (possibly empty) country over whatever the
  // initializer picked. This effect is declared AFTER the restore so it runs
  // second on mount, and it re-runs whenever the locale finally settles.
  //
  // It never overrides a real choice: the functional updater re-reads the live
  // state, so a restored draft value wins, and `countryTouched` means the user
  // has picked (or deliberately cleared) the field themselves.
  useEffect(() => {
    if (countryTouched) return;
    const preferred = defaultRegisterCountryForLocale(locale);
    if (!preferred) return;
    setFormData((prev) => {
      if ((prev.address?.country || "").trim()) return prev;
      return { ...prev, address: { ...prev.address, country: preferred } };
    });
  }, [locale, countryTouched]);

  // Save the full funnel draft to localStorage on change (country, city,
  // and business type survive a closed tab — not just
  // the three step-1 text fields).
  useEffect(() => {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify(
          buildRegistrationDraft({ formData }),
        ),
      );
    } catch {
      // Ignore storage errors
    }
  }, [
    formData.name,
    formData.owner_name,
    formData.email,
    formData.address,
    formData.business_type,
    // formData object identity changes with every field edit; the individual
    // deps above keep the write scoped to draft-relevant changes.
    formData,
  ]);

  // Whether the auth step is needed at all. We compute this once here so
  // the progress bar and "Step N of M" label reflect the *visible* path
  // through the form — otherwise an already-authenticated user sees the
  // bar jump from step 2 to step 4 with the auth step never rendered.
  // A bare Wagmi connection (isConnected && address) is NOT a verified
  // principal — there's no SIWE/OAuth/email session behind it yet, so
  // user_id is undefined. isOAuthUser/isWeb3User both already require a
  // resolved backend session (HybridAuthProvider), so gate on those alone;
  // otherwise we'd auto-skip the auth step with no owner behind the request.
  const isAuthenticated = providerOAuthUser || providerWeb3User;

  // Form steps configuration
  const allSteps: { key: FormStep }[] = [
    { key: "business" },
    { key: "auth" },
  ];
  const steps = allSteps.filter(
    (s) => !(isAuthenticated && s.key === "auth"),
  );

  // Auto-skip auth step if already authenticated
  useEffect(() => {
    if (currentStep === "auth" && isAuthenticated) {
      setCurrentStep("business");
      writeUrl({ step: "business" });
    }
  }, [currentStep, isAuthenticated, writeUrl]);

  const currentStepIndex = steps.findIndex((step) => step.key === currentStep);
  const progress = ((currentStepIndex + 1) / steps.length) * 100;

  // Keep the progress + step heading in view on every step, including URL
  // restore (reload / back-forward / OAuth return). Do not gate on a
  // last-step ref: Strict Mode cleanup cancels the rAF after that ref is
  // updated, so the remount would skip the scroll. Move focus to the
  // heading with preventScroll so the first field cannot hide orientation.
  const urlStep = getSearchParam(searchParams, "step");
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      const prefersReducedMotion = window.matchMedia(
        "(prefers-reduced-motion: reduce)",
      ).matches;
      const behavior = prefersReducedMotion ? "auto" : "smooth";
      const context = stepContextRef.current;
      if (typeof context?.scrollIntoView === "function") {
        context.scrollIntoView({ block: "start", behavior });
      }
      const heading = stepHeadingRef.current;
      if (typeof heading?.focus === "function") {
        heading.focus({ preventScroll: true });
      }
    });

    return () => window.cancelAnimationFrame(frame);
  }, [currentStep, urlStep]);

  // Authentication check - driven by HybridAuthProvider
  useEffect(() => {
    if (!authProviderInitialized || authProviderLoading) {
      return;
    }

    if (providerOAuthUser && oauthData?.email) {
      // OAuth user - create temp user from provider data
      if (!user) {
        const tempUser: User = {
          username: oauthData.email.split("@")[0] || "",
          email: oauthData.email,
          address: "",
          role: oauthData.role || "user",
          joined_at: new Date().toISOString(),
          language_selected: "en",
          notifications: [],
        };
        useUserStore.getState().setUser(tempUser);
      }
      setError(null);
      return;
    }

    if (providerWeb3User && isConnected && address) {
      if (!user) {
        // Try to fetch user data if not in store
        const loadUser = async () => {
          try {
            const userData = await getUserProfile(address);
            if (userData) {
              useUserStore.getState().setUser(userData);
            } else {
              const tempUser: User = {
                username: "",
                email: "",
                address: address.toLowerCase(),
                role: "user",
                joined_at: new Date().toISOString(),
                language_selected: "en",
                notifications: [],
              };
              useUserStore.getState().setUser(tempUser);
            }
          } catch {
            const tempUser: User = {
              username: "",
              email: "",
              address: address.toLowerCase(),
              role: "user",
              joined_at: new Date().toISOString(),
              language_selected: "en",
              notifications: [],
            };
            useUserStore.getState().setUser(tempUser);
          }
          setError(null);
        };
        loadUser();
        return;
      }
      setError(null);
      return;
    }

    // Not authenticated - allow page to load (tutorial will show).catch((err) => console.error("loadUser failed:", err));
  }, [
    authProviderInitialized,
    authProviderLoading,
    providerOAuthUser,
    providerWeb3User,
    oauthData,
    isConnected,
    address,
    user,
  ]);

  // Clear error when user successfully logs in
  useEffect(() => {
    if (user && isConnected && error !== null) {
      const hasSignInError = error.indexOf("sign in") !== -1;
      const hasAuthFailedError = error.indexOf("Authentication failed") !== -1;
      if (hasSignInError || hasAuthFailedError) {
        setError(null);
      }
    }
  }, [user, isConnected, error]);

  // Update settlement address when wallet is connected
  useEffect(() => {
    if (address && isConnected) {
      setFormData((prev) => ({
        ...prev,
        settlement_address: address,
        tipping_address: address, // Default to same address, user can change
      }));
    }
  }, [address, isConnected]);

  // Create the workspace directly under the authenticated owner.
  const handleSubmit = async (authenticatedByModal = false) => {
    // Don't act on a submit before the auth provider has resolved — the same
    // guard the auth effect uses.
    if (!authProviderInitialized || authProviderLoading) {
      return;
    }
    // Re-validate at submit time so stale auth/deep-link returns cannot create
    // a workspace with silent USD/UTC defaults.
    if (!isBusinessStepComplete(formData)) {
      setCountryTouched(true);
      setCurrentStep("business");
      writeUrl({ step: "business" });
      return;
    }
    if (!authenticatedByModal && !isAuthenticated) {
      setError(tString("errors.connectWalletOrSignIn"));
      setShowAuthModal(true);
      return;
    }
    if (workspaceCreationInFlightRef.current) return;
    workspaceCreationInFlightRef.current = true;
    setLoading(true);
    setError(null);

    trackClick("registration-submit", "form", "workspace");
    void trackOptionalActivationEvent("registration_started", {
      locale: safeActivationToken(locale || "en", "en"),
      device_class:
        window.innerWidth < 768
          ? "mobile"
          : window.innerWidth < 1024
            ? "tablet"
            : "desktop",
      elapsed_ms: Math.max(0, Date.now() - registrationStartedAtRef.current),
    }).catch(() => undefined);

    const localeDefaults = getDefaultsForCountry(formData.address.country);
    try {
      const businessRequest: CreateBusinessRequest = {
        ...formData,
        business_type: formData.business_type || "other",
        default_currency: localeDefaults.currency,
        display_currency: localeDefaults.currency,
        timezone: localeDefaults.timezone,
        default_language: locale || "en",
        source_language: locale || "en",
        counter_count: 3,
        counter_prefix: "C",
      };
      const business = await createBusiness(businessRequest, {
        idempotencyKey: workspaceIdempotencyKeyForPayload(businessRequest),
      });
      try {
        localStorage.removeItem(STORAGE_KEY);
        localStorage.removeItem(WORKSPACE_IDEMPOTENCY_STORAGE_KEY);
        sessionStorage.removeItem("pendingRegistration");
      } catch {}
      router.push(getBusinessFirstValuePath(business));
    } catch (workspaceError) {
      workspaceCreationInFlightRef.current = false;
      setError(localizedErrorMessage(workspaceError, currentLocale));
      setLoading(false);
    }
  };

  const updateFormData = (field: string, value: any) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  // Step navigation
  const nextStep = () => {
    const currentIndex = steps.findIndex((step) => step.key === currentStep);
    if (currentIndex < steps.length - 1) {
      const nextStepKey = steps[currentIndex + 1].key;
      trackClick(
        `registration-next-step-${currentStep}`,
        "navigation",
        nextStepKey,
      );
      setCurrentStep(nextStepKey);
      writeUrl({ step: nextStepKey }, "push");
    }
  };

  const prevStep = () => {
    const currentIndex = steps.findIndex((step) => step.key === currentStep);
    if (currentIndex > 0) {
      const prevStepKey = steps[currentIndex - 1].key;
      trackClick(
        `registration-prev-step-${currentStep}`,
        "navigation",
        prevStepKey,
      );
      setCurrentStep(prevStepKey);
      writeUrl({ step: prevStepKey }, "push");
    }
  };

  // Form validation for each step
  const isStepValid = (step: FormStep): boolean => {
    switch (step) {
      case "business":
        return isBusinessStepComplete(formData);
      case "auth": {
        // Same rule as the visible-steps guard: a bare wallet connection is
        // not a verified principal, so it cannot satisfy the auth step.
        return !!(providerOAuthUser || providerWeb3User);
      }
      default:
        return true;
    }
  };

  // Show loading screen only during form submission operations
  if (loading) {
    return (
      <div className="relative min-h-screen overflow-hidden bg-warm-50">
        {/* Subtle ambient brand glow — matches landing/register chrome */}
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          <div className="absolute -top-40 -right-40 h-80 w-80 rounded-full bg-brand/10 blur-3xl opacity-50 motion-safe:animate-pulse" />
          <div
            className="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-brand/[0.08] blur-3xl opacity-40 motion-safe:animate-pulse"
            style={{ animationDelay: "2s" }}
          />
        </div>

        <div className="relative z-10 flex min-h-screen items-center justify-center">
          <div className="text-center">
            <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl border border-brand/20 bg-brand-50">
              <div className="h-8 w-8 animate-spin rounded-full border-2 border-brand/20 border-t-brand" />
            </div>
            <p className="text-warm-700 tracking-wide">
              {tString("messages.loading")}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Authentication Modal */}
      <AuthModal
        isOpen={showAuthModal}
        onClose={() => setShowAuthModal(false)}
        defaultTab="signup"
        initialEmail={formData.email || ""}
        redirectUrl="/business/register"
        onSuccess={() => {
          setShowAuthModal(false);
          setError(null);
          void handleSubmit(true);
        }}
      />

      <div className="relative isolate min-h-screen overflow-hidden border-t border-warm-200 bg-warm-50">
        {/* Ambient brand glow so the registration flow feels like marketing
            chrome, not a settings page. Same anchor pattern as the landing
            hero / pricing page. */}
        <div
          aria-hidden
          className="pointer-events-none absolute -top-32 right-[6%] h-[420px] w-[620px] rounded-full bg-brand/10 blur-3xl"
        />
        <div
          aria-hidden
          className="pointer-events-none absolute -bottom-40 left-[10%] h-[360px] w-[520px] rounded-full bg-brand/[0.06] blur-3xl"
        />

        <div className="relative mx-auto max-w-5xl px-4 py-6 sm:px-6 sm:py-10">
          <p className="sr-only">
            {tString("pageTitle") || "Register your business"}
          </p>
          {/* Multi-Step Form */}
          <div className="space-y-6">
            {/* Step Content */}
            <Card className="rounded-2xl border border-warm-200 bg-white shadow-sm shadow-warm-950/[0.04]">
              <CardBody className="p-6 sm:p-8">
                {/* Progress Section */}
                <div
                  ref={stepContextRef}
                  data-testid="register-step-context"
                  className="mb-4 scroll-mt-20"
                >
                  <div className="flex items-center justify-between mb-2">
                    <Link
                      href={homeHref}
                      className="inline-flex h-8 items-center gap-1.5 px-1 text-sm font-medium text-warm-600 transition-colors hover:text-warm-900"
                    >
                      <ArrowLeft size={16} aria-hidden />
                      {tString("messages.backToDashboard")}
                    </Link>
                    <div className="flex items-center gap-3">
                      <span className="text-sm text-warm-600">
                        {tString("messages.stepProgress")
                          .replace(
                            "{current}",
                            (currentStepIndex + 1).toString(),
                          )
                          .replace("{total}", steps.length.toString())}
                      </span>
                      <span className="text-sm font-medium text-warm-950">
                        {Math.round(progress)}%
                      </span>
                    </div>
                  </div>
                  <Progress
                    value={progress}
                    aria-label={tString("progressAria")}
                    color="primary"
                    size="sm"
                    classNames={{
                      track: "bg-warm-100",
                      indicator: "bg-brand",
                    }}
                  />

                  {/* Error Message */}
                  {error && (
                    <div className="mt-2 p-2 bg-red-50 border border-red-200 rounded-lg">
                      <p className="text-red-700 text-sm flex items-center gap-2">
                        <span className="w-2 h-2 bg-red-500 rounded-full"></span>
                        {error}
                      </p>
                    </div>
                  )}
                </div>

                {/* Step 1: Business Information (3 fields) */}
                {currentStep === "business" && (
                  <div className="space-y-8">
                    <div className="mb-4">
                      <h1
                        ref={stepHeadingRef}
                        tabIndex={-1}
                        className="font-title text-3xl text-warm-950 sm:text-4xl outline-none"
                        style={{ letterSpacing: "-0.025em", textWrap: "balance" }}
                      >
                        {tString("businessInfo.title")}
                      </h1>
                      <p className="mt-3 max-w-2xl text-sm leading-relaxed text-warm-600">
                        {getTrustMessage(0)}
                      </p>
                    </div>

                    <div className="max-w-md mx-auto space-y-5">
                      <Input
                        label={tString("businessInfo.fields.name.label")}
                        placeholder={tString(
                          "businessInfo.fields.name.placeholder",
                        )}
                        aria-label={tString("businessInfo.fields.name.label")}
                        autoComplete="organization"
                        value={formData.name}
                        onChange={(e) => updateFormData("name", e.target.value)}
                        isRequired
                        variant="bordered"
                        size="lg"
                        classNames={{
                          inputWrapper: "h-14 border-2 bg-white",
                          label: "font-medium",
                        }}
                      />

                      <Input
                        label={tString("businessInfo.fields.ownerName.label")}
                        placeholder={tString(
                          "businessInfo.fields.ownerName.placeholder",
                        )}
                        aria-label={tString(
                          "businessInfo.fields.ownerName.label",
                        )}
                        autoComplete="name"
                        value={formData.owner_name || ""}
                        onChange={(e) =>
                          updateFormData("owner_name", e.target.value)
                        }
                        isRequired
                        variant="bordered"
                        size="lg"
                        classNames={{
                          inputWrapper: "h-14 border-2 bg-white",
                          label: "font-medium",
                        }}
                      />

                      <Input
                        label={tString("businessInfo.fields.email.label")}
                        placeholder={tString(
                          "businessInfo.fields.email.placeholder",
                        )}
                        aria-label={tString("businessInfo.fields.email.label")}
                        type="email"
                        autoComplete="email"
                        value={formData.email || ""}
                        onChange={(e) =>
                          updateFormData("email", e.target.value)
                        }
                        isRequired
                        variant="bordered"
                        size="lg"
                        isInvalid={
                          !!formData.email && !isValidEmail(formData.email)
                        }
                        errorMessage={
                          formData.email && !isValidEmail(formData.email)
                            ? tString("errors.invalidEmail")
                            : ""
                        }
                        classNames={{
                          inputWrapper: "h-14 border-2 bg-white",
                          label: "font-medium",
                        }}
                      />

                      <Autocomplete
                        label={tString("businessInfo.fields.country.label")}
                        placeholder={tString(
                          "businessInfo.fields.country.placeholder",
                        )}
                        aria-label={tString("businessInfo.fields.country.label")}
                        defaultItems={allCountryOptions}
                        selectedKey={formData.address?.country || null}
                        onSelectionChange={(key) => {
                          setCountryTouched(true);
                          updateFormData("address", {
                            ...formData.address,
                            country: (key as string) || "",
                          });
                        }}
                        onClose={() => setCountryTouched(true)}
                        isRequired
                        isInvalid={
                          countryTouched &&
                          !(formData.address?.country || "").trim()
                        }
                        errorMessage={
                          countryTouched &&
                          !(formData.address?.country || "").trim()
                            ? tString("businessInfo.fields.country.required")
                            : ""
                        }
                        variant="bordered"
                        size="lg"
                        inputProps={{
                          classNames: {
                            inputWrapper: "h-14 border-2 bg-white",
                            label: "font-medium",
                          },
                        }}
                      >
                        {(c) => (
                          <AutocompleteItem key={c.code}>
                            {c.name}
                          </AutocompleteItem>
                        )}
                      </Autocomplete>

                      <Input
                        label={tString("businessInfo.fields.city.label")}
                        placeholder={tString(
                          "businessInfo.fields.city.placeholder",
                        )}
                        aria-label={tString("businessInfo.fields.city.label")}
                        value={formData.address?.city || ""}
                        onChange={(e) =>
                          updateFormData("address", {
                            ...formData.address,
                            city: e.target.value,
                          })
                        }
                        variant="bordered"
                        size="lg"
                        classNames={{
                          inputWrapper: "h-14 border-2 bg-white",
                          label: "font-medium",
                        }}
                      />

                      <Select
                        label={tString(
                          "businessInfo.fields.businessType.label",
                        )}
                        placeholder={tString(
                          "businessInfo.fields.businessType.placeholder",
                        )}
                        aria-label={tString(
                          "businessInfo.fields.businessType.label",
                        )}
                        selectedKeys={[formData.business_type || "other"]}
                        onSelectionChange={(keys) => {
                          updateFormData(
                            "business_type",
                            (Array.from(keys)[0] as string) || "other",
                          );
                        }}
                        isRequired
                        variant="bordered"
                        size="lg"
                        classNames={{ trigger: "h-14 border-2 bg-white" }}
                      >
                        {BUSINESS_TYPE_OPTIONS.map((b) => (
                          <SelectItem key={b.value}>
                            {tString(b.labelKey)}
                          </SelectItem>
                        ))}
                      </Select>

                      {formData.address?.country && (
                        <p className="text-sm leading-relaxed text-warm-600">
                          {tString("businessInfo.localeHint")
                            .replace(
                              "{currency}",
                              getDefaultsForCountry(formData.address.country)
                                .currency,
                            )
                            .replace(
                              "{timezone}",
                              getDefaultsForCountry(formData.address.country)
                                .timezone,
                            )}
                        </p>
                      )}
                    </div>
                  </div>
                )}

                {/* Step 2: Auth */}
                {currentStep === "auth" && (
                  <div className="space-y-8">
                    <div className="mb-4">
                      <h2
                        ref={stepHeadingRef}
                        tabIndex={-1}
                        className="font-title text-3xl text-warm-950 sm:text-4xl outline-none"
                        style={{ letterSpacing: "-0.025em", textWrap: "balance" }}
                      >
                        {tString("authStep.title")}
                      </h2>
                      <p className="mt-3 max-w-2xl text-sm leading-relaxed text-warm-600">
                        {getTrustMessage(1)}
                      </p>
                    </div>

                    <div className="max-w-md mx-auto space-y-4 text-center">
                      <p className="text-warm-700">
                        {tString("authStep.description")}
                      </p>
                      <Button
                        onPress={() => setShowAuthModal(true)}
                        className="h-12 rounded-full bg-brand px-10 font-semibold text-white shadow-lg shadow-brand/25 transition-all hover:bg-brand-dark hover:shadow-xl hover:shadow-brand/30 active:scale-[0.98]"
                        size="lg"
                      >
                        {tString("authStep.cta")}
                      </Button>
                    </div>
                  </div>
                )}
              </CardBody>
              <div className="border-t border-warm-200 px-6 py-4 sm:px-8 sm:py-5">
                <div className="flex flex-col items-stretch justify-between gap-4 sm:flex-row sm:items-center">
                  {/* Hide Back on the first step — a disabled Back is still a
                      competing control on a 0%-conversion funnel. */}
                  {currentStepIndex > 0 ? (
                    <Button
                      variant="bordered"
                      onPress={prevStep}
                      startContent={<ChevronLeft size={18} />}
                      className="h-12 rounded-full border-warm-300 px-6 font-semibold text-warm-800 hover:border-warm-400 hover:bg-warm-50"
                      size="lg"
                    >
                      {tString("navigation.back")}
                    </Button>
                  ) : (
                    <span className="hidden sm:block sm:min-w-[7rem]" aria-hidden />
                  )}

                  <div className="flex w-full flex-col gap-3 sm:w-auto sm:flex-row sm:items-center sm:gap-4">
                    <button
                      type="button"
                      onClick={() => router.back()}
                      className="h-12 rounded-full px-4 text-sm font-medium text-warm-600 transition-colors hover:text-warm-900"
                    >
                      {tString("navigation.cancel")}
                    </button>

                    {currentStepIndex === steps.length - 1 ? (
                      <Button
                        onPress={() => void handleSubmit()}
                        color="primary"
                        isLoading={loading}
                        isDisabled={!isStepValid(currentStep)}
                        size="lg"
                        data-testid="register-primary-cta"
                        className="h-12 rounded-full bg-brand px-8 font-semibold text-white shadow-lg shadow-brand/25 transition-all hover:bg-brand-dark hover:shadow-xl hover:shadow-brand/30 active:scale-[0.98]"
                        endContent={!loading ? <ChevronRight size={18} /> : undefined}
                      >
                        {loading
                          ? tString("messages.loading")
                          : tString("navigation.submit")}
                      </Button>
                    ) : (
                      <Button
                        onPress={nextStep}
                        color="primary"
                        isDisabled={!isStepValid(currentStep)}
                        endContent={<ChevronRight size={18} />}
                        data-testid="register-primary-cta"
                        className="h-12 rounded-full bg-brand px-7 font-semibold text-white shadow-lg shadow-brand/25 transition-all hover:bg-brand-dark hover:shadow-xl hover:shadow-brand/30 active:scale-[0.98]"
                        size="lg"
                      >
                        {tString("navigation.next")}
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            </Card>
          </div>
        </div>
      </div>
    </>
  );
}

export default function BusinessRegisterPage() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <BusinessRegisterPageInner />
    </Suspense>
  );
}

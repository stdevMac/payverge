"use client";

import { useState, useEffect, useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  Button,
  Divider,
} from "@nextui-org/react";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { KeyRound, Mail, User, Loader2 } from "lucide-react";
import { authAPI } from "@/api/auth";
import { apiErrorDetail, getApiErrorData, getLocalizedApiError } from "@/utils/apiError";
import { mapLoginRequestError } from "@/utils/loginRequestError";
import { useRouter } from "next/navigation";
import toast from "react-hot-toast";
import StaffLogin from "@/components/staff/StaffLogin";
import { useAuth } from "@/providers/HybridAuthProvider";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { SESSION_HINT_KEY } from "@/utils/refreshAuth";
import { resolvePostLoginRedirect } from "@/utils/safeRedirect";
import PasswordField from "./PasswordField";
import { buildForgotPasswordHref } from "./authRedirect";
import { useRegistrationMode } from "./useRegistrationMode";
import { isRegistrationClosedError } from "@/api/registrationMode";
import { useInstance } from "@/hooks/useInstance";
import { isPublicDemo } from "@/lib/instance/instanceInfo";
import DemoLoginButtons from "@/components/demo/DemoLoginButtons";
import type { DemoStaff } from "@/api/demo";
import { setStaffData } from "@/utils/staffAuth";
import { getOperatorDashboardPath } from "@/utils/businessUrl";
import { GoogleG } from "@/components/legal-brand-icons/GoogleG";

interface AuthModalProps {
  isOpen: boolean;
  onClose: () => void;
  defaultTab?: "signin" | "signup" | "staff";
  onSuccess?: () => void;
  redirectUrl?: string;
  initialEmail?: string;
}

export function readVerificationError(data: any): {
  requiresVerification: boolean;
  message: string;
  verificationUrl?: string;
} {
  const params = data?.params ?? {};
  const requiresVerification = Boolean(
    params.requires_email_verification ?? data?.requires_email_verification,
  );
  return {
    requiresVerification,
    message: data?.error || "Authentication failed. Please try again.",
    verificationUrl: params.verification_url ?? data?.verification_url,
  };
}

function navigateToVerification(
  router: ReturnType<typeof useRouter>,
  url: string | undefined,
) {
  if (!url) {
    router.push("/verify-email");
    return;
  }
  if (url.startsWith("http://") || url.startsWith("https://")) {
    window.location.href = url;
    return;
  }
  router.push(url);
}

export function AuthModal({ isOpen, onClose, defaultTab = "signin", onSuccess, redirectUrl, initialEmail }: AuthModalProps) {
  // Open-redirect hygiene (#296): never navigate/persist absolute or
  // protocol-relative redirectUrl values from callers or query params.
  const safeRedirectUrl =
    resolvePostLoginRedirect(redirectUrl) ??
    (typeof window === "undefined"
      ? undefined
      : resolvePostLoginRedirect(
          new URLSearchParams(window.location.search).get("redirect"),
        ) ?? undefined);
  const router = useRouter();
  const { refreshStaffData, refreshSession } = useAuth();
  const { locale } = useSimpleLocale();
  const [activeTab, setActiveTab] = useState<"signin" | "signup" | "staff">(defaultTab);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState("");
  // When a sign-in fails because the email is unverified, we surface a dedicated
  // nudge with a "Resend verification email" action instead of a dead-end error.
  const [needsVerification, setNeedsVerification] = useState(false);
  const [resending, setResending] = useState(false);
  // Instance REGISTRATION_MODE: open hides the invite field, invite requires
  // it, closed replaces the signup form with a sign-in-only notice. Unknown
  // (null: loading or unreachable) keeps the invite field optional and lets
  // the backend decide.
  const { mode: probedRegistrationMode, markClosed: markRegistrationClosed } =
    useRegistrationMode(isOpen);
  const { instance, isOff } = useInstance();
  // A closed refusal seen at runtime wins; otherwise GET /instance (seeded on
  // first paint) answers before the dedicated probe does.
  const registrationMode =
    probedRegistrationMode === "closed"
      ? "closed"
      : probedRegistrationMode ?? instance?.registration_mode ?? null;
  const signupClosed = registrationMode === "closed";
  // Invite mode: no public sign-up form. An invite link (?invite_code=)
  // or "I have an invite code" opens it; otherwise the sign-up tab says to ask
  // the administrator, and sign-in offers no sign-up tab at all.
  const [inviteRevealed, setInviteRevealed] = useState(false);
  // No Google button when the instance has no Google OAuth client.
  const showGoogle = !isOff("google_oauth");
  const showInviteField = registrationMode !== "open";
  const inviteRequired = registrationMode === "invite";
  const showSignupClosedNotice = activeTab === "signup" && signupClosed;
  // Google on the sign-in tab cannot create an account in closed mode; say so
  // before a new user is bounced by the OAuth callback.
  const showGoogleExistingOnly =
    showGoogle && activeTab === "signin" && signupClosed;

  // Translation helper — keys live under `authModal.*`. Falls back to the
  // key itself so any new string accidentally added without translation
  // is still readable.
  const t = (key: string): string => {
    const result = getTranslation(`authModal.${key}`, locale);
    if (typeof result === "string" && result.length > 0) return result;
    return key;
  };

  // Form fields
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [inviteCode, setInviteCode] = useState("");
  const inviteOnly = registrationMode === "invite";
  const signupFormAvailable =
    !inviteOnly || inviteRevealed || inviteCode.trim() !== "";
  const showInviteOnlyNotice =
    activeTab === "signup" && inviteOnly && !signupFormAvailable;
  const showTabSwitcher = !signupClosed && signupFormAvailable;
  const passwordRef = useRef<HTMLInputElement>(null);


  // Reset form when modal opens/closes or tab changes
  useEffect(() => {
    if (isOpen) {
      setActiveTab(defaultTab);
      setError("");
      setNeedsVerification(false);
      const params = new URLSearchParams(window.location.search);
      const linkedInvite = params.get("invite_code") ?? "";
      if (linkedInvite) setInviteCode(linkedInvite);
    }
  }, [isOpen, defaultTab]);

  useEffect(() => {
    setError("");
    setNeedsVerification(false);
  }, [activeTab]);

  useEffect(() => {
    if (initialEmail) {
      setEmail(initialEmail);
    }
  }, [initialEmail]);

  const hydrateAfterEmailAuth = async (): Promise<boolean> => {
    try {
      localStorage.setItem(SESSION_HINT_KEY, "1");
    } catch {
      // Storage unavailable — session-info still hydrates this tab.
    }
    try {
      return (await refreshSession()) === true;
    } catch {
      return false;
    }
  };

  const handleEmailAuth = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setNeedsVerification(false);
    setIsLoading(true);

    try {
      if (activeTab === "signup") {
        // Validation
        if (password.length < 8) {
          setError(t("passwordTooShort"));
          passwordRef.current?.focus();
          setIsLoading(false);
          return;
        }

        const response = await authAPI.register({
          email,
          password,
          name,
          invite_code: inviteCode.trim() || undefined,
          language: locale,
        });
        if (response.success && response.token) {
          // The backend now issues a full session at registration, so the
          // signup stays in the paid funnel instead of being ejected to the
          // inbox. Email verification is async: surface a "keep going, verify
          // later" notice but proceed as authenticated.
          setPassword("");
          // P0-1: the backend just issued the session cookie, but on public
          // surfaces the provider stays anonymous until told. Persist the
          // session hint and hydrate the provider BEFORE handing control
          // back, so funnel guards (register checkout) see the principal.
          // False or thrown hydration must not look like a completed login.
          const hydrated = await hydrateAfterEmailAuth();
          if (!hydrated) {
            setError(t("genericAuthError"));
            return;
          }
          // Email off on this install: no mail was sent, so promise none.
          if (response.verification_required && !isOff("email")) {
            toast.success(
              t("verificationSentKeepGoing").replace("{{email}}", email),
            );
          } else {
            toast.success(t("accountCreated"));
          }
          onClose();
          if (onSuccess) {
            onSuccess();
          } else {
            router.push(safeRedirectUrl || "/dashboard");
            router.refresh();
          }
          return;
        }
        if (response.verification_required) {
          // Legacy fallback: a backend that still withholds the token on an
          // unverified signup — keep the old verify-email hand-off.
          toast.success(response.message || t("accountCreatedVerify"));
          setPassword("");
          onClose();
          navigateToVerification(router, response.verification_url);
          return;
        }
        // 200 with success:false / no token throws no error — surface a
        // generic failure so the user isn't left staring at an inert modal.
        setError(t("genericAuthError"));
      } else {
        const response = await authAPI.login({ email, password });
        if (response.success && response.token) {
          // P0-1: same provider hydration as the signup branch — a returning
          // user signing in mid-funnel must not be bounced back to the modal.
          // Do not toast or navigate until the principal is actually present.
          const hydrated = await hydrateAfterEmailAuth();
          if (!hydrated) {
            setError(t("genericAuthError"));
            return;
          }
          toast.success(t("welcomeBack"));
          onClose();
          if (onSuccess) {
            onSuccess();
          } else {
            router.push(safeRedirectUrl || "/dashboard");
            router.refresh();
          }
        } else {
          setError(t("genericAuthError"));
        }
      }
    } catch (err: unknown) {
      const responseData = getApiErrorData(err);
      if (activeTab === "signup" && isRegistrationClosedError(responseData)) {
        // The operator closed signup after this page loaded: switch to the
        // sign-in-only notice instead of a dead-end error.
        markRegistrationClosed();
        return;
      }
      const verification = readVerificationError(responseData);
      if (verification.requiresVerification) {
        // Verification flow carries a specific backend message; surface it with
        // an inline "Resend verification email" action instead of collapsing to
        // a generic failure or bouncing the operator off to another page.
        setError(verification.message);
        setNeedsVerification(true);
        return;
      }
      // Ordinary auth failure: localize via the structured backend `code`
      // (apiErrors.json), falling back to the raw backend string, then a
      // generic localized message — in the operator's active locale.
      // Transport / CORS / no-response: one honest reachability message.
      // The login request skips the interceptor toast so this is the only surface.
      setError(
        mapLoginRequestError(
          err,
          getLocalizedApiError(err, locale),
          t("networkError"),
        ),
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleStaffLoginSuccess = async (staffData: any) => {
    try {
      await refreshStaffData();
      new Promise((resolve) => setTimeout(resolve, 100));
      onClose();
      if (onSuccess) {
        onSuccess();
      } else if (staffData.business_id) {
        router.push(getOperatorDashboardPath(String(staffData.business_slug || staffData.business_id)));
      } else {
        router.push("/dashboard");
      }
    } catch (error) {
      console.error("Failed to refresh staff auth data:", error);
      onClose();
      if (staffData.business_id) {
        router.push(getOperatorDashboardPath(String(staffData.business_slug || staffData.business_id)));
      } else {
        router.push("/dashboard");
      }
    }
  };

  // Public demo (DEMO_MODE): the server already set the session cookies.
  // Like the email sign-in, a host that passes onSuccess (the dashboard's own
  // sign-in gate) decides what happens next: pushing to the route it already
  // renders would leave its "signed out" state in place.
  const handleDemoOwnerSignedIn = async (redirect: string) => {
    await hydrateAfterEmailAuth();
    onClose();
    if (onSuccess) {
      onSuccess();
      return;
    }
    const target = redirect || "/dashboard";
    // Already there (the header's modal over the dashboard's sign-in gate):
    // a client push is a no-op, so load the page again with the new session.
    if (window.location.pathname === target) {
      window.location.reload();
      return;
    }
    router.push(target);
    router.refresh();
  };

  const handleDemoStaffSignedIn = async (staff: DemoStaff) => {
    setStaffData({ ...staff, is_active: true });
    await handleStaffLoginSuccess(staff);
  };

  const handleGoogleAuth = async () => {
    setIsLoading(true);
    setError("");
    try {
      // Store only a safe internal redirect before OAuth redirect (#296).
      if (safeRedirectUrl) {
        localStorage.setItem("auth_redirect_url", safeRedirectUrl);
      }
      
      const response = await authAPI.getGoogleAuthURL(
        activeTab === "signup" ? inviteCode : undefined,
      );
      if (response.url) {
        // Redirect to Google OAuth
        window.location.href = response.url;
      } else {
        setError(t("googleNotConfigured"));
      }
    } catch (err: unknown) {
      setError(apiErrorDetail(err) || t("googleInitError"));
    } finally {
      setIsLoading(false);
    }
  };

  const handleResendVerification = async () => {
    if (!email || resending) return;
    setResending(true);
    try {
      const res = await authAPI.resendVerification(email);
      toast.success(res.message || t("verificationResent"));
    } catch {
      toast.error(t("verificationResendError"));
    } finally {
      setResending(false);
    }
  };

  const closeLabel =
    activeTab === "signin"
      ? t("closeSignInDialog")
      : activeTab === "signup"
      ? t("closeSignUpDialog")
      : t("closeStaffDialog");

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      size="md"
      placement="center"
      aria-label={closeLabel}
      classNames={{
        backdrop: "bg-black/50 backdrop-blur-sm",
        base: "border border-gray-200 bg-white",
        header: "border-b border-gray-200",
        body: "py-6",
        closeButton: "hover:bg-gray-100 active:bg-gray-200",
      }}
      closeButton={
        <button
          type="button"
          onClick={onClose}
          aria-label={closeLabel}
          className="absolute right-3 top-3 z-10 rounded-full p-1 hover:bg-gray-100"
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      }
    >
      <ModalContent>
        {() => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <h2 className="text-xl tracking-wide text-gray-900">
                {activeTab === "signin" ? t("welcomeBack")
                 : showSignupClosedNotice ? t("registrationClosedTitle")
                 : showInviteOnlyNotice ? t("inviteOnlyTitle")
                 : activeTab === "signup" ? t("getStarted")
                 : ""}
              </h2>
              <p className="text-sm text-gray-700">
                {activeTab === "signin"
                  ? t("signInSubtitle")
                  : showSignupClosedNotice || showInviteOnlyNotice
                  ? ""
                  : activeTab === "signup"
                  ? t("signUpSubtitle")
                  : ""}
              </p>
            </ModalHeader>
            <ModalBody>
              {activeTab === "signin" && isPublicDemo(instance) ? (
                <DemoLoginButtons
                  onOwnerSignedIn={handleDemoOwnerSignedIn}
                  onStaffSignedIn={handleDemoStaffSignedIn}
                />
              ) : null}
              {activeTab === "staff" ? (
                <StaffLogin isModal onLoginSuccess={handleStaffLoginSuccess} />
              ) : (
                <>
                  {/* Tab Switcher (sign-in only when signup is closed) */}
              {showTabSwitcher && (
              <div className="flex gap-2 mb-4">
                <Button
                  variant={activeTab === "signin" ? "flat" : "light"}
                  className={`flex-1 ${
                    activeTab === "signin"
                      ? "!bg-gray-900 !text-white"
                      : "text-gray-700"
                  }`}
                  onPress={() => setActiveTab("signin")}
                >
                  {t("signIn")}
                </Button>
                <Button
                  variant={activeTab === "signup" ? "flat" : "light"}
                  className={`flex-1 ${
                    activeTab === "signup"
                      ? "!bg-gray-900 !text-white"
                      : "text-gray-700"
                  }`}
                  onPress={() => setActiveTab("signup")}
                >
                  {t("signUp")}
                </Button>
              </div>
              )}

              {showSignupClosedNotice ? (
                <div role="status" className="space-y-4">
                  <p className="text-sm text-gray-700">
                    {t("registrationClosedBody")}
                  </p>
                  <Button
                    className="w-full bg-brand text-white font-medium hover:bg-brand-dark"
                    size="lg"
                    onPress={() => setActiveTab("signin")}
                  >
                    {t("signInInstead")}
                  </Button>
                </div>
              ) : showInviteOnlyNotice ? (
                <div role="status" className="space-y-4">
                  <p className="text-sm text-gray-700">{t("inviteOnlyBody")}</p>
                  <Button
                    className="w-full bg-brand text-white font-medium hover:bg-brand-dark"
                    size="lg"
                    onPress={() => setActiveTab("signin")}
                  >
                    {t("signInInstead")}
                  </Button>
                  <Button
                    variant="light"
                    className="w-full text-gray-700"
                    onPress={() => setInviteRevealed(true)}
                  >
                    {t("haveInviteCode")}
                  </Button>
                </div>
              ) : (
              <>

              {/* OAuth Buttons */}
              {showGoogle && (
              <>
              <div className="space-y-3 mb-4">
                <Button
                  className="w-full bg-white border-2 border-gray-200 hover:border-gray-300 hover:bg-gray-50 text-gray-700 font-medium"
                  size="lg"
                  onPress={handleGoogleAuth}
                  isDisabled={isLoading}
                  aria-describedby={showGoogleExistingOnly ? "auth-google-existing-only" : undefined}
                >
                  <GoogleG />
                  {t("continueWithGoogle")}
                </Button>
                {showGoogleExistingOnly && (
                  <p id="auth-google-existing-only" className="text-xs text-gray-500 text-center">
                    {t("googleExistingAccountsOnly")}
                  </p>
                )}
              </div>

              <div className="flex items-center gap-4 my-4">
                <Divider className="flex-1" />
                <span className="text-gray-400 text-sm">{t("or")}</span>
                <Divider className="flex-1" />
              </div>
              </>
              )}

              {/* Error Message */}
              {error && !needsVerification && (
                <div role="alert" className="bg-rose-50 border border-rose-200 text-rose-700 px-4 py-3 rounded-xl text-sm mb-4">
                  {error}
                </div>
              )}

              {/* Email-not-verified nudge — specific message + resend action */}
              {needsVerification && (
                <div className="mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
                  <p>{error || t("emailNotVerified")}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="flat"
                    className="mt-2 bg-amber-100 text-amber-900 hover:bg-amber-200"
                    isLoading={resending}
                    onPress={handleResendVerification}
                  >
                    {resending ? t("resendingVerification") : t("resendVerification")}
                  </Button>
                </div>
              )}

              {/* Email/Password Form */}
              <form onSubmit={handleEmailAuth} className="space-y-4">
                {activeTab === "signup" && (
                  <>
                    <AccessibleInput
                      type="text"
                      label={t("fullName")}
                      placeholder={t("enterName")}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      startContent={<User className="w-5 h-5 text-gray-400" />}
                      autoComplete="name"
                      classNames={{
                        inputWrapper: "border-2 border-gray-200 hover:border-gray-300 bg-white",
                      }}
                      required
                    />
                    {showInviteField && (
                      <AccessibleInput
                        type="text"
                        label={t("inviteCode")}
                        placeholder={t("enterInviteCode")}
                        description={inviteRequired ? t("inviteCodeHint") : undefined}
                        value={inviteCode}
                        onChange={(e) => setInviteCode(e.target.value)}
                        startContent={<KeyRound className="w-5 h-5 text-gray-400" />}
                        autoComplete="off"
                        classNames={{
                          inputWrapper: "border-2 border-gray-200 hover:border-gray-300 bg-white",
                        }}
                        required={inviteRequired}
                      />
                    )}
                  </>
                )}

                <AccessibleInput
                  type="email"
                  label={t("email")}
                  placeholder={t("enterEmail")}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  startContent={<Mail className="w-5 h-5 text-gray-400" />}
                  autoComplete="email"
                  classNames={{
                    inputWrapper: "border-2 border-gray-200 hover:border-gray-300 bg-white",
                  }}
                  required
                />

                <PasswordField
                  label={t("password")}
                  placeholder={
                    activeTab === "signup"
                      ? t("createPasswordPlaceholder")
                      : t("enterPassword")
                  }
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete={activeTab === "signup" ? "new-password" : "current-password"}
                  showLabel={t("showPassword")}
                  hideLabel={t("hidePassword")}
                  inputRef={passwordRef}
                  minimumLength={activeTab === "signup" ? 8 : undefined}
                  minimumLengthLabel={
                    activeTab === "signup" ? t("passwordRequirement") : undefined
                  }
                />

                {/* Password reset is delivered by email: no email, no promise. */}
                {activeTab === "signin" && !isOff("email") && (
                  <div className="flex justify-end">
                    <button
                      type="button"
                      onClick={() => {
                        onClose();
                        router.push(buildForgotPasswordHref(safeRedirectUrl));
                      }}
                      className="text-sm text-gray-700 hover:text-gray-700"
                    >
                      {t("forgotPassword")}
                    </button>
                  </div>
                )}

                <Button
                  type="submit"
                  className="w-full bg-brand text-white font-medium hover:bg-brand-dark"
                  size="lg"
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <Loader2 className="w-5 h-5 animate-spin" />
                  ) : activeTab === "signin" ? (
                    t("signIn")
                  ) : (
                    t("createAccount")
                  )}
                </Button>
              </form>

              {activeTab === "signin" && inviteOnly && !signupFormAvailable && (
                <p className="mt-3 text-sm text-center text-gray-700">
                  {t("inviteOnlySignInHint")}{" "}
                  <button
                    type="button"
                    className="font-medium text-brand hover:underline"
                    onClick={() => {
                      setInviteRevealed(true);
                      setActiveTab("signup");
                    }}
                  >
                    {t("haveInviteCode")}
                  </button>
                </p>
              )}

              <Divider className="my-4" />

              {/* Info text */}
              <p className="text-xs text-center text-gray-700">
                {t("agreementPrefix")}{" "}
                <a href="/terms-and-conditions" className="text-gray-700 hover:underline">
                  {t("termsOfService")}
                </a>{" "}
                {t("agreementConnector")}{" "}
                <a href="/privacy-policy" className="text-gray-700 hover:underline">
                  {t("privacyPolicy")}
                </a>
              </p>
              </>
              )}
                </>
              )}
            </ModalBody>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}

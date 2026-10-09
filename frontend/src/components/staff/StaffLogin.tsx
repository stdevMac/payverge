"use client";

import React, { useState, useEffect } from "react";
import { Card, CardBody, Button, Divider } from "@nextui-org/react";
import { AccessibleInput } from "@/components/ui/AccessibleInput";
import { Mail, Key, ArrowRight, Users } from "lucide-react";
import { toast } from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import * as StaffAPI from "../../api/staff";
import { setStaffData } from "@/utils/staffAuth";
import { isSafeRedirectUrl } from "@/utils/safeRedirect";
import { useApiErrorMessage } from "@/i18n/useApiErrorMessage";
import { useInstance } from "@/hooks/useInstance";

interface StaffLoginProps {
  onLoginSuccess?: (staffData: any) => void;
  isModal?: boolean;
}

export default function StaffLogin({ onLoginSuccess, isModal = false }: StaffLoginProps) {
  const { locale } = useSimpleLocale();
  const localizeError = useApiErrorMessage();
  // EMAIL_PROVIDER=log: no email is sent (the code reaches the server log only when EMAIL_LOG_CONTENT=true).
  const instance = useInstance();
  const emailOff = instance.isOff("email");
  // No Google button when the instance has no Google OAuth client.
  const showGoogle = !instance.isOff("google_oauth");
  const sentKey = emailOff ? "messages.codeLogged" : "messages.emailSent";
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [step, setStep] = useState<"email" | "code" | "membership">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [codeLoading, setCodeLoading] = useState(false);
  const [googleLoading, setGoogleLoading] = useState(false);
  const [resendLoading, setResendLoading] = useState(false);
  const [resendCooldown, setResendCooldown] = useState(0);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const [selectionToken, setSelectionToken] = useState("");
  const [memberships, setMemberships] = useState<StaffAPI.StaffMembershipChoice[]>([]);

  const RESEND_COOLDOWN_SECONDS = 30;

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  // Google OAuth cannot choose a business for a multi-membership identity.
  // The backend returns the existing short-lived signed selection proof and
  // display-only membership choices in a URL fragment (never a query string).
  // Consume and remove it immediately, then use the same scoped exchange as the
  // email-code flow when the user chooses a business.
  useEffect(() => {
    if (typeof window === "undefined" || !window.location.hash) return;
    const fragment = new URLSearchParams(window.location.hash.slice(1));
    const raw = fragment.get("staff_membership_selection");
    if (!raw) return;

    window.history.replaceState(
      {},
      document.title,
      window.location.pathname + window.location.search,
    );
    try {
      const payload = JSON.parse(raw) as {
        selection_token?: unknown;
        memberships?: unknown;
      };
      if (
        typeof payload.selection_token !== "string" ||
        payload.selection_token.length === 0 ||
        !Array.isArray(payload.memberships) ||
        payload.memberships.length < 2 ||
        !payload.memberships.every(
          (membership) =>
            membership !== null &&
            typeof membership === "object" &&
            typeof (membership as StaffAPI.StaffMembershipChoice).business_id === "number" &&
            typeof (membership as StaffAPI.StaffMembershipChoice).business_name === "string" &&
            typeof (membership as StaffAPI.StaffMembershipChoice).role === "string",
        )
      ) {
        return;
      }
      setSelectionToken(payload.selection_token);
      setMemberships(payload.memberships as StaffAPI.StaffMembershipChoice[]);
      setStep("membership");
    } catch {
      // Malformed fragments grant no access and simply leave the normal login
      // entry point visible; the fragment has already been scrubbed.
    }
  }, []);

  // Tick down the resend cooldown so the button shows "Resend in Ns".
  useEffect(() => {
    if (resendCooldown <= 0) return;
    const timer = setTimeout(() => setResendCooldown((s) => s - 1), 1000);
    return () => clearTimeout(timer);
  }, [resendCooldown]);

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `staffLogin.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const validateEmail = (email: string) => {
    if (!email.trim()) {
      return tString("validation.emailRequired");
    }
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(email)) {
      return tString("validation.emailInvalid");
    }
    return null;
  };

  const validateCode = (code: string) => {
    if (!code.trim()) {
      return tString("validation.codeRequired");
    }
    if (code.length !== 6 || !/^\d{6}$/.test(code)) {
      return tString("validation.codeInvalid");
    }
    return null;
  };

  // Request login code
  const handleRequestCode = async () => {
    const normalizedEmail = email.trim().toLowerCase();
    const emailError = validateEmail(normalizedEmail);
    if (emailError) {
      setFieldErrors((prev) => ({ ...prev, email: emailError }));
      toast.error(emailError);
      return;
    }

    try {
      setLoading(true);
      setEmail(normalizedEmail);
      // Clear prior field errors so a successful send never leaves "Email is
      // required" / "invalid email" banners stacked under the code step.
      setFieldErrors({});
      await StaffAPI.requestLoginCode({ email: normalizedEmail });
      toast.success(tString(sentKey).replace("{email}", normalizedEmail));
      setResendCooldown(RESEND_COOLDOWN_SECONDS);
      setStep("code");
    } catch (error) {
      console.error("Error requesting login code:", error);
      toast.error(localizeError(error) || tString("messages.networkError"));
    } finally {
      setLoading(false);
    }
  };

  // Resend uses its own loading flag (so it doesn't couple to the initial
  // "Send code" spinner) and is throttled by a cooldown countdown.
  const handleResendCode = async () => {
    if (resendCooldown > 0 || resendLoading) return;
    const normalizedEmail = email.trim().toLowerCase();
    try {
      setResendLoading(true);
      await StaffAPI.requestLoginCode({ email: normalizedEmail });
      toast.success(tString(sentKey).replace("{email}", normalizedEmail));
      setResendCooldown(RESEND_COOLDOWN_SECONDS);
    } catch (error) {
      console.error("Error resending login code:", error);
      toast.error(localizeError(error) || tString("messages.networkError"));
    } finally {
      setResendLoading(false);
    }
  };

  // Verify login code
  const completeLogin = (data: StaffAPI.StaffLoginResponse) => {
    const staffData = {
      ...data.staff,
      is_active: true,
    };
    setStaffData(staffData);
    toast.success(tString("messages.loginSuccess"));
    onLoginSuccess?.(data.staff);
  };

  const handleVerifyCode = async () => {
    const codeError = validateCode(code);
    if (codeError) {
      setFieldErrors((prev) => ({ ...prev, code: codeError }));
      toast.error(codeError);
      return;
    }

    try {
      setCodeLoading(true);
      const normalizedEmail = email.trim().toLowerCase();
      const normalizedCode = code.trim();
      const data = await StaffAPI.verifyLoginCode({ email: normalizedEmail, code: normalizedCode });

      if (StaffAPI.isMembershipSelectionResponse(data)) {
        setSelectionToken(data.selection_token);
        setMemberships(data.memberships);
        setStep("membership");
        return;
      }
      completeLogin(data);
    } catch (error) {
      console.error("Error verifying code:", error);
      const message = localizeError(error);
      const lower = message.toLowerCase();
      const mentionsInvalid = lower.includes("invalid") || lower.includes("inválid");
      const mentionsExpired = lower.includes("expired") || lower.includes("expir");
      // Prefer coded catalog messages; keep heuristics for uncoded English
      // backend strings (invalid vs expired login code).
      if (mentionsExpired && !mentionsInvalid) {
        toast.error(tString("messages.codeExpired"));
        setStep("email");
        setCode("");
      } else if (mentionsInvalid || mentionsExpired) {
        toast.error(tString("messages.invalidCode"));
        setCode("");
      } else {
        toast.error(message || tString("messages.unknownError"));
      }
    } finally {
      setCodeLoading(false);
    }
  };

  const handleMembershipSelect = async (businessId: number) => {
    try {
      setCodeLoading(true);
      const data = await StaffAPI.verifyLoginCode({
        selection_token: selectionToken,
        business_id: businessId,
      });
      if (StaffAPI.isMembershipSelectionResponse(data)) {
        throw new Error(tString("messages.unknownError"));
      }
      completeLogin(data);
    } catch (error) {
      console.error("Error selecting staff membership:", error);
      toast.error(localizeError(error) || tString("messages.unknownError"));
    } finally {
      setCodeLoading(false);
    }
  };

  const handleBackToEmail = () => {
    setStep("email");
    setCode("");
    setSelectionToken("");
    setMemberships([]);
    setFieldErrors({});
  };

  const handleGoogleLogin = async () => {
    try {
      setGoogleLoading(true);
      if (typeof window !== "undefined") {
        // Persist the post-login destination across the Google OAuth round-trip.
        // Prefer the `redirect` query param set by middleware/AuthGate; fall back
        // to the current pathname only when it's not the login page itself.
        const params = new URLSearchParams(window.location.search);
        const redirectParam = params.get("redirect") ?? "";
        const candidate =
          isSafeRedirectUrl(redirectParam)
            ? redirectParam
            : window.location.pathname.startsWith("/staff/login")
              ? ""
              : window.location.pathname;
        if (candidate) {
          localStorage.setItem("staff_auth_redirect_url", candidate);
        } else {
          localStorage.removeItem("staff_auth_redirect_url");
        }
      }
      const response = await StaffAPI.getStaffGoogleAuthURL(window.location.pathname);
      if (response.url) {
        window.location.href = response.url;
      } else {
        toast.error(tString("messages.networkError"));
      }
    } catch (error) {
      console.error("Error initiating staff Google login:", error);
      toast.error(localizeError(error) || tString("messages.networkError"));
    } finally {
      setGoogleLoading(false);
    }
  };
  const content = (
    <>
      <div className="flex flex-col items-center text-center w-full mb-6">
        <div className="mb-4 p-3 bg-brand/10 rounded-full">
          <Users className="w-8 h-8 text-brand" />
        </div>
        {!isModal && (
          <>
            <h1 className="font-title text-4xl md:text-5xl text-ink-950 mb-2">
              {tString("title")}
            </h1>
            <p className="text-gray-600">{tString("subtitle")}</p>
          </>
        )}
      </div>

      {step === "email" ? (
              <div className="space-y-6">
                {showGoogle && (
                  <>
                    <Button
                      className="w-full bg-white border-2 border-gray-200 hover:border-gray-300 hover:bg-gray-50 text-gray-700 font-medium"
                      size="lg"
                      onPress={handleGoogleLogin}
                      isLoading={googleLoading}
                    >
                      {/* eslint-disable no-restricted-syntax -- Google brand colors required for legal recognizability */}
                      <svg className="w-5 h-5 mr-2" viewBox="0 0 24 24">
                        <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
                        <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
                        <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
                        <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
                      </svg>
                      {/* eslint-enable no-restricted-syntax */}
                      {tString("steps.email.googleButton")}
                    </Button>

                    <Divider className="my-2" />
                  </>
                )}

                <AccessibleInput
                  type="email"
                  autoComplete="email"
                  label={tString("steps.email.title")}
                  placeholder={tString("steps.email.placeholder")}
                  value={email}
                  onValueChange={(v) => {
                    setEmail(v);
                    if (fieldErrors.email) setFieldErrors((prev) => ({ ...prev, email: null }));
                  }}
                  isInvalid={!!fieldErrors.email}
                  errorMessage={fieldErrors.email ?? undefined}
                  startContent={<Mail className="w-4 h-4 text-gray-400" />}
                  variant="bordered"
                  size="lg"
                  classNames={{
                    input: "text-base",
                    inputWrapper:
                      "border-gray-200 hover:border-gray-300 focus-within:border-brand",
                  }}
                  onKeyPress={(e) => {
                    if (e.key === "Enter") {
                      handleRequestCode();
                    }
                  }}
                />

                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                  onPress={handleRequestCode}
                  isLoading={loading}
                  endContent={!loading && <ArrowRight className="w-4 h-4" />}
                >
                  {loading
                    ? tString("steps.email.sending")
                    : tString("steps.email.button")}
                </Button>

                <div className="text-center">
                  <p className="text-sm text-gray-500">
                    {tString("steps.email.subtitle")}
                  </p>
                </div>
              </div>
            ) : step === "code" ? (
              <div className="space-y-6">
                <div className="text-center">
                  <div className="inline-flex items-center space-x-2 bg-brand/10 px-4 py-2 rounded-full">
                    <Mail className="w-4 h-4 text-brand" />
                    <span className="text-sm text-brand-dark font-medium">
                      {email}
                    </span>
                  </div>
                </div>

                <AccessibleInput
                  type="text"
                  label={tString("steps.code.title")}
                  placeholder={tString("steps.code.placeholder")}
                  value={code}
                  onValueChange={(v) => {
                    setCode(v.replace(/\D/g, "").slice(0, 6));
                    if (fieldErrors.code) setFieldErrors((prev) => ({ ...prev, code: null }));
                  }}
                  isInvalid={!!fieldErrors.code}
                  errorMessage={fieldErrors.code ?? undefined}
                  startContent={<Key className="w-4 h-4 text-gray-400" />}
                  variant="bordered"
                  size="lg"
                  classNames={{
                    input: "text-base text-center tracking-widest font-mono",
                    inputWrapper:
                      "border-gray-200 hover:border-gray-300 focus-within:border-brand",
                  }}
                  onKeyPress={(e) => {
                    if (e.key === "Enter") {
                      handleVerifyCode();
                    }
                  }}
                />

                <div className="space-y-3">
                  <Button
                    color="primary"
                    size="lg"
                    className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                    onPress={handleVerifyCode}
                    isLoading={codeLoading}
                    isDisabled={code.length !== 6}
                  >
                    {codeLoading
                      ? tString("steps.code.verifying")
                      : tString("steps.code.button")}
                  </Button>

                  <div className="flex space-x-2">
                    <Button
                      variant="light"
                      size="lg"
                      className="flex-1"
                      onPress={handleBackToEmail}
                    >
                      {tString("steps.code.backToEmail")}
                    </Button>
                    <Button
                      variant="light"
                      size="lg"
                      className="flex-1"
                      onPress={handleResendCode}
                      isLoading={resendLoading}
                      isDisabled={resendCooldown > 0 || resendLoading}
                    >
                      {resendCooldown > 0
                        ? tString("steps.code.resendIn").replace(
                            "{seconds}",
                            String(resendCooldown),
                          )
                        : tString("steps.code.resend")}
                    </Button>
                  </div>
                </div>

                <div className="text-center space-y-1">
                  <p className="text-sm text-gray-500">
                    {tString(
                      emailOff
                        ? "steps.code.subtitleNoEmail"
                        : "steps.code.subtitle",
                    )}
                  </p>
                  <p className="text-xs text-gray-400">
                    {tString(
                      emailOff
                        ? "steps.code.notReceivingNoEmail"
                        : "steps.code.notReceiving",
                    )}
                  </p>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                <div className="text-center space-y-2">
                  <h2 className="font-title text-2xl text-ink-950">
                    {tString("steps.membership.title")}
                  </h2>
                  <p className="text-sm text-ink-600">
                    {tString("steps.membership.subtitle")}
                  </p>
                </div>
                <div className="space-y-3">
                  {memberships.map((membership) => (
                    <Button
                      key={membership.business_id}
                      variant="bordered"
                      size="lg"
                      className="w-full justify-between border-ink-200 text-ink-900"
                      onPress={() => handleMembershipSelect(membership.business_id)}
                      isLoading={codeLoading}
                    >
                      <span>{membership.business_name || tString("steps.membership.unnamedBusiness")}</span>
                      <span className="text-sm text-ink-500">{membership.role}</span>
                    </Button>
                  ))}
                </div>
                <Button variant="light" className="w-full" onPress={handleBackToEmail}>
                  {tString("steps.membership.backToEmail")}
                </Button>
              </div>
            )}

            <Divider className="my-6" />

          <div className="text-center">
            <p className="text-xs text-gray-400">
              {tString("footer.poweredBy")}
            </p>
          </div>
    </>
  );

  if (isModal) {
    return <div className="w-full">{content}</div>;
  }

  return (
    <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
      <div className="relative w-full max-w-md">
        <Card className="bg-white border border-ink-200 shadow-sm">
          <CardBody className="py-8 px-6">
            {content}
          </CardBody>
        </Card>
      </div>
    </div>
  );
}

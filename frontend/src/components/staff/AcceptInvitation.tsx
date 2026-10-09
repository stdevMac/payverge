"use client";

import React, { useState, useEffect, useCallback } from "react";
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Input,
  Chip,
  Divider,
} from "@nextui-org/react";
import { UserPlus, Building, User, CheckCircle } from "lucide-react";
import { toast } from "react-hot-toast";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useAuth } from "@/providers/HybridAuthProvider";
import { clearStaffSession, setStaffData } from "@/utils/staffAuth";
import { logout as logoutCurrentSession } from "@/api/auth";
import * as StaffAPI from "../../api/staff";
import type { StaffInvitationPreviewResponse } from "../../api/staff";
import { getSearchParam } from "@/utils/nextRouteParams";
import { classifyInvitationError, type InvitationErrorKind } from "./invitationError";
import { useApiErrorMessage } from "@/i18n/useApiErrorMessage";
import { getApiErrorCode } from "@/utils/apiError";
import { getOperatorDashboardPath } from "@/utils/businessUrl";

interface AcceptInvitationProps {
  token?: string;
}

const roleColors = {
  manager: "primary",
  server: "success",
  host: "warning",
  kitchen: "secondary",
} as const;

export default function AcceptInvitation({
  token: propToken,
}: AcceptInvitationProps) {
  const { locale } = useSimpleLocale();
  const localizeError = useApiErrorMessage();
  const {
    refreshStaffData,
    isInitialized,
    isStaffUser,
    isOAuthUser,
    isWeb3User,
    oauthData,
    staffData,
    walletAddress,
  } = useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = (propToken || getSearchParam(searchParams, "token") || "").trim();

  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [invitationData, setInvitationData] =
    useState<StaffInvitationPreviewResponse | null>(null);
  const [step, setStep] = useState<"loading" | "form" | "success" | "error">(
    token ? "loading" : "error",
  );
  const [errorMessage, setErrorMessage] = useState("");
  const [errorKind, setErrorKind] = useState<InvitationErrorKind>(
    token ? "transient" : "permanent",
  );

  // P2-22: the accept POST overwrites the shared refresh_token cookie. If this
  // browser already holds a session (owner on the front-desk computer, another
  // staff member), require an explicit sign-out before accepting so we never
  // silently clobber someone else's session.
  const [sessionAcknowledged, setSessionAcknowledged] = useState(false);
  const [signingOut, setSigningOut] = useState(false);

  const activeSessionLabel =
    oauthData?.email || staffData?.email || walletAddress || "";
  const hasForeignSession =
    isInitialized &&
    !sessionAcknowledged &&
    (isOAuthUser || isWeb3User || isStaffUser);

  const handleSignOutAndContinue = async () => {
    try {
      setSigningOut(true);
      if (isStaffUser) {
        await clearStaffSession();
      } else {
        await logoutCurrentSession();
      }
      setSessionAcknowledged(true);
    } catch (error) {
      console.error("Error signing out before invitation accept:", error);
      toast.error(tString("existingSession.signOutFailed"));
    } finally {
      setSigningOut(false);
    }
  };

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `staffInvitation.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Get role label with translation
  const getRoleLabel = useCallback(
    (role: string): string => {
      return tString(`roles.${role}`) || role;
    },
    [tString],
  );

  // Get translated list items
  const getWhatHappensNextItems = useCallback((): string[] => {
    const items = getTranslation(
      "staffInvitation.form.whatHappensNext.items",
      locale,
    );
    return Array.isArray(items) ? items : [];
  }, [locale]);

  useEffect(() => {
    if (!token) {
      setErrorMessage(tString("messages.invalidLink"));
      setErrorKind("permanent");
      setStep("error");
      return;
    }

    let isMounted = true;

    const loadInvitation = async () => {
      try {
        const preview = await StaffAPI.getInvitationPreview(token);
        if (!isMounted) {
          return;
        }
        setInvitationData(preview);
        setName(preview.name || "");
        setErrorMessage("");
        setStep("form");
      } catch (error) {
        if (!isMounted) {
          return;
        }
        const message =
          localizeError(error) || tString("messages.acceptFailed");
        setInvitationData(null);
        setErrorMessage(message);
        setErrorKind(classifyInvitationError(message, getApiErrorCode(error)));
        setStep("error");
        toast.error(message);
      }
    };

    setStep("loading");
    void loadInvitation();

    return () => {
      isMounted = false;
    };
  }, [token, tString, localizeError]);

  const handleAcceptInvitation = async () => {
    if (!name.trim()) {
      toast.error(tString("messages.nameRequired"));
      return;
    }

    if (!token) {
      toast.error(tString("messages.invalidToken"));
      return;
    }

    try {
      setLoading(true);
      const data = await StaffAPI.acceptInvitation({
        token,
        name: name.trim(),
      });

      // The accept response may refine the role / business name; keep the
      // rest of the validated preview (email, business_custom_url, etc.) that
      // the success screen and dashboard redirect rely on.
      setInvitationData((prev) =>
        prev
          ? {
              ...prev,
              role: data.role ?? prev.role,
              business_name: data.business_name ?? prev.business_name,
            }
          : prev,
      );
      setStaffData({
        ...data.staff,
        is_active: true,
      });
      setStep("success");
      toast.success(tString("messages.accountCreated"));
    } catch (error) {
      console.error("Error accepting invitation:", error);
      toast.error(localizeError(error) || tString("messages.acceptFailed"));
    } finally {
      setLoading(false);
    }
  };

  const handleContinueToDashboard = async () => {
    // Only the operator slug (business_slug = businesses.business_id) is safe
    // for a user-visible dashboard URL: the numeric id would leak into the
    // address bar, and custom_url is a storefront path the operator routes do
    // not resolve. Without a slug we route to staff/login.
    const businessSlug = invitationData?.business_slug;
    try {
      await refreshStaffData();
    } catch (error) {
      console.error("Error refreshing staff data after invitation acceptance:", error);
    }

    if (businessSlug) {
      router.push(getOperatorDashboardPath(businessSlug));
      return;
    }

    router.push("/staff/login");
  };

  if (step === "loading") {
    return (
      <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
        <Card className="bg-white border border-ink-200 shadow-sm">
          <CardBody className="p-8 text-center">
            <div className="animate-spin w-8 h-8 border-2 border-brand border-t-transparent rounded-full mx-auto mb-4"></div>
            <p className="text-gray-600">{tString("loading.validating")}</p>
          </CardBody>
        </Card>
      </div>
    );
  }

  if (step === "success") {
    return (
      <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
        <div className="relative w-full max-w-md">
          <Card className="bg-white border border-ink-200 shadow-sm">
            <CardBody className="p-8 text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 bg-brand/10 rounded-2xl">
                  <CheckCircle className="w-12 h-12 text-brand" />
                </div>
              </div>

              <div>
                <h1 className="text-2xl font-light tracking-wide text-gray-900 mb-2">
                  {tString("success.title")}
                </h1>
                <p className="text-gray-600">{tString("success.subtitle")}</p>
              </div>

              {invitationData && (
                <div className="bg-gray-50 p-4 rounded-xl space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-600">
                      {tString("success.businessLabel")}
                    </span>
                    <span className="font-medium text-gray-900">
                      {invitationData.business_name ||
                        tString("success.defaultBusiness")}
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-gray-600">
                      {tString("success.roleLabel")}
                    </span>
                    <Chip
                      color={
                        roleColors[
                          invitationData.role as keyof typeof roleColors
                        ] || "default"
                      }
                      variant="flat"
                      size="sm"
                    >
                      {getRoleLabel(invitationData.role)}
                    </Chip>
                  </div>
                </div>
              )}

              <div className="space-y-3">
                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                  onPress={handleContinueToDashboard}
                >
                  {tString("success.continueButton")}
                </Button>
              </div>

              <div className="text-center">
                <p className="text-sm text-gray-500">
                  {tString("success.loginInfo")}
                </p>
              </div>
            </CardBody>
          </Card>
        </div>
      </div>
    );
  }

  if (step === "error") {
    const isPermanent = errorKind === "permanent";
    return (
      <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
        <div className="relative w-full max-w-md">
          <Card className="bg-white border border-ink-200 shadow-sm">
            <CardBody className="p-8 text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 bg-red-50 border border-red-200 rounded-2xl">
                  <UserPlus className="w-10 h-10 text-red-700" />
                </div>
              </div>
              <div>
                <h1 className="text-2xl font-light tracking-wide text-gray-900 mb-2">
                  {isPermanent
                    ? tString("messages.expiredTitle")
                    : tString("messages.transientTitle")}
                </h1>
                <p className="text-gray-600">
                  {isPermanent
                    ? tString("messages.expiredHelp")
                    : tString("messages.transientHelp")}
                </p>
              </div>
              {isPermanent ? (
                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                  onPress={() => router.push("/staff/login")}
                >
                  {tString("actions.backToLogin")}
                </Button>
              ) : (
                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                  onPress={() => window.location.reload()}
                >
                  {tString("actions.tryAgain")}
                </Button>
              )}
            </CardBody>
          </Card>
        </div>
      </div>
    );
  }

  if (hasForeignSession) {
    return (
      <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
        <div className="relative w-full max-w-md">
          <Card className="bg-white border border-ink-200 shadow-sm">
            <CardBody className="p-8 text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 bg-amber-50 border border-amber-200 rounded-2xl">
                  <UserPlus className="w-10 h-10 text-amber-700" />
                </div>
              </div>
              <div>
                <h1 className="text-2xl font-light tracking-wide text-gray-900 mb-2">
                  {tString("existingSession.title")}
                </h1>
                <p className="text-gray-600">
                  {tString("existingSession.description")}
                </p>
                {activeSessionLabel ? (
                  <p className="mt-2 text-sm font-medium text-gray-900">
                    {activeSessionLabel}
                  </p>
                ) : null}
              </div>
              <div className="space-y-3">
                <Button
                  color="primary"
                  size="lg"
                  className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                  onPress={handleSignOutAndContinue}
                  isLoading={signingOut}
                >
                  {tString("existingSession.signOutCta")}
                </Button>
                <Button
                  variant="bordered"
                  size="lg"
                  className="w-full"
                  onPress={() => router.push("/")}
                >
                  {tString("existingSession.cancelCta")}
                </Button>
              </div>
            </CardBody>
          </Card>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-warm-50 flex items-center justify-center p-4">
      <div className="relative w-full max-w-md">
        <Card className="bg-white border border-ink-200 shadow-sm">
          <CardHeader className="text-center pb-2">
            <div className="w-full">
              <div className="flex justify-center mb-4">
                <div className="p-3 bg-brand/10 rounded-2xl">
                  <UserPlus className="w-8 h-8 text-brand" />
                </div>
              </div>
              <h1 className="text-2xl font-light tracking-wide text-gray-900">
                {tString("title")}
              </h1>
              <p className="text-gray-600 mt-2">{tString("subtitle")}</p>
            </div>
          </CardHeader>

          <CardBody className="pt-2">
            <div className="space-y-6">
              {/* Invitation Info */}
              <div className="bg-brand/10 p-4 rounded-xl space-y-3">
                <div className="flex items-center space-x-2">
                  <Building className="w-4 h-4 text-brand" />
                  <span className="text-sm font-medium text-brand-dark">
                    {invitationData?.business_name || tString("form.invitationInfo.title")}
                  </span>
                </div>
                <p className="text-sm text-brand-dark">
                  {tString("form.invitationInfo.description")}
                </p>
                {invitationData ? (
                  <div className="space-y-2 rounded-xl bg-white border border-ink-200 p-3 text-sm text-gray-700">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-gray-500">
                        {tString("form.invitationInfo.emailLabel")}
                      </span>
                      <span className="font-medium text-gray-900">{invitationData.email}</span>
                    </div>
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-gray-500">{tString("success.roleLabel")}</span>
                      <Chip
                        color={roleColors[invitationData.role as keyof typeof roleColors] || "default"}
                        variant="flat"
                        size="sm"
                      >
                        {getRoleLabel(invitationData.role)}
                      </Chip>
                    </div>
                  </div>
                ) : null}
              </div>

              {/* Name Input */}
              <Input
                type="text"
                label={tString("form.nameField.label")}
                placeholder={tString("form.nameField.placeholder")}
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoComplete="name"
                startContent={<User className="w-4 h-4 text-gray-400" />}
                variant="bordered"
                size="lg"
                classNames={{
                  input: "text-base",
                  inputWrapper:
                    "border-gray-200 hover:border-gray-300 focus-within:border-brand",
                }}
                onKeyPress={(e) => {
                  if (e.key === "Enter") {
                    void handleAcceptInvitation();
                  }
                }}
              />

              {/* Accept Button */}
              <Button
                color="primary"
                size="lg"
                className="w-full bg-brand hover:bg-brand-dark text-white font-medium"
                onPress={handleAcceptInvitation}
                isLoading={loading}
                isDisabled={!name.trim()}
              >
                {loading
                  ? tString("form.buttons.accepting")
                  : tString("form.buttons.accept")}
              </Button>

              {/* Info */}
              <div className="bg-gray-50 p-4 rounded-xl">
                <h4 className="font-medium text-gray-900 mb-2">
                  {tString("form.whatHappensNext.title")}
                </h4>
                <ul className="text-sm text-gray-600 space-y-1">
                  {getWhatHappensNextItems().map((item, index) => (
                    <li key={index}>• {item}</li>
                  ))}
                </ul>
              </div>
            </div>

            <Divider className="my-6" />

            <div className="text-center">
              <p className="text-xs text-gray-400">
                {tString("footer.poweredBy")}
              </p>
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}

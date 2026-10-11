"use client";

import React, { useState, useCallback } from "react";
import { Button } from "@nextui-org/react";
import { UserCircle, Gift, Star } from "lucide-react";
import CustomerAuthModal from "../customer/CustomerAuthModal";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { checkInCustomerToTable } from "@/api/customerTable";
import { useCustomerAuth } from "@/contexts/CustomerAuthContext";

interface CRMSignupCardProps {
  businessName: string;
  tableCode: string;
  crmEnabled?: boolean;
  /**
   * full: marketing card with earn/get grid (open, no bill).
   * compact: single-row CTA when closed or already in service (open bill).
   */
  variant?: "full" | "compact";
}

export default function CRMSignupCard({
  businessName,
  tableCode,
  crmEnabled = false,
  variant = "full",
}: CRMSignupCardProps) {
  const { t: guestT } = useGuestTranslation();
  const { customer } = useCustomerAuth();
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const [authMode, setAuthMode] = useState<"login" | "register">("register");
  const [customerId, setCustomerId] = useState<number | null>(null);
  const [checkInError, setCheckInError] = useState("");

  const t = useCallback(
    (key: string): string => {
      return guestT(`crm.${key}`);
    },
    [guestT],
  );
  const joinFailedMessage = t("joinFailed");
  const joinFailedMessageRef = React.useRef(joinFailedMessage);

  React.useEffect(() => {
    joinFailedMessageRef.current = joinFailedMessage;
  }, [joinFailedMessage]);

  const joinCustomerToTable = useCallback(
    async (id: number) => {
      await checkInCustomerToTable(tableCode);
      setCustomerId(id);
      setCheckInError("");
    },
    [tableCode],
  );

  // CustomerAuthProvider owns the cookie-backed session check. Reusing its
  // state avoids protected-profile probes for anonymous guests.
  React.useEffect(() => {
    if (!crmEnabled || !customer || authModalOpen || customerId === customer.id) {
      return;
    }
    let mounted = true;
    const loadSession = async () => {
      try {
        await checkInCustomerToTable(tableCode);
        if (mounted) {
          setCustomerId(customer.id);
          setCheckInError("");
        }
      } catch {
        if (mounted) {
          setCustomerId(null);
          setCheckInError(joinFailedMessageRef.current);
        }
      }
    };
    loadSession().catch((err) => console.error("loadSession failed:", err));
    return () => {
      mounted = false;
    };
  }, [authModalOpen, crmEnabled, customer, customerId, tableCode]);

  const handleAuthSuccess = async (id: number) => {
    try {
      await joinCustomerToTable(id);
    } catch {
      setCheckInError(joinFailedMessage);
    }
    setAuthModalOpen(false);
  };

  const openSignup = () => {
    setAuthMode("register");
    setAuthModalOpen(true);
  };

  const openLogin = () => {
    setAuthMode("login");
    setAuthModalOpen(true);
  };

  // Listen for the header "Sign in" button — it dispatches this event
  // because navigating to /login 404s. Both register + login open the
  // same modal; the mode flips the visible tab.
  React.useEffect(() => {
    if (!crmEnabled) return;
    const handler = (event: Event) => {
      const detail = (event as CustomEvent<{ mode?: "login" | "register" }>)
        .detail;
      setAuthMode(detail?.mode === "register" ? "register" : "login");
      setAuthModalOpen(true);
    };
    window.addEventListener("guest-auth-open", handler);
    return () => window.removeEventListener("guest-auth-open", handler);
  }, [crmEnabled]);

  // Don't show if CRM is not enabled or customer is already logged in
  if (!crmEnabled || customerId) {
    return null;
  }

  const authModal = (
    <CustomerAuthModal
      isOpen={authModalOpen}
      onClose={() => setAuthModalOpen(false)}
      onSuccess={handleAuthSuccess}
      businessName={businessName}
      defaultMode={authMode}
    />
  );

  if (variant === "compact") {
    return (
      <>
        <section
          data-testid="crm-signup-compact"
          className="flex items-center gap-3 rounded-xl border border-warm-200 bg-white px-4 py-3"
        >
          <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-brand/10 text-brand">
            <Gift className="h-4 w-4" strokeWidth={1.5} />
          </div>
          <div className="min-w-0 flex-1">
            <button
              type="button"
              onClick={openSignup}
              // RSP-1: 129×21 as a bare text button. `py-1 -my-1` → 29px, over
              // the 24px WCAG 2.5.8 AA floor, with no layout displacement.
              className="text-left text-body-sm font-semibold text-ink-900 underline-offset-2 hover:text-brand hover:underline py-1 -my-1"
            >
              {t("signupPrompt")}
            </button>
            {checkInError && (
              <p className="mt-1 text-xs text-rose-700">{checkInError}</p>
            )}
          </div>
          <button
            type="button"
            onClick={openLogin}
            // RSP-1: 16px tall. Padding-plus-negative-margin keeps the flex row
            // height identical (the margin box is unchanged) while the hit area
            // grows to 28px.
            className="flex-shrink-0 text-label uppercase text-ink-500 transition-colors hover:text-ink-900 py-1.5 -my-1.5"
          >
            {t("signIn")}
          </button>
        </section>
        {authModal}
      </>
    );
  }

  return (
    <>
      <section
        data-testid="crm-signup-full"
        className="rounded-2xl border border-warm-200 bg-white p-5 sm:p-6"
      >
        <div className="flex items-start gap-4">
          <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-2xl bg-brand/10 text-brand">
            <Gift className="h-6 w-6" strokeWidth={1.5} />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="font-title text-heading-md text-ink-950">
              {t("signupPrompt")}
            </h3>
            <p className="mt-1 text-body-sm text-ink-600">
              {t("signupDescription")}
            </p>
            {checkInError && (
              <p className="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                {checkInError}
              </p>
            )}
            <div className="mt-4 flex items-center gap-4 text-label uppercase text-ink-500">
              <span className="inline-flex items-center gap-1.5">
                <Star className="h-4 w-4 text-amber-500" strokeWidth={1.75} />
                {t("earnPoints") || "Earn Points"}
              </span>
              <span className="inline-flex items-center gap-1.5">
                <Gift className="h-4 w-4 text-brand" strokeWidth={1.75} />
                {t("getRewards") || "Get Rewards"}
              </span>
            </div>
            <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center">
              <Button
                onPress={openSignup}
                startContent={<UserCircle className="h-5 w-5" strokeWidth={1.75} />}
                className="h-11 rounded-xl bg-brand font-semibold text-white hover:bg-brand-dark sm:flex-1"
              >
                {t("signupPrompt")}
              </Button>
              <button
                onClick={openLogin}
                // RSP-1: same bare-label defect as the compact variant. Not caught
                // in the 390px sweep because only the compact variant rendered
                // there, but it is the identical control in the full card.
                className="text-label uppercase text-ink-500 transition-colors hover:text-ink-900 py-1.5 -my-1.5"
              >
                {t("alreadyMember")}{" "}
                <span className="text-brand">{t("signIn")}</span>
              </button>
            </div>
          </div>
        </div>
      </section>

      {authModal}
    </>
  );
}

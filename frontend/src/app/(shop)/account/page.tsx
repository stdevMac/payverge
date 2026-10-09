"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useAuth } from "@/providers/HybridAuthProvider";
import { useUserStore } from "@/store/useUserStore";
import { AccountPrivacyPanel } from "@/components/account/AccountPrivacyPanel";
import type { AccountPrivacyCopy } from "@/components/account/AccountPrivacyPanel";
import { AccountNotificationsSection } from "@/components/account/AccountNotificationsSection";
import type { AccountNotificationsCopy } from "@/components/account/AccountNotificationsSection";
import { AccountEmailPreferences } from "@/components/account/AccountEmailPreferences";
import type { AccountEmailPreferencesCopy } from "@/components/account/AccountEmailPreferences";
import { getMyBusinesses, type Business } from "@/api/business";
import {
  ACCOUNT_TABS,
  resolveAccountTab,
  type AccountTab,
} from "@/utils/accountTabs";
import { User, Mail, Wallet, ArrowLeft, Bell, ShieldCheck } from "lucide-react";
import Link from "next/link";
import { Button } from "@nextui-org/react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";

const TAB_ICONS: Record<AccountTab, typeof User> = {
  account: User,
  notifications: Bell,
  privacy: ShieldCheck,
};

function AccountPageInner() {
  const { oauthData, isOAuthUser, isStaffUser, isWeb3User, isInitialized } =
    useAuth();
  const { user } = useUserStore();
  const { locale } = useSimpleLocale();
  const router = useRouter();
  const searchParams = useSearchParams();

  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(`account.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const isAuthenticated = isOAuthUser || isStaffUser || isWeb3User;

  // Tab is driven by `?tab=` so it's shareable and so the email footer link
  // (straight to /account?tab=notifications) lands on Notifications. A plain button strip (not NextUI <Tabs>) avoids the
  // selectedKey↔router.replace feedback loop documented for that component.
  const [activeTab, setActiveTab] = useState<AccountTab>(() =>
    resolveAccountTab(searchParams?.get("tab")),
  );
  useEffect(() => {
    setActiveTab(resolveAccountTab(searchParams?.get("tab")));
  }, [searchParams]);

  const handleTab = useCallback(
    (tab: AccountTab) => {
      setActiveTab(tab);
      const url = new URL(window.location.href);
      if (tab === "account") {
        url.searchParams.delete("tab");
      } else {
        url.searchParams.set("tab", tab);
      }
      router.replace(url.pathname + url.search, { scroll: false });
    },
    [router],
  );

  // Email preferences are account-level (AccountEmailPreferences). The
  // per-business list below is for the genuinely per-business alerts
  // (operational sounds, Telegram) — load the operator's businesses so it can
  // deep-link each one to its Settings → Notifications tab.
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [businessesLoading, setBusinessesLoading] = useState(false);
  const [businessesError, setBusinessesError] = useState(false);

  useEffect(() => {
    if (!isInitialized || !isAuthenticated) return;
    let cancelled = false;
    setBusinessesLoading(true);
    setBusinessesError(false);
    getMyBusinesses()
      .then((data) => {
        if (!cancelled) setBusinesses(data);
      })
      .catch(() => {
        if (!cancelled) setBusinessesError(true);
      })
      .finally(() => {
        if (!cancelled) setBusinessesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [isInitialized, isAuthenticated]);

  if (!isInitialized) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-white">
        <div
          role="status"
          className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-brand/20 border-t-brand motion-safe:animate-spin"
        />
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-white">
        <div className="text-center max-w-md mx-auto px-6">
          <div className="w-16 h-16 bg-ink-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-ink-100">
            <User className="w-8 h-8 text-ink-400" />
          </div>
          <h1 className="font-title text-3xl text-ink-900 mb-3">
            {t("unauthenticated.title")}
          </h1>
          <p className="text-ink-600 mb-8">
            {t("unauthenticated.description")}
          </p>
          <Button
            as={Link}
            href="/dashboard"
            className="bg-ink-900 text-white font-semibold px-8 py-3 rounded-xl"
          >
            {t("unauthenticated.goToDashboard")}
          </Button>
        </div>
      </div>
    );
  }

  const email = oauthData?.email || user?.email;
  const walletAddress = user?.address;
  const accountIdentifier = email || walletAddress || "";

  const notificationsCopy: AccountNotificationsCopy = {
    sectionTitle: t("notifications.sectionTitle"),
    sectionDescription: t("notifications.sectionDescription"),
    manageLink: t("notifications.manageLink"),
    emptyTitle: t("notifications.emptyTitle"),
    emptyDescription: t("notifications.emptyDescription"),
    errorMessage: t("notifications.errorMessage"),
    loadingLabel: t("notifications.loadingLabel"),
  };

  const emailPrefsCopy: AccountEmailPreferencesCopy = {
    sectionTitle: t("emailPrefs.sectionTitle"),
    sectionDescription: t("emailPrefs.sectionDescription"),
    loadingLabel: t("emailPrefs.loadingLabel"),
    errorMessage: t("emailPrefs.errorMessage"),
    savedToast: t("emailPrefs.savedToast"),
    saveErrorToast: t("emailPrefs.saveErrorToast"),
    items: {
      transactional: {
        label: t("emailPrefs.items.transactional.label"),
        description: t("emailPrefs.items.transactional.description"),
      },
      reports: {
        label: t("emailPrefs.items.reports.label"),
        description: t("emailPrefs.items.reports.description"),
      },
    },
  };

  const privacyCopy: AccountPrivacyCopy = {
    sectionTitle: t("privacy.sectionTitle"),
    sectionDescription: t("privacy.sectionDescription"),
    exportTitle: t("privacy.exportTitle"),
    exportDescription: t("privacy.exportDescription"),
    exportButton: t("privacy.exportButton"),
    exportingButton: t("privacy.exportingButton"),
    exportSuccess: t("privacy.exportSuccess"),
    exportError: t("privacy.exportError"),
    deleteTitle: t("privacy.deleteTitle"),
    deleteDescription: t("privacy.deleteDescription"),
    deleteButton: t("privacy.deleteButton"),
    modalTitle: t("privacy.modalTitle"),
    modalIntro: t("privacy.modalIntro"),
    modalConfirmLabel: t("privacy.modalConfirmLabel"),
    modalReasonLabel: t("privacy.modalReasonLabel"),
    modalReasonPlaceholder: t("privacy.modalReasonPlaceholder"),
    modalCancel: t("privacy.modalCancel"),
    modalConfirm: t("privacy.modalConfirm"),
    modalConfirming: t("privacy.modalConfirming"),
    modalError: t("privacy.modalError"),
    modalSuccess: t("privacy.modalSuccess"),
  };

  return (
    <div className="min-h-screen bg-white font-sans">
      <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-12 lg:py-16">
        <div className="mb-10">
          <Link
            href="/dashboard"
            className="inline-flex items-center gap-1.5 text-sm text-ink-500 hover:text-ink-700 transition-colors mb-6"
          >
            <ArrowLeft className="w-4 h-4" />
            {t("backToDashboard")}
          </Link>
          <h1 className="text-3xl lg:text-4xl font-bold tracking-tight text-ink-900">
            {t("title")}
          </h1>
          <p className="text-ink-600 mt-2">{t("subtitle")}</p>
        </div>

        {/* Tab strip */}
        <div
          role="tablist"
          aria-label={t("title")}
          data-testid="account-tablist"
          className="flex flex-wrap gap-x-1 border-b border-ink-200 mb-10"
        >
          {ACCOUNT_TABS.map((tab) => {
            const Icon = TAB_ICONS[tab];
            const isActive = activeTab === tab;
            return (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={isActive}
                data-testid={`account-tab-${tab}`}
                onClick={() => handleTab(tab)}
                className={`inline-flex shrink-0 items-center gap-2 px-2.5 sm:px-4 py-3 text-sm font-medium whitespace-nowrap border-b-2 -mb-px transition-colors ${
                  isActive
                    ? "border-ink-900 text-ink-900"
                    : "border-transparent text-ink-500 hover:text-ink-800"
                }`}
              >
                <Icon className="hidden h-4 w-4 sm:block" aria-hidden />
                {t(`tabs.${tab}`)}
              </button>
            );
          })}
        </div>

        {activeTab === "account" && (
          <section className="mb-12">
            <h2 className="text-lg font-semibold text-ink-900 mb-4">
              {t("info.title")}
            </h2>
            <div className="border border-ink-200 rounded-2xl divide-y divide-ink-100">
              {email && (
                <div className="flex items-center gap-3 px-6 py-4">
                  <Mail className="w-4 h-4 text-ink-400 shrink-0" />
                  <div>
                    <p className="text-xs text-ink-500 font-medium uppercase tracking-wide">
                      {t("info.email")}
                    </p>
                    <p className="text-sm text-ink-900">{email}</p>
                  </div>
                </div>
              )}
              {walletAddress && (
                <div className="flex items-center gap-3 px-6 py-4">
                  <Wallet className="w-4 h-4 text-ink-400 shrink-0" />
                  <div>
                    <p className="text-xs text-ink-500 font-medium uppercase tracking-wide">
                      {t("info.wallet")}
                    </p>
                    <p className="text-sm text-ink-900 font-mono">
                      {walletAddress.slice(0, 6)}...{walletAddress.slice(-4)}
                    </p>
                  </div>
                </div>
              )}
            </div>
          </section>
        )}

        {activeTab === "notifications" && (
          <>
            <AccountEmailPreferences copy={emailPrefsCopy} />
            <AccountNotificationsSection
              businesses={businesses}
              loading={businessesLoading}
              error={businessesError}
              copy={notificationsCopy}
            />
          </>
        )}

        {activeTab === "privacy" && (
          <AccountPrivacyPanel
            accountIdentifier={accountIdentifier}
            copy={privacyCopy}
          />
        )}
      </div>
    </div>
  );
}

// useSearchParams() must sit under a Suspense boundary or Next fails to
// statically render this (static, non-dynamic) route at build time.
export default function AccountPage() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <AccountPageInner />
    </Suspense>
  );
}

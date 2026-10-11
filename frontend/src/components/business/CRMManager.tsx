"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Button } from "@nextui-org/react";
import { Download, Gift, Tags, UserPlus, Users } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useOptionalUrlState, useSetUrlParams } from "@/hooks/useUrlState";
import type { UrlParamPatch } from "@/hooks/urlState";
import { businessCRMAPI } from "@/api/crm";
import { runWithFeedback } from "@/lib/runWithFeedback";
import { localDateKey } from "@/lib/localDate";
import DashboardLockedTabView from "./DashboardLockedTabView";
import CRMToggle, { useCRMStatus } from "./CRMToggle";
import CustomersTab from "./crm/CustomersTab";
import { CRMCustomersSkeleton } from "./crm/CRMCustomersSkeleton";
import SegmentsTab from "./crm/SegmentsTab";
import LoyaltyTab from "./crm/LoyaltyTab";
import DashboardTabShell from "./shared/DashboardTabShell";
import { btnPrimaryNextUI, btnSecondaryNextUI } from "@/components/ui/buttonStyles";
import { DashboardTabTransition, PremiumPanel } from "./premium";

/** CRM segment drilldown values accepted by `?focus=` (L5-11 / S-7). */
export const CRM_FOCUS_VALUES = ["lapsed", "vip", "new", "at-risk"] as const;
export type CrmFocus = (typeof CRM_FOCUS_VALUES)[number];

/** Atomic `tab` + `sub` + `focus` patches for a Customers segment drilldown (#376). */
export function crmCustomersFocusPatches(
  segment?: string | null,
): UrlParamPatch[] {
  const focus =
    segment && (CRM_FOCUS_VALUES as readonly string[]).includes(segment)
      ? segment
      : null;
  return [
    { key: "tab", value: "crm" },
    { key: "sub", value: "customers" },
    { key: "focus", value: focus },
  ];
}

interface CRMManagerProps {
  businessId: number;
  subTab?: Tab;
  onSubTabChange?: (tab: Tab) => void;
  onNavigateToTab?: (tab: string) => void;
}

type Tab = "customers" | "segments" | "loyalty";

const TABS: Tab[] = ["customers", "segments", "loyalty"];

// Tab labels resolve via the locale-aware `t()` helper at render time so the
// CRM sub-tabs follow the dashboard's active language.
const TAB_I18N_KEYS: Record<Tab, string> = {
  customers: "tabs.customers",
  segments: "tabs.segments",
  loyalty: "tabs.loyalty",
};

const TAB_ICONS: Record<Tab, typeof Users> = {
  customers: Users,
  segments: Tags,
  loyalty: Gift,
};

const CRMManager = React.memo(function CRMManager({
  businessId,
  subTab: controlledSubTab,
  onSubTabChange,
  onNavigateToTab,
}: CRMManagerProps) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [internalTab, setInternalTab] = useState<Tab>("customers");
  const [optimisticTab, setOptimisticTab] = useState<Tab | null>(null);
  const tab = optimisticTab ?? controlledSubTab ?? internalTab;
  const setTab = onSubTabChange ?? setInternalTab;
  const [isExporting, setIsExporting] = useState(false);
  const [addCustomerOpen, setAddCustomerOpen] = useState(false);
  // L5-11 / Root B reference: `?focus=` is the source of the segment drilldown.
  const [customersSegment, setCustomersSegment] = useOptionalUrlState<CrmFocus>({
    key: "focus",
    valid: CRM_FOCUS_VALUES,
  });
  const setUrlParams = useSetUrlParams();

  useEffect(() => {
    setOptimisticTab(null);
  }, [controlledSubTab]);

  const applyCustomersFocus = useCallback(
    (segment?: string | null) => {
      // One replace for tab=crm, sub=customers, focus=<segment>. Calling
      // setTab/onSubTabChange here would fire a second replace that drops focus.
      setUrlParams(crmCustomersFocusPatches(segment));
      setOptimisticTab("customers");
      if (!onSubTabChange) {
        setInternalTab("customers");
      }
    },
    [setUrlParams, onSubTabChange],
  );

  // Marketing win-back deep link (?focus=<segment>) lands on the Customers
  // sub-tab so the filtered list is visible (not stuck on Segments).
  // Depend only on customersSegment — including `tab` would bounce a chrome
  // click to Segments/Loyalty back to Customers while focus is set.
  useEffect(() => {
    if (!customersSegment) return;
    applyCustomersFocus(customersSegment);
  }, [customersSegment, applyCustomersFocus]);

  useEffect(() => {
    if (tab !== "customers") {
      setAddCustomerOpen(false);
    }
  }, [tab]);

  const openSegment = useCallback(
    (segment?: string) => {
      applyCustomersFocus(segment);
    },
    [applyCustomersFocus],
  );

  // Operational lock (admin suspend/close) and RBAC access
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(businessId.toString());

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.crm.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const isLocked = !hasAccess || isSuspended;
  // Single source of truth for CRM status: the toggles below are controlled
  // by this state and report successful toggles back through setCrmEnabled,
  // so pause/resume re-renders the tab in place (no page reload).
  const {
    enabled: crmEnabled,
    loading: crmStatusLoading,
    setEnabled: setCrmEnabled,
  } = useCRMStatus(businessId, isLocked);

  const handleExport = useCallback(async () => {
    await runWithFeedback(
      async () => {
        const blob = await businessCRMAPI.exportCustomers(businessId, locale);
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `customers_${businessId}_${localDateKey()}.csv`;
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        document.body.removeChild(a);
      },
      {
        success: t("toasts.exportSuccess"),
        error: t("toasts.exportError"),
        setBusy: setIsExporting,
      },
    );
  }, [businessId, t, locale]);

  // If the business is suspended or closed, show lockdown (after all hooks)
  if (!accessLoading && (!hasAccess || isSuspended)) {
    return (
      <DashboardLockedTabView
        title={t("title")}
        subtitle={t("description")}
        businessId={businessId}
      />
    );
  }

  return (
    <DashboardTabShell
      header={{
        title: t("title"),
        subtitle: t("description"),
        // The shell short-circuits to `loading` while the status fetch is in
        // flight, so the header (and tabs below) only ever render resolved.
        status: {
          label: crmEnabled ? t("shell.activeMode") : t("shell.pausedMode"),
          tone: crmEnabled ? "positive" : "attention",
        },
        actions:
          tab === "customers" && crmEnabled ? (
            <>
              <Button
                onPress={() => setAddCustomerOpen(true)}
                radius="full"
                className={btnPrimaryNextUI}
                startContent={<UserPlus size={16} />}
                data-testid="crm-add-customer"
              >
                {t("addCustomer")}
              </Button>
              <Button
                onPress={handleExport}
                radius="full"
                variant="bordered"
                className={btnSecondaryNextUI}
                startContent={!isExporting && <Download size={16} />}
                isLoading={isExporting}
                isDisabled={isExporting}
                data-testid="crm-export-customers"
              >
                {t("export")} (CSV)
              </Button>
            </>
          ) : undefined,
      }}
      loading={accessLoading || crmStatusLoading ? <CRMCustomersSkeleton /> : null}
      tabs={
        crmEnabled
          ? {
              items: TABS.map((tabKey) => ({
                key: tabKey,
                label: t(TAB_I18N_KEYS[tabKey]),
                icon: TAB_ICONS[tabKey],
              })),
              activeKey: tab,
              onChange: (key) => setTab(key as Tab),
              ariaLabel: t("title"),
            }
          : undefined
      }
    >
      {crmEnabled ? (
        <>
          <PremiumPanel className="p-4 sm:p-6" withTexture={false}>
            <DashboardTabTransition tabKey={tab}>
              {tab === "customers" && (
                <CustomersTab
                  businessId={businessId}
                  isLocked={isLocked}
                  onNavigateToTab={onNavigateToTab}
                  initialSegment={customersSegment ?? undefined}
                  onClearSegment={() => setCustomersSegment(null)}
                  addModalOpen={addCustomerOpen}
                  onAddModalOpenChange={setAddCustomerOpen}
                  hideListAddButton
                />
              )}
              {tab === "segments" && (
                <SegmentsTab
                  businessId={businessId}
                  onJumpToCustomers={openSegment}
                  highlightSegment={
                    customersSegment === "lapsed" ? "lapsed" : undefined
                  }
                />
              )}
              {tab === "loyalty" && <LoyaltyTab businessId={businessId} />}
            </DashboardTabTransition>
          </PremiumPanel>
          <div className="mt-12 border-t border-warm-200/80 pt-6">
            <CRMToggle
              businessId={businessId}
              isLocked={isLocked}
              enabled={crmEnabled}
              onStatusChange={setCrmEnabled}
              variant="button"
            />
          </div>
        </>
      ) : (
        <CRMToggle
          businessId={businessId}
          isLocked={isLocked}
          enabled={crmEnabled}
          onStatusChange={setCrmEnabled}
          variant="card"
        />
      )}
    </DashboardTabShell>
  );
});

export default CRMManager;

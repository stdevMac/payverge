"use client";

import React, { useCallback } from "react";
import { BarChart3, History, Settings, Truck, Users } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useUrlState } from "@/hooks/useUrlState";
import { DeliverySkeleton } from "./DeliverySkeleton";
import DashboardLockedTabView from "@/components/business/DashboardLockedTabView";
import DeliverySettings from "@/components/business/DeliverySettings";
import { useDeliveryStatus } from "@/components/business/DeliveryToggle";
import DispatchConsole from "./DispatchConsole";
import DeliveryHistory from "./DeliveryHistory";
import DriversManager from "./DriversManager";
import DriverPerformance from "./DriverPerformance";
import DashboardTabShell from "@/components/business/shared/DashboardTabShell";
import {
  DashboardTabTransition,
  PremiumPanel,
} from "@/components/business/premium";

/** Delivery shell sub-tabs mirrored to `?sub=` (L3-43). */
export const DELIVERY_SUB_VALUES = [
  "configuration",
  "dispatch",
  "history",
  "drivers",
  "performance",
] as const;
export type DeliverySub = (typeof DELIVERY_SUB_VALUES)[number];

interface DeliveryAdminProps {
  businessId: number;
  onSave?: () => void;
}

export default function DeliveryAdmin({
  businessId,
  onSave,
}: DeliveryAdminProps) {
  const { locale } = useSimpleLocale();
  const { hasAccess, loading: accessLoading } = useBusinessAccess(businessId);
  // Single source of truth for the header status chip: the toggle inside the
  // Configuration sub-tab reports back through setDeliveryEnabled so the chip
  // flips in place instead of waiting for a remount.
  const {
    enabled: deliveryEnabled,
    loading: statusLoading,
    setEnabled: setDeliveryEnabled,
  } = useDeliveryStatus(businessId, !hasAccess);
  // L3-43: sub-tabs write back to `?sub=` (mirrors CRM). Whitelisted under
  // delivery in tabParams.ts so cross-rail switches strip the param.
  const [urlSub, setUrlSub] = useUrlState({
    key: "sub",
    valid: DELIVERY_SUB_VALUES,
    fallback: "configuration" as DeliverySub,
    omitDefault: false,
    activeWhen: { key: "tab", values: ["delivery"] },
  });
  // Optimistic pill/content so a click updates the board before router.replace
  // lands — URL lag was highlighting Despacho while Historial stayed mounted (#717).
  const [optimisticSub, setOptimisticSub] = React.useState<DeliverySub | null>(
    null,
  );
  // The URL value the optimistic write was made against, and the setter that
  // made it. `useUrlState.setValue` silently drops the write while `?tab=` has
  // not settled on delivery (activeWhen), so a click could leave the rail on
  // Despacho with `?sub=history` forever — pill and URL permanently desynced.
  const optimisticBaseRef = React.useRef<DeliverySub | null>(null);
  const optimisticWriterRef = React.useRef<((next: DeliverySub) => void) | null>(
    null,
  );
  React.useEffect(() => {
    if (optimisticSub == null) return;
    if (urlSub === optimisticSub) {
      // The write landed: the URL is the single source of truth again.
      setOptimisticSub(null);
      optimisticBaseRef.current = null;
      optimisticWriterRef.current = null;
      return;
    }
    if (urlSub !== optimisticBaseRef.current) {
      // The URL moved somewhere else on its own (back/forward, cross-rail
      // nav). The URL wins; drop the optimistic value.
      setOptimisticSub(null);
      optimisticBaseRef.current = null;
      optimisticWriterRef.current = null;
      return;
    }
    if (setUrlSub === optimisticWriterRef.current) return;
    // A new setter identity means `activeWhen` just settled, so the dropped
    // write can now land. Re-drive it instead of leaving the URL behind.
    optimisticWriterRef.current = setUrlSub;
    setUrlSub(optimisticSub);
  }, [urlSub, optimisticSub, setUrlSub]);
  const activeTab = optimisticSub ?? urlSub;
  const setActiveTab = (next: DeliverySub) => {
    optimisticBaseRef.current = urlSub;
    optimisticWriterRef.current = setUrlSub;
    setOptimisticSub(next);
    setUrlSub(next);
  };

  const tString = useCallback(
    (key: string): string => {
      const result = getTranslation(`deliverySettings.${key}`, locale);
      return Array.isArray(result)
        ? result[0] || key
        : (result as string) || key;
    },
    [locale],
  );

  // S-9: loading and the access gate resolve through the shell, so the Delivery
  // title and sub-tab rail stay mounted while the tier check runs instead of
  // the whole tab blinking out to a bare skeleton.
  return (
    <DashboardTabShell
      loading={accessLoading ? <DeliverySkeleton /> : null}
      locked={
        !accessLoading && !hasAccess ? (
          <DashboardLockedTabView
            title={tString("focused.locked.title")}
            subtitle={tString("focused.locked.subtitle")}
            businessId={businessId}
          />
        ) : null
      }
      header={{
        title: tString("focused.headerTitle"),
        subtitle: tString("focused.headerDescription"),
        status: statusLoading
          ? undefined
          : {
              label: tString(
                deliveryEnabled ? "focused.status.on" : "focused.status.off",
              ),
              tone: deliveryEnabled ? "positive" : "attention",
            },
      }}
      tabs={{
        items: [
          {
            key: "configuration",
            label: tString("dispatch.tabs.configuration"),
            icon: Settings,
          },
          {
            key: "dispatch",
            label: tString("dispatch.tabs.dispatch"),
            icon: Truck,
          },
          {
            key: "history",
            label: tString("dispatch.tabs.history"),
            icon: History,
          },
          {
            key: "drivers",
            label: tString("dispatch.tabs.drivers"),
            icon: Users,
          },
          {
            key: "performance",
            label: tString("dispatch.tabs.performance"),
            icon: BarChart3,
          },
        ],
        activeKey: activeTab,
        onChange: (key: string) => {
          if ((DELIVERY_SUB_VALUES as readonly string[]).includes(key)) {
            setActiveTab(key as DeliverySub);
          }
        },
        ariaLabel: tString("dispatch.tabsAria") || "Delivery sections",
      }}
    >
      <DashboardTabTransition tabKey={activeTab}>
        {activeTab === "configuration" && (
          <DeliverySettings
            businessId={businessId}
            onSave={onSave}
            hideHeader
            onDeliveryStatusChange={setDeliveryEnabled}
          />
        )}
        {activeTab === "dispatch" && (
          <DispatchConsole businessId={businessId} />
        )}
        {activeTab === "history" && (
          <PremiumPanel className="p-4 sm:p-6" withTexture={false}>
            <DeliveryHistory businessId={businessId} />
          </PremiumPanel>
        )}
        {activeTab === "drivers" && (
          <PremiumPanel className="p-4 sm:p-6" withTexture={false}>
            <DriversManager businessId={businessId} />
          </PremiumPanel>
        )}
        {activeTab === "performance" && (
          <DriverPerformance businessId={businessId} />
        )}
      </DashboardTabTransition>
    </DashboardTabShell>
  );
}

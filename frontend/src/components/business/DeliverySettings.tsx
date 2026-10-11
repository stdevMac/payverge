import React, { useState, useEffect, useCallback } from "react";
import { Card, CardBody } from "@nextui-org/react";
import { Truck } from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useDirtyForm } from "@/hooks/useDirtyForm";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import DashboardLockedTabView from "./DashboardLockedTabView";
import DashboardTabShell from "./shared/DashboardTabShell";
import SaveBar from "./SaveBar";
import { PrimarySpinner } from "@/components/ui/spinners/PrimarySpinner";
import { DeliveryToggle } from "./DeliveryToggle";
import {
  deliveryApi,
  DeliveryOrder,
  DeliverySettingsDto,
  UpdateDeliverySettingsInput,
} from "@/api/delivery";
import { asDollars } from "@/types/money";
import { ConfigurationSection } from "./delivery/ConfigurationSection";
import { deriveCurrencyPrefix } from "./delivery/currencyPrefix";
import {
  ZonesSection,
  ensureZoneClientKeys,
  reconcileZoneClientKeys,
} from "./delivery/ZonesSection";
import { HoursSection } from "./delivery/HoursSection";
import { PartnersSection } from "./delivery/PartnersSection";
import { CustomerNoteSection } from "./delivery/CustomerNoteSection";
import { useDeliveryBusiness } from "./delivery/useDeliveryQueries";
import type { Locale } from "@/i18n/config";
import { surfaceBackendError } from "@/utils/localizedError";
import { captureClientError } from "@/lib/sentry/reporting";


interface DeliverySettingsProps {
  businessId: number;
  onSave?: (settings: DeliverySettingsDto) => void;
  // When this component is embedded inside DeliveryAdmin (the sub-tabs
  // shell), the page-level "Delivery" heading already renders one level
  // up — so we hide the duplicate header here. Standalone callers leave
  // this off and keep the in-component header.
  hideHeader?: boolean;
  // Reports a successful on/off toggle upward so embedding views (the
  // DeliveryAdmin header status chip) stay in sync without a remount.
  onDeliveryStatusChange?: (enabled: boolean) => void;
}

const defaultSettings: DeliverySettingsDto = {
  business_id: 0,
  delivery_enabled: false,
  in_house_delivery_enabled: false,
  third_party_enabled: false,
  payment_mode: "cash_on_delivery",
  online_payment_available: false,
  flat_delivery_fee: asDollars(0),
  free_delivery_minimum: asDollars(0),
  minimum_order_amount: asDollars(0),
  estimated_prep_time: 0,
  max_concurrent_deliveries: 0,
  delivery_hours_same_as_business: true,
  delivery_start_time: "",
  delivery_end_time: "",
  delivery_instructions: "",
  external_partner_links: [],
  zones: [],
  partner_fallback_available: false,
  estimated_delivery_minutes: 0,
};

/** Terminal delivery statuses — orders past these are no longer "in flight". */
const TERMINAL_DELIVERY_STATUSES: ReadonlySet<string> = new Set([
  "delivered",
  "cancelled",
  "failed",
]);

/**
 * Filters a delivery list down to in-flight (non-terminal) orders for the
 * payment-mode mismatch banner (DEL-OP-2 follow-up). Exported for tests.
 */
function filterInFlightDeliveries(
  deliveries: DeliveryOrder[],
): DeliveryOrder[] {
  return deliveries.filter((d) => !TERMINAL_DELIVERY_STATUSES.has(d.status));
}

/**
 * Pure helper that constructs the save payload from the local settings state.
 * Extracted so tests can import and exercise it directly rather than
 * duplicating the logic.
 */
export function buildDeliverySettingsPayload(
  settings: DeliverySettingsDto,
): UpdateDeliverySettingsInput {
  // L3-38: _client_key is a client-only expansion identity — strip it here so
  // it never reaches the wire.
  const zonesPayload = settings.zones.map(
    ({ id, _client_key: _clientKey, ...zone }) =>
      id != null && id > 0 ? { id, ...zone } : zone,
  );
  // Strip read-only DTO fields so the update payload carries only settings.
  const {
    business_id: _businessID,
    online_payment_available: _onlineAvailable,
    live_order_count: _liveCount,
    partner_fallback_available: _partnerFallback,
    estimated_delivery_minutes: _estimatedMinutes,
    ...editableSettings
  } = settings;
  const payload: UpdateDeliverySettingsInput = {
    ...editableSettings,
    zones: zonesPayload,
    in_house_delivery_enabled: settings.zones.some((z) => z.is_active),
    third_party_enabled: settings.external_partner_links.length > 0,
  };
  // DEL-OP-2: the GET DTO carries the RESOLVED payment mode — the backend maps
  // a stored 'online' preference to 'cash_on_delivery' whenever online payments
  // are unavailable (plugin disabled, no settlement address, …). Round-tripping
  // that resolved value would silently overwrite the operator's stored 'online'
  // preference. While online payments are unavailable the 'online' radio is
  // disabled, so the field cannot be meaningfully edited — omit it and leave
  // the stored preference untouched.
  if (!settings.online_payment_available) {
    delete payload.payment_mode;
  }
  return payload;
}

/**
 * Resolves a delivery-settings save error for operator toasts.
 * Prefers structured `code` → apiErrors catalog when locale is provided;
 * otherwise surfaces the backend English `error` string (DEL-OP-3).
 * Exported for tests.
 */
export function deliverySaveErrorMessage(
  err: unknown,
  fallback: string,
  locale?: string,
): string {
  // Wave C / L3-36: shared operator surface (catalog → CO-1 detail → generic).
  return surfaceBackendError(err, (locale || "en") as Locale, fallback);
}

export default function DeliverySettings({
  businessId,
  onSave,
  hideHeader = false,
  onDeliveryStatusChange,
}: DeliverySettingsProps) {
  const { showSuccess, showError } = useToast();
  const { hasAccess, loading: accessLoading } = useBusinessAccess(businessId);
  const { locale: currentLocale } = useSimpleLocale();

  const tString = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`deliverySettings.${key}`, currentLocale);
      let value = Array.isArray(result) ? result[0] || key : (result as string);
      if (typeof value !== "string") return key;
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          value = value.replace(`{${k}}`, String(v));
        }
      }
      return value;
    },
    [currentLocale],
  );

  const [settings, setSettings] = useState<DeliverySettingsDto>(defaultSettings);
  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  // Deep-compare against the last loaded/saved snapshot (not a sticky
  // "I touched a field" flag) so reverting an edit clears dirty.
  const { dirty: hasChanges, markClean } = useDirtyForm(settings);

  // H2a: prompt before leaving (tab close / reload / in-app tab switch) while
  // there are unsaved delivery-settings edits.
  useUnsavedChangesGuard(hasChanges, "delivery-settings");
  // Shared React Query business read (currency) — same key as Dispatch/Performance.
  const { data: business } = useDeliveryBusiness(businessId);
  const businessCurrency = business?.default_currency || "USD";
  // In-flight orders feed the ConfigurationSection payment-mode mismatch
  // banner (orders whose stored creation-time mode differs from the current
  // setting). Best-effort: a fetch failure (e.g. a role without dispatch
  // read) just leaves the banner off. Limited to the most recent 100 rows
  // (surfaced honestly in the banner copy).
  const [inFlightOrders, setInFlightOrders] = useState<DeliveryOrder[]>([]);

  useEffect(() => {
    let cancelled = false;
    if (!businessId) return;
    deliveryApi
      .getBusinessDeliveries(businessId, { limit: 100 })
      .then((res) => {
        if (cancelled) return;
        setInFlightOrders(filterInFlightDeliveries(res.deliveries ?? []));
      })
      .catch(() => {
        // Best-effort — mismatch banner simply stays hidden.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  const loadDeliveryData = useCallback(async () => {
    try {
      setIsLoading(true);
      const res = await deliveryApi.getDeliverySettings(businessId);
      const next: DeliverySettingsDto = {
        ...defaultSettings,
        ...res,
        external_partner_links: res.external_partner_links || [],
        // L3-38: stamp client-only identities so the zone editor keys off
        // something stable rather than server ids that shift on save.
        zones: ensureZoneClientKeys(res.zones || []),
      };
      setSettings(next);
      markClean(next);
    } catch (err) {
      // The toast stays generic — an operator should not be shown a stack
      // trace. But the cause must not be discarded: a 500 and a TypeError
      // thrown by our own zone normalization look identical from here, and a
      // bare `catch {}` means nobody ever learns which one ran.
      captureClientError({
        error: err as Error | string,
        component: "DeliverySettings",
        functionName: "loadDeliveryData",
        additionalInfo: { businessId },
      });
      showError(tString("focused.toasts.loadError"));
    } finally {
      setIsLoading(false);
    }
  }, [businessId, showError, tString, markClean]);

  useEffect(() => {
    if (businessId) {
      loadDeliveryData().catch((err) =>
        console.error("loadDeliveryData failed:", err),
      );
    }
  }, [businessId, loadDeliveryData]);

  const updateSetting = <K extends keyof DeliverySettingsDto>(
    key: K,
    value: DeliverySettingsDto[K],
  ) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
  };

  const handleSave = async () => {
    try {
      setIsSaving(true);
      const payload = buildDeliverySettingsPayload(settings);
      const saved = await deliveryApi.updateDeliverySettings(businessId, payload);
      // L3-38: the response is re-sorted and brand-new zones only get their
      // real id here — carry the client keys over so the card the operator was
      // editing stays open and does not swap contents.
      const next: DeliverySettingsDto = {
        ...saved,
        zones: reconcileZoneClientKeys(settings.zones, saved.zones || []),
      };
      setSettings(next);
      markClean(next);
      onSave?.(next);
      showSuccess(tString("focused.toasts.saved"));
    } catch (err) {
      // Surface the backend's specific validation message (zone priority /
      // name / boundaries) instead of a generic failure (DEL-OP-3).
      showError(
        deliverySaveErrorMessage(
          err,
          tString("focused.toasts.saveError"),
          currentLocale,
        ),
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleToggleStatusChange = (enabled: boolean) => {
    setSettings((prev) => ({ ...prev, delivery_enabled: enabled }));
    onDeliveryStatusChange?.(enabled);
  };

  const showLoading = isLoading || accessLoading;

  // Content slot only — the shell owns the centered canvas and keeps the
  // header mounted while this loads (S-9).
  const loadingView = (
    <div className="flex justify-center items-center py-16">
      <PrimarySpinner />
    </div>
  );

  const lockedView = (
    <DashboardLockedTabView
      title={tString("focused.locked.title")}
      subtitle={tString("focused.locked.subtitle")}
      businessId={businessId}
    />
  );

  // Embedded inside DeliveryAdmin: the parent DashboardTabShell already owns
  // the page header + max-width container, so loading/locked render as
  // content-level states within the parent's tabpanel.
  if (hideHeader) {
    if (showLoading) return loadingView;
    if (!hasAccess) return lockedView;
  }

  const partnerCount = settings.external_partner_links.length;
  const zoneCount = settings.zones.filter((z) => z.is_active).length;

  // The delivery form body. Reused by both the standalone (own shell) and the
  // embedded (parent shell) render paths.
  const body = (
    <>
      {!settings.delivery_enabled ? (
        <DeliveryToggle
          businessId={businessId}
          variant="card"
          enabled={settings.delivery_enabled}
          onStatusChange={handleToggleStatusChange}
        />
      ) : (
        <>
          {/* Status card + toggle button */}
          <Card>
            <CardBody className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
              <div className="flex items-start gap-3">
                <div className="w-10 h-10 rounded-xl bg-emerald-50 border border-emerald-100 flex items-center justify-center flex-shrink-0">
                  <Truck className="w-5 h-5 text-emerald-700" />
                </div>
                <div>
                  <p className="font-medium text-ink-900">
                    {tString("focused.status.onTitle")}
                  </p>
                  <p className="text-sm text-ink-700">
                    {zoneCount > 0
                      ? zoneCount === 1
                        ? tString("focused.status.activeZones", { count: zoneCount })
                        : tString("focused.status.activeZonesPlural", { count: zoneCount })
                      : partnerCount === 0
                        ? tString("focused.status.addProviderHint")
                        : partnerCount === 1
                          ? tString("focused.status.showingProviders", {
                              count: partnerCount,
                            })
                          : tString("focused.status.showingProvidersPlural", {
                              count: partnerCount,
                            })}
                  </p>
                </div>
              </div>
              <DeliveryToggle
                businessId={businessId}
                variant="button"
                enabled={settings.delivery_enabled}
                onStatusChange={handleToggleStatusChange}
              />
            </CardBody>
          </Card>

          <ConfigurationSection
            settings={settings}
            onChange={updateSetting}
            tString={tString}
            currency={businessCurrency}
            inFlightOrders={inFlightOrders}
          />

          <ZonesSection
            zones={settings.zones}
            onChange={(zones) => updateSetting("zones", zones)}
            tString={tString}
            currencyPrefix={deriveCurrencyPrefix(businessCurrency, "symbol")}
          />

          <HoursSection
            settings={settings}
            onChange={updateSetting}
            tString={tString}
          />

          <PartnersSection
            businessId={businessId}
            links={settings.external_partner_links}
            onChange={(links) => updateSetting("external_partner_links", links)}
            tString={tString}
          />

          <CustomerNoteSection
            settings={settings}
            onChange={(value) => updateSetting("delivery_instructions", value)}
            tString={(key) => tString(`focused.${key}`)}
          />
        </>
      )}

      {/* Shared sticky save bar */}
      {settings.delivery_enabled && (
        <SaveBar
          mode="button"
          isSaving={isSaving}
          dirty={hasChanges}
          onSave={handleSave}
          labels={{
            save: tString("focused.save.button"),
            saving: tString("focused.save.saving"),
            unsaved: tString("focused.save.unsavedChanges"),
            auto: tString("focused.save.allSaved"),
            clean: tString("focused.save.allSaved"),
          }}
        />
      )}
    </>
  );

  // Embedded: render the body directly into the parent DeliveryAdmin shell's
  // tabpanel (no nested shell/header/container).
  if (hideHeader) {
    return <div className="space-y-6">{body}</div>;
  }

  // Standalone: own the shared tab shell so the title size/layout matches
  // sibling settings tabs. Loading/locked resolve through the shell so the
  // header never disappears.
  return (
    <DashboardTabShell
      loading={showLoading ? loadingView : null}
      locked={!showLoading && !hasAccess ? lockedView : null}
      header={{
        title: tString("focused.headerTitle"),
        subtitle: tString("focused.headerDescription"),
        status: {
          label: settings.delivery_enabled
            ? tString("focused.status.on")
            : tString("focused.status.off"),
          tone: settings.delivery_enabled ? "positive" : "attention",
        },
      }}
    >
      {body}
    </DashboardTabShell>
  );
}

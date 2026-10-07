import React, { useState, useEffect, useCallback, useMemo, useId } from "react";
import toast from "react-hot-toast";
import {
  Input,
  Button,
  SelectItem,
} from "@nextui-org/react";
import { NamedSwitch } from "@/components/ui/NamedSwitch";
import { NamedSelect } from "@/components/ui/NamedSelect";
import { Settings } from "lucide-react";
import {
  reservationAPI,
  ReservationSettings as ReservationSettingsType,
} from "@/api/reservations";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import ExternalPartnerLinksEditor from "./ExternalPartnerLinksEditor";
import { parseIntegerInput } from "./reservationSettingsParsers";
import { reservationErrorMessage } from "./reservationFormHelpers";
import { PremiumPanel } from "./premium";
import SaveBar from "./SaveBar";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";

interface ReservationSettingsProps {
  businessId: number;
}

const reservationSettingsSectionTitleClass =
  "mb-4 text-sm font-semibold uppercase tracking-[0.16em] text-brand-700";
const reservationSettingsSwitchRowClass =
  "flex items-center justify-between gap-4 rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3";
const reservationSettingsOptionTitleClass = "font-medium text-ink-950";
const reservationSettingsOptionCopyClass = "text-sm leading-5 text-ink-600";
const reservationSettingsUnitClass = "text-sm text-ink-500";
const reservationSettingsInputClassNames = {
  label: "text-ink-700",
  input: "text-ink-900",
  inputWrapper:
    "border-warm-200 bg-white/90 shadow-sm hover:border-brand/25 focus-within:border-brand",
};

const RESERVATION_SETTINGS_FALLBACKS: Readonly<Record<string, string>> = {
  loadError: "Couldn't load reservation settings.",
  retry: "Retry",
  partySizeRangeError:
    "Minimum party size can't be larger than the maximum.",
};

export default function ReservationSettings({
  businessId,
}: ReservationSettingsProps) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  const [settings, setSettings] = useState<ReservationSettingsType | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [saving, setSaving] = useState(false);
  // Serialized baseline of the last loaded/saved settings; drives the dirty
  // flag so the Save bar mirrors DeliverySettings (audit L6 #19).
  const [baseline, setBaseline] = useState<string | null>(null);
  const settingsA11yId = useId();

  const hasChanges = useMemo(() => {
    if (!settings || baseline === null) return false;
    return JSON.stringify(settings) !== baseline;
  }, [settings, baseline]);

  useUnsavedChangesGuard(hasChanges, "reservation-settings");

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.reservations.settings.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const tSaveBar = useCallback(
    (key: string): string => {
      const result = getTranslation(`businessSettings.saveBar.${key}`, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // English defaults for keys pending the businessDashboard.json translation
  // pass (I18N-NEEDED). getTranslation returns the raw key path when a key is
  // missing; detect that and fall back so the UI never shows a dotted path.
  const tFallback = useCallback(
    (key: string): string => {
      const value = t(key);
      if (value.startsWith("businessDashboard.reservations.settings.")) {
        return RESERVATION_SETTINGS_FALLBACKS[key] ?? value;
      }
      return value;
    },
    [t],
  );

  const loadSettings = useCallback(async () => {
    try {
      setLoading(true);
      setLoadError(false);
      const data = await reservationAPI.getSettings(businessId);
      const normalized = {
        ...data,
        external_partner_links: data.external_partner_links || [],
      };
      setSettings(normalized);
      setBaseline(JSON.stringify(normalized));
    } catch (error) {
      console.error("[ReservationSettings] Failed to load settings:", error);
      // Surface a retryable error instead of an indefinite "Loading…" spinner.
      setLoadError(true);
    } finally {
      setLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    loadSettings().catch((err) => console.error("loadSettings failed:", err));
  }, [loadSettings]);

  const handleSave = async () => {
    if (!settings) return;

    // Reject contradictory cross-field bounds before hitting the server so the
    // operator gets a precise, local message instead of a generic 400.
    if (settings.min_party_size > settings.max_party_size) {
      toast.error(tFallback("partySizeRangeError"));
      return;
    }

    setSaving(true);
    try {
      const savedSettings = await reservationAPI.updateSettings(businessId, settings);
      const normalized = {
        ...savedSettings,
        external_partner_links: savedSettings.external_partner_links || [],
      };
      setSettings(normalized);
      setBaseline(JSON.stringify(normalized));
      toast.success(t("saveSuccess"));
    } catch (error) {
      console.error("Failed to save settings:", error);
      // Preserve the backend's reason (e.g. a validation detail) rather than a
      // generic "couldn't save" that hides why.
      toast.error(reservationErrorMessage(error, t("saveError")));
    } finally {
      setSaving(false);
    }
  };

  const handlePartnerLinksChange = (
    links: ReservationSettingsType["external_partner_links"],
  ) => {
    if (!settings) return;

    setSettings({
      ...settings,
      external_partner_links: links,
    });
  };

  if (loadError && !settings) {
    return (
      <PremiumPanel className="p-6" withTexture={false}>
        <div className="flex flex-col items-center gap-3 text-center">
          <p className="text-sm text-ink-600">{tFallback("loadError")}</p>
          <Button
            onPress={() => {
              loadSettings().catch((err) =>
                console.error("loadSettings retry failed:", err),
              );
            }}
            className="bg-brand text-white font-medium hover:bg-brand-dark"
          >
            {tFallback("retry")}
          </Button>
        </div>
      </PremiumPanel>
    );
  }

  if (loading || !settings) {
    return (
      <PremiumPanel className="p-6" withTexture={false}>
        <p className="text-center text-sm text-ink-600">{t("loading")}</p>
      </PremiumPanel>
    );
  }

  return (
    <PremiumPanel className="overflow-hidden" withTexture={false}>
      <div className="flex gap-3 border-b border-warm-200/80 px-5 py-4 sm:px-6">
        <div className="flex h-10 w-10 items-center justify-center rounded-2xl border border-brand/15 bg-brand/5 text-brand">
          <Settings className="h-5 w-5" />
        </div>
        <div className="flex flex-col">
          <p className="text-base font-semibold text-ink-950">{t("title")}</p>
          <p className="text-sm leading-5 text-ink-600">
            {t("description")}
          </p>
        </div>
      </div>
      <div className="space-y-8 p-5 sm:p-6">
        {/* Capacity Settings */}
        <div>
          <h4 className={reservationSettingsSectionTitleClass}>
            {t("capacity")}
          </h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label={t("minPartySize")}
              type="number"
              value={settings.min_party_size.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  min_party_size: parseIntegerInput(value, settings.min_party_size),
                })
              }
              min={1}
              max={settings.max_party_size}
              description={t("minPartySizeDesc")}
              classNames={{
                ...reservationSettingsInputClassNames,
              }}
            />
            <Input
              label={t("maxPartySize")}
              type="number"
              value={settings.max_party_size.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  max_party_size: parseIntegerInput(value, settings.max_party_size),
                })
              }
              min={settings.min_party_size}
              max={100}
              description={t("maxPartySizeDesc")}
              classNames={{
                ...reservationSettingsInputClassNames,
              }}
            />
          </div>
        </div>

        {/* Booking Window */}
        <div>
          <h4 className={reservationSettingsSectionTitleClass}>
            {t("bookingWindow")}
          </h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label={t("maxAdvanceDays")}
              type="number"
              value={settings.max_advance_days.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  max_advance_days: parseIntegerInput(value, settings.max_advance_days),
                })
              }
              min={1}
              max={365}
              endContent={
                <span className={reservationSettingsUnitClass}>
                  {t("days")}
                </span>
              }
              description={t("maxAdvanceDaysDesc")}
              classNames={{
                ...reservationSettingsInputClassNames,
              }}
            />
            <Input
              label={t("defaultDuration")}
              type="number"
              value={settings.default_duration.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  default_duration: parseIntegerInput(value, settings.default_duration),
                })
              }
              min={30}
              max={480}
              endContent={
                <span className={reservationSettingsUnitClass}>
                  {t("minutes")}
                </span>
              }
              description={t("defaultDurationDesc")}
              classNames={{
                ...reservationSettingsInputClassNames,
              }}
            />
          </div>
        </div>

        <div>
          <h4 className={reservationSettingsSectionTitleClass}>
            {t("slotCadenceHeader")}
          </h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label={t("minAdvanceNotice")}
              type="number"
              value={settings.min_advance_minutes.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  min_advance_minutes: parseIntegerInput(value, settings.min_advance_minutes),
                })
              }
              min={0}
              endContent={<span className={reservationSettingsUnitClass}>{t("minutesUnit")}</span>}
              classNames={reservationSettingsInputClassNames}
            />
            <Input
              label={t("slotInterval")}
              type="number"
              value={settings.slot_interval_minutes.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  slot_interval_minutes: parseIntegerInput(value, settings.slot_interval_minutes),
                })
              }
              min={5}
              endContent={<span className={reservationSettingsUnitClass}>{t("minutesUnit")}</span>}
              classNames={reservationSettingsInputClassNames}
            />
            <Input
              label={t("serviceBuffer")}
              type="number"
              value={settings.service_buffer_minutes.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  service_buffer_minutes: parseIntegerInput(value, settings.service_buffer_minutes),
                })
              }
              min={0}
              endContent={<span className={reservationSettingsUnitClass}>{t("minutesUnit")}</span>}
              classNames={reservationSettingsInputClassNames}
            />
            <Input
              label={t("maxCoversPerSlot")}
              type="number"
              value={settings.max_covers_per_slot.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  max_covers_per_slot: parseIntegerInput(value, settings.max_covers_per_slot),
                })
              }
              min={0}
              description={t("maxCoversPerSlotDesc")}
              classNames={reservationSettingsInputClassNames}
            />
          </div>
          <div className="space-y-4 mt-4">
            <div className={reservationSettingsSwitchRowClass}>
              <div>
                <p
                  id={`${settingsA11yId}-auto-assign-label`}
                  className={reservationSettingsOptionTitleClass}
                >
                  {t("autoAssignTables")}
                </p>
                <p
                  id={`${settingsA11yId}-auto-assign-desc`}
                  className={reservationSettingsOptionCopyClass}
                >
                  {t("autoAssignTablesDesc")}
                </p>
              </div>
              <NamedSwitch
                name={t("autoAssignTables")}
                labelId={`${settingsA11yId}-auto-assign-label`}
                descriptionId={`${settingsA11yId}-auto-assign-desc`}
                isSelected={settings.auto_assign_tables}
                onValueChange={(value) =>
                  setSettings({ ...settings, auto_assign_tables: value })
                }
              />
            </div>
            <div className={reservationSettingsSwitchRowClass}>
              <div>
                <p
                  id={`${settingsA11yId}-waitlist-label`}
                  className={reservationSettingsOptionTitleClass}
                >
                  {t("allowWaitlist")}
                </p>
                <p
                  id={`${settingsA11yId}-waitlist-desc`}
                  className={reservationSettingsOptionCopyClass}
                >
                  {t("allowWaitlistDesc")}
                </p>
              </div>
              <NamedSwitch
                name={t("allowWaitlist")}
                labelId={`${settingsA11yId}-waitlist-label`}
                descriptionId={`${settingsA11yId}-waitlist-desc`}
                isSelected={settings.allow_waitlist}
                onValueChange={(value) =>
                  setSettings({ ...settings, allow_waitlist: value })
                }
              />
            </div>
          </div>
        </div>

        {/* Policies */}
        <div>
          <h4 className={reservationSettingsSectionTitleClass}>
            {t("policies")}
          </h4>
          <div className="space-y-4">
            <div className={`${reservationSettingsSwitchRowClass} flex-col sm:flex-row sm:items-center sm:justify-between`}>
              <div>
                <p
                  id={`${settingsA11yId}-approval-label`}
                  className={reservationSettingsOptionTitleClass}
                >
                  {t("approvalModeHeader")}
                </p>
                <p
                  id={`${settingsA11yId}-approval-desc`}
                  className={reservationSettingsOptionCopyClass}
                >
                  {t("approvalModeDesc")}
                </p>
              </div>
              <NamedSelect
                name={t("approvalModeHeader")}
                valueLabel={
                  settings.approval_mode === "manual"
                    ? t("approvalModeManual")
                    : t("approvalModeAuto")
                }
                descriptionId={`${settingsA11yId}-approval-desc`}
                disallowEmptySelection
                selectedKeys={[settings.approval_mode]}
                onChange={(e) => {
                  // Guard the empty-deselection event NextUI can still emit:
                  // never silently flip a manual-approval business to auto.
                  if (e.target.value !== "manual" && e.target.value !== "auto") {
                    return;
                  }
                  setSettings({
                    ...settings,
                    approval_mode: e.target.value,
                  });
                }}
                className="max-w-xs"
              >
                <SelectItem key="auto" value="auto">
                  {t("approvalModeAuto")}
                </SelectItem>
                <SelectItem key="manual" value="manual">
                  {t("approvalModeManual")}
                </SelectItem>
              </NamedSelect>
            </div>
            <div className={reservationSettingsSwitchRowClass}>
              <div>
                <p
                  id={`${settingsA11yId}-cancel-label`}
                  className={reservationSettingsOptionTitleClass}
                >
                  {t("allowCancellation")}
                </p>
                <p
                  id={`${settingsA11yId}-cancel-desc`}
                  className={reservationSettingsOptionCopyClass}
                >
                  {t("allowCancellationDesc")}
                </p>
              </div>
              <NamedSwitch
                name={t("allowCancellation")}
                labelId={`${settingsA11yId}-cancel-label`}
                descriptionId={`${settingsA11yId}-cancel-desc`}
                isSelected={settings.allow_cancellation}
                onValueChange={(value) =>
                  setSettings({ ...settings, allow_cancellation: value })
                }
              />
            </div>
            {settings.allow_cancellation && (
              <Input
                label={t("cancellationDeadline")}
                type="number"
                value={settings.cancellation_deadline.toString()}
                onValueChange={(value) =>
                  setSettings({
                    ...settings,
                    cancellation_deadline: parseIntegerInput(value, settings.cancellation_deadline),
                  })
                }
                min={1}
                max={168}
                endContent={
                  <span className={reservationSettingsUnitClass}>
                    {t("hours")}
                  </span>
                }
                description={t("cancellationDeadlineDesc")}
                classNames={reservationSettingsInputClassNames}
              />
            )}
            <Input
              label={t("holdDuration")}
              type="number"
              value={settings.hold_duration_minutes.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  hold_duration_minutes: parseIntegerInput(value, settings.hold_duration_minutes),
                })
              }
              min={0}
              endContent={<span className={reservationSettingsUnitClass}>{t("minutesUnit")}</span>}
              description={t("holdDurationDesc")}
              classNames={reservationSettingsInputClassNames}
            />
            <Input
              label={t("noShowGracePeriod")}
              type="number"
              value={settings.no_show_grace_minutes.toString()}
              onValueChange={(value) =>
                setSettings({
                  ...settings,
                  no_show_grace_minutes: parseIntegerInput(value, settings.no_show_grace_minutes),
                })
              }
              min={0}
              endContent={<span className={reservationSettingsUnitClass}>{t("minutesUnit")}</span>}
              classNames={reservationSettingsInputClassNames}
            />
          </div>
        </div>

        {/* Email Notifications */}
        <div>
          <h4 className={reservationSettingsSectionTitleClass}>
            {t("notifications")}
          </h4>
          <div className="space-y-4">
            <div className={reservationSettingsSwitchRowClass}>
              <div>
                <p
                  id={`${settingsA11yId}-confirm-email-label`}
                  className={reservationSettingsOptionTitleClass}
                >
                  {t("sendConfirmationEmail")}
                </p>
                <p
                  id={`${settingsA11yId}-confirm-email-desc`}
                  className={reservationSettingsOptionCopyClass}
                >
                  {t("sendConfirmationEmailDesc")}
                </p>
              </div>
              <NamedSwitch
                name={t("sendConfirmationEmail")}
                labelId={`${settingsA11yId}-confirm-email-label`}
                descriptionId={`${settingsA11yId}-confirm-email-desc`}
                isSelected={settings.send_confirmation_email}
                onValueChange={(value) =>
                  setSettings({ ...settings, send_confirmation_email: value })
                }
              />
            </div>
            <div className={reservationSettingsSwitchRowClass}>
              <div>
                <p
                  id={`${settingsA11yId}-reminder-email-label`}
                  className={reservationSettingsOptionTitleClass}
                >
                  {t("sendReminderEmail")}
                </p>
                <p
                  id={`${settingsA11yId}-reminder-email-desc`}
                  className={reservationSettingsOptionCopyClass}
                >
                  {t("sendReminderEmailDesc")}
                </p>
              </div>
              <NamedSwitch
                name={t("sendReminderEmail")}
                labelId={`${settingsA11yId}-reminder-email-label`}
                descriptionId={`${settingsA11yId}-reminder-email-desc`}
                isSelected={settings.send_reminder_email}
                onValueChange={(value) =>
                  setSettings({ ...settings, send_reminder_email: value })
                }
              />
            </div>
            {settings.send_reminder_email && (
              <Input
                label={t("reminderHoursBefore")}
                type="number"
                value={settings.reminder_hours_before.toString()}
                onValueChange={(value) =>
                  setSettings({
                    ...settings,
                    reminder_hours_before: parseIntegerInput(value, settings.reminder_hours_before),
                  })
                }
                min={1}
                max={168}
                endContent={
                  <span className={reservationSettingsUnitClass}>
                    {t("hours")}
                  </span>
                }
                description={t("reminderHoursBeforeDesc")}
                classNames={reservationSettingsInputClassNames}
              />
            )}
          </div>
        </div>

        <PremiumPanel tone="accent" className="p-4 sm:p-5" withTexture={false}>
          <div>
            <p className="text-sm font-semibold text-brand-800">
              {t("previewTitle")}
            </p>
            <p className="mt-1 text-sm leading-5 text-brand-700">
              {t("previewDescription")}
            </p>
          </div>
          <div className="mt-4 space-y-3 text-sm text-brand-800">
            <div className="flex flex-wrap gap-2">
              <span className="rounded-full border border-brand/20 bg-white px-3 py-1">
                {settings.approval_mode === "manual" ? t("approvalModeManualSummary") : t("approvalModeAutoSummary")}
              </span>
              <span className="rounded-full border border-brand/20 bg-white px-3 py-1">
                {t("minAdvanceNoticeChip").replace("{min}", String(settings.min_advance_minutes))}
              </span>
              <span className="rounded-full border border-brand/20 bg-white px-3 py-1">
                {t("defaultDurationChip").replace("{min}", String(settings.default_duration))}
              </span>
              {settings.allow_waitlist ? (
                <span className="rounded-full border border-brand/20 bg-white px-3 py-1">
                  {t("waitlistEnabled")}
                </span>
              ) : null}
            </div>
            <div className="space-y-1 text-brand-dark">
              <p>
                {t("partySizesSummary")
                  .replace("{min}", String(settings.min_party_size))
                  .replace("{max}", String(settings.max_party_size))}
              </p>
              <p>
                {t("slotCadenceSummary")
                  .replace("{interval}", String(settings.slot_interval_minutes))
                  .replace("{buffer}", String(settings.service_buffer_minutes))}
              </p>
              <p>
                {settings.allow_cancellation
                  ? t("cancellationAllowed").replace(
                      "{hours}",
                      String(settings.cancellation_deadline),
                    )
                : t("cancellationDisabled")}
              </p>
            </div>
          </div>
        </PremiumPanel>

        <ExternalPartnerLinksEditor
          businessId={businessId}
          links={settings.external_partner_links || []}
          onChange={handlePartnerLinksChange}
          labels={{
            title:
              t("partnerLinks.title") || "External Partner Links",
            description:
              t("partnerLinks.description") ||
              "Show fallback reservation partners when direct booking cannot satisfy the guest request.",
            addProvider:
              t("partnerLinks.addProvider") || "Add Provider",
            nameLabel:
              t("partnerLinks.nameLabel") || "Provider Name",
            urlLabel:
              t("partnerLinks.urlLabel") || "Provider URL",
            iconSourceLabel:
              t("partnerLinks.iconSourceLabel") || "Icon Source",
            sourcePredefined:
              t("partnerLinks.sourcePredefined") || "Predefined",
            sourceCustom:
              t("partnerLinks.sourceCustom") || "Custom",
            providerLabel:
              t("partnerLinks.providerLabel") || "Provider Icon",
            iconUrlLabel:
              t("partnerLinks.iconUrlLabel") || "Custom Icon URL",
            uploadIcon:
              t("partnerLinks.uploadIcon") || "Upload Icon",
            removeProvider:
              t("partnerLinks.removeProvider") || "Remove",
            maxReached:
              t("partnerLinks.maxReached") || "Maximum {max} providers",
            uploadError: t("partnerLinks.uploadError"),
            imageOnlyError: t("partnerLinks.imageOnlyError"),
          }}
        />
        <p className="-mt-4 text-xs leading-5 text-ink-600">
          {t("partnerLinks.fallbackNote")}
        </p>

        <SaveBar
          mode="button"
          isSaving={saving}
          dirty={hasChanges}
          onSave={() => void handleSave()}
          labels={{
            save: tSaveBar("save"),
            saving: tSaveBar("saving"),
            unsaved: tSaveBar("unsaved"),
            auto: tSaveBar("auto"),
            clean: tSaveBar("clean"),
          }}
        />
      </div>
    </PremiumPanel>
  );
}

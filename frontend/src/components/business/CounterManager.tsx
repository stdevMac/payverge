"use client";

import React, { useState, useEffect, useCallback, useMemo } from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { Input } from "@nextui-org/react";
import {
  Coffee,
  Settings,
  CheckCircle,
  AlertCircle,
  Info,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import {
  updateCounterSettings,
  getBusinessCounters,
  COUNTER_PREFIX_MAX_LENGTH,
  Counter,
} from "../../api/counters";
import {
  desiredCounterName,
  listCounterNamingMismatches,
} from "../../api/counterNaming";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useDirtyForm } from "@/hooks/useDirtyForm";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";
import { surfaceBackendError } from "@/utils/localizedError";
import DashboardLockedTabView from "./DashboardLockedTabView";
import { CounterToggle } from "./CounterToggle";
import { CounterSkeleton } from "./CounterSkeleton";
import { PremiumPanel } from "./premium";
import DashboardTabShell from "./shared/DashboardTabShell";
import SaveBar from "./SaveBar";

interface CounterManagerProps {
  businessId: number;
}

interface CounterSettings {
  counter_enabled: boolean;
  counter_count: number;
  counter_prefix: string;
}

const COUNTER_COUNT_MIN = 1;
const COUNTER_COUNT_MAX = 20;

const clampCounterCount = (value: number): number =>
  Math.min(COUNTER_COUNT_MAX, Math.max(COUNTER_COUNT_MIN, value));

const CounterManager: React.FC<CounterManagerProps> = ({
  businessId,
}) => {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  const { hasAccess, loading: accessLoading } = useBusinessAccess(businessId);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.counterManager.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const saveBarString = useCallback(
    (key: string): string => {
      const result = getTranslation(
        `businessSettings.saveBar.${key}`,
        currentLocale,
      );
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [applyingPrefix, setApplyingPrefix] = useState(false);
  const [counters, setCounters] = useState<Counter[]>([]);
  const [showArchived, setShowArchived] = useState(false);
  const [settings, setSettings] = useState<CounterSettings>({
    counter_enabled: false,
    counter_count: 3,
    counter_prefix: "C",
  });
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  // Deep-compare against the last server-loaded settings for SaveBar + guard.
  const { dirty, markClean } = useDirtyForm(settings);
  useUnsavedChangesGuard(dirty, "counter-settings");

  const liveCounters = useMemo(
    () => counters.filter((counter) => counter.is_active),
    [counters],
  );
  const archivedCounters = useMemo(
    () => counters.filter((counter) => !counter.is_active),
    [counters],
  );

  const namingMismatches = useMemo(
    () => listCounterNamingMismatches(liveCounters, settings.counter_prefix),
    [liveCounters, settings.counter_prefix],
  );
  const applyableMismatches = useMemo(
    () => namingMismatches.filter((m) => m.applyable),
    [namingMismatches],
  );
  const customKeptMismatches = useMemo(
    () => namingMismatches.filter((m) => !m.applyable),
    [namingMismatches],
  );

  const loadCounterData = useCallback(async () => {
    try {
      setLoading(true);
      const response = await getBusinessCounters(businessId);
      // L2-31 / L2-36: trust business.counter_enabled from the API — never
      // infer enablement from row count (inactive rows still exist after
      // soft-disable). Prefer counter_prefix from business, not name regex.
      const rows = response.counters || [];
      setCounters(rows);
      if (response.business) {
        const activeCount = rows.filter((c) => c.is_active).length;
        const next: CounterSettings = {
          counter_enabled: Boolean(response.business.counter_enabled),
          counter_count:
            response.business.counter_count > 0
              ? response.business.counter_count
              : Math.max(activeCount, 1),
          counter_prefix:
            (response.business.counter_prefix || "C").trim() || "C",
        };
        setSettings(next);
        markClean(next);
      } else {
        markClean();
      }
    } catch (error) {
      console.error("Error loading counter data:", error);
      setError(tString("error.loadSettings"));
    } finally {
      setLoading(false);
    }
  }, [businessId, tString, markClean]);

  useEffect(() => {
    if (!accessLoading && hasAccess) {
      loadCounterData().catch((err) =>
        console.error("loadCounterData failed:", err),
      );
    }
  }, [loadCounterData, accessLoading, hasAccess]);

  const counterCountInvalid =
    !Number.isInteger(settings.counter_count) ||
    settings.counter_count < COUNTER_COUNT_MIN ||
    settings.counter_count > COUNTER_COUNT_MAX;

  const trimmedPrefix = settings.counter_prefix.trim();
  const counterPrefixInvalid =
    trimmedPrefix.length === 0 ||
    trimmedPrefix.length > COUNTER_PREFIX_MAX_LENGTH;

  const handleSaveSettings = async () => {
    // L2-33: refuse 0 (and out-of-range) with an inline error instead of
    // silently clamping to 1 on save.
    if (counterCountInvalid) {
      setError(
        tString("settings.counterCountInvalid") ||
          `Enter a counter count between ${COUNTER_COUNT_MIN} and ${COUNTER_COUNT_MAX}.`,
      );
      return;
    }
    // L2-34: refuse empty/whitespace prefix with S-5 inline validation.
    if (counterPrefixInvalid) {
      setError(
        tString("settings.counterPrefixInvalid") ||
          `Enter a prefix of 1–${COUNTER_PREFIX_MAX_LENGTH} characters.`,
      );
      return;
    }
    try {
      setSaving(true);
      setError(null);
      setSuccess(null);

      const safeSettings = {
        ...settings,
        counter_count: clampCounterCount(settings.counter_count),
        counter_prefix: trimmedPrefix,
      };
      await updateCounterSettings(businessId, safeSettings);

      setSuccess(tString("success.settingsUpdated"));
      await loadCounterData();

      setTimeout(() => setSuccess(null), 3000);
    } catch (error) {
      console.error("Error updating counter settings:", error);
      // L2-34: surface backend message instead of a silent generic string.
      // surfaceBackendError is pure — its return value IS the message to show.
      setError(
        surfaceBackendError(
          error,
          currentLocale,
          tString("error.updateSettings"),
        ),
      );
    } finally {
      setSaving(false);
    }
  };

  const handleSettingChange = (key: keyof CounterSettings, value: any) => {
    setSettings((prev) => ({
      ...prev,
      [key]: value,
    }));
  };

  const handleCounterStatusChange = (enabled: boolean) => {
    setSettings((prev) => ({
      ...prev,
      counter_enabled: enabled,
    }));
    // Reload data to reflect the new state
    loadCounterData().catch((err) =>
      console.error("loadCounterData failed:", err),
    );
  };

  /** One-click: persist current prefix/count so BE rewrites seed/default names (#192). */
  const handleApplyPrefix = async () => {
    if (counterCountInvalid || counterPrefixInvalid) {
      setError(
        counterPrefixInvalid
          ? tString("settings.counterPrefixInvalid")
          : tString("settings.counterCountInvalid"),
      );
      return;
    }
    try {
      setApplyingPrefix(true);
      setError(null);
      setSuccess(null);
      await updateCounterSettings(businessId, {
        counter_enabled: settings.counter_enabled,
        counter_count: clampCounterCount(settings.counter_count),
        counter_prefix: trimmedPrefix,
      });
      setSuccess(tString("success.prefixApplied"));
      await loadCounterData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      console.error("Error applying counter prefix:", err);
      setError(
        surfaceBackendError(
          err,
          currentLocale,
          tString("error.updateSettings"),
        ),
      );
    } finally {
      setApplyingPrefix(false);
    }
  };

  return (
    <DashboardTabShell
      locked={
        !accessLoading && !hasAccess ? (
          <DashboardLockedTabView
            title={tString("title")}
            subtitle={tString("subtitle")}
            businessId={businessId}
          />
        ) : null
      }
      loading={loading || accessLoading ? <CounterSkeleton /> : null}
      header={{
        title: tString("title"),
        subtitle: tString("subtitle"),
        status: {
          label: settings.counter_enabled
            ? tString("header.statusOn")
            : tString("header.statusOff"),
          // Neutral tone — enabled means naming setup is on, not a live queue.
          tone: "neutral",
        },
        stats: settings.counter_enabled
          ? [
              {
                label: tString("header.counters"),
                value: settings.counter_count,
              },
            ]
          : [],
      }}
    >
      {/* Honesty banner: this tab is setup-only until a real pickup board ships. */}
      <div
        className="rounded-2xl border border-amber-200 bg-amber-50 p-4"
        data-testid="counter-honesty-banner"
        role="status"
      >
        <div className="flex items-start gap-3">
          <Info
            className="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-800"
            aria-hidden="true"
          />
          <div className="space-y-1">
            <p className="font-semibold text-amber-950">
              {tString("honesty.title")}
            </p>
            <p className="text-sm leading-6 text-amber-900">
              {tString("honesty.body")}
            </p>
          </div>
        </div>
      </div>

      {/* Activation Card (when disabled) */}
      {!settings.counter_enabled && (
        <CounterToggle
          businessId={businessId}
          variant="card"
          enabled={settings.counter_enabled}
          onStatusChange={handleCounterStatusChange}
        />
      )}

      {/* Error Alert */}
      {error && (
        <div className="rounded-2xl border border-rose-200 bg-rose-50 p-4">
          <div className="flex items-center gap-3">
            <AlertCircle className="w-5 h-5 text-rose-700 flex-shrink-0" />
            <p className="font-medium text-rose-800">{error}</p>
          </div>
        </div>
      )}

      {/* Success Alert */}
      {success && (
        <div className="rounded-2xl border border-emerald-200 bg-emerald-50 p-4">
          <div className="flex items-center gap-3">
            <CheckCircle className="w-5 h-5 text-emerald-700 flex-shrink-0" />
            <p className="font-medium text-emerald-800">{success}</p>
          </div>
        </div>
      )}

      {/* Counter Settings (only when enabled) */}
      {settings.counter_enabled && (
        <>
          <PremiumPanel className="overflow-hidden" withTexture={false}>
            <div className="border-b border-warm-200/80 p-5 sm:p-6">
              <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 items-start gap-3">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-brand/10 text-brand">
                    <Settings className="h-6 w-6" aria-hidden="true" />
                  </div>
                  <div className="min-w-0">
                    <h2 className="text-base font-semibold text-ink-900">
                      {tString("settings.title")}
                    </h2>
                    <p className="text-sm leading-6 text-ink-600">
                      {tString("settings.subtitle")}
                    </p>
                  </div>
                </div>
                <div className="shrink-0 self-start sm:self-center">
                  <CounterToggle
                    businessId={businessId}
                    variant="button"
                    enabled={settings.counter_enabled}
                    onStatusChange={handleCounterStatusChange}
                  />
                </div>
              </div>
            </div>
            <div className="space-y-6 p-5 sm:p-6">
              {/* Counter Count & Prefix */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                  <Input
                    type="text"
                    inputMode="numeric"
                    variant="bordered"
                    label={tString("settings.counterCount")}
                    placeholder="3"
                    value={settings.counter_count.toString()}
                    onValueChange={(raw) => {
                      // Allow clearing mid-edit without snapping; invalid
                      // values surface isInvalid and block save (L2-33).
                      if (raw === "") {
                        handleSettingChange("counter_count", 0);
                        return;
                      }
                      const parsed = parseInt(raw, 10);
                      if (Number.isNaN(parsed)) return;
                      handleSettingChange(
                        "counter_count",
                        Math.min(COUNTER_COUNT_MAX, parsed),
                      );
                    }}
                    isInvalid={counterCountInvalid}
                    errorMessage={
                      counterCountInvalid
                        ? tString("settings.counterCountInvalid") ||
                          `Enter a counter count between ${COUNTER_COUNT_MIN} and ${COUNTER_COUNT_MAX}.`
                        : undefined
                    }
                    description={tString("settings.counterCountDescription")}
                    data-testid="counter-count-input"
                  />
                </div>

                <div>
                  <Input
                    variant="bordered"
                    label={tString("settings.counterPrefix")}
                    placeholder="C"
                    value={settings.counter_prefix}
                    onChange={(e) =>
                      handleSettingChange(
                        "counter_prefix",
                        e.target.value.slice(0, COUNTER_PREFIX_MAX_LENGTH),
                      )
                    }
                    maxLength={COUNTER_PREFIX_MAX_LENGTH}
                    isInvalid={counterPrefixInvalid}
                    errorMessage={
                      counterPrefixInvalid
                        ? tString("settings.counterPrefixInvalid") ||
                          `Enter a prefix of 1–${COUNTER_PREFIX_MAX_LENGTH} characters.`
                        : undefined
                    }
                    description={(() => {
                      const prefix =
                        (settings.counter_prefix || "C").trim() || "C";
                      const base =
                        tString("settings.counterPrefixDescription").replaceAll(
                          "{prefix}",
                          prefix,
                        ) || `Up to ${COUNTER_PREFIX_MAX_LENGTH} characters.`;
                      const examples = [1, 2, 3]
                        .slice(
                          0,
                          Math.min(3, Math.max(1, settings.counter_count)),
                        )
                        .map((n) => `${prefix}${n}`)
                        .join(", ");
                      const preview = tString(
                        "settings.counterPrefixPreview",
                      ).replace("{examples}", examples);
                      return preview.includes("counterPrefixPreview")
                        ? base
                        : `${base} ${preview}`;
                    })()}
                    data-testid="counter-prefix-input"
                  />
                </div>
              </div>
            </div>
          </PremiumPanel>

          {/* Live counters — matches business.counter_count. Excess rows
              stay in the archived section so a 2→3→2 shrink cannot contradict
              the summary (#393). */}
          {liveCounters.length > 0 && (
            <PremiumPanel className="p-5 sm:p-6" withTexture={false}>
              <div className="mb-5 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-base font-semibold text-ink-900">
                    {tString("activeCounters.title")}
                  </h2>
                  <p className="text-sm leading-6 text-ink-600">
                    {tString("activeCounters.subtitle")}
                  </p>
                </div>
                {applyableMismatches.length > 0 && (
                  <button
                    type="button"
                    data-testid="counter-apply-prefix"
                    onClick={() => void handleApplyPrefix()}
                    disabled={applyingPrefix || saving || counterPrefixInvalid}
                    className="inline-flex shrink-0 items-center justify-center rounded-full bg-brand px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {applyingPrefix
                      ? tString("activeCounters.applyingPrefix")
                      : tString("activeCounters.applyPrefix")
                          .replace("{prefix}", trimmedPrefix || "C")
                          .replace(
                            "{count}",
                            String(applyableMismatches.length),
                          )}
                  </button>
                )}
              </div>

              {applyableMismatches.length > 0 && (
                <div
                  className="mb-4 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-950"
                  data-testid="counter-prefix-mismatch-banner"
                  role="status"
                >
                  {tString("activeCounters.applyPrefixHint")
                    .replace("{prefix}", trimmedPrefix || "C")
                    .replace(
                      "{examples}",
                      applyableMismatches
                        .slice(0, 3)
                        .map((m) => `${m.current} → ${m.desired}`)
                        .join(", "),
                    )}
                </div>
              )}

              {customKeptMismatches.length > 0 && (
                <div
                  className="mb-4 rounded-2xl border border-warm-200 bg-warm-50 px-4 py-3 text-sm leading-6 text-ink-700"
                  data-testid="counter-custom-names-banner"
                  role="status"
                >
                  {tString("activeCounters.customNamesKept")
                    .replace("{count}", String(customKeptMismatches.length))
                    .replace(
                      "{names}",
                      customKeptMismatches.map((m) => m.current).join(", "),
                    )}
                </div>
              )}

              <div
                className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6"
                data-testid="counter-primary-grid"
              >
                {liveCounters.map((counter) => {
                  const desired = desiredCounterName(
                    settings.counter_prefix,
                    counter.counter_number,
                  );
                  const mismatch = namingMismatches.find(
                    (m) => m.id === counter.id,
                  );
                  return (
                    <div
                      key={counter.id}
                      className="rounded-2xl border border-warm-200 bg-white/80 p-4 text-center shadow-sm"
                      data-testid={`counter-tile-${counter.counter_number}`}
                    >
                      <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-brand/10 text-brand">
                        <Coffee className="h-6 w-6" aria-hidden="true" />
                      </div>
                      <h3 className="font-semibold text-ink-900">
                        {counter.name}
                      </h3>
                      {mismatch?.applyable && (
                        <p
                          className="mt-1 text-xs font-medium text-amber-800"
                          data-testid={`counter-will-become-${counter.counter_number}`}
                        >
                          {tString("activeCounters.willBecome").replace(
                            "{name}",
                            desired,
                          )}
                        </p>
                      )}
                      {mismatch && !mismatch.applyable && (
                        <p
                          className="mt-1 text-xs font-medium text-ink-500"
                          data-testid={`counter-custom-kept-${counter.counter_number}`}
                        >
                          {tString("activeCounters.customKeptBadge")}
                        </p>
                      )}
                      <span className="mt-3 inline-flex rounded-full bg-emerald-50 px-2 py-1 text-xs font-semibold text-emerald-700">
                        {tString("status.active")}
                      </span>
                    </div>
                  );
                })}
              </div>
            </PremiumPanel>
          )}

          {archivedCounters.length > 0 && (
            <PremiumPanel className="p-5 sm:p-6" withTexture={false}>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-base font-semibold text-ink-900">
                    {tString("archivedCounters.title")}
                  </h2>
                  <p className="text-sm leading-6 text-ink-600">
                    {tString("archivedCounters.subtitle")}
                  </p>
                </div>
                <button
                  type="button"
                  data-testid="counter-archived-toggle"
                  aria-expanded={showArchived}
                  onClick={() => setShowArchived((open) => !open)}
                  className="inline-flex shrink-0 items-center justify-center gap-1.5 rounded-full border border-warm-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-warm-50"
                >
                  {showArchived ? (
                    <>
                      <ChevronUp className="h-4 w-4" aria-hidden="true" />
                      {tString("archivedCounters.hide")}
                    </>
                  ) : (
                    <>
                      <ChevronDown className="h-4 w-4" aria-hidden="true" />
                      {tString("archivedCounters.show").replace(
                        "{count}",
                        String(archivedCounters.length),
                      )}
                    </>
                  )}
                </button>
              </div>
              {showArchived && (
                <div
                  className="mt-5 grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6"
                  data-testid="counter-archived-grid"
                >
                  {archivedCounters.map((counter) => (
                    <div
                      key={counter.id}
                      className="rounded-2xl border border-warm-200 bg-warm-50/80 p-4 text-center shadow-sm"
                      data-testid={`counter-archived-tile-${counter.counter_number}`}
                    >
                      <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-warm-100 text-ink-500">
                        <Coffee className="h-6 w-6" aria-hidden="true" />
                      </div>
                      <h3 className="font-semibold text-ink-700">
                        {counter.name}
                      </h3>
                      <span className="mt-3 inline-flex rounded-full bg-warm-100 px-2 py-1 text-xs font-semibold text-ink-500">
                        {tString("status.inactive")}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </PremiumPanel>
          )}

          {/* Clearance under sticky SaveBar so the last counters aren't clipped. */}
          <div className="pb-24" aria-hidden="true" />

          <SaveBar
            mode="button"
            isSaving={saving}
            dirty={dirty}
            onSave={() => void handleSaveSettings()}
            labels={{
              save: saveBarString("save"),
              saving: saveBarString("saving"),
              unsaved: saveBarString("unsaved"),
              auto: saveBarString("auto"),
              clean: saveBarString("clean"),
            }}
          />
        </>
      )}
    </DashboardTabShell>
  );
};

export default CounterManager;

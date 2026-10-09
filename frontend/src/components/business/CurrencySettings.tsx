"use client";

import React, {
  forwardRef,
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
  useImperativeHandle,
} from "react";
import {
  Button,
  Divider,
  Input,
  Select,
  SelectItem,
  Spinner,
} from "@nextui-org/react";
import {
  DollarSign,
  Check,
  X,
  Info,
  Languages,
  Search,
  Clock,
} from "lucide-react";
import {
  getSupportedCurrencies,
  getSupportedLanguages,
  getBusinessLanguages,
  updateBusinessLanguages,
  SupportedCurrency,
  SupportedLanguage,
  formatCurrency,
  convertAmount,
} from "../../api/currency";
import { businessApi } from "../../api/business";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { useUnsavedChangesGuard } from "@/hooks/useUnsavedChangesGuard";
import { TIMEZONE_OPTIONS } from "@/utils/timezones";
import {
  applyUnlockedLanguageDraft,
  buildBusinessLanguagesPayload,
  canSaveLanguageDraft,
} from "@/utils/businessLanguages";

const SERVICE_DAY_OPTIONS = [0, 120, 180, 240, 300, 360] as const;

interface VenueTimeSettings {
  timezone: string;
  service_day_start_minute: number;
}

interface CurrencySettingsProps {
  businessId: number;
  onSave?: () => void;
  /**
   * Notifies the parent when the local dirty state of the Currency
   * or Language section changes. The page-header "Save All Changes"
   * button uses this to enable/disable itself on the localization tab.
   */
  onDirtyChange?: (isDirty: boolean) => void;
}

export interface CurrencySettingsHandle {
  /**
   * Persist any dirty Currency and/or Language sections. The page-header
   * "Save All Changes" CTA calls this — IMP-30 collapsed the previous
   * per-section save buttons into a single canonical save.
   */
  save: () => Promise<void>;
}

interface BusinessCurrencySettings {
  default_currency: string;
  display_currency: string;
}

interface SectionStatus {
  type: "success" | "error";
  message: string;
}

const CurrencySettings = forwardRef<
  CurrencySettingsHandle,
  CurrencySettingsProps
>(function CurrencySettings({ businessId, onSave, onDirtyChange }, ref) {
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `currencySettings.${key}`;
      const result = getTranslation(fullKey, currentLocale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  // Currency State
  const [supportedCurrencies, setSupportedCurrencies] = useState<
    SupportedCurrency[]
  >([]);
  // Both start null (not {USD,USD}) so a non-USD business doesn't briefly diff
  // against a USD placeholder during loadData and trip a false dirty/discard
  // state on first paint (F16). Dirty checks below no-op until both are set.
  const [settings, setSettings] = useState<BusinessCurrencySettings | null>(
    null,
  );
  const [originalSettings, setOriginalSettings] =
    useState<BusinessCurrencySettings | null>(null);
  // A business is considered to have the on-chain USDC settlement rail
  // enabled when it has a settlement wallet configured. Without it, USDC
  // conversion previews and "Payment made in USDC" copy are misleading —
  // those operators settle through fiat payment plugins (Stripe/PayPal/etc).
  const [hasCryptoRail, setHasCryptoRail] = useState(false);

  // Language State
  const [supportedLanguages, setSupportedLanguages] = useState<
    SupportedLanguage[]
  >([]);
  const [selectedLanguages, setSelectedLanguages] = useState<string[]>([]);
  const [defaultLanguage, setDefaultLanguage] = useState<string>("en");
  // Mirror currency's null-until-loaded guard (F16 / #224): originals start
  // empty and language dirty no-ops until loadData has stamped a baseline.
  // Starting originalLanguages at ["en"] while selectedLanguages is [] made
  // switching to Localization light "unsaved changes" before any edit.
  const [originalLanguages, setOriginalLanguages] = useState<string[]>([]);
  const [originalDefaultLanguage, setOriginalDefaultLanguage] =
    useState<string>("en");
  const [languagesLoaded, setLanguagesLoaded] = useState(false);
  const [languageSearch, setLanguageSearch] = useState("");

  // Venue timezone + service-day cutoff (Localization tab root cause for
  // schedule 4AM / wrong greetings when operators only looked in Settings).
  const [venueSettings, setVenueSettings] = useState<VenueTimeSettings | null>(
    null,
  );
  const [originalVenueSettings, setOriginalVenueSettings] =
    useState<VenueTimeSettings | null>(null);
  const [savingVenue, setSavingVenue] = useState(false);
  const [venueStatus, setVenueStatus] = useState<SectionStatus | null>(null);

  // UI state
  const [loading, setLoading] = useState(true);
  const [savingCurrency, setSavingCurrency] = useState(false);
  const [savingLanguage, setSavingLanguage] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [currencyStatus, setCurrencyStatus] = useState<SectionStatus | null>(
    null,
  );
  const [languageStatus, setLanguageStatus] = useState<SectionStatus | null>(
    null,
  );

  // Sample conversion for preview
  const [sampleAmount] = useState(100);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [conversionPreview, setConversionPreview] = useState<{
    defaultToUSDC: number;
    displayToUSDC: number;
    defaultToDisplay: number;
  }>({ defaultToUSDC: 100, displayToUSDC: 100, defaultToDisplay: 100 });
  const previewRequestRef = useRef(0);

  const loadData = useCallback(async () => {
    try {
      setLoading(true);
      setLanguagesLoaded(false);
      setLoadError(null);
      setCurrencyStatus(null);
      setLanguageStatus(null);

      // Load supported currencies, languages, and business settings
      const [currencies, languages, business, businessLang] = await Promise.all(
        [
          getSupportedCurrencies(),
          getSupportedLanguages(),
          businessApi.getBusiness(businessId),
          getBusinessLanguages(businessId),
        ],
      );

      setSupportedCurrencies(currencies);
      setSupportedLanguages(languages);

      if (business) {
        const businessSettings = {
          default_currency: business.default_currency || "USD",
          display_currency: business.display_currency || "USD",
        };
        setSettings(businessSettings);
        setOriginalSettings(businessSettings);
        setHasCryptoRail(Boolean(business.settlement_address?.trim()));
        const venue: VenueTimeSettings = {
          timezone: business.timezone || "UTC",
          service_day_start_minute:
            typeof business.service_day_start_minute === "number"
              ? business.service_day_start_minute
              : 0,
        };
        setVenueSettings(venue);
        setOriginalVenueSettings(venue);
      }

      // Set language selections and treat English as the default fallback.
      const langCodes = businessLang.map((l) => l.language_code);
      const fallbackLanguages = langCodes.length > 0 ? langCodes : ["en"];
      const fallbackDefaultLanguage =
        businessLang.find((l) => l.is_default)?.language_code ||
        fallbackLanguages[0] ||
        "en";

      setOriginalLanguages(fallbackLanguages);
      setOriginalDefaultLanguage(fallbackDefaultLanguage);
      setSelectedLanguages(fallbackLanguages);
      setDefaultLanguage(fallbackDefaultLanguage);
      setLanguageSearch("");
      setLanguagesLoaded(true);
    } catch (err: unknown) {
      console.error("Error loading settings:", err);
      setLoadError(tString("errors.loadFailed"));
      setLanguagesLoaded(false);
    } finally {
      setLoading(false);
    }
  }, [businessId, tString]);

  const updateConversionPreview = useCallback(async () => {
    // Nothing to preview until the business settings have loaded.
    if (!settings) {
      return;
    }
    const requestId = previewRequestRef.current + 1;
    previewRequestRef.current = requestId;

    try {
      setPreviewLoading(true);
      const previews = {
        defaultToUSDC: sampleAmount,
        displayToUSDC: sampleAmount,
        defaultToDisplay: sampleAmount,
      };

      // Convert from default currency to USDC
      if (settings.default_currency !== "USDC") {
        try {
          const result = await convertAmount(
            sampleAmount,
            settings.default_currency,
            "USDC",
          );
          previews.defaultToUSDC = result.converted_amount;
        } catch (error) {
          console.warn(
            `Failed to convert ${settings.default_currency} to USDC:`,
            error,
          );
        }
      } else {
        previews.defaultToUSDC = sampleAmount;
      }

      // Convert from display currency to USDC
      if (settings.display_currency !== "USDC") {
        try {
          const result = await convertAmount(
            sampleAmount,
            settings.display_currency,
            "USDC",
          );
          previews.displayToUSDC = result.converted_amount;
        } catch (error) {
          console.warn(
            `Failed to convert ${settings.display_currency} to USDC:`,
            error,
          );
        }
      } else {
        previews.displayToUSDC = sampleAmount;
      }

      // Convert from default to display currency
      if (settings.default_currency !== settings.display_currency) {
        try {
          const result = await convertAmount(
            sampleAmount,
            settings.default_currency,
            settings.display_currency,
          );
          previews.defaultToDisplay = result.converted_amount;
        } catch (error) {
          console.warn(
            `Failed to convert ${settings.default_currency} to ${settings.display_currency}:`,
            error,
          );
        }
      } else {
        previews.defaultToDisplay = sampleAmount;
      }

      if (previewRequestRef.current === requestId) {
        setConversionPreview(previews);
      }
    } catch (error) {
      console.warn("Failed to update conversion preview:", error);
    } finally {
      if (previewRequestRef.current === requestId) {
        setPreviewLoading(false);
      }
    }
  }, [settings, sampleAmount]);

  useEffect(() => {
    loadData().catch((err) => console.error("loadData failed:", err));
  }, [loadData]);

  useEffect(() => {
    void updateConversionPreview();
  }, [updateConversionPreview]);

  useEffect(() => {
    setCurrencyStatus(null);
  }, [settings?.default_currency, settings?.display_currency]);

  useEffect(() => {
    setLanguageStatus(null);
  }, [selectedLanguages, defaultLanguage]);

  useEffect(() => {
    setVenueStatus(null);
  }, [venueSettings?.timezone, venueSettings?.service_day_start_minute]);

  const handleSaveCurrency = useCallback(async () => {
    if (!settings) {
      return;
    }
    try {
      setSavingCurrency(true);
      setCurrencyStatus(null);

      // Save currency settings
      await businessApi.updateBusiness(businessId, {
        default_currency: settings.default_currency,
        display_currency: settings.display_currency,
      });

      setOriginalSettings({ ...settings });
      setCurrencyStatus({
        type: "success",
        message: tString("currency.savedMessage"),
      });
      onSave?.();
    } catch (err: unknown) {
      console.error("Error saving currency settings:", err);
      setCurrencyStatus({
        type: "error",
        message: tString("errors.saveCurrencyFailed"),
      });
    } finally {
      setSavingCurrency(false);
    }
  }, [businessId, onSave, settings, tString]);

  const handleSaveVenue = useCallback(async () => {
    if (!venueSettings) {
      return;
    }
    try {
      setSavingVenue(true);
      setVenueStatus(null);
      await businessApi.updateBusiness(businessId, {
        timezone: venueSettings.timezone,
        service_day_start_minute: venueSettings.service_day_start_minute,
      });
      setOriginalVenueSettings({ ...venueSettings });
      setVenueStatus({
        type: "success",
        message: tString("venue.savedMessage"),
      });
      onSave?.();
    } catch (err: unknown) {
      console.error("Error saving venue time settings:", err);
      setVenueStatus({
        type: "error",
        message: tString("errors.saveVenueFailed"),
      });
    } finally {
      setSavingVenue(false);
    }
  }, [businessId, onSave, tString, venueSettings]);

  const handleSaveLanguage = useCallback(async () => {
    try {
      setSavingLanguage(true);
      setLanguageStatus(null);

      // Validate language settings
      if (selectedLanguages.length === 0) {
        setLanguageStatus({
          type: "error",
          message: tString("errors.selectLanguage"),
        });
        return;
      }

      if (
        !canSaveLanguageDraft({
          selected: selectedLanguages,
          defaultLanguage,
        })
      ) {
        setLanguageStatus({
          type: "error",
          message: tString("errors.selectDefaultLanguage"),
        });
        return;
      }

      // Save language settings
      await updateBusinessLanguages(
        businessId,
        buildBusinessLanguagesPayload({
          selected: selectedLanguages,
          defaultLanguage,
        }),
      );

      setOriginalLanguages([...selectedLanguages]);
      setOriginalDefaultLanguage(defaultLanguage);
      setLanguageStatus({
        type: "success",
        message: tString("language.savedMessage"),
      });
      onSave?.();
    } catch (err: unknown) {
      console.error("Error saving language settings:", err);
      setLanguageStatus({
        type: "error",
        message: tString("errors.saveLanguageFailed"),
      });
    } finally {
      setSavingLanguage(false);
    }
  }, [businessId, defaultLanguage, onSave, selectedLanguages, tString]);

  const handleLanguageToggle = (languageCode: string) => {
    if (
      selectedLanguages.includes(languageCode) &&
      selectedLanguages.length === 1
    ) {
      setLanguageStatus({
        type: "error",
        message: tString("errors.keepAtLeastOneLanguage"),
      });
      return;
    }

    const nextSelected = selectedLanguages.includes(languageCode)
      ? selectedLanguages.filter((l) => l !== languageCode)
      : [...selectedLanguages, languageCode];

    const result = applyUnlockedLanguageDraft(
      { selected: selectedLanguages, defaultLanguage },
      nextSelected,
    );
    setSelectedLanguages(result.selected);
    setDefaultLanguage(result.defaultLanguage);
    if (result.needsDefaultChoice) {
      setLanguageStatus({
        type: "error",
        message: tString("errors.selectDefaultLanguage"),
      });
    }
  };

  const handleResetCurrency = () => {
    if (originalSettings) {
      setSettings({ ...originalSettings });
    }
  };

  const handleResetVenue = () => {
    if (originalVenueSettings) {
      setVenueSettings({ ...originalVenueSettings });
    }
  };

  const handleResetLanguage = () => {
    setSelectedLanguages([...originalLanguages]);
    setDefaultLanguage(originalDefaultLanguage);
  };

  const handleSelectAllLanguages = () => {
    const allLanguageCodes = supportedLanguages.map(
      (language) => language.code,
    );
    setSelectedLanguages(allLanguageCodes);
    if (!allLanguageCodes.includes(defaultLanguage) && allLanguageCodes[0]) {
      setDefaultLanguage(allLanguageCodes[0]);
    }
  };

  const hasCurrencyChanges = useMemo(() => {
    // Not dirty until data has loaded — both null means "not loaded yet", so a
    // non-USD business no longer diffs against a USD placeholder on load (F16).
    if (!settings || !originalSettings) {
      return false;
    }
    return (
      settings.default_currency !== originalSettings.default_currency ||
      settings.display_currency !== originalSettings.display_currency
    );
  }, [settings, originalSettings]);

  const hasLanguageChanges = useMemo(() => {
    // Not dirty until languages have loaded — prevents Localization sub-tab
    // switches from raising "unsaved changes" against the empty initial state.
    if (!languagesLoaded) {
      return false;
    }
    const originalLanguageCodes = [...originalLanguages].sort();
    const currentLanguageCodes = [...selectedLanguages].sort();

    return (
      JSON.stringify(originalLanguageCodes) !==
        JSON.stringify(currentLanguageCodes) ||
      originalDefaultLanguage !== defaultLanguage
    );
  }, [
    languagesLoaded,
    defaultLanguage,
    selectedLanguages,
    originalLanguages,
    originalDefaultLanguage,
  ]);

  const hasVenueChanges = useMemo(() => {
    if (!venueSettings || !originalVenueSettings) {
      return false;
    }
    return (
      venueSettings.timezone !== originalVenueSettings.timezone ||
      venueSettings.service_day_start_minute !==
        originalVenueSettings.service_day_start_minute
    );
  }, [venueSettings, originalVenueSettings]);

  // Push dirty state up so the page-header "Save All Changes" CTA in
  // BusinessSettings can enable/disable itself on the localization tab.
  // Stay clean for the whole "Loading settings…" window (#224).
  useEffect(() => {
    if (loading) {
      onDirtyChange?.(false);
      return;
    }
    onDirtyChange?.(
      hasCurrencyChanges || hasLanguageChanges || hasVenueChanges,
    );
  }, [
    loading,
    hasCurrencyChanges,
    hasLanguageChanges,
    hasVenueChanges,
    onDirtyChange,
  ]);

  // Clear parent dirty on unmount so a mid-load Localization visit can't leave
  // localizationDirty stuck true after the operator leaves the sub-tab (#224).
  useEffect(() => {
    return () => {
      onDirtyChange?.(false);
    };
  }, [onDirtyChange]);

  // H2a: prompt before leaving (tab close / reload / in-app tab switch) while
  // the currency or language section has unsaved edits.
  useUnsavedChangesGuard(
    hasCurrencyChanges || hasLanguageChanges || hasVenueChanges,
    "currency-settings",
  );

  // IMP-30: expose a single imperative save() so the page-header
  // "Save All Changes" CTA is the canonical save for this tab. It
  // fans out to the existing per-section handlers — only sections
  // with dirty state hit the network.
  useImperativeHandle(
    ref,
    () => ({
      save: async () => {
        const tasks: Promise<void>[] = [];
        if (hasCurrencyChanges) tasks.push(handleSaveCurrency());
        if (hasLanguageChanges) tasks.push(handleSaveLanguage());
        if (hasVenueChanges) tasks.push(handleSaveVenue());
        if (tasks.length === 0) return;
        await Promise.all(tasks);
      },
    }),
    [
      handleSaveCurrency,
      handleSaveLanguage,
      handleSaveVenue,
      hasCurrencyChanges,
      hasLanguageChanges,
      hasVenueChanges,
    ],
  );

  const selectedLanguageOptions = useMemo(() => {
    const selectedSet = new Set(selectedLanguages);
    return supportedLanguages.filter((language) =>
      selectedSet.has(language.code),
    );
  }, [selectedLanguages, supportedLanguages]);

  const visibleLanguages = useMemo(() => {
    const normalizedSearch = languageSearch.trim().toLowerCase();
    const selectedSet = new Set(selectedLanguages);

    const filteredLanguages = supportedLanguages.filter((language) => {
      if (!normalizedSearch) {
        return true;
      }

      return (
        language.name.toLowerCase().includes(normalizedSearch) ||
        language.native_name.toLowerCase().includes(normalizedSearch) ||
        language.code.toLowerCase().includes(normalizedSearch)
      );
    });

    return [...filteredLanguages].sort((a, b) => {
      const aSelected = selectedSet.has(a.code);
      const bSelected = selectedSet.has(b.code);

      if (aSelected !== bSelected) {
        return aSelected ? -1 : 1;
      }

      return a.name.localeCompare(b.name);
    });
  }, [languageSearch, selectedLanguages, supportedLanguages]);

  const getCurrencyInfo = (code: string) => {
    return supportedCurrencies.find((c) => c.code === code);
  };

  const sectionIconClass =
    "p-2 rounded-xl bg-brand/10 border border-brand/20 shadow-sm shadow-brand/10";
  const sectionTitleClass = "text-lg font-semibold text-ink-950";
  const sectionDescriptionClass = "text-sm text-ink-600";
  const fieldCopyClass = "text-xs text-warm-600 mt-1";
  const controlClassNames = {
    trigger: "border-warm-200 bg-white/90 shadow-sm",
    value: "text-ink-900",
  };

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center rounded-3xl border border-warm-200/80 bg-warm-50/70 py-12 text-center shadow-sm shadow-warm-900/5">
        <Spinner size="lg" />
        <p className="mt-4 text-sm font-medium text-ink-700">
          {tString("loading")}
        </p>
      </div>
    );
  }

  // Load resolved but settings never arrived (e.g. getBusiness failed): surface
  // the load error instead of rendering the form against null settings.
  if (!settings) {
    return (
      <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 flex items-center gap-2 text-rose-700 text-sm">
        <X className="w-4 h-4" />
        <span>{loadError ?? tString("errors.loadFailed")}</span>
      </div>
    );
  }

  return (
    <div className="space-y-8 min-h-[50vh]">
      {loadError && (
        <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 flex items-center gap-2 text-rose-700 text-sm">
          <X className="w-4 h-4" />
          <span>{loadError}</span>
        </div>
      )}

      {/* VENUE TIME — timezone + service-day cutoff (#204) */}
      {venueSettings ? (
        <div className="space-y-5 rounded-3xl border border-warm-200/80 bg-gradient-to-br from-white via-warm-50/75 to-brand/5 p-5 shadow-sm shadow-warm-900/5">
          <div className="flex items-center gap-3">
            <div className={sectionIconClass}>
              <Clock className="w-5 h-5 text-brand" />
            </div>
            <div>
              <h3 className={sectionTitleClass}>{tString("venue.title")}</h3>
              <p className={sectionDescriptionClass}>
                {tString("venue.description")}
              </p>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <div className="space-y-2">
              <Select
                label={tString("venue.timezoneLabel")}
                placeholder={tString("venue.timezonePlaceholder")}
                selectedKeys={
                  venueSettings.timezone ? [venueSettings.timezone] : []
                }
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys)[0] as string;
                  if (!selected) return;
                  setVenueSettings((prev) =>
                    prev ? { ...prev, timezone: selected } : prev,
                  );
                }}
                classNames={controlClassNames}
                data-testid="venue-timezone-select"
              >
                {TIMEZONE_OPTIONS.map((tz) => (
                  <SelectItem key={tz.value} textValue={tz.label}>
                    {tz.label}
                  </SelectItem>
                ))}
              </Select>
              <p className={fieldCopyClass}>{tString("venue.timezoneHelp")}</p>
            </div>
            <div className="space-y-2">
              <Select
                label={tString("venue.serviceDayLabel")}
                selectedKeys={[String(venueSettings.service_day_start_minute)]}
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys)[0] as string;
                  if (selected == null) return;
                  const minute = Number(selected);
                  if (!Number.isFinite(minute)) return;
                  setVenueSettings((prev) =>
                    prev ? { ...prev, service_day_start_minute: minute } : prev,
                  );
                }}
                classNames={controlClassNames}
                data-testid="venue-service-day-select"
              >
                {SERVICE_DAY_OPTIONS.map((minute) => (
                  <SelectItem key={String(minute)} textValue={String(minute)}>
                    {tString(`venue.serviceDayOptions.${minute}`)}
                  </SelectItem>
                ))}
              </Select>
              <p className={fieldCopyClass}>
                {tString("venue.serviceDayHelp")}
              </p>
            </div>
          </div>
          {venueStatus ? (
            <div
              className={`flex items-center gap-2 rounded-xl px-3 py-2 text-sm ${
                venueStatus.type === "success"
                  ? "border border-emerald-200 bg-emerald-50 text-emerald-700"
                  : "border border-rose-200 bg-rose-50 text-rose-700"
              }`}
            >
              {venueStatus.type === "success" ? (
                <Check className="h-4 w-4" />
              ) : (
                <X className="h-4 w-4" />
              )}
              <span>{venueStatus.message}</span>
            </div>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button
              size="sm"
              variant="light"
              onPress={handleResetVenue}
              isDisabled={!hasVenueChanges || savingVenue}
              className="rounded-xl font-semibold text-ink-700"
            >
              {tString("actions.discard")}
            </Button>
            {hasVenueChanges ? (
              <Button
                color="primary"
                isLoading={savingVenue}
                onPress={() => void handleSaveVenue()}
                className="bg-brand text-white"
              >
                {tString("venue.saveButton")}
              </Button>
            ) : null}
          </div>
        </div>
      ) : null}

      {/* CURRENCY */}
      <div className="space-y-5 rounded-3xl border border-warm-200/80 bg-gradient-to-br from-white via-warm-50/75 to-brand/5 p-5 shadow-sm shadow-warm-900/5">
        <div className="flex items-center gap-3">
          <div className={sectionIconClass}>
            <DollarSign className="w-5 h-5 text-brand" />
          </div>
          <div>
            <h3 className={sectionTitleClass}>{tString("currency.title")}</h3>
            <p className={sectionDescriptionClass}>
              {tString("currency.description")}
            </p>
          </div>
        </div>
        <div className="space-y-5">
          {/* Default / Display currency as a flat 2-col form row — the
              previous mini-card chrome ("rounded-xl border bg-warm-50/70")
              produced three nested card depths on the Localization tab. */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <div className="space-y-2">
              <div>
                <h4 className="text-sm font-semibold text-ink-950">
                  {tString("currency.defaultTitle")}
                </h4>
                <p className={fieldCopyClass}>
                  {tString("currency.defaultDescription")}
                </p>
              </div>
              <Select
                label={tString("currency.defaultLabel")}
                placeholder={tString("currency.defaultPlaceholder")}
                selectedKeys={
                  settings.default_currency
                    ? [settings.default_currency]
                    : undefined
                }
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys)[0] as string;
                  if (selected) {
                    setSettings((prev) =>
                      prev ? { ...prev, default_currency: selected } : prev,
                    );
                  }
                }}
                className="w-full"
                size="lg"
                variant="bordered"
                disallowEmptySelection
                isDisabled={savingCurrency}
                classNames={controlClassNames}
              >
                {supportedCurrencies.map((currency) => (
                  <SelectItem
                    key={currency.code}
                    value={currency.code}
                    textValue={`${currency.code} - ${currency.name}`}
                  >
                    {currency.symbol} {currency.code} - {currency.name}
                  </SelectItem>
                ))}
              </Select>
            </div>

            <div className="space-y-2">
              <div>
                <h4 className="text-sm font-semibold text-ink-950">
                  {tString("currency.displayTitle")}
                </h4>
                <p className={fieldCopyClass}>
                  {tString("currency.displayDescription")}
                </p>
              </div>
              <Select
                label={tString("currency.displayLabel")}
                placeholder={tString("currency.displayPlaceholder")}
                selectedKeys={
                  settings.display_currency
                    ? [settings.display_currency]
                    : undefined
                }
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys)[0] as string;
                  if (selected) {
                    setSettings((prev) =>
                      prev ? { ...prev, display_currency: selected } : prev,
                    );
                  }
                }}
                className="w-full"
                size="lg"
                variant="bordered"
                disallowEmptySelection
                isDisabled={savingCurrency}
                classNames={controlClassNames}
              >
                {supportedCurrencies.map((currency) => (
                  <SelectItem
                    key={currency.code}
                    value={currency.code}
                    textValue={`${currency.code} - ${currency.name}`}
                  >
                    {currency.symbol} {currency.code} - {currency.name}
                  </SelectItem>
                ))}
              </Select>
            </div>
          </div>

          {/* Conversion Preview — each card renders only when its conversion
              is both non-identity AND meaningful for this business. USDC
              cards only appear when the on-chain settlement rail is wired
              (settlement_address set); otherwise USDC pricing is irrelevant
              to operators paid via Stripe/PayPal/MercadoPago/etc. */}
          {(() => {
            const showDefaultToUSDC =
              hasCryptoRail && settings.default_currency !== "USDC";
            const showDefaultToDisplay =
              settings.default_currency !== settings.display_currency;
            // Display → USDC duplicates Default → USDC whenever default ===
            // display, so suppress it in that case to avoid the visible dupe.
            const showDisplayToUSDC =
              hasCryptoRail &&
              settings.display_currency !== "USDC" &&
              settings.display_currency !== settings.default_currency;

            if (
              !showDefaultToUSDC &&
              !showDefaultToDisplay &&
              !showDisplayToUSDC
            ) {
              return null;
            }

            return (
              <div className="space-y-3 rounded-2xl border border-warm-200/80 bg-white/80 p-4 shadow-sm shadow-warm-900/5">
                <div className="flex items-center justify-between gap-2">
                  <h4 className="text-sm font-semibold text-ink-950">
                    {tString("currency.conversionPreview")}
                  </h4>
                  {previewLoading && (
                    <div className="flex items-center gap-2 text-xs text-warm-700">
                      <Spinner size="sm" />
                      <span>{tString("currency.updatingPreview")}</span>
                    </div>
                  )}
                </div>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  {showDefaultToUSDC && (
                    <div className="bg-warm-50/80 rounded-2xl border border-warm-200/80 p-3">
                      <div className="text-xs font-medium text-warm-700 mb-1">
                        {tString("currency.pricingToUsdc")}
                      </div>
                      <div className="font-semibold text-sm text-ink-950">
                        {formatCurrency(
                          sampleAmount,
                          settings.default_currency,
                        )}{" "}
                        →{" "}
                        {formatCurrency(
                          conversionPreview.defaultToUSDC,
                          "USDC",
                        )}
                      </div>
                    </div>
                  )}

                  {showDefaultToDisplay && (
                    <div className="bg-warm-50/80 rounded-2xl border border-warm-200/80 p-3">
                      <div className="text-xs font-medium text-warm-700 mb-1">
                        {tString("currency.pricingToDisplay")}
                      </div>
                      <div className="font-semibold text-sm text-ink-950">
                        {formatCurrency(
                          sampleAmount,
                          settings.default_currency,
                        )}{" "}
                        →{" "}
                        {formatCurrency(
                          conversionPreview.defaultToDisplay,
                          settings.display_currency,
                        )}
                      </div>
                    </div>
                  )}

                  {showDisplayToUSDC && (
                    <div className="bg-warm-50/80 rounded-2xl border border-warm-200/80 p-3">
                      <div className="text-xs font-medium text-warm-700 mb-1">
                        {tString("currency.displayToUsdc")}
                      </div>
                      <div className="font-semibold text-sm text-ink-950">
                        {formatCurrency(
                          sampleAmount,
                          settings.display_currency,
                        )}{" "}
                        →{" "}
                        {formatCurrency(
                          conversionPreview.displayToUSDC,
                          "USDC",
                        )}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            );
          })()}

          {/* Info Box — body is composed from two pieces so we never claim
              USDC settlement for businesses that don't have the on-chain
              rail wired up (no settlement_address). */}
          {(() => {
            const sameCurrency =
              settings.default_currency === settings.display_currency;
            const defaultName =
              getCurrencyInfo(settings.default_currency)?.name ||
              settings.default_currency;
            const displayName =
              getCurrencyInfo(settings.display_currency)?.name ||
              settings.display_currency;

            const baseLine = sameCurrency
              ? tString("currency.paymentFlowSame").replace(
                  "{currency}",
                  displayName,
                )
              : tString("currency.paymentFlowConverted")
                  .replace("{display}", displayName)
                  .replace("{default}", defaultName);

            // Crypto suffix only when USDC is meaningfully a settlement
            // step — i.e., the rail is on AND USDC isn't already what the
            // customer sees.
            const showCryptoSuffix =
              hasCryptoRail && settings.display_currency !== "USDC";

            return (
              <div className="bg-brand/10 border border-brand/20 rounded-2xl p-4 shadow-sm shadow-brand/10">
                <div className="flex items-start gap-2">
                  <Info className="w-4 h-4 text-brand-dark mt-0.5" />
                  <div className="text-sm text-brand-dark">
                    <p className="font-medium">
                      {tString("currency.paymentFlow")}
                    </p>
                    <p className="text-brand-dark">
                      {baseLine}
                      {showCryptoSuffix && (
                        <> {tString("currency.paymentFlowCryptoSuffix")}</>
                      )}
                    </p>
                  </div>
                </div>
              </div>
            );
          })()}

          {/* Save Controls */}
          <div className="space-y-3 border-t border-warm-200 pt-4">
            {currencyStatus && (
              <div
                className={`flex items-center gap-2 text-sm ${currencyStatus.type === "success" ? "text-emerald-700" : "text-rose-700"}`}
                role="status"
                aria-live="polite"
              >
                {currencyStatus.type === "success" ? (
                  <Check className="w-4 h-4" />
                ) : (
                  <X className="w-4 h-4" />
                )}
                <span>{currencyStatus.message}</span>
              </div>
            )}

            {/* IMP-30: section-level "Save Currency Settings" CTA was
                collapsed into the page-header "Save All Changes". A
                "Discard changes" link remains so operators can revert
                ONLY this section's dirty state. */}
            <div className="flex justify-end">
              <Button
                size="sm"
                variant="light"
                onPress={handleResetCurrency}
                isDisabled={!hasCurrencyChanges || savingCurrency}
                className="rounded-xl font-semibold text-ink-700"
              >
                {tString("actions.discard")}
              </Button>
            </div>
          </div>
        </div>
      </div>

      <Divider className="my-2 bg-warm-200/70" />

      {/* LANGUAGE */}
      <div className="space-y-5 rounded-3xl border border-warm-200/80 bg-white/80 p-5 shadow-sm shadow-warm-900/5">
        <div className="flex items-center gap-3">
          <div className={sectionIconClass}>
            <Languages className="w-5 h-5 text-brand" />
          </div>
          <div>
            <h3 className={sectionTitleClass}>{tString("language.title")}</h3>
            <p className={sectionDescriptionClass}>
              {tString("language.description")}
            </p>
          </div>
        </div>
        <div className="space-y-5">
          {/* Language Selection */}
          <div className="space-y-4">
            <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-3">
              <div>
                <h4 className="text-sm font-semibold text-ink-950">
                  {tString("language.supportedTitle")}
                </h4>
                <p className={fieldCopyClass}>
                  {tString("language.supportedDescription")}
                </p>
              </div>
              <p className="text-xs font-semibold text-brand-dark bg-brand/10 rounded-full px-3 py-1 w-fit">
                {tString("language.selectedCount").replace(
                  "{count}",
                  selectedLanguages.length.toString(),
                )}
              </p>
            </div>

            <div className="flex flex-col md:flex-row gap-2">
              <Input
                size="sm"
                isClearable
                value={languageSearch}
                onValueChange={setLanguageSearch}
                startContent={<Search className="w-4 h-4 text-warm-500" />}
                placeholder={tString("language.searchPlaceholder")}
                className="w-full md:max-w-sm"
                variant="bordered"
                classNames={{
                  inputWrapper: "border-warm-200 bg-white/90 shadow-sm",
                  input: "text-ink-900 placeholder:text-warm-500",
                }}
              />
              <div className="flex items-center gap-2 md:ml-auto">
                <Button
                  size="sm"
                  variant="flat"
                  onPress={handleSelectAllLanguages}
                  isDisabled={
                    savingLanguage ||
                    supportedLanguages.length === 0 ||
                    selectedLanguages.length === supportedLanguages.length
                  }
                  className="rounded-xl bg-brand/10 font-semibold text-brand-dark"
                >
                  {tString("language.selectAllButton")}
                </Button>
                <Button
                  size="sm"
                  variant="flat"
                  onPress={handleResetLanguage}
                  isDisabled={!hasLanguageChanges || savingLanguage}
                  className="rounded-xl bg-warm-100 font-semibold text-ink-700"
                >
                  {tString("actions.discard")}
                </Button>
              </div>
            </div>

            {visibleLanguages.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-warm-300 bg-warm-50 p-6 text-center text-sm text-warm-700">
                {tString("language.noResults")}
              </div>
            ) : (
              <div className="max-h-[360px] overflow-y-auto pr-1">
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                  {visibleLanguages.map((language) => {
                    const isSelected = selectedLanguages.includes(
                      language.code,
                    );
                    const isDefault =
                      isSelected && defaultLanguage === language.code;

                    return (
                      <button
                        type="button"
                        key={language.code}
                        onClick={() => handleLanguageToggle(language.code)}
                        aria-pressed={isSelected}
                        disabled={savingLanguage}
                        className={`p-3 rounded-2xl border text-left shadow-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
                          isSelected
                            ? "border-brand/60 bg-brand/10 shadow-brand/10"
                            : "border-warm-200 bg-white hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-md hover:shadow-brand/10"
                        }`}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <p className="font-semibold text-sm text-ink-950">
                              {language.native_name}
                            </p>
                            <p className="text-xs text-warm-700">
                              {language.name}
                            </p>
                            <div className="mt-1.5 flex items-center gap-2 flex-wrap">
                              <span className="px-1.5 py-0.5 rounded-lg bg-warm-100 text-[10px] uppercase tracking-wide text-warm-700">
                                {language.code}
                              </span>
                              {isDefault && (
                                <span className="px-1.5 py-0.5 rounded-lg bg-brand/10 text-[10px] font-semibold text-brand-dark">
                                  {tString("language.defaultBadge")}
                                </span>
                              )}
                            </div>
                          </div>
                          {isSelected && (
                            <Check className="w-4 h-4 text-brand mt-0.5" />
                          )}
                        </div>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>

          {/* Default Language Selection */}
          {selectedLanguages.length > 1 && (
            <div className="rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4 space-y-3">
              <div>
                <h4 className="text-sm font-semibold text-ink-950">
                  {tString("language.defaultTitle")}
                </h4>
                <p className={fieldCopyClass}>
                  {tString("language.defaultDescription")}
                </p>
              </div>
              <Select
                label={tString("language.defaultLabel")}
                placeholder={tString("language.defaultPlaceholder")}
                selectedKeys={defaultLanguage ? [defaultLanguage] : undefined}
                onSelectionChange={(keys) => {
                  const selected = Array.from(keys)[0] as string;
                  if (selected) {
                    setDefaultLanguage(selected);
                  }
                }}
                className="w-full md:max-w-sm"
                variant="bordered"
                disallowEmptySelection
                isDisabled={savingLanguage}
                classNames={controlClassNames}
              >
                {selectedLanguageOptions.map((language) => (
                  <SelectItem
                    key={language.code}
                    value={language.code}
                    textValue={`${language.native_name} (${language.name})`}
                  >
                    {language.native_name} ({language.name})
                  </SelectItem>
                ))}
              </Select>
            </div>
          )}

          {/* Info Box */}
          <div className="bg-brand/10 border border-brand/20 rounded-2xl p-4 shadow-sm shadow-brand/10">
            <div className="flex items-start gap-2">
              <Info className="w-4 h-4 text-brand-dark mt-0.5" />
              <div className="text-sm text-brand-dark">
                <p className="font-medium">{tString("language.supportInfo")}</p>
                <p className="text-brand-dark">
                  {tString("language.supportInfoDescription")}
                </p>
              </div>
            </div>
          </div>

          {/* Save Controls */}
          <div className="space-y-3 border-t border-warm-200 pt-4">
            {languageStatus && (
              <div
                className={`flex items-center gap-2 text-sm ${languageStatus.type === "success" ? "text-emerald-700" : "text-rose-700"}`}
                role="status"
                aria-live="polite"
              >
                {languageStatus.type === "success" ? (
                  <Check className="w-4 h-4" />
                ) : (
                  <X className="w-4 h-4" />
                )}
                <span>{languageStatus.message}</span>
              </div>
            )}

            {/* IMP-30: section-level "Save Language Settings" CTA was
                collapsed into the page-header "Save All Changes". A
                "Discard changes" link remains so operators can revert
                ONLY this section's dirty state. */}
            <div className="flex justify-end">
              <Button
                size="sm"
                variant="light"
                onPress={handleResetLanguage}
                isDisabled={!hasLanguageChanges || savingLanguage}
                className="rounded-xl font-semibold text-ink-700"
              >
                {tString("actions.discard")}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
});

export default CurrencySettings;

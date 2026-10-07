"use client";

import React from "react";
import {
  Button,
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  Select,
  SelectItem,
} from "@nextui-org/react";
import { Check, RotateCcw } from "lucide-react";
import {
  MARKETING_PLAYS,
  type MarketingCreativeProfile,
  type MarketingPlay,
  type MarketingSettings,
} from "@/api/marketing";
import { normalizeMarketingSettings } from "./hooks/useMarketingSettings";
import { kitBiasFromVisualMood } from "./brandLock";

const VISUAL_MOODS = [
  "natural",
  "bright",
  "moody",
  "editorial",
  "rustic",
] as const;
const CTA_STYLES = ["soft", "direct", "urgent"] as const;
const HASHTAG_BEHAVIORS = ["none", "light", "standard"] as const;
const TONES = ["warm", "playful", "elegant", "punchy"] as const;
const MAX_AVOID_PHRASES = 10;
const MAX_AVOID_PHRASE_RUNES = 60;
const LANGUAGES = [
  "en",
  "es",
  "es-AR",
  "pt",
  "fr",
  "de",
  "it",
  "ja",
  "zh",
  "ar",
  "da",
  "hi",
  "ko",
  "nl",
  "no",
  "pl",
  "ru",
  "sv",
  "th",
  "tr",
  "vi",
] as const;

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

type AvoidPhrasesError = "tooMany" | "tooLong" | null;

function parseAvoidPhrases(value: string): {
  phrases: string[];
  error: AvoidPhrasesError;
} {
  const phrases: string[] = [];
  const seen = new Set<string>();
  for (const rawPhrase of value.split(/[,\n]/)) {
    const phrase = rawPhrase.trim();
    if (!phrase) continue;
    const folded = phrase.toLowerCase();
    if (seen.has(folded)) continue;
    seen.add(folded);
    phrases.push(phrase);
  }
  if (phrases.length > MAX_AVOID_PHRASES) {
    return { phrases, error: "tooMany" };
  }
  if (
    phrases.some((phrase) => Array.from(phrase).length > MAX_AVOID_PHRASE_RUNES)
  ) {
    return { phrases, error: "tooLong" };
  }
  return { phrases, error: null };
}

/** Live brand inputs shown in the Brand lock summary (from business design). */
interface MarketingBrandPreview {
  logoUrl?: string;
  primaryColor?: string;
  secondaryColor?: string;
  handle?: string;
}

interface MarketingSettingsDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  settings: MarketingSettings;
  hasSnapshot: boolean;
  loading: boolean;
  canEdit: boolean;
  saving: boolean;
  loadError: string | null;
  saveError: string | null;
  rollbackOccurred: boolean;
  saved: boolean;
  retryingLoad?: boolean;
  onSave: (settings: MarketingSettings) => void;
  onRetryLoad: () => void;
  onRetrySave: () => void;
  t: Translate;
  /** Optional live brand assets for the Brand lock summary. */
  brandPreview?: MarketingBrandPreview;
}

const controlClass =
  "w-full rounded-xl border border-warm-200 bg-white px-3.5 py-2.5 text-sm text-ink-900 outline-none transition placeholder:text-ink-400 hover:border-warm-300 focus:border-brand focus:ring-2 focus:ring-brand/20 disabled:cursor-not-allowed disabled:bg-warm-50 disabled:text-ink-600";

function useFocusReturn(isOpen: boolean) {
  const returnFocusRef = React.useRef<HTMLElement | null>(null);
  const wasOpenRef = React.useRef(false);

  React.useLayoutEffect(() => {
    if (isOpen && !wasOpenRef.current) {
      const active = document.activeElement;
      returnFocusRef.current =
        active instanceof HTMLElement && active !== document.body
          ? active
          : null;
    } else if (!isOpen && wasOpenRef.current) {
      const returnTarget = returnFocusRef.current;
      queueMicrotask(() => returnTarget?.focus());
    }
    wasOpenRef.current = isOpen;
  }, [isOpen]);
}

function isNestedEscapeTarget(target: EventTarget | null) {
  return (
    target instanceof Element &&
    Boolean(
      target.closest(
        'select,[role="combobox"],[role="listbox"],[role="option"],[aria-haspopup="listbox"]',
      ),
    )
  );
}

function Toggle({
  checked,
  disabled,
  label,
  onChange,
}: {
  checked: boolean;
  disabled: boolean;
  label: string;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={onChange}
      className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-50 ${
        checked ? "border-brand bg-brand" : "border-warm-300 bg-warm-200"
      }`}
    >
      <span
        aria-hidden="true"
        className={`block h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${
          checked ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}

function SectionHeading({ title, body }: { title: string; body: string }) {
  return (
    <div className="space-y-1">
      <h3 className="text-base font-semibold tracking-tight text-ink-950">
        {title}
      </h3>
      <p className="max-w-lg text-sm leading-5 text-ink-600">{body}</p>
    </div>
  );
}

function Field({
  id,
  label,
  helper,
  children,
}: {
  id: string;
  label: string;
  helper: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <label htmlFor={id} className="block text-sm font-semibold text-ink-800">
        {label}
      </label>
      {children}
      <p id={`${id}-helper`} className="text-xs leading-5 text-ink-500">
        {helper}
      </p>
    </div>
  );
}

function SettingsSkeleton() {
  return (
    <div
      data-testid="marketing-settings-skeleton"
      className="space-y-8"
      aria-hidden="true"
    >
      {[2, 2, 3].map((fields, section) => (
        <div key={section} className="space-y-4">
          <div className="h-5 w-32 animate-pulse rounded-lg bg-warm-200" />
          <div className="h-3 w-64 max-w-full animate-pulse rounded-lg bg-warm-100" />
          {Array.from({ length: fields }, (_, index) => (
            <div key={index} className="space-y-2">
              <div className="h-3 w-24 animate-pulse rounded-lg bg-warm-200" />
              <div className="h-11 animate-pulse rounded-xl bg-warm-100" />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

export function MarketingSettingsDrawer({
  isOpen,
  onClose,
  settings,
  hasSnapshot,
  loading,
  canEdit,
  saving,
  loadError,
  saveError,
  rollbackOccurred,
  saved,
  retryingLoad = false,
  onSave,
  onRetryLoad,
  onRetrySave,
  t,
  brandPreview,
}: MarketingSettingsDrawerProps) {
  useFocusReturn(isOpen);
  // The settings query is mounted by the dashboard, not by this drawer, so a
  // failed load latched for the lifetime of the page: closing and reopening
  // never re-drove the fetch and the operator was stuck on the error card with
  // no way back to creative settings (#693). Opening the drawer re-drives a
  // failed load once per open; the explicit retry button covers the rest.
  const wasOpenRef = React.useRef<boolean | null>(null);
  React.useEffect(() => {
    const previouslyOpen = wasOpenRef.current;
    wasOpenRef.current = isOpen;
    // Only a closed -> open transition counts; the first commit does not.
    if (previouslyOpen !== false || !isOpen) return;
    if (hasSnapshot || !loadError) return;
    onRetryLoad();
  }, [isOpen, hasSnapshot, loadError, onRetryLoad]);
  const titleId = React.useId();
  const normalized = React.useMemo(
    () => normalizeMarketingSettings(settings),
    [settings],
  );
  const [draft, setDraft] = React.useState(normalized);
  const [avoidPhrasesDraft, setAvoidPhrasesDraft] = React.useState(
    () => normalized?.creative_profile?.avoid_phrases.join(", ") ?? "",
  );
  const [avoidPhrasesError, setAvoidPhrasesError] =
    React.useState<AvoidPhrasesError>(null);
  const [avoidPhrasesDirty, setAvoidPhrasesDirty] = React.useState(false);

  React.useEffect(() => {
    setDraft(normalized);
  }, [normalized]);

  React.useEffect(() => {
    const storedPhrases = normalized?.creative_profile?.avoid_phrases ?? [];
    if (avoidPhrasesDirty) {
      const parsed = parseAvoidPhrases(avoidPhrasesDraft);
      if (
        !parsed.error &&
        JSON.stringify(parsed.phrases) === JSON.stringify(storedPhrases)
      ) {
        setAvoidPhrasesDirty(false);
      }
      return;
    }
    setAvoidPhrasesDraft(storedPhrases.join(", "));
    setAvoidPhrasesError(null);
  }, [avoidPhrasesDirty, avoidPhrasesDraft, normalized]);

  const commit = React.useCallback(
    (next: MarketingSettings) => {
      const normalizedNext = normalizeMarketingSettings(next);
      setDraft(normalizedNext);
      if (canEdit) onSave(normalizedNext);
    },
    [canEdit, onSave],
  );

  const setProfileDraft = <Key extends keyof MarketingCreativeProfile>(
    key: Key,
    value: MarketingCreativeProfile[Key],
  ) => {
    setDraft((current) => ({
      ...current,
      creative_profile: { ...current.creative_profile!, [key]: value },
    }));
  };

  const commitProfile = <Key extends keyof MarketingCreativeProfile>(
    key: Key,
    value: MarketingCreativeProfile[Key],
  ) => {
    commit({
      ...draft,
      creative_profile: { ...draft.creative_profile!, [key]: value },
    });
  };

  const togglePlay = (play: MarketingPlay) => {
    const disabled = new Set(draft.disabled_plays);
    if (disabled.has(play)) disabled.delete(play);
    else disabled.add(play);
    commit({
      ...draft,
      disabled_plays: MARKETING_PLAYS.filter((key) => disabled.has(key)),
    });
  };

  const textField = (
    key: "audience" | "voice",
    options?: { multiline?: boolean },
  ) => {
    const id = `marketing-settings-${key}`;
    const shared = {
      id,
      value: draft.creative_profile?.[key] ?? "",
      disabled: !canEdit || saving,
      "aria-describedby": `${id}-helper`,
      maxLength: 200,
      onChange: (
        event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
      ) => setProfileDraft(key, event.target.value),
      onBlur: (
        event: React.FocusEvent<HTMLInputElement | HTMLTextAreaElement>,
      ) => commitProfile(key, event.target.value),
      className: `${controlClass} ${options?.multiline ? "min-h-24 resize-y" : ""}`,
      placeholder: t(`settings.fields.${key}.placeholder`),
    };
    return (
      <Field
        id={id}
        label={t(`settings.fields.${key}.label`)}
        helper={t(`settings.fields.${key}.helper`)}
      >
        {options?.multiline ? (
          <textarea {...shared} />
        ) : (
          <input type="text" {...shared} />
        )}
      </Field>
    );
  };

  const selectField = <Key extends keyof MarketingCreativeProfile>(
    key: Key,
    translationKey: string,
    choices: readonly string[],
    choiceNamespace: string,
  ) => {
    const id = `marketing-settings-${String(key)}`;
    const label = t(`settings.fields.${translationKey}.label`);
    const current = String(draft.creative_profile?.[key] ?? "");
    // One collection: an "automatic" sentinel (empty value) followed by each
    // choice. Built as a single array mapped through the `items` prop, not a
    // static <SelectItem> beside a mapped array — NextUI's CollectionElement
    // typing rejects the mixed form (TS2322).
    const options = [
      { value: "", label: t("settings.automatic") },
      ...choices.map((choice) => ({
        value: choice,
        label: t(`${choiceNamespace}.${choice}`),
      })),
    ];
    return (
      <Field id={id} label={label} helper={t(`settings.fields.${translationKey}.helper`)}>
        <Select
          id={id}
          aria-label={label}
          aria-describedby={`${id}-helper`}
          selectedKeys={[current]}
          isDisabled={!canEdit || saving}
          radius="lg"
          variant="bordered"
          items={options}
          classNames={{
            trigger:
              "border border-warm-200 bg-white shadow-sm hover:border-warm-300 data-[open=true]:border-brand data-[focus=true]:border-brand data-[focus=true]:ring-2 data-[focus=true]:ring-brand/20 data-[disabled=true]:cursor-not-allowed data-[disabled=true]:bg-warm-50",
          }}
          onChange={(event) =>
            commitProfile(
              key,
              event.target.value as MarketingCreativeProfile[Key],
            )
          }
        >
          {(option) => (
            <SelectItem key={option.value}>{option.label}</SelectItem>
          )}
        </Select>
      </Field>
    );
  };

  return (
    <Drawer
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      placement="right"
      size="lg"
      isDismissable={hasSnapshot}
      isKeyboardDismissDisabled
      classNames={{
        base: "flex h-[100dvh] max-h-[100dvh] w-full max-w-[min(36rem,100vw)] flex-col overflow-hidden border-l border-warm-200 bg-white shadow-2xl shadow-warm-900/15",
        wrapper: "items-stretch justify-end",
        closeButton:
          "text-ink-500 hover:bg-warm-100 focus-visible:ring-2 focus-visible:ring-brand",
      }}
    >
      <DrawerContent
        data-testid="marketing-settings-drawer"
        className="flex h-[100dvh] max-h-[100dvh] max-w-[min(36rem,100vw)] flex-col overflow-hidden"
        aria-label={t("settings.title")}
        aria-labelledby={titleId}
        onKeyDown={(event) => {
          if (event.key !== "Escape") return;
          if (event.defaultPrevented) return;
          if (isNestedEscapeTarget(event.target)) {
            event.stopPropagation();
            return;
          }
          event.preventDefault();
          event.stopPropagation();
          onClose();
        }}
      >
        {() => (
          <>
            <DrawerHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-5 py-5 sm:px-6">
              <h2
                id={titleId}
                className="font-title text-xl font-semibold tracking-tight text-ink-950"
              >
                {t("settings.title")}
              </h2>
              <p className="max-w-lg text-sm font-normal leading-5 text-ink-600">
                {t("settings.subtitle")}
              </p>
            </DrawerHeader>

            <DrawerBody className="min-h-0 flex-1 overflow-y-auto px-5 py-6 sm:px-6">
              {loading || (!hasSnapshot && !loadError) ? (
                <SettingsSkeleton />
              ) : !hasSnapshot ? (
                <div
                  role="alert"
                  aria-label={t("settings.loadError.title")}
                  className="mx-auto my-8 max-w-md rounded-2xl border border-rose-200 bg-rose-50 px-5 py-6 text-center"
                >
                  <h3 className="text-base font-semibold text-rose-800">
                    {t("settings.loadError.title")}
                  </h3>
                  <p className="mt-2 text-sm leading-6 text-rose-700">
                    {t("settings.loadError.body")}
                  </p>
                  <Button
                    type="button"
                    className="mt-5 bg-rose-700 font-semibold text-white active:scale-[0.98]"
                    radius="full"
                    startContent={<RotateCcw className="h-4 w-4" />}
                    isLoading={retryingLoad}
                    onPress={() => onRetryLoad()}
                  >
                    {t("settings.loadError.retry")}
                  </Button>
                </div>
              ) : (
                <div className="space-y-8">
                  {!canEdit ? (
                    <div className="rounded-xl border border-warm-200 bg-warm-50 px-4 py-3 text-sm text-ink-700">
                      {t("settings.readOnly")}
                    </div>
                  ) : null}

                  {saveError ? (
                    <div
                      className="flex items-start justify-between gap-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3"
                      role="alert"
                    >
                      <p className="text-sm font-medium text-rose-700">
                        {t(
                          rollbackOccurred
                            ? "settings.status.error"
                            : "settings.status.errorGeneric",
                        )}
                      </p>
                      {canEdit ? (
                        <Button
                          size="sm"
                          variant="light"
                          className="h-auto min-w-0 shrink-0 px-2 py-1 font-semibold text-rose-700"
                          startContent={<RotateCcw className="h-3.5 w-3.5" />}
                          onPress={onRetrySave}
                        >
                          {t("settings.status.retry")}
                        </Button>
                      ) : null}
                    </div>
                  ) : null}

                  <section
                    className="space-y-4 rounded-xl border border-warm-200 bg-warm-50/80 p-4"
                    data-testid="brand-lock-summary"
                  >
                    <SectionHeading
                      title={t("settings.brandLock.title")}
                      body={t("settings.brandLock.body")}
                    />
                    <div className="flex flex-wrap items-center gap-4">
                      {brandPreview?.logoUrl ? (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={brandPreview.logoUrl}
                          alt=""
                          width={48}
                          height={48}
                          className="h-12 w-12 rounded-full border border-warm-200 bg-white object-cover"
                          data-testid="brand-lock-logo"
                        />
                      ) : (
                        <div
                          className="flex h-12 w-12 items-center justify-center rounded-full border border-dashed border-warm-300 bg-white text-[10px] font-semibold uppercase tracking-wide text-ink-400"
                          data-testid="brand-lock-logo-empty"
                        >
                          {t("settings.brandLock.noLogo")}
                        </div>
                      )}
                      <div className="flex items-center gap-2">
                        <span
                          className="h-8 w-8 rounded-full border border-warm-200 shadow-sm"
                          style={{
                            // Live brand swatch from design_settings (operator-owned hex).
                            // eslint-disable-next-line no-restricted-syntax -- dynamic brand color, not a hard-coded UI token
                            backgroundColor: brandPreview?.primaryColor || "#1a6b6a",
                          }}
                          title={t("settings.brandLock.primary")}
                          data-testid="brand-lock-primary"
                        />
                        <span
                          className="h-8 w-8 rounded-full border border-warm-200 shadow-sm"
                          style={{
                            // Live brand swatch from design_settings (operator-owned hex).
                            // eslint-disable-next-line no-restricted-syntax -- dynamic brand color, not a hard-coded UI token
                            backgroundColor: brandPreview?.secondaryColor || "#0f3d3c",
                          }}
                          title={t("settings.brandLock.secondary")}
                          data-testid="brand-lock-secondary"
                        />
                      </div>
                      <div className="min-w-0 flex-1 space-y-1 text-xs leading-5 text-ink-600">
                        <p>
                          <span className="font-semibold text-ink-800">
                            {t("settings.brandLock.defaultKit")}:{" "}
                          </span>
                          {(() => {
                            const moodKit = kitBiasFromVisualMood(
                              draft.creative_profile?.visual_mood,
                            );
                            return moodKit
                              ? t(`kits.${moodKit}`)
                              : t("settings.brandLock.kitFromPlay");
                          })()}
                        </p>
                        <p>{t("settings.brandLock.fontNote")}</p>
                        {brandPreview?.handle ? (
                          <p className="font-medium text-ink-700">
                            {brandPreview.handle}
                          </p>
                        ) : null}
                      </div>
                    </div>
                  </section>

                  <section className="space-y-5">
                    <SectionHeading
                      title={t("settings.autonomy.title")}
                      body={t("settings.autonomy.body")}
                    />
                    <div className="flex items-center justify-between gap-6 rounded-xl border border-warm-200 bg-warm-50 px-4 py-3.5">
                      <div>
                        <p className="text-sm font-semibold text-ink-900">
                          {t("settings.autonomy.master")}
                        </p>
                        <p className="mt-0.5 text-xs leading-5 text-ink-500">
                          {t("settings.autonomy.masterHint")}
                        </p>
                      </div>
                      <Toggle
                        checked={draft.enabled}
                        disabled={!canEdit || saving}
                        label={t("settings.autonomy.master")}
                        onChange={() =>
                          commit({ ...draft, enabled: !draft.enabled })
                        }
                      />
                    </div>
                    <div className="space-y-1">
                      <p className="pb-2 text-sm font-semibold text-ink-800">
                        {t("settings.autonomy.plays")}
                      </p>
                      {MARKETING_PLAYS.map((play) => (
                        <div
                          key={play}
                          className="flex items-start justify-between gap-5 rounded-xl px-2 py-2.5 transition-colors hover:bg-warm-50"
                        >
                          <div className="min-w-0">
                            <p className="text-sm font-medium text-ink-800">
                              {t(`plays.${play}`)}
                            </p>
                            <p className="mt-0.5 text-xs leading-5 text-ink-500">
                              {t(`automation.descriptions.${play}`)}
                            </p>
                          </div>
                          <Toggle
                            checked={!draft.disabled_plays.includes(play)}
                            disabled={!canEdit || saving}
                            label={t(`plays.${play}`)}
                            onChange={() => togglePlay(play)}
                          />
                        </div>
                      ))}
                    </div>
                  </section>

                  <section className="space-y-5 border-t border-warm-200 pt-8">
                    <SectionHeading
                      title={t("settings.brandVoice.title")}
                      body={t("settings.brandVoice.body")}
                    />
                    {textField("audience")}
                    {textField("voice", { multiline: true })}
                  </section>

                  <section className="space-y-5 border-t border-warm-200 pt-8">
                    <SectionHeading
                      title={t("settings.visualDirection.title")}
                      body={t("settings.visualDirection.body")}
                    />
                    {selectField(
                      "visual_mood",
                      "visualMood",
                      VISUAL_MOODS,
                      "settings.options.visualMood",
                    )}
                  </section>

                  <section className="space-y-5 border-t border-warm-200 pt-8">
                    <SectionHeading
                      title={t("settings.copyDefaults.title")}
                      body={t("settings.copyDefaults.body")}
                    />
                    <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
                      {selectField(
                        "cta_style",
                        "ctaStyle",
                        CTA_STYLES,
                        "settings.options.ctaStyle",
                      )}
                      {selectField(
                        "hashtag_behavior",
                        "hashtags",
                        HASHTAG_BEHAVIORS,
                        "settings.options.hashtags",
                      )}
                      {selectField(
                        "default_language",
                        "language",
                        LANGUAGES,
                        "settings.options.languages",
                      )}
                      {selectField("default_tone", "tone", TONES, "tones")}
                    </div>
                    <div className="space-y-2">
                      <label
                        htmlFor="marketing-settings-avoid-phrases"
                        className="block text-sm font-semibold text-ink-800"
                      >
                        {t("settings.fields.avoidPhrases.label")}
                      </label>
                      <input
                        id="marketing-settings-avoid-phrases"
                        type="text"
                        value={avoidPhrasesDraft}
                        disabled={!canEdit || saving}
                        aria-invalid={!!avoidPhrasesError}
                        aria-describedby={`marketing-settings-avoid-phrases-helper${
                          avoidPhrasesError
                            ? " marketing-settings-avoid-phrases-error"
                            : ""
                        }`}
                        className={controlClass}
                        placeholder={t(
                          "settings.fields.avoidPhrases.placeholder",
                        )}
                        onChange={(event) => {
                          const nextDraft = event.target.value;
                          setAvoidPhrasesDraft(nextDraft);
                          setAvoidPhrasesDirty(true);
                          setAvoidPhrasesError(
                            parseAvoidPhrases(nextDraft).error,
                          );
                        }}
                        onBlur={() => {
                          const parsed = parseAvoidPhrases(avoidPhrasesDraft);
                          setAvoidPhrasesError(parsed.error);
                          if (parsed.error) return;
                          const normalizedDraft = parsed.phrases.join(", ");
                          setAvoidPhrasesDraft(normalizedDraft);
                          commitProfile("avoid_phrases", parsed.phrases);
                        }}
                      />
                      <p
                        id="marketing-settings-avoid-phrases-helper"
                        className="text-xs leading-5 text-ink-500"
                      >
                        {t("settings.fields.avoidPhrases.helper")}
                      </p>
                      {avoidPhrasesError ? (
                        <p
                          id="marketing-settings-avoid-phrases-error"
                          role="alert"
                          className="text-xs font-medium leading-5 text-rose-700"
                        >
                          {t(
                            `settings.fields.avoidPhrases.${avoidPhrasesError}`,
                            {
                              max:
                                avoidPhrasesError === "tooMany"
                                  ? MAX_AVOID_PHRASES
                                  : MAX_AVOID_PHRASE_RUNES,
                            },
                          )}
                        </p>
                      ) : null}
                    </div>
                  </section>
                </div>
              )}
            </DrawerBody>

            <DrawerFooter className="min-h-16 shrink-0 items-center justify-between border-t border-warm-200/80 bg-warm-50/70 px-5 py-3 sm:px-6">
              <div className="min-w-0 text-sm text-ink-600" aria-live="polite">
                {!canEdit && hasSnapshot ? (
                  <span role="status">{t("settings.status.readOnly")}</span>
                ) : null}
                {canEdit && saving ? (
                  <span role="status">{t("settings.status.saving")}</span>
                ) : null}
                {canEdit && !saving && saved && !saveError && hasSnapshot ? (
                  <span
                    role="status"
                    className="inline-flex items-center gap-1.5 font-medium text-emerald-700"
                  >
                    <Check className="h-4 w-4" />
                    {t("settings.status.saved")}
                  </span>
                ) : null}
                {canEdit && !saving && !saved && !saveError && hasSnapshot ? (
                  <span>{t("settings.status.autoSave")}</span>
                ) : null}
              </div>
              <Button
                variant="flat"
                radius="full"
                className="shrink-0 bg-warm-100 font-semibold text-ink-800 hover:bg-warm-200 active:scale-[0.98]"
                onPress={onClose}
              >
                {t("settings.close")}
              </Button>
            </DrawerFooter>
          </>
        )}
      </DrawerContent>
    </Drawer>
  );
}

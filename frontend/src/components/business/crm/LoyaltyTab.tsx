"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";
import React, { useCallback, useEffect, useRef, useState } from "react";
import { Button, Switch } from "@nextui-org/react";
import { useToast } from "@/contexts/ToastContext";
import {
  getLoyalty,
  putLoyalty,
  previewLoyalty,
  LoyaltyTier,
} from "@/api/loyalty";
import { getBusiness } from "@/api/business";
import TierEditor, { validateTiersOnCommit } from "./loyalty/TierEditor";
import TierPreview from "./loyalty/TierPreview";
import SaveBar from "../SaveBar";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { formatMoneyAmount } from "@/lib/moneyFormat";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { CRMLoyaltySkeleton } from "./CRMLoyaltySkeleton";

interface LoyaltyTabProps {
  businessId: number;
}

// Stable structural comparison of the editable loyalty state. Wraps
// JSON.stringify with a key-sorted serializer so out-of-order keys don't
// trip the dirty detector. Used by the Save button to disable itself when
// nothing has changed.
function loyaltySignature(args: {
  enabled: boolean;
  earnRate: number;
  redeemRate: number;
  tiers: LoyaltyTier[];
}) {
  const tiers = args.tiers.map((t) => ({
    name: t.name,
    min_lifetime_spent: t.min_lifetime_spent,
    color: t.color ?? "",
    sort_order: t.sort_order,
  }));
  return JSON.stringify({
    enabled: args.enabled,
    earnRate: args.earnRate,
    redeemRate: args.redeemRate,
    tiers,
  });
}

function tierSignature(tiers: LoyaltyTier[]) {
  return JSON.stringify(
    tiers.map((tier) => ({
      name: tier.name,
      min_lifetime_spent: tier.min_lifetime_spent,
      color: tier.color ?? "",
      sort_order: tier.sort_order,
    })),
  );
}

function formatRateLabel(rate: number, locale: string): string {
  return Number.isInteger(rate)
    ? String(rate)
    : new Intl.NumberFormat(locale, {
        maximumFractionDigits: 2,
      }).format(rate);
}

type RateFieldState = {
  empty: boolean;
  outOfRange: boolean;
  invalid: boolean;
  effective: number;
  errorMessage: string | undefined;
};

function parseRateField(
  text: string,
  fallback: number,
  min: number,
  max: number,
  t: (key: string) => string,
): RateFieldState {
  const parsed = tryParseLocaleDecimal(text);
  const empty = text.trim() === "";
  const outOfRange = parsed !== null && (parsed < min || parsed > max);
  const invalid = empty || parsed === null || outOfRange;
  const effective = !invalid && parsed !== null ? parsed : fallback;
  const errorMessage = empty
    ? t("rateRequired")
    : parsed === null
      ? t("rateInvalid")
      : outOfRange
        ? t("rateOutOfRange")
        : undefined;
  return { empty, outOfRange, invalid, effective, errorMessage };
}

export default function LoyaltyTab({ businessId }: LoyaltyTabProps) {
  const { showSuccess, showError } = useToast();
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.crm.loyalty.${key}`;
      const result = getTranslation(fullKey, locale);
      return typeof result === "string" ? result : fullKey;
    },
    [locale],
  );
  // Shared SaveBar copy lives under businessSettings.saveBar.* so the "unsaved"
  // affordance reads identically across settings tabs.
  const saveBarString = useCallback(
    (key: string): string => {
      const result = getTranslation(`businessSettings.saveBar.${key}`, locale);
      return typeof result === "string" ? result : key;
    },
    [locale],
  );
  const [initialLoading, setInitialLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [enabled, setEnabled] = useState(true);
  const [earnRate, setEarnRate] = useState(1);
  const [earnRateText, setEarnRateText] = useState("1");
  const [redeemRate, setRedeemRate] = useState(100);
  const [redeemRateText, setRedeemRateText] = useState("100");
  const [tiers, setTiers] = useState<LoyaltyTier[]>([]);
  const [savedSignature, setSavedSignature] = useState<string>("");
  const [invalidLoadedTierSignature, setInvalidLoadedTierSignature] = useState<
    string | null
  >(null);
  const [currency, setCurrency] = useState<string>("USD");
  const [preview, setPreview] = useState<{
    distribution: Record<string, number>;
    total: number;
  } | null>(null);
  const [saving, setSaving] = useState(false);
  const saveGeneration = useRef(0);

  useEffect(() => {
    // Invalidate any save that belongs to the previous business or reload.
    // The old request may still resolve after this effect starts.
    ++saveGeneration.current;
    setSaving(false);
    let active = true;
    setInitialLoading(true);
    setLoadError(false);
    getLoyalty(businessId)
      .then((data) => {
        if (!active) return;
        const earn = data.program.points_per_dollar;
        // Missing/legacy rows without a redeem field fall back to 100 pts/$1.
        const redeem =
          data.program.redemption_points_per_dollar &&
          data.program.redemption_points_per_dollar > 0
            ? data.program.redemption_points_per_dollar
            : 100;
        setEnabled(data.program.enabled);
        setEarnRate(earn);
        setEarnRateText(String(earn));
        setRedeemRate(redeem);
        setRedeemRateText(String(redeem));
        setTiers(data.tiers);
        const loadedInvalid =
          data.valid === false || Boolean(data.validation_error);
        setInvalidLoadedTierSignature(
          loadedInvalid ? tierSignature(data.tiers) : null,
        );
        setPreview(null);
        setSavedSignature(
          loyaltySignature({
            enabled: data.program.enabled,
            earnRate: earn,
            redeemRate: redeem,
            tiers: data.tiers,
          }),
        );
      })
      .catch(() => {
        if (!active) return;
        // Loading the program failed. DON'T render editable defaults
        // (enabled=true, rates) — that arms a dirty Save whose PUT
        // would wipe the whole program. Surface an error state instead and
        // keep the editor/Save disabled until a successful reload.
        setLoadError(true);
      })
      .finally(() => {
        if (active) setInitialLoading(false);
      });
    getBusiness(businessId)
      .then((b) => {
        if (!active) return;
        if (b?.default_currency) setCurrency(b.default_currency);
      })
      .catch(() => {
        // Currency label is best-effort; falling back to USD is fine.
      });

    return () => {
      active = false;
      // Invalidate saves on unmount as well as on business/reload changes.
      saveGeneration.current += 1;
    };
  }, [businessId, reloadKey]);

  const loadedTierIsUnchanged =
    invalidLoadedTierSignature !== null &&
    invalidLoadedTierSignature === tierSignature(tiers);

  const earnField = parseRateField(earnRateText, earnRate, 0, 100, t);
  const redeemField = parseRateField(
    redeemRateText,
    redeemRate,
    0,
    10000,
    (key) => {
      if (key === "rateOutOfRange") return t("redeemRateOutOfRange");
      return t(key);
    },
  );
  const ratesInvalid = earnField.invalid || redeemField.invalid;

  useEffect(() => {
    const validationError = validateTiersOnCommit(tiers);
    if (validationError || loadedTierIsUnchanged || ratesInvalid) {
      setPreview(null);
      return;
    }
    if (tiers.length === 0) {
      setPreview(null);
      return;
    }
    let active = true;
    const timer = setTimeout(() => {
      previewLoyalty(businessId, {
        points_per_dollar: earnField.effective,
        redemption_points_per_dollar: redeemField.effective,
        tiers,
      })
        .then((p) => {
          if (!active) return;
          setPreview({
            distribution: p.tier_distribution,
            total: p.total_customers,
          });
        })
        // A failed debounce preview must not silently leave an obsolete
        // distribution visible. Clear only if this request is still current.
        .catch(() => {
          if (active) setPreview(null);
        });
    }, 400);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [
    earnField.effective,
    redeemField.effective,
    tiers,
    businessId,
    loadedTierIsUnchanged,
    ratesInvalid,
  ]);

  const dirty =
    !ratesInvalid &&
    loyaltySignature({
      enabled,
      earnRate: earnField.effective,
      redeemRate: redeemField.effective,
      tiers,
    }) !== savedSignature;

  const save = async () => {
    if (validateTiersOnCommit(tiers) || loadedTierIsUnchanged) {
      showError(t("toast.saveError"), t("invalidLoaded"));
      return;
    }
    const parsedEarn = tryParseLocaleDecimal(earnRateText);
    const parsedRedeem = tryParseLocaleDecimal(redeemRateText);
    if (
      parsedEarn === null ||
      parsedEarn < 0 ||
      parsedEarn > 100 ||
      earnRateText.trim() === ""
    ) {
      showError(t("toast.saveError"), t("rateOutOfRange"));
      return;
    }
    if (
      parsedRedeem === null ||
      parsedRedeem < 0 ||
      parsedRedeem > 10000 ||
      redeemRateText.trim() === ""
    ) {
      showError(t("toast.saveError"), t("redeemRateOutOfRange"));
      return;
    }
    setEarnRate(parsedEarn);
    setEarnRateText(String(parsedEarn));
    setRedeemRate(parsedRedeem);
    setRedeemRateText(String(parsedRedeem));
    const generation = saveGeneration.current;
    setSaving(true);
    try {
      await putLoyalty(businessId, {
        enabled,
        points_per_dollar: parsedEarn,
        redemption_points_per_dollar: parsedRedeem,
        tiers,
      });
      if (generation !== saveGeneration.current) return;
      setSavedSignature(
        loyaltySignature({
          enabled,
          earnRate: parsedEarn,
          redeemRate: parsedRedeem,
          tiers,
        }),
      );
      showSuccess(t("toast.saved"));
    } catch (err) {
      if (generation !== saveGeneration.current) return;
      showError(
        t("toast.saveError"),
        getSafeApiErrorMessage(err, t("toast.saveError")),
      );
    } finally {
      if (generation === saveGeneration.current) setSaving(false);
    }
  };

  if (initialLoading) return <CRMLoyaltySkeleton />;

  if (loadError) {
    return (
      <div className="max-w-2xl rounded-xl border border-amber-200 bg-amber-50 px-4 py-6 text-center">
        <p className="text-sm text-amber-800" role="alert">
          {t("loadError")}
        </p>
        <Button
          variant="bordered"
          size="sm"
          radius="lg"
          onPress={() => setReloadKey((k) => k + 1)}
          className="mt-3 border-amber-300 bg-white font-medium text-amber-900 hover:bg-amber-100"
        >
          {t("retry")}
        </Button>
      </div>
    );
  }

  const unitAmount = formatMoneyAmount(1, currency, {
    surface: "operator",
    locale,
  });

  return (
    <div className="space-y-6 max-w-2xl">
      <div className="flex items-center gap-3">
        <Switch
          isSelected={enabled}
          onValueChange={setEnabled}
          aria-label={t("enabled")}
        />
        <span className="text-sm text-ink-800">{t("enabled")}</span>
      </div>
      {/* L5-12: flex+gap+pt-1 (not space-y) preserves NextUI outside-label
          headroom so the rate label does not collide with the Switch row. */}
      <div className="flex flex-col gap-4">
        <div
          className="flex flex-col gap-2 pt-1"
          data-testid="loyalty-rate-field-wrap"
        >
          <DecimalInput
            label={t("pointsPerCurrency").replace("{amount}", unitAmount)}
            labelPlacement="outside"
            value={earnRateText}
            onValueChange={setEarnRateText}
            onParsedChange={(n) => {
              if (n === null) return;
              setEarnRate(Math.min(100, Math.max(0, n)));
            }}
            min={0}
            max={100}
            isInvalid={earnRateText.trim() !== "" && earnField.invalid}
            errorMessage={
              earnRateText.trim() !== "" && earnField.invalid
                ? earnField.errorMessage
                : undefined
            }
            variant="bordered"
            radius="lg"
            className="w-full sm:w-48 tabular-nums"
            data-testid="loyalty-points-rate"
            classNames={{
              label: "text-sm font-medium text-ink-700",
            }}
          />
          {!earnField.invalid && earnField.effective > 0 && (
            <p
              className="text-sm text-ink-500"
              data-testid="loyalty-earn-explanation"
            >
              {t("earnExplanation")
                .replace("{rate}", formatRateLabel(earnField.effective, locale))
                .replace("{amount}", unitAmount)}
            </p>
          )}
        </div>

        <div
          className="flex flex-col gap-2"
          data-testid="loyalty-redeem-rate-field-wrap"
        >
          <DecimalInput
            label={t("redeemPointsPerCurrency").replace("{amount}", unitAmount)}
            labelPlacement="outside"
            value={redeemRateText}
            onValueChange={setRedeemRateText}
            onParsedChange={(n) => {
              if (n === null) return;
              setRedeemRate(Math.min(10000, Math.max(0, n)));
            }}
            min={0}
            max={10000}
            isInvalid={redeemRateText.trim() !== "" && redeemField.invalid}
            errorMessage={
              redeemRateText.trim() !== "" && redeemField.invalid
                ? redeemField.errorMessage
                : undefined
            }
            variant="bordered"
            radius="lg"
            className="w-full sm:w-48 tabular-nums"
            data-testid="loyalty-redeem-rate"
            classNames={{
              label: "text-sm font-medium text-ink-700",
            }}
          />
          {!redeemField.invalid && redeemField.effective > 0 && (
            <div
              className="text-sm text-ink-500 space-y-0.5"
              data-testid="loyalty-rate-explanations"
            >
              <p data-testid="loyalty-redeem-explanation">
                {t("redeemExplanation")
                  .replace(
                    "{rate}",
                    formatRateLabel(redeemField.effective, locale),
                  )
                  .replace("{amount}", unitAmount)}
              </p>
              {!earnField.invalid &&
                earnField.effective > 0 &&
                redeemField.effective > earnField.effective && (
                  <p data-testid="loyalty-cashback-hint">
                    {t("cashbackHint").replace(
                      "{pct}",
                      formatRateLabel(
                        (earnField.effective / redeemField.effective) * 100,
                        locale,
                      ),
                    )}
                  </p>
                )}
            </div>
          )}
        </div>
      </div>
      <div>
        <h3 className="font-semibold text-ink-900 mb-2">{t("tierLadder")}</h3>
        {(loadedTierIsUnchanged || validateTiersOnCommit(tiers)) && (
          <p
            role="alert"
            className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900"
          >
            {t("invalidLoaded")}
          </p>
        )}
        <TierEditor tiers={tiers} onChange={setTiers} currency={currency} />
      </div>
      {preview && (
        <div>
          <h3 className="font-semibold text-ink-900 mb-2">{t("preview")}</h3>
          <TierPreview
            distribution={preview.distribution}
            total={preview.total}
            tiers={tiers}
            t={t}
          />
        </div>
      )}
      <SaveBar
        mode="button"
        isSaving={saving}
        dirty={dirty && !ratesInvalid}
        onSave={() => void save()}
        labels={{
          save: t("saveChanges"),
          saving: t("saving"),
          unsaved: saveBarString("unsaved"),
          auto: saveBarString("auto"),
          clean: saveBarString("clean"),
        }}
      />
    </div>
  );
}

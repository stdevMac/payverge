import React, { useCallback, useEffect, useState } from "react";
import { Button, Input } from "@nextui-org/react";
import { Plus, Trash2 } from "lucide-react";
import type { LoyaltyTier } from "@/api/loyalty";
import { normalizeLoyaltyTierName } from "./tierNameNormalization";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { formatMoneyAmount } from "@/lib/moneyFormat";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import ConfirmationModal from "../../modals/ConfirmationModal";

interface TierEditorProps {
  tiers: LoyaltyTier[];
  onChange: (tiers: LoyaltyTier[]) => void;
  currency?: string;
}

/** Currency glyph/code for the threshold prefix (locale-aware, not bare "USD"). */
function thresholdCurrencyLabel(currency: string, locale: string): string {
  try {
    const part = new Intl.NumberFormat(locale, {
      style: "currency",
      currency,
    })
      .formatToParts(0)
      .find((p) => p.type === "currency");
    return part?.value ?? currency;
  } catch {
    return currency;
  }
}

// A validation error keyed by translation string + optional interpolation.
// Kept translator-agnostic so the pure validator can be unit-tested without
// wiring i18n.
export interface TierValidationError {
  key:
    | "errorBlankName"
    | "errorDuplicateName"
    | "errorDuplicateThreshold"
    | "errorThresholdOrder";
  params?: Record<string, string>;
}

// Threshold-only validator: rejects duplicate `min_lifetime_spent` thresholds
// and (once sorted by threshold) any ladder that isn't strictly increasing.
// Returns the first violation, or null when the ladder is valid. Name
// collisions are intentionally NOT checked here — a duplicate name is a
// transient state while the operator types ("Gold" → "Golden") and blocking it
// per-keystroke swallows characters. Names are validated on commit instead.
export function validateTiers(
  tiers: LoyaltyTier[],
): TierValidationError | null {
  const sorted = tiers.slice().sort((a, b) => a.sort_order - b.sort_order);
  for (let i = 1; i < sorted.length; i += 1) {
    if (sorted[i].min_lifetime_spent === sorted[i - 1].min_lifetime_spent) {
      return { key: "errorDuplicateThreshold" };
    }
    if (sorted[i].min_lifetime_spent < sorted[i - 1].min_lifetime_spent) {
      return { key: "errorThresholdOrder" };
    }
  }

  return null;
}

// Full validator including duplicate (case-insensitive) tier names. Run this on
// commit (blur / save) rather than per keystroke. Exported for unit testing.
export function validateTiersOnCommit(
  tiers: LoyaltyTier[],
): TierValidationError | null {
  const seenNames = new Map<string, string>();
  for (const tier of tiers) {
    const normalizedName = normalizeLoyaltyTierName(tier.name ?? "");
    if (!normalizedName) {
      return { key: "errorBlankName" };
    }
    if (seenNames.has(normalizedName)) {
      return { key: "errorDuplicateName", params: { name: tier.name } };
    }
    seenNames.set(normalizedName, tier.name);
  }
  return validateTiers(tiers);
}

export default function TierEditor({
  tiers,
  onChange,
  currency = "USD",
}: TierEditorProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.crm.loyalty.editor.${key}`;
      const result = getTranslation(fullKey, locale);
      return typeof result === "string" ? result : fullKey;
    },
    [locale],
  );

  const [error, setError] = useState<TierValidationError | null>(null);
  // Deleting a tier is destructive (customers drop a rung), so confirm first.
  const [pendingDelete, setPendingDelete] = useState<number | null>(null);
  // A3b: threshold string state so "250." / "1,5" survive mid-keystroke.
  const [thresholdTexts, setThresholdTexts] = useState<string[]>(() =>
    tiers.map((t) => String(t.min_lifetime_spent ?? 0)),
  );
  useEffect(() => {
    setThresholdTexts(tiers.map((t) => String(t.min_lifetime_spent ?? 0)));
  }, [tiers]);

  // Per-keystroke commit: validate thresholds only (never names). A transient
  // duplicate name mid-typing must not drop the edit. On threshold failure we
  // surface the message inline and drop the bad edit so an overlapping
  // threshold never reaches `onChange` (and thus the save path).
  const commit = (next: LoyaltyTier[]): boolean => {
    const violation = validateTiers(next);
    if (violation) {
      setError(violation);
      return false;
    }
    setError(null);
    onChange(next);
    return true;
  };

  // Name-field blur: now that the operator has finished typing, run the full
  // check (names + thresholds) so a genuine duplicate name still surfaces.
  const commitNames = () => {
    setError(validateTiersOnCommit(tiers));
  };

  const update = (i: number, patch: Partial<LoyaltyTier>) =>
    commit(
      tiers.map((tier, idx) => (idx === i ? { ...tier, ...patch } : tier)),
    );
  const remove = (i: number) => commit(tiers.filter((_, idx) => idx !== i));
  // Seed the new tier ABOVE the current ceiling so it never collides with the
  // base $0 tier (the default Bronze=0 ladder) and gets rejected before it can
  // render. The operator adjusts name/threshold afterwards.
  const add = () => {
    const ceiling = tiers.reduce(
      (max, tier) => Math.max(max, tier.min_lifetime_spent ?? 0),
      0,
    );
    const nextSortOrder = tiers.reduce(
      (max, tier) => Math.max(max, tier.sort_order),
      -1,
    ) + 1;
    commit([
      ...tiers,
      {
        name: t("newTierName"),
        min_lifetime_spent: ceiling + 1,
        sort_order: nextSortOrder,
      },
    ]);
  };

  const errorMessage = error
    ? error.params
      ? Object.entries(error.params).reduce(
          (msg, [k, v]) => msg.replace(`{${k}}`, v),
          t(error.key),
        )
      : t(error.key)
    : null;

  return (
    <div className="space-y-3">
      {tiers.map((tier, i) => (
        <div key={i} className="flex items-end gap-2">
          <Input
            label={t("tierName")}
            labelPlacement="outside"
            placeholder={t("tierName")}
            value={tier.name}
            onValueChange={(v) => update(i, { name: v })}
            onBlur={commitNames}
            variant="bordered"
            radius="lg"
            size="sm"
            className="flex-1"
          />
          <DecimalInput
            min={0}
            maxFractionDigits={2}
            label={t("tierThreshold")}
            labelPlacement="outside"
            value={thresholdTexts[i] ?? String(tier.min_lifetime_spent ?? 0)}
            onValueChange={(v) => {
              setThresholdTexts((prev) => {
                const next = prev.slice();
                next[i] = v;
                return next;
              });
            }}
            onParsedChange={(n) => {
              if (n === null) return;
              update(i, { min_lifetime_spent: n });
            }}
            isInvalid={
              (thresholdTexts[i] ?? "").trim() !== "" &&
              tryParseLocaleDecimal(thresholdTexts[i] ?? "") === null
            }
            startContent={
              <span
                className="text-ink-500 text-sm"
                data-testid={`tier-threshold-currency-${i}`}
              >
                {thresholdCurrencyLabel(currency, locale)}
              </span>
            }
            // S-4: screen-reader + hover see full locale money, not "USD 250".
            aria-label={`${t("tierThreshold")}: ${formatMoneyAmount(
              tier.min_lifetime_spent,
              currency,
              { surface: "operator", locale },
            )}`}
            variant="bordered"
            radius="lg"
            size="sm"
            className="w-40 tabular-nums"
            data-testid={`tier-threshold-${i}`}
          />
          <div className="flex flex-col gap-1">
            <span className="text-xs text-ink-500">{t("tierColor")}</span>
            <input
              type="color"
              // eslint-disable-next-line no-restricted-syntax -- color-input fallback, must be hex
              value={tier.color || "#94A3B8"}
              onChange={(e) => update(i, { color: e.target.value })}
              className="h-9 w-10 cursor-pointer rounded border border-warm-200"
              title={t("tierColor")}
              aria-label={t("colorForTier").replace(
                "{tier}",
                tier.name || t("tierFallback"),
              )}
            />
          </div>
          <Button
            isIconOnly
            variant="light"
            color="danger"
            aria-label={t("deleteTier")}
            onPress={() => setPendingDelete(i)}
            className="mb-0.5"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ))}
      <Button
        variant="bordered"
        size="sm"
        radius="lg"
        startContent={<Plus className="h-4 w-4" />}
        onPress={add}
        className="border-warm-200 font-medium text-brand-dark hover:border-brand hover:bg-brand/5"
      >
        {t("addTier")}
      </Button>
      {errorMessage && (
        <p role="alert" className="text-sm text-rose-600 mt-1">
          {errorMessage}
        </p>
      )}

      <ConfirmationModal
        isOpen={pendingDelete !== null}
        onOpenChange={() => setPendingDelete(null)}
        title={t("deleteTierConfirmTitle")}
        description={
          pendingDelete !== null
            ? t("deleteTierConfirmBody").replace(
                "{tier}",
                tiers[pendingDelete]?.name || t("tierFallback"),
              )
            : ""
        }
        confirmLabel={t("deleteTier")}
        isDanger
        onConfirm={() => {
          if (pendingDelete !== null) remove(pendingDelete);
          setPendingDelete(null);
        }}
      />
    </div>
  );
}

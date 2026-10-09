"use client";

import { useMemo } from "react";
import { Button, Input, Chip, Switch, Divider } from "@nextui-org/react";
import { CreditCard, Copy, Percent, Receipt } from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import IconTile from "@/components/ui/IconTile";
import { formatRatePercent } from "@/utils/formatRatePercent";
import {
  SAMPLE_CHECK_SUBTOTAL,
  formatSampleCheckMoney,
  previewSampleCheck,
} from "@/utils/sampleCheckPreview";
import { localDateKey } from "@/lib/localDate";

// H3: canonical EVM address shape for the USDC settlement / tipping payout
// wallets. A malformed address means funds are unrecoverable, so we validate
// inline and block Save until both are well-formed.
const EVM_ADDRESS_REGEX = /^0x[a-fA-F0-9]{40}$/;

// M17: rates are percentages — negatives and 900% are nonsense that would
// corrupt every bill total. Constrain to [0, 100].
const RATE_MIN = 0;
const RATE_MAX = 100;

interface PaymentProfile {
  settlement_address: string;
  tipping_address: string;
  tax_rate: number;
  service_fee_rate: number;
  tax_inclusive: boolean;
  service_inclusive: boolean;
}

interface BusinessPaymentSettingsTabProps {
  profile: PaymentProfile;
  handleInputChange: (
    field:
      | "settlement_address"
      | "tipping_address"
      | "tax_rate"
      | "service_fee_rate"
      | "tax_inclusive"
      | "service_inclusive",
    value: string | number | boolean,
  ) => void;
  // P1-12: staff sessions cannot move the owner's payout wallets — render
  // those two inputs read-only with an explanatory description.
  walletFieldsDisabled?: boolean;
}

export default function BusinessPaymentSettingsTab({
  profile,
  handleInputChange,
  walletFieldsDisabled,
}: BusinessPaymentSettingsTabProps) {
  const { locale: currentLocale } = useSimpleLocale();
  const { showSuccess, showError } = useToast();

  const tString = (key: string): string => {
    const fullKey = `businessSettings.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const samplePreview = useMemo(
    () =>
      previewSampleCheck({
        tax_rate: profile.tax_rate,
        service_fee_rate: profile.service_fee_rate,
        tax_inclusive: profile.tax_inclusive,
        service_inclusive: profile.service_inclusive,
      }),
    [
      profile.tax_rate,
      profile.service_fee_rate,
      profile.tax_inclusive,
      profile.service_inclusive,
    ],
  );

  // #266: dated effective-on-save disclosure (rates apply immediately; no
  // silent free-type). Locale-aware calendar date for the operator's browser.
  const effectiveDateLabel = useMemo(() => {
    try {
      return new Intl.DateTimeFormat(currentLocale || "en", {
        dateStyle: "medium",
      }).format(new Date());
    } catch {
      return localDateKey();
    }
  }, [currentLocale]);

  // H3: an empty address is allowed here (isRequired handles the "must fill"
  // hint separately) — only a NON-empty malformed value is flagged invalid so
  // operators aren't fought while typing an address into an empty field.
  const settlementInvalid =
    profile.settlement_address.trim().length > 0 &&
    !EVM_ADDRESS_REGEX.test(profile.settlement_address.trim());
  const tippingInvalid =
    profile.tipping_address.trim().length > 0 &&
    !EVM_ADDRESS_REGEX.test(profile.tipping_address.trim());

  const settlementTrimmed = profile.settlement_address.trim();
  const tippingTrimmed = profile.tipping_address.trim();
  const sameWalletInvalid =
    settlementTrimmed.length > 0 &&
    tippingTrimmed.length > 0 &&
    settlementTrimmed.toLowerCase() === tippingTrimmed.toLowerCase();

  // M17: flag out-of-range rates. NaN (blank field parsed as NaN upstream) is
  // treated as invalid so Save can't push a broken rate.
  const serviceFeeInvalid =
    !Number.isFinite(profile.service_fee_rate) ||
    profile.service_fee_rate < RATE_MIN ||
    profile.service_fee_rate > RATE_MAX;
  const taxRateInvalid =
    !Number.isFinite(profile.tax_rate) ||
    profile.tax_rate < RATE_MIN ||
    profile.tax_rate > RATE_MAX;

  // NOTE: the parent (BusinessSettings) computes the authoritative Save-block
  // from the same shared profile values, so it stays enforced even while this
  // tab is unmounted. These locals only drive the inline field states below.

  const copyToClipboard = async (value: string, label: string) => {
    if (!value) {
      showError(
        tString("payment.copyEmptyTitle"),
        tString("payment.copyEmptyDescription"),
      );
      return;
    }

    try {
      await navigator.clipboard.writeText(value);
      showSuccess(
        tString("payment.copySuccessTitle"),
        `${label} ${tString("payment.copySuccessSuffix")}`,
      );
    } catch (err) {
      showError(
        tString("payment.copyFailedTitle"),
        tString("payment.copyFailedDescription"),
      );
    }
  };

  // Fees & taxes lead; optional crypto payout wallets sit at the bottom so a
  // dinner venue is not asked for an Ethereum address as the first setting.
  return (
    <div className="space-y-8 w-full">
      {/* Fees & Taxes — primary operator concern */}
      <div className="space-y-6">
        <div className="flex items-center gap-3 mb-2">
          <IconTile icon={Percent} />
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {tString("payment.feesAndTaxesTitle") || "Fees & Taxes"}
            </h3>
            <p className="text-sm text-ink-600">
              {tString("payment.feesAndTaxesDescription") ||
                "How service fees and taxes apply to each order"}
            </p>
          </div>
        </div>

        <div className="rounded-2xl border border-brand/15 bg-brand/5 p-4 text-sm text-ink-700">
          {tString("payment.checkoutRailsNotice")}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            type="number"
            label={tString("payment.serviceFeeRateLabel")}
            placeholder={tString("payment.serviceFeeRatePlaceholder")}
            value={profile.service_fee_rate.toString()}
            onValueChange={(value) =>
              handleInputChange(
                "service_fee_rate",
                Number.parseFloat(value || "0"),
              )
            }
            description={tString("payment.serviceFeeRateDescription")}
            variant="bordered"
            min={RATE_MIN}
            max={RATE_MAX}
            step={0.01}
            isInvalid={serviceFeeInvalid}
            errorMessage={
              serviceFeeInvalid ? tString("payment.rateOutOfRange") : undefined
            }
            endContent={<span className="text-sm text-ink-500">%</span>}
          />

          <Input
            type="number"
            label={tString("payment.taxRateLabel")}
            placeholder={tString("payment.taxRatePlaceholder")}
            value={profile.tax_rate.toString()}
            onValueChange={(value) =>
              handleInputChange("tax_rate", Number.parseFloat(value || "0"))
            }
            description={tString("payment.taxRateDescription")}
            variant="bordered"
            min={RATE_MIN}
            max={RATE_MAX}
            step={0.01}
            isInvalid={taxRateInvalid}
            errorMessage={
              taxRateInvalid ? tString("payment.rateOutOfRange") : undefined
            }
            endContent={<span className="text-sm text-ink-500">%</span>}
          />
        </div>

        <div className="space-y-3 rounded-2xl border border-warm-200 bg-warm-50/70 p-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-semibold text-ink-950">
                {tString("payment.taxInclusiveTitle")}
              </p>
              <p className="text-xs text-ink-600">
                {tString("payment.taxInclusiveDescription")}
              </p>
            </div>
            <Switch
              isSelected={profile.tax_inclusive}
              onValueChange={(value) => handleInputChange("tax_inclusive", value)}
            />
          </div>

          <div className="border-t border-warm-200" />

          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-semibold text-ink-950">
                {tString("payment.serviceFeeInclusiveTitle")}
              </p>
              <p className="text-xs text-ink-600">
                {tString("payment.serviceFeeInclusiveDescription")}
              </p>
            </div>
            <Switch
              isSelected={profile.service_inclusive}
              onValueChange={(value) =>
                handleInputChange("service_inclusive", value)
              }
            />
          </div>
        </div>

        {/* #266: effective date + live sample $100 check before dinner save */}
        <div className="space-y-4 rounded-2xl border border-warm-200 bg-white/75 p-4 shadow-sm">
          <div className="flex items-start gap-3">
            <IconTile icon={Receipt} />
            <div className="min-w-0 flex-1 space-y-3">
              <div>
                <h4 className="text-sm font-semibold text-ink-950">
                  {tString("payment.sampleCheckTitle")}
                </h4>
                <p className="text-xs text-ink-600 mt-0.5">
                  {tString("payment.sampleCheckDescription").replace(
                    "{{amount}}",
                    formatSampleCheckMoney(SAMPLE_CHECK_SUBTOTAL),
                  )}
                </p>
              </div>

              <div
                className="rounded-xl border border-amber-200/80 bg-amber-50/70 px-3 py-2"
                data-testid="fee-effective-date"
              >
                <p className="text-xs font-semibold uppercase tracking-[0.14em] text-amber-800">
                  {tString("payment.effectiveDateLabel")}
                </p>
                <p className="mt-1 text-sm font-medium text-ink-950">
                  {tString("payment.effectiveDateValue").replace(
                    "{{date}}",
                    effectiveDateLabel,
                  )}
                </p>
                <p className="mt-0.5 text-xs text-ink-600">
                  {tString("payment.effectiveDateDescription")}
                </p>
              </div>

              <dl
                className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4"
                data-testid="sample-check-preview"
              >
                <div className="space-y-0.5">
                  <dt className="text-xs text-ink-600">
                    {tString("payment.sampleCheckSubtotal")}
                  </dt>
                  <dd className="font-title text-base font-semibold text-ink-950">
                    {formatSampleCheckMoney(samplePreview.subtotal)}
                  </dd>
                </div>
                <div className="space-y-0.5">
                  <dt className="text-xs text-ink-600">
                    {tString("payment.sampleCheckTax")}{" "}
                    <span className="text-ink-400">
                      ({formatRatePercent(profile.tax_rate)})
                    </span>
                  </dt>
                  <dd className="font-title text-base font-semibold text-ink-950">
                    {formatSampleCheckMoney(samplePreview.tax)}
                  </dd>
                </div>
                <div className="space-y-0.5">
                  <dt className="text-xs text-ink-600">
                    {tString("payment.sampleCheckServiceFee")}{" "}
                    <span className="text-ink-400">
                      ({formatRatePercent(profile.service_fee_rate)})
                    </span>
                  </dt>
                  <dd className="font-title text-base font-semibold text-ink-950">
                    {formatSampleCheckMoney(samplePreview.serviceFee)}
                  </dd>
                </div>
                <div className="space-y-0.5">
                  <dt className="text-xs text-ink-600">
                    {tString("payment.sampleCheckTotal")}
                  </dt>
                  <dd className="font-title text-base font-semibold text-brand">
                    {formatSampleCheckMoney(samplePreview.total)}
                  </dd>
                </div>
              </dl>

              {(profile.tax_inclusive || profile.service_inclusive) && (
                <p className="text-xs text-ink-500">
                  {tString("payment.sampleCheckInclusiveNote")}
                </p>
              )}

              <p className="text-xs text-ink-500">
                {tString("payment.effectiveDateOpenBillsNote")}
              </p>
            </div>
          </div>

          <div className="border-t border-warm-200 pt-3">
            <p className="mb-3 text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
              {tString("payment.feeSummaryTitle")}
            </p>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
              <div className="space-y-0.5">
                <p className="text-xs text-ink-600">
                  {tString("payment.serviceFeeRate")}
                </p>
                <p className="font-title text-base font-semibold tracking-0 text-ink-950">
                  {formatRatePercent(profile.service_fee_rate)}
                </p>
              </div>
              <div className="space-y-0.5">
                <p className="text-xs text-ink-600">
                  {tString("payment.taxRate")}
                </p>
                <p className="font-title text-base font-semibold tracking-0 text-ink-950">
                  {formatRatePercent(profile.tax_rate)}
                </p>
              </div>
              <div className="space-y-0.5">
                <p className="text-xs text-ink-600">
                  {tString("payment.taxInclusive")}
                </p>
                <Chip
                  size="sm"
                  variant="flat"
                  color={profile.tax_inclusive ? "success" : "default"}
                >
                  {profile.tax_inclusive
                    ? getTranslation("common.yes", currentLocale)
                    : getTranslation("common.no", currentLocale)}
                </Chip>
              </div>
              <div className="space-y-0.5">
                <p className="text-xs text-ink-600">
                  {tString("payment.serviceInclusive")}
                </p>
                <Chip
                  size="sm"
                  variant="flat"
                  color={profile.service_inclusive ? "success" : "default"}
                >
                  {profile.service_inclusive
                    ? getTranslation("common.yes", currentLocale)
                    : getTranslation("common.no", currentLocale)}
                </Chip>
              </div>
            </div>
          </div>
        </div>
      </div>

      <Divider className="my-6" />

      {/* Optional crypto payout wallets — never the door-lead */}
      <div className="space-y-6">
        <div className="flex items-center gap-3 mb-2">
          <IconTile icon={CreditCard} />
          <div>
            <h3 className="text-lg font-semibold text-ink-950">
              {tString("payment.cryptoAddressesTitle")}
            </h3>
            <p className="text-sm text-ink-600">
              {tString("payment.payoutNotice")}
            </p>
          </div>
        </div>

        {sameWalletInvalid ? (
          <div
            role="alert"
            className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800"
          >
            {tString("payment.tipWalletMustDiffer")}
          </div>
        ) : null}

        <Input
          label={tString("payment.settlementAddressLabel")}
          placeholder={tString("payment.settlementAddressPlaceholder")}
          value={profile.settlement_address}
          onValueChange={(value) =>
            handleInputChange("settlement_address", value)
          }
          description={
            walletFieldsDisabled
              ? tString("payment.walletOwnerOnly")
              : tString("payment.settlementAddressDescription")
          }
          variant="bordered"
          isDisabled={walletFieldsDisabled}
          isInvalid={settlementInvalid || sameWalletInvalid}
          errorMessage={
            settlementInvalid
              ? tString("payment.addressInvalid")
              : sameWalletInvalid
                ? tString("payment.tipWalletMustDiffer")
                : undefined
          }
          endContent={
            <Button
              isIconOnly
              size="sm"
              variant="light"
              className="h-8 min-w-8 w-8 rounded-full text-ink-600 hover:bg-brand/10 hover:text-brand"
              aria-label={tString("payment.copySettlementAddress")}
              isDisabled={!profile.settlement_address}
              onPress={() =>
                copyToClipboard(
                  profile.settlement_address,
                  tString("payment.settlementAddressLabel"),
                )
              }
            >
              <Copy className="w-4 h-4" />
            </Button>
          }
        />

        <Input
          label={tString("payment.tippingAddressLabel")}
          placeholder={tString("payment.tippingAddressPlaceholder")}
          value={profile.tipping_address}
          onValueChange={(value) => handleInputChange("tipping_address", value)}
          description={
            walletFieldsDisabled
              ? tString("payment.walletOwnerOnly")
              : tString("payment.tippingAddressDescription")
          }
          variant="bordered"
          isDisabled={walletFieldsDisabled}
          isInvalid={tippingInvalid || sameWalletInvalid}
          errorMessage={
            tippingInvalid
              ? tString("payment.addressInvalid")
              : sameWalletInvalid
                ? tString("payment.tipWalletMustDiffer")
                : undefined
          }
          endContent={
            <Button
              isIconOnly
              size="sm"
              variant="light"
              className="h-8 min-w-8 w-8 rounded-full text-ink-600 hover:bg-brand/10 hover:text-brand"
              aria-label={tString("payment.copyTippingAddress")}
              isDisabled={!profile.tipping_address}
              onPress={() =>
                copyToClipboard(
                  profile.tipping_address,
                  tString("payment.tippingAddressLabel"),
                )
              }
            >
              <Copy className="w-4 h-4" />
            </Button>
          }
        />
      </div>
    </div>
  );
}

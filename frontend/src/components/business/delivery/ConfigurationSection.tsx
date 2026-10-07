"use client";

import React, { useEffect, useState } from "react";
import {
  Card,
  CardHeader,
  CardBody,
  Input,
  RadioGroup,
  Radio,
} from "@nextui-org/react";
import { Settings, AlertTriangle } from "lucide-react";
import type { DeliveryOrder, DeliverySettingsDto } from "@/api/delivery";
import { asDollars } from "@/types/money";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { deriveCurrencyPrefix } from "./currencyPrefix";

interface ConfigurationSectionProps {
  settings: Pick<
    DeliverySettingsDto,
    | "flat_delivery_fee"
    | "free_delivery_minimum"
    | "minimum_order_amount"
    | "estimated_prep_time"
    | "max_concurrent_deliveries"
    | "payment_mode"
    | "online_payment_available"
  >;
  onChange: <
    K extends
      | "flat_delivery_fee"
      | "free_delivery_minimum"
      | "minimum_order_amount"
      | "estimated_prep_time"
      | "max_concurrent_deliveries"
      | "payment_mode"
  >(
    key: K,
    value: DeliverySettingsDto[K],
  ) => void;
  tString: (key: string, params?: Record<string, string | number>) => string;
  // ISO currency code (USD, AED, EUR). Used to derive the input
  // prefix so an AED business doesn't see "$" on its delivery fees.
  currency?: string;
  /**
   * In-flight (non-terminal) delivery orders, used to flag orders that were
   * placed under a payment mode differing from the current setting
   * (payment_mode_stored follow-up, DEL-OP-2). Orders with an empty/missing
   * stored mode predate the column and are treated as unknown — no signal.
   */
  inFlightOrders?: Array<Pick<DeliveryOrder, "payment_mode_stored">>;
}

function parseIntInput(raw: string): number {
  const n = parseInt(raw, 10);
  return isNaN(n) ? 0 : n;
}

export function ConfigurationSection({
  settings,
  onChange,
  tString,
  currency = "USD",
  inFlightOrders,
}: ConfigurationSectionProps) {
  const currencyPrefix = deriveCurrencyPrefix(currency, "symbol");
  const onlinePaymentAvailable = settings.online_payment_available;

  // A3b: string state for money so "12," / "12.5" survive mid-keystroke.
  const [baseFeeText, setBaseFeeText] = useState(() =>
    String(settings.flat_delivery_fee ?? 0),
  );
  const [freeAboveText, setFreeAboveText] = useState(() =>
    String(settings.free_delivery_minimum ?? 0),
  );
  const [minimumOrderText, setMinimumOrderText] = useState(() =>
    String(settings.minimum_order_amount ?? 0),
  );
  useEffect(() => {
    setBaseFeeText(String(settings.flat_delivery_fee ?? 0));
  }, [settings.flat_delivery_fee]);
  useEffect(() => {
    setFreeAboveText(String(settings.free_delivery_minimum ?? 0));
  }, [settings.free_delivery_minimum]);
  useEffect(() => {
    setMinimumOrderText(String(settings.minimum_order_amount ?? 0));
  }, [settings.minimum_order_amount]);

  const moneyInvalid = (text: string) =>
    text.trim() !== "" && tryParseLocaleDecimal(text) === null;
  const moneyError = tString("focused.configuration.invalidMoney");

  // Mismatch: saved mode is online but payment isn't available
  const showMismatchBanner =
    settings.payment_mode === "online" && !onlinePaymentAvailable;

  // In-flight mismatch: orders created under a different (known) payment
  // mode than the one currently selected. Empty stored mode = unknown
  // (legacy rows) and never counts as a mismatch.
  const inFlightMismatchCount = (inFlightOrders ?? []).filter(
    (o) =>
      !!o.payment_mode_stored &&
      o.payment_mode_stored !== settings.payment_mode,
  ).length;

  return (
    <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
      <CardHeader className="flex gap-3">
        <Settings className="h-5 w-5 text-brand" />
        <div>
          <p className="font-semibold text-ink-950">
            {tString("focused.configuration.cardTitle")}
          </p>
          <p className="text-sm text-ink-600">
            {tString("focused.configuration.cardSubtitle")}
          </p>
        </div>
      </CardHeader>
      <CardBody className="space-y-4">
        {/* How guests pay */}
        {showMismatchBanner && (
          <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
            <AlertTriangle className="mt-0.5 w-4 h-4 shrink-0 text-amber-600" />
            <span>{tString("focused.configuration.paymentModeMismatch")}</span>
          </div>
        )}

        {inFlightMismatchCount > 0 && (
          <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
            <AlertTriangle className="mt-0.5 w-4 h-4 shrink-0 text-amber-600" />
            <span>
              {tString("focused.configuration.paymentModeInFlightMismatch", {
                count: inFlightMismatchCount,
              })}
            </span>
          </div>
        )}

        <div className="space-y-2">
          <p className="text-sm font-semibold text-ink-950">
            {tString("focused.configuration.paymentModeTitle")}
          </p>
          <RadioGroup
            value={settings.payment_mode}
            onValueChange={(v) =>
              onChange("payment_mode", v as "online" | "cash_on_delivery")
            }
          >
            <Radio
              value="online"
              isDisabled={!onlinePaymentAvailable}
              description={
                onlinePaymentAvailable
                  ? tString("focused.configuration.paymentModeOnlineHelp")
                  : tString("focused.configuration.paymentModeOnlineDisabled")
              }
            >
              {tString("focused.configuration.paymentModeOnline")}
            </Radio>
            <Radio
              value="cash_on_delivery"
              description={tString("focused.configuration.paymentModeCODHelp")}
            >
              {tString("focused.configuration.paymentModeCOD")}
            </Radio>
          </RadioGroup>
        </div>

        {/* Fee + minimum inputs — A3b DecimalInput (string state, blur parse) */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <DecimalInput
            label={tString("focused.configuration.baseFee")}
            min={0}
            step={0.01}
            maxFractionDigits={2}
            value={baseFeeText}
            onValueChange={setBaseFeeText}
            onParsedChange={(n) => {
              if (n === null) return;
              onChange("flat_delivery_fee", asDollars(n));
            }}
            isInvalid={moneyInvalid(baseFeeText)}
            errorMessage={
              moneyInvalid(baseFeeText) ? moneyError : undefined
            }
            startContent={
              <span className="pointer-events-none text-sm text-ink-700">{currencyPrefix}</span>
            }
            variant="bordered"
            data-testid="delivery-base-fee"
          />

          <DecimalInput
            label={tString("focused.configuration.freeAboveMinimum")}
            description={tString("focused.configuration.freeAboveHelp")}
            min={0}
            step={0.01}
            maxFractionDigits={2}
            value={freeAboveText}
            onValueChange={setFreeAboveText}
            onParsedChange={(n) => {
              if (n === null) return;
              onChange("free_delivery_minimum", asDollars(n));
            }}
            isInvalid={moneyInvalid(freeAboveText)}
            errorMessage={
              moneyInvalid(freeAboveText) ? moneyError : undefined
            }
            startContent={
              <span className="pointer-events-none text-sm text-ink-700">{currencyPrefix}</span>
            }
            variant="bordered"
            data-testid="delivery-free-above"
          />

          <DecimalInput
            label={tString("focused.configuration.minimumOrder")}
            min={0}
            step={0.01}
            maxFractionDigits={2}
            value={minimumOrderText}
            onValueChange={setMinimumOrderText}
            onParsedChange={(n) => {
              if (n === null) return;
              onChange("minimum_order_amount", asDollars(n));
            }}
            isInvalid={moneyInvalid(minimumOrderText)}
            errorMessage={
              moneyInvalid(minimumOrderText) ? moneyError : undefined
            }
            startContent={
              <span className="pointer-events-none text-sm text-ink-700">{currencyPrefix}</span>
            }
            variant="bordered"
            data-testid="delivery-minimum-order"
          />
        </div>

        {/* Timing + capacity */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <Input
            label={tString("focused.configuration.estimatedPrep")}
            type="number"
            min="0"
            step="1"
            value={String(settings.estimated_prep_time ?? 0)}
            onChange={(e) =>
              onChange("estimated_prep_time", parseIntInput(e.target.value))
            }
            endContent={
              <span className="pointer-events-none text-sm text-ink-700">
                {tString("focused.configuration.minutes")}
              </span>
            }
            variant="bordered"
          />

          <Input
            label={tString("focused.configuration.maxConcurrent")}
            type="number"
            min="0"
            step="1"
            value={String(settings.max_concurrent_deliveries ?? 0)}
            onChange={(e) =>
              onChange(
                "max_concurrent_deliveries",
                parseIntInput(e.target.value),
              )
            }
            variant="bordered"
          />
        </div>
      </CardBody>
    </Card>
  );
}

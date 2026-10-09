import React, { useEffect, useMemo, useState } from "react";
import { Input, Switch } from "@nextui-org/react";
import type { DeliveryZoneDto } from "@/api/delivery";
import { asDollars } from "@/types/money";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { canActivateZone, zoneActivationBlockReason } from "./zoneActivation";

interface ZoneEditorProps {
  zone: DeliveryZoneDto;
  onChange: (zone: DeliveryZoneDto) => void;
  tString: (key: string) => string;
  // Pre-derived currency prefix ("$", "€", "AED"). Passed in by the
  // parent so we don't re-derive it per zone row.
  currencyPrefix?: string;
}

function parseIntInput(raw: string): number {
  const n = parseInt(raw, 10);
  return isNaN(n) ? 0 : n;
}

function getStringArray(
  boundaries: Record<string, string[]> | undefined,
  key: string,
): string[] {
  return boundaries?.[key] ?? [];
}

function arrayToText(arr: string[]): string {
  return arr.join(", ");
}

function textToArray(text: string): string[] {
  return text
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

export function ZoneEditor({
  zone,
  onChange,
  tString,
  currencyPrefix = "$",
}: ZoneEditorProps) {
  const patch = (partial: Partial<DeliveryZoneDto>) => {
    onChange({ ...zone, ...partial });
  };

  const postalCodes = useMemo(
    () => getStringArray(zone.boundaries, "postal_codes"),
    [zone.boundaries],
  );
  const cities = useMemo(
    () => getStringArray(zone.boundaries, "cities"),
    [zone.boundaries],
  );

  // Keep raw text while typing so commas survive; normalize to the array on
  // blur (or when the parent zone identity changes).
  const [postalText, setPostalText] = useState(() => arrayToText(postalCodes));
  const [citiesText, setCitiesText] = useState(() => arrayToText(cities));
  // A3b: money fields as string state so "12." / "12,5" survive mid-edit
  // (parent zone.delivery_fee is a number — String(n) on change destroyed seps).
  const [feeText, setFeeText] = useState(() =>
    String(zone.delivery_fee ?? 0),
  );
  const [minimumText, setMinimumText] = useState(() =>
    String(zone.minimum_order_amount ?? 0),
  );
  // L3-40: confirm before activating; block activation when invalid.
  const [activateConfirmOpen, setActivateConfirmOpen] = useState(false);
  const [activationError, setActivationError] = useState<string | null>(null);

  useEffect(() => {
    setPostalText(arrayToText(postalCodes));
  }, [zone.id, postalCodes]);

  useEffect(() => {
    setCitiesText(arrayToText(cities));
  }, [zone.id, cities]);

  useEffect(() => {
    setFeeText(String(zone.delivery_fee ?? 0));
    setMinimumText(String(zone.minimum_order_amount ?? 0));
  }, [zone.id, zone.delivery_fee, zone.minimum_order_amount]);

  const commitPostal = () => {
    patch({
      boundaries: {
        ...(zone.boundaries ?? {}),
        postal_codes: textToArray(postalText),
      },
    });
  };

  const commitCities = () => {
    patch({
      boundaries: {
        ...(zone.boundaries ?? {}),
        cities: textToArray(citiesText),
      },
    });
  };

  const requestActiveChange = (checked: boolean) => {
    setActivationError(null);
    if (!checked) {
      patch({ is_active: false });
      return;
    }
    // Build the zone as the operator currently sees it (incl. uncommitted text).
    const candidate: DeliveryZoneDto = {
      ...zone,
      boundaries: {
        ...(zone.boundaries ?? {}),
        postal_codes: textToArray(postalText),
        cities: textToArray(citiesText),
      },
    };
    const block = zoneActivationBlockReason(candidate);
    if (block === "name") {
      setActivationError(tString("focused.zones.activationNeedsName"));
      return;
    }
    if (block === "geography" || !canActivateZone(candidate)) {
      setActivationError(tString("focused.zones.activationNeedsGeography"));
      return;
    }
    setActivateConfirmOpen(true);
  };

  const confirmActivate = () => {
    patch({
      is_active: true,
      boundaries: {
        ...(zone.boundaries ?? {}),
        postal_codes: textToArray(postalText),
        cities: textToArray(citiesText),
      },
    });
    setActivateConfirmOpen(false);
  };

  const feeInvalid =
    feeText.trim() !== "" && tryParseLocaleDecimal(feeText) === null;
  const minimumInvalid =
    minimumText.trim() !== "" && tryParseLocaleDecimal(minimumText) === null;
  const moneyError = tString("focused.zones.invalidMoney");

  return (
    <div className="space-y-3 pt-2">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Input
          label={tString("focused.zones.name")}
          placeholder={tString("focused.zones.namePlaceholder")}
          value={zone.name}
          onValueChange={(v) => patch({ name: v })}
          variant="bordered"
        />
        <Input
          label={tString("focused.zones.priority")}
          type="number"
          min="1"
          step="1"
          value={String(zone.priority)}
          onChange={(e) => patch({ priority: parseIntInput(e.target.value) })}
          variant="bordered"
        />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <DecimalInput
          label={tString("focused.zones.fee")}
          min={0}
          step={0.01}
          maxFractionDigits={2}
          value={feeText}
          onValueChange={setFeeText}
          onParsedChange={(n) => {
            if (n === null) return;
            patch({ delivery_fee: asDollars(n) });
          }}
          isInvalid={feeInvalid}
          errorMessage={feeInvalid ? moneyError : undefined}
          startContent={
            <span className="pointer-events-none text-sm text-ink-700">{currencyPrefix}</span>
          }
          variant="bordered"
          data-testid="zone-delivery-fee"
        />
        <DecimalInput
          label={tString("focused.zones.minimum")}
          min={0}
          step={0.01}
          maxFractionDigits={2}
          value={minimumText}
          onValueChange={setMinimumText}
          onParsedChange={(n) => {
            if (n === null) return;
            patch({ minimum_order_amount: asDollars(n) });
          }}
          isInvalid={minimumInvalid}
          errorMessage={minimumInvalid ? moneyError : undefined}
          startContent={
            <span className="pointer-events-none text-sm text-ink-700">{currencyPrefix}</span>
          }
          variant="bordered"
          data-testid="zone-minimum-order"
        />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Input
          label={tString("focused.zones.estimatedTime")}
          type="number"
          min="0"
          step="1"
          value={String(zone.estimated_time ?? 0)}
          onChange={(e) =>
            patch({ estimated_time: parseIntInput(e.target.value) })
          }
          variant="bordered"
        />
        <Input
          label={tString("focused.zones.cutoffBuffer")}
          type="number"
          min="0"
          step="1"
          value={String(zone.cutoff_buffer_minutes ?? 0)}
          onChange={(e) =>
            patch({ cutoff_buffer_minutes: parseIntInput(e.target.value) })
          }
          variant="bordered"
        />
      </div>

      <Input
        label={tString("focused.zones.postalCodes")}
        placeholder={tString("focused.zones.postalCodesHelp")}
        value={postalText}
        onValueChange={setPostalText}
        onBlur={commitPostal}
        variant="bordered"
        data-testid="zone-postal-codes"
      />

      <Input
        label={tString("focused.zones.cities")}
        placeholder={tString("focused.zones.citiesHelp")}
        value={citiesText}
        onValueChange={setCitiesText}
        onBlur={commitCities}
        variant="bordered"
        data-testid="zone-cities"
      />

      <div className="flex items-center justify-between rounded-2xl border border-warm-200 bg-white p-3">
        <p className="text-sm font-semibold text-ink-950">
          {zone.is_active
            ? tString("focused.zones.active")
            : tString("focused.zones.inactive")}
        </p>
        <Switch
          isSelected={zone.is_active}
          onValueChange={requestActiveChange}
          aria-label={tString("focused.zones.active")}
          data-testid="zone-active-switch"
          classNames={{
            wrapper: "group-data-[selected=true]:bg-brand",
          }}
        />
      </div>
      {activationError ? (
        <p
          className="text-xs text-rose-700"
          role="alert"
          data-testid="zone-activation-error"
        >
          {activationError}
        </p>
      ) : null}
      <ConfirmationModal
        isOpen={activateConfirmOpen}
        onOpenChange={() => setActivateConfirmOpen(false)}
        // getTranslation never returns falsy (it falls back to the
        // sentence-cased key leaf), so `|| "English literal"` here was dead
        // code that hid the missing keys — resolve the real strings instead.
        title={tString("focused.zones.activateConfirmTitle")}
        description={tString("focused.zones.activateConfirmBody")}
        confirmLabel={tString("focused.zones.activateConfirm")}
        cancelLabel={tString("focused.zones.cancel")}
        onConfirm={confirmActivate}
      />
    </div>
  );
}

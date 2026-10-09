import React from "react";
import { Button, Chip, Switch } from "@nextui-org/react";
import { Car, Edit2, Trash2 } from "lucide-react";
import type { DeliveryDriver } from "@/api/delivery";

const STATUS_CLASS_MAP: Record<string, string> = {
  online: "border border-emerald-200 bg-emerald-50 text-emerald-700",
  busy: "border border-amber-200 bg-amber-50 text-amber-700",
  offline: "border border-warm-200 bg-warm-100 text-ink-600",
  on_break: "border border-amber-200 bg-amber-50 text-amber-700",
};

interface DriverRowProps {
  driver: DeliveryDriver;
  onEdit: (driver: DeliveryDriver) => void;
  onRemove: (driver: DeliveryDriver) => void;
  onToggleAvailability: (driver: DeliveryDriver, next: boolean) => Promise<void>;
  /** When true the availability switch is disabled (caller is tracking inflight toggle) */
  isToggling?: boolean;
  tString: (key: string) => string;
}

export function DriverRow({
  driver,
  onEdit,
  onRemove,
  onToggleAvailability,
  isToggling = false,
  tString,
}: DriverRowProps) {
  const statusKey = driver.status as string;
  const statusLabel =
    tString(`drivers.status.${statusKey}`) || statusKey;

  const vehicleLabel = driver.vehicle_type
    ? tString(`drivers.vehicleTypes.${driver.vehicle_type}`) || driver.vehicle_type
    : null;
  const vehicleDisplay = [vehicleLabel, driver.vehicle_plate]
    .filter(Boolean)
    .join(" • ");

  const handleAvailability = async (next: boolean) => {
    if (isToggling) return; // guard: ignore while caller's API call is in-flight
    await onToggleAvailability(driver, next);
  };

  return (
    <div
      className="flex flex-col gap-3 rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5 sm:flex-row sm:items-center"
      role="listitem"
    >
      {/* Left: icon + details */}
      <div className="flex items-center gap-3 flex-1 min-w-0">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-warm-200 bg-warm-100">
          <Car size={16} className="text-ink-700" />
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="truncate text-sm font-semibold text-ink-950">
              {driver.name}
            </span>
            {!driver.is_active && (
              <Chip
                size="sm"
                variant="flat"
                className="border border-rose-200 bg-rose-50 text-rose-700"
              >
                {tString("drivers.inactive")}
              </Chip>
            )}
          </div>
          <p className="truncate text-xs text-ink-600">{driver.phone}</p>
          {vehicleDisplay && (
            <p className="truncate text-xs text-ink-500">{vehicleDisplay}</p>
          )}
        </div>
      </div>

      {/* Middle: status chip */}
      <div className="flex items-center gap-2">
        <Chip
          size="sm"
          variant="flat"
          className={STATUS_CLASS_MAP[statusKey] ?? "border border-warm-200 bg-warm-100 text-ink-600"}
        >
          {statusLabel}
        </Chip>
      </div>

      {/* Right: availability toggle + actions */}
      <div className="flex items-center gap-3 shrink-0">
        <Switch
          size="sm"
          isSelected={driver.is_available}
          isDisabled={isToggling || !driver.is_active}
          onValueChange={handleAvailability}
          aria-label={`${tString("drivers.availability")} — ${driver.name}`}
          classNames={{
            wrapper: "group-data-[selected=true]:bg-brand",
          }}
        >
          <span className="text-xs text-ink-600">
            {tString("drivers.availability")}
          </span>
        </Switch>
        <Button
          isIconOnly
          size="sm"
          variant="light"
          onPress={() => onEdit(driver)}
          aria-label={`${tString("drivers.edit")} ${driver.name}`}
          className="text-ink-600 hover:bg-warm-100 hover:text-ink-950"
        >
          <Edit2 size={14} aria-hidden="true" />
        </Button>
        <Button
          isIconOnly
          size="sm"
          variant="light"
          onPress={() => onRemove(driver)}
          aria-label={`${tString("drivers.remove")} ${driver.name}`}
          className="text-rose-600 hover:bg-rose-50"
        >
          <Trash2 size={14} aria-hidden="true" />
        </Button>
      </div>
    </div>
  );
}

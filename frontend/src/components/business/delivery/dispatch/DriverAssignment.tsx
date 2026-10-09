import React, { useState } from "react";
import { Button, Select, SelectItem } from "@nextui-org/react";
import type { DeliveryDriver } from "@/api/delivery";
import { btnPrimaryNextUI } from "@/components/ui/buttonStyles";

interface DriverAssignmentProps {
  drivers: DeliveryDriver[];
  onAssign: (driverId: number) => Promise<void>;
  tString: (key: string) => string;
  disabled?: boolean;
}

export function DriverAssignment({
  drivers,
  onAssign,
  tString,
  disabled,
}: DriverAssignmentProps) {
  const [selectedDriverId, setSelectedDriverId] = useState<string>("");
  const [loading, setLoading] = useState(false);

  const handleAssign = async () => {
    if (!selectedDriverId) return;
    setLoading(true);
    try {
      await onAssign(Number(selectedDriverId));
      setSelectedDriverId("");
    } catch {
      // The parent (DispatchConsole.handleAssignDriver) already surfaced an
      // error toast. Keep the selection so the operator can retry, and don't
      // let the rejection escape the press handler as an unhandled rejection.
    } finally {
      setLoading(false);
    }
  };

  const hasDrivers = drivers.length > 0;

  return (
    <div className="flex items-center gap-2">
      <Select
        size="sm"
        placeholder={tString("dispatch.actions.selectDriver")}
        selectedKeys={selectedDriverId ? [selectedDriverId] : []}
        onSelectionChange={(keys) => {
          const key = Array.from(keys)[0];
          setSelectedDriverId(key ? String(key) : "");
        }}
        className="min-w-[160px]"
        // With no drivers there is nothing to pick — disable the control and
        // surface an explanatory item so the operator isn't left with a dead,
        // empty dropdown (audit L6 #9).
        isDisabled={disabled || loading || !hasDrivers}
        aria-label={tString("dispatch.actions.assignDriver")}
        disabledKeys={hasDrivers ? [] : ["__no_drivers__"]}
      >
        {hasDrivers ? (
          drivers.map((driver) => (
            <SelectItem key={String(driver.id)} value={String(driver.id)}>
              {driver.name}
            </SelectItem>
          ))
        ) : (
          <SelectItem key="__no_drivers__" isReadOnly>
            {tString("dispatch.actions.noDriversAvailable")}
          </SelectItem>
        )}
      </Select>
      <Button
        size="sm"
        radius="full"
        className={btnPrimaryNextUI}
        isDisabled={!selectedDriverId || disabled || loading}
        isLoading={loading}
        onPress={handleAssign}
      >
        {tString("dispatch.actions.assign")}
      </Button>
    </div>
  );
}

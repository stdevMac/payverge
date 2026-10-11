import React from "react";
import { Card, CardHeader, CardBody, Switch, Input } from "@nextui-org/react";
import { Clock } from "lucide-react";
import type { DeliverySettingsDto } from "@/api/delivery";

interface HoursSectionProps {
  settings: Pick<
    DeliverySettingsDto,
    | "delivery_hours_same_as_business"
    | "delivery_start_time"
    | "delivery_end_time"
  >;
  onChange: <K extends "delivery_hours_same_as_business" | "delivery_start_time" | "delivery_end_time">(
    key: K,
    value: DeliverySettingsDto[K],
  ) => void;
  tString: (key: string) => string;
}

export function HoursSection({
  settings,
  onChange,
  tString,
}: HoursSectionProps) {
  return (
    <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
      <CardHeader className="flex gap-3">
        <Clock className="h-5 w-5 text-brand" />
        <div>
          <p className="font-semibold text-ink-950">
            {tString("focused.hours.cardTitle")}
          </p>
          <p className="text-sm text-ink-600">
            {tString("focused.hours.cardSubtitle")}
          </p>
        </div>
      </CardHeader>
      <CardBody className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-semibold text-ink-950">
              {tString("focused.hours.useBusinessHours")}
            </p>
            <p className="mt-0.5 text-xs text-ink-600">
              {tString("focused.hours.useBusinessHoursHelp")}
            </p>
          </div>
          <Switch
            isSelected={settings.delivery_hours_same_as_business}
            onValueChange={(checked) =>
              onChange("delivery_hours_same_as_business", checked)
            }
            aria-label={tString("focused.hours.useBusinessHours")}
            classNames={{
              wrapper: "group-data-[selected=true]:bg-brand",
            }}
          />
        </div>

        {!settings.delivery_hours_same_as_business && (
          <div className="grid grid-cols-2 gap-3 pt-1">
            <Input
              label={tString("focused.hours.startTime")}
              type="time"
              value={settings.delivery_start_time || ""}
              onChange={(e) => onChange("delivery_start_time", e.target.value)}
              variant="bordered"
            />
            <Input
              label={tString("focused.hours.endTime")}
              type="time"
              value={settings.delivery_end_time || ""}
              onChange={(e) => onChange("delivery_end_time", e.target.value)}
              variant="bordered"
            />
          </div>
        )}
      </CardBody>
    </Card>
  );
}

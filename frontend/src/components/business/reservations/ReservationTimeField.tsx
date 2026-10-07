"use client";

import { Input, Radio, RadioGroup } from "@nextui-org/react";

import {
  switchWallTimeZone,
  wallTimeToInstant,
} from "@/utils/zonedDateTime";

export type ReservationTimeEntryMode = "business" | "device";

interface ReservationTimeLabels {
  entryMode: string;
  businessTime: string;
  deviceTime: string;
  dateTime: string;
  conversionPreview: string;
}

interface ReservationTimeFieldProps {
  value: string;
  mode: ReservationTimeEntryMode;
  businessTimeZone: string;
  deviceTimeZone: string;
  labels: ReservationTimeLabels;
  onValueChange: (value: string) => void;
  onModeChange: (mode: ReservationTimeEntryMode) => void;
  /**
   * Floor the picker at today (in the entry zone). Create mode only: edit
   * mode must keep existing past reservation times renderable without the
   * native picker flagging them invalid (L1-1).
   */
  enforceTodayMin?: boolean;
}

function todayFloorWallTime(timeZone: string): string {
  // en-CA renders YYYY-MM-DD, matching the datetime-local wire format.
  const today = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
  return `${today}T00:00`;
}

function formatPreview(instant: Date, timeZone: string): string {
  const formatted = new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone,
  }).format(instant);
  return `${formatted} · ${timeZone}`;
}

export function ReservationTimeField({
  value,
  mode,
  businessTimeZone,
  deviceTimeZone,
  labels,
  onValueChange,
  onModeChange,
  enforceTodayMin,
}: ReservationTimeFieldProps) {
  const entryZone = mode === "business" ? businessTimeZone : deviceTimeZone;
  let instant: Date | null = null;
  if (value) {
    try {
      instant = wallTimeToInstant(value, entryZone);
    } catch {
      instant = null;
    }
  }

  const changeMode = (nextValue: string) => {
    const next = nextValue as ReservationTimeEntryMode;
    if (next === mode) return;
    const nextZone = next === "business" ? businessTimeZone : deviceTimeZone;
    if (value) {
      try {
        onValueChange(switchWallTimeZone(value, entryZone, nextZone));
      } catch {
        // Keep the typed wall time when it cannot be converted; the form can
        // show its localized invalid-gap validation without losing user input.
      }
    }
    onModeChange(next);
  };

  return (
    <div className="space-y-3">
      <RadioGroup
        aria-label={labels.entryMode}
        orientation="horizontal"
        value={mode}
        onValueChange={changeMode}
      >
        <Radio value="business">{labels.businessTime}</Radio>
        <Radio value="device">{labels.deviceTime}</Radio>
      </RadioGroup>
      <Input
        aria-label={labels.dateTime}
        label={labels.dateTime}
        type="datetime-local"
        value={value}
        min={enforceTodayMin ? todayFloorWallTime(entryZone) : undefined}
        onValueChange={onValueChange}
      />
      {instant && businessTimeZone !== deviceTimeZone ? (
        <dl
          aria-label={labels.conversionPreview}
          className="grid gap-2 rounded-xl border border-warm-200 bg-warm-50 p-3 text-sm"
        >
          <div>
            <dt className="font-medium text-ink-700">{labels.businessTime}</dt>
            <dd className="text-ink-950">
              {formatPreview(instant, businessTimeZone)}
            </dd>
          </div>
          <div>
            <dt className="font-medium text-ink-700">{labels.deviceTime}</dt>
            <dd className="text-ink-950">
              {formatPreview(instant, deviceTimeZone)}
            </dd>
          </div>
        </dl>
      ) : null}
    </div>
  );
}

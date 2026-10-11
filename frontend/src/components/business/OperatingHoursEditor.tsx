"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Button, Input, Switch } from "@nextui-org/react";
import { Plus, X } from "lucide-react";
import {
  BusinessOperatingException,
  BusinessOperatingHours,
} from "@/api/business";
import { getDayName } from "@/components/business/operatingHoursDayName";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface OperatingHoursEditorProps {
  businessId: number;
  hours: BusinessOperatingHours[];
  onHoursChange: (hours: BusinessOperatingHours[]) => void;
  exceptions?: BusinessOperatingException[];
  onExceptionsChange?: (exceptions: BusinessOperatingException[]) => void;
}

/** One open window within a day (split shifts: up to two). */
interface TimePeriod {
  open_time: string;
  close_time: string;
  kitchen_close_time?: string;
}

interface DayHours {
  day_of_week: number;
  is_closed: boolean;
  periods: TimePeriod[];
}

const DEFAULT_PERIOD: TimePeriod = { open_time: "09:00", close_time: "17:00" };
const SECOND_PERIOD_DEFAULT: TimePeriod = {
  open_time: "19:00",
  close_time: "23:00",
};
const MAX_PERIODS = 2;

function groupHoursByDay(hours: BusinessOperatingHours[]): DayHours[] {
  const byDay = new Map<number, BusinessOperatingHours[]>();
  for (const h of hours) {
    const list = byDay.get(h.day_of_week) ?? [];
    list.push(h);
    byDay.set(h.day_of_week, list);
  }

  const days: DayHours[] = [];
  for (let day = 0; day < 7; day++) {
    const rows = byDay.get(day) ?? [];
    if (rows.length === 0) {
      days.push({
        day_of_week: day,
        is_closed: false,
        periods: [{ ...DEFAULT_PERIOD }],
      });
      continue;
    }
    const closed = rows.some((r) => r.is_closed);
    if (closed) {
      days.push({ day_of_week: day, is_closed: true, periods: [{ ...DEFAULT_PERIOD }] });
      continue;
    }
    const periods = rows
      .filter((r) => !r.is_closed)
      .map((r) => ({
        open_time: r.open_time || DEFAULT_PERIOD.open_time,
        close_time: r.close_time || DEFAULT_PERIOD.close_time,
        kitchen_close_time: r.kitchen_close_time || undefined,
      }))
      .slice(0, MAX_PERIODS);
    days.push({
      day_of_week: day,
      is_closed: false,
      periods: periods.length > 0 ? periods : [{ ...DEFAULT_PERIOD }],
    });
  }
  return days;
}

function flattenDays(days: DayHours[]): BusinessOperatingHours[] {
  const out: BusinessOperatingHours[] = [];
  for (const day of days) {
    if (day.is_closed) {
      out.push({
        day_of_week: day.day_of_week,
        open_time: day.periods[0]?.open_time || DEFAULT_PERIOD.open_time,
        close_time: day.periods[0]?.close_time || DEFAULT_PERIOD.close_time,
        kitchen_close_time: null,
        is_closed: true,
      } as BusinessOperatingHours);
      continue;
    }
    for (const p of day.periods) {
      out.push({
        day_of_week: day.day_of_week,
        open_time: p.open_time,
        close_time: p.close_time,
        kitchen_close_time: p.kitchen_close_time?.trim()
          ? p.kitchen_close_time.trim()
          : null,
        is_closed: false,
      } as BusinessOperatingHours);
    }
  }
  return out;
}

export default function OperatingHoursEditor({
  businessId: _businessId,
  hours,
  onHoursChange,
  exceptions = [],
  onExceptionsChange,
}: OperatingHoursEditorProps) {
  const [dayHours, setDayHours] = useState<DayHours[]>([]);
  const { locale } = useSimpleLocale();

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessSettings.operatingHoursEditor.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  useEffect(() => {
    setDayHours(groupHoursByDay(hours));
  }, [hours]);

  const propagate = useCallback(
    (next: DayHours[]) => {
      setDayHours(next);
      onHoursChange(flattenDays(next));
    },
    [onHoursChange],
  );

  const updateDayClosed = useCallback(
    (dayOfWeek: number, isClosed: boolean) => {
      const next = dayHours.map((day) =>
        day.day_of_week === dayOfWeek
          ? {
              ...day,
              is_closed: isClosed,
              periods:
                day.periods.length > 0 ? day.periods : [{ ...DEFAULT_PERIOD }],
            }
          : day,
      );
      propagate(next);
    },
    [dayHours, propagate],
  );

  const updatePeriod = useCallback(
    (
      dayOfWeek: number,
      periodIndex: number,
      field: keyof TimePeriod,
      value: string,
    ) => {
      const next = dayHours.map((day) => {
        if (day.day_of_week !== dayOfWeek) return day;
        const periods = day.periods.map((p, i) =>
          i === periodIndex ? { ...p, [field]: value } : p,
        );
        return { ...day, periods };
      });
      propagate(next);
    },
    [dayHours, propagate],
  );

  const addPeriod = useCallback(
    (dayOfWeek: number) => {
      const next = dayHours.map((day) => {
        if (day.day_of_week !== dayOfWeek) return day;
        if (day.is_closed || day.periods.length >= MAX_PERIODS) return day;
        return {
          ...day,
          periods: [...day.periods, { ...SECOND_PERIOD_DEFAULT }],
        };
      });
      propagate(next);
    },
    [dayHours, propagate],
  );

  const removePeriod = useCallback(
    (dayOfWeek: number, periodIndex: number) => {
      const next = dayHours.map((day) => {
        if (day.day_of_week !== dayOfWeek) return day;
        if (day.periods.length <= 1) return day;
        return {
          ...day,
          periods: day.periods.filter((_, i) => i !== periodIndex),
        };
      });
      propagate(next);
    },
    [dayHours, propagate],
  );

  const copyToAllDays = useCallback(
    (sourceDayOfWeek: number) => {
      const sourceDay = dayHours.find(
        (day) => day.day_of_week === sourceDayOfWeek,
      );
      if (!sourceDay) return;
      const next = dayHours.map((day) => ({
        ...day,
        is_closed: sourceDay.is_closed,
        periods: sourceDay.periods.map((p) => ({ ...p })),
      }));
      propagate(next);
    },
    [dayHours, propagate],
  );

  const applyPreset = useCallback(
    (preset: "business" | "restaurant" | "lunchDinner" | "twentyFourSeven") => {
      const next = dayHours.map((day) => {
        if (preset === "business") {
          return {
            ...day,
            is_closed: day.day_of_week === 0 || day.day_of_week === 6,
            periods: [{ open_time: "09:00", close_time: "17:00" }],
          };
        }
        if (preset === "restaurant") {
          return {
            ...day,
            is_closed: false,
            periods: [
              {
                open_time: "11:00",
                close_time: "23:00",
                kitchen_close_time: "22:00",
              },
            ],
          };
        }
        if (preset === "lunchDinner") {
          return {
            ...day,
            is_closed: false,
            periods: [
              { open_time: "11:00", close_time: "15:00" },
              {
                open_time: "18:00",
                close_time: "23:00",
                kitchen_close_time: "22:00",
              },
            ],
          };
        }
        return {
          ...day,
          is_closed: false,
          periods: [{ open_time: "00:00", close_time: "23:59" }],
        };
      });
      propagate(next);
    },
    [dayHours, propagate],
  );

  const updateException = useCallback(
    (index: number, patch: Partial<BusinessOperatingException>) => {
      if (!onExceptionsChange) return;
      const next = exceptions.map((row, i) =>
        i === index ? { ...row, ...patch } : row,
      );
      onExceptionsChange(next);
    },
    [exceptions, onExceptionsChange],
  );

  const addException = useCallback(() => {
    if (!onExceptionsChange) return;
    const today = new Date();
    const iso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
    onExceptionsChange([
      ...exceptions,
      {
        exception_date: iso,
        is_closed: true,
        label: "",
      } as BusinessOperatingException,
    ]);
  }, [exceptions, onExceptionsChange]);

  const removeException = useCallback(
    (index: number) => {
      if (!onExceptionsChange) return;
      onExceptionsChange(exceptions.filter((_, i) => i !== index));
    },
    [exceptions, onExceptionsChange],
  );

  return (
    <div className="space-y-3">
      <p className="px-1 text-xs text-ink-600">{t("honestyNote")}</p>
      <div className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
        {dayHours.map((day, idx) => (
          <div
            key={day.day_of_week}
            className={`flex flex-col gap-3 px-4 py-3 md:flex-row md:items-start ${
              idx !== dayHours.length - 1 ? "border-b border-warm-100" : ""
            }`}
          >
            <div className="flex items-center justify-between md:w-32 md:flex-shrink-0 md:pt-1.5">
              <p className="text-sm font-semibold text-ink-950">
                {getDayName(day.day_of_week, locale)}
              </p>
              <div className="md:hidden flex items-center gap-2">
                <Switch
                  size="sm"
                  aria-label={`${getDayName(day.day_of_week, locale)} ${t("open")}`}
                  isSelected={!day.is_closed}
                  onValueChange={(isOpen) =>
                    updateDayClosed(day.day_of_week, !isOpen)
                  }
                  classNames={{
                    wrapper: "group-data-[selected=true]:bg-brand",
                  }}
                />
              </div>
            </div>

            <div className="hidden md:flex items-center gap-2 md:w-24 md:pt-1.5">
              <Switch
                size="sm"
                aria-label={`${getDayName(day.day_of_week, locale)} ${t("open")}`}
                isSelected={!day.is_closed}
                onValueChange={(isOpen) =>
                  updateDayClosed(day.day_of_week, !isOpen)
                }
                classNames={{
                  wrapper: "group-data-[selected=true]:bg-brand",
                }}
              />
              <span className="text-xs text-ink-700">
                {day.is_closed ? t("closed") : t("open")}
              </span>
            </div>

            {!day.is_closed ? (
              <div className="flex flex-1 flex-col gap-2">
                {day.periods.map((period, pIdx) => (
                  <div
                    key={`${day.day_of_week}-p${pIdx}`}
                    className="flex flex-col gap-2"
                  >
                    <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-center">
                      <Input
                        type="time"
                        aria-label={t("opens")}
                        value={period.open_time}
                        onValueChange={(value) =>
                          updatePeriod(day.day_of_week, pIdx, "open_time", value)
                        }
                        size="sm"
                        classNames={{
                          base: "w-full sm:w-32",
                          inputWrapper:
                            "h-8 min-h-8 bg-warm-50 border border-warm-200 shadow-none",
                        }}
                      />
                      <span className="hidden text-xs text-ink-400 sm:inline">
                        →
                      </span>
                      <div className="flex w-full items-center gap-1.5 sm:w-auto">
                        <Input
                          type="time"
                          aria-label={t("closes")}
                          value={period.close_time}
                          onValueChange={(value) =>
                            updatePeriod(
                              day.day_of_week,
                              pIdx,
                              "close_time",
                              value,
                            )
                          }
                          size="sm"
                          classNames={{
                            base: "w-full sm:w-32",
                            inputWrapper:
                              "h-8 min-h-8 bg-warm-50 border border-warm-200 shadow-none",
                          }}
                        />
                        {period.close_time <= period.open_time && (
                          <span
                            title={t("nextDayHint")}
                            className="whitespace-nowrap rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800"
                          >
                            +1 {t("nextDay")}
                          </span>
                        )}
                        {day.periods.length > 1 ? (
                          <Button
                            isIconOnly
                            size="sm"
                            variant="light"
                            aria-label={t("removePeriod")}
                            onPress={() => removePeriod(day.day_of_week, pIdx)}
                            className="text-ink-500 hover:bg-warm-100"
                          >
                            <X className="h-3.5 w-3.5" />
                          </Button>
                        ) : null}
                      </div>
                    </div>
                    <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-2">
                      <Input
                        type="time"
                        aria-label={t("kitchenCloses")}
                        label={t("kitchenCloses")}
                        labelPlacement="outside-left"
                        value={period.kitchen_close_time || ""}
                        onValueChange={(value) =>
                          updatePeriod(
                            day.day_of_week,
                            pIdx,
                            "kitchen_close_time",
                            value,
                          )
                        }
                        size="sm"
                        classNames={{
                          base: "w-full sm:w-auto",
                          label: "text-xs text-ink-600 whitespace-nowrap",
                          inputWrapper:
                            "h-8 min-h-8 bg-warm-50 border border-warm-200 shadow-none w-32",
                        }}
                      />
                      <span className="text-[11px] text-ink-500">
                        {t("kitchenClosesHint")}
                      </span>
                    </div>
                  </div>
                ))}
                {day.periods.length < MAX_PERIODS ? (
                  <Button
                    size="sm"
                    variant="light"
                    startContent={<Plus className="h-3.5 w-3.5" />}
                    onPress={() => addPeriod(day.day_of_week)}
                    className="w-fit font-semibold text-brand-dark hover:bg-brand/10"
                    aria-label={t("addSecondPeriod")}
                  >
                    {t("addSecondPeriod")}
                  </Button>
                ) : null}
              </div>
            ) : (
              <span className="flex-1 text-xs italic text-ink-500 md:pt-1.5">
                {t("closedAllDay")}
              </span>
            )}

            <Button
              size="sm"
              variant="light"
              onPress={() => copyToAllDays(day.day_of_week)}
              className="font-semibold text-brand-dark hover:bg-brand/10 md:ml-auto md:pt-1"
            >
              {t("copyToAll")}
            </Button>
          </div>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2 px-1">
        <span className="text-xs font-semibold text-ink-700">
          {t("quickPresets")}
        </span>
        <Button
          size="sm"
          variant="flat"
          onPress={() => applyPreset("business")}
          className="bg-warm-100 font-semibold text-ink-700 hover:bg-warm-200"
        >
          {t("businessHours")}
        </Button>
        <Button
          size="sm"
          variant="flat"
          onPress={() => applyPreset("restaurant")}
          className="bg-warm-100 font-semibold text-ink-700 hover:bg-warm-200"
        >
          {t("restaurantHours")}
        </Button>
        <Button
          size="sm"
          variant="flat"
          onPress={() => applyPreset("lunchDinner")}
          className="bg-warm-100 font-semibold text-ink-700 hover:bg-warm-200"
        >
          {t("lunchDinner")}
        </Button>
        <Button
          size="sm"
          variant="flat"
          onPress={() => applyPreset("twentyFourSeven")}
          className="bg-warm-100 font-semibold text-ink-700 hover:bg-warm-200"
        >
          {t("twentyFourSeven")}
        </Button>
      </div>

      {onExceptionsChange ? (
        <div className="mt-4 space-y-3 rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5">
          <div>
            <h4 className="text-sm font-semibold text-ink-950">
              {t("exceptionsTitle")}
            </h4>
            <p className="mt-1 text-xs text-ink-600">{t("exceptionsHelp")}</p>
          </div>
          {exceptions.length === 0 ? (
            <p className="text-xs italic text-ink-500">{t("noExceptions")}</p>
          ) : (
            <ul className="space-y-2">
              {exceptions.map((row, index) => (
                <li
                  key={`${row.exception_date}-${index}`}
                  className="flex flex-col gap-2 rounded-2xl border border-warm-100 bg-warm-50/70 p-3 sm:flex-row sm:items-center"
                >
                  <Input
                    type="date"
                    aria-label={t("exceptionDate")}
                    value={row.exception_date}
                    onValueChange={(value) =>
                      updateException(index, { exception_date: value })
                    }
                    size="sm"
                    classNames={{
                      base: "w-full sm:w-44",
                      inputWrapper:
                        "h-8 min-h-8 bg-white border border-warm-200 shadow-none",
                    }}
                  />
                  <Input
                    aria-label={t("exceptionLabel")}
                    placeholder={t("exceptionLabelPlaceholder")}
                    value={row.label || ""}
                    onValueChange={(value) =>
                      updateException(index, { label: value })
                    }
                    size="sm"
                    classNames={{
                      base: "w-full flex-1",
                      inputWrapper:
                        "h-8 min-h-8 bg-white border border-warm-200 shadow-none",
                    }}
                  />
                  <Button
                    size="sm"
                    variant="light"
                    aria-label={t("removeException")}
                    onPress={() => removeException(index)}
                    className="font-semibold text-rose-700 hover:bg-rose-50"
                  >
                    {t("removeException")}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <Button
            size="sm"
            variant="flat"
            startContent={<Plus className="h-3.5 w-3.5" />}
            onPress={addException}
            className="bg-warm-100 font-semibold text-ink-700 hover:bg-warm-200"
          >
            {t("addException")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

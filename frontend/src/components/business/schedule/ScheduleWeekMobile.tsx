"use client";

import React, { useMemo, useState } from "react";
import { CalendarOff, CalendarX2, Plus } from "lucide-react";
import type { Shift } from "@/api/schedule";
import type { Position } from "@/api/positions";
import type { StaffMember } from "@/api/staff";
import type { TimeOffRequest } from "@/api/availability";
import { intlLocaleFor } from "@/utils/intlLocale";
import { localDateKey } from "@/lib/localDate";
import { translatePositionName } from "./positionLabel";
import { addShiftAccessibleName } from "./addShiftLabel";
import {
  cellPosture,
  type TeamAvailability,
} from "./scheduleAvailability";

interface MobileRow {
  key: string;
  label: string;
  staffId?: number | null;
  positionId?: number;
  colorHex?: string;
}

export interface ScheduleWeekMobileProps {
  days: string[];
  /** Venue-local YYYY-MM-DD for "today"; defaults to the device calendar day. */
  todayKey?: string;
  rows: MobileRow[];
  mode: "role" | "employee";
  locale: string;
  t: (key: string) => string;
  shiftsForCell: (row: MobileRow, dayKey: string) => Shift[];
  positionsById: Map<number, Position>;
  staffById: Map<number, StaffMember>;
  teamAvailability: TeamAvailability;
  approvedTimeOff: TimeOffRequest[];
  dayTotalMinutes: Map<string, number>;
  dayCoverage?: Map<string, { lunchMin: number; dinnerMin: number }>;
  formatTimeRange: (startIso: string, endIso: string, locale: string) => string;
  fmtMinutes: (total: number) => string;
  onEditShift: (shift: Shift) => void;
  onCreateShift: (args: {
    dayKey: string;
    staffId: number | null;
    positionId: number | null;
  }) => void;
}

/**
 * Phone-sized view of the schedule week. The desktop grid is a 7-column table
 * that needs ~860px; below md that shows barely one day. This renders a day
 * switcher (chips, defaulting to today when today is in the week) and a single
 * day's shifts as a vertical list, using the SAME data + edit/create actions
 * as the grid. The parent owns all data logic — this is presentational.
 */
export default function ScheduleWeekMobile({
  days,
  todayKey: todayKeyProp,
  rows,
  mode,
  locale,
  t,
  shiftsForCell,
  positionsById,
  staffById,
  teamAvailability,
  approvedTimeOff,
  dayTotalMinutes,
  dayCoverage,
  formatTimeRange,
  fmtMinutes,
  onEditShift,
  onCreateShift,
}: ScheduleWeekMobileProps) {
  const todayKey = todayKeyProp ?? localDateKey(new Date());
  const defaultDay = days.includes(todayKey) ? todayKey : days[0];
  const [selectedDay, setSelectedDay] = useState<string>(defaultDay);
  // If the week changes under us, keep the selection valid.
  const activeDay = days.includes(selectedDay) ? selectedDay : days[0];

  const intl = intlLocaleFor(locale);
  const dayMins = dayTotalMinutes.get(activeDay) ?? 0;

  const chips = useMemo(
    () =>
      days.map((dayKey) => {
        const [y, m, d] = dayKey.split("-").map(Number);
        const date = new Date(y, m - 1, d);
        return {
          dayKey,
          weekday: new Intl.DateTimeFormat(intl, { weekday: "short" }).format(
            date,
          ),
          dayNum: new Intl.DateTimeFormat(intl, { day: "numeric" }).format(date),
          isToday: dayKey === todayKey,
        };
      }),
    [days, intl, todayKey],
  );

  return (
    <div className="md:hidden">
      {/* Day switcher */}
      <div
        role="tablist"
        aria-label={t("dayTotals")}
        className="flex gap-1.5 overflow-x-auto pb-2 scrollbar-thin scrollbar-thumb-warm-300"
      >
        {chips.map((chip) => {
          const isActive = chip.dayKey === activeDay;
          return (
            <button
              key={chip.dayKey}
              type="button"
              role="tab"
              aria-selected={isActive}
              aria-current={chip.isToday ? "date" : undefined}
              onClick={() => setSelectedDay(chip.dayKey)}
              className={`flex min-w-[52px] flex-col items-center rounded-xl border px-2.5 py-1.5 text-center transition ${
                isActive
                  ? "border-brand bg-brand/10 text-brand"
                  : chip.isToday
                    ? "border-brand/30 bg-white text-ink-700"
                    : "border-warm-200 bg-white text-ink-600"
              }`}
            >
              <span className="text-[11px] font-semibold uppercase leading-none">
                {chip.weekday}
              </span>
              <span className="mt-1 text-sm font-semibold tabular-nums leading-none">
                {chip.dayNum}
              </span>
            </button>
          );
        })}
      </div>

      {/* Selected day total */}
      <div className="mb-2 flex items-center justify-between px-0.5 text-[11px] text-ink-500">
        <span className="font-medium text-ink-600">
          {new Intl.DateTimeFormat(intl, {
            weekday: "long",
            day: "numeric",
            month: "short",
          }).format(
            (() => {
              const [y, m, d] = activeDay.split("-").map(Number);
              return new Date(y, m - 1, d);
            })(),
          )}
        </span>
        <span className="tabular-nums">
          {dayMins > 0 ? fmtMinutes(dayMins) : "—"}
        </span>
      </div>
      {dayCoverage ? (
        <p
          className="mb-2 flex gap-3 px-0.5 text-[10px] font-medium text-ink-500"
          data-testid={`schedule-coverage-${activeDay}`}
        >
          <span
            className={
              (dayCoverage.get(activeDay)?.lunchMin ?? 0) === 0
                ? "text-ink-400"
                : ""
            }
          >
            {t("coverage.lunch").replace(
              "{hours}",
              (dayCoverage.get(activeDay)?.lunchMin ?? 0) > 0
                ? fmtMinutes(dayCoverage.get(activeDay)!.lunchMin)
                : t("coverage.none"),
            )}
          </span>
          <span
            className={
              (dayCoverage.get(activeDay)?.dinnerMin ?? 0) === 0
                ? "text-rose-700"
                : "text-ink-600"
            }
          >
            {t("coverage.dinner").replace(
              "{hours}",
              (dayCoverage.get(activeDay)?.dinnerMin ?? 0) > 0
                ? fmtMinutes(dayCoverage.get(activeDay)!.dinnerMin)
                : t("coverage.none"),
            )}
          </span>
        </p>
      ) : null}

      {/* Rows for the selected day */}
      <div className="space-y-2.5">
        {rows.map((row) => {
          const cellShifts = shiftsForCell(row, activeDay);
          const posture =
            mode === "employee" && row.staffId != null
              ? cellPosture(
                  row.staffId,
                  activeDay,
                  teamAvailability,
                  approvedTimeOff,
                )
              : { kind: "none" as const };
          return (
            <div
              key={row.key}
              className="rounded-2xl border border-warm-200 bg-white p-2.5"
            >
              <div className="mb-1.5 flex items-center gap-2">
                {row.colorHex ? (
                  <span
                    className="h-2.5 w-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: row.colorHex }}
                    aria-hidden="true"
                  />
                ) : null}
                <span className="truncate text-sm font-semibold text-ink-800">
                  {row.label}
                </span>
              </div>

              {posture.kind === "timeoff" ? (
                <span className="mb-1.5 inline-flex items-center gap-1 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700">
                  <CalendarOff className="h-2.5 w-2.5" aria-hidden="true" />
                  {t("overlay.timeOff")}
                </span>
              ) : posture.kind === "unavailable" ? (
                <span className="mb-1.5 inline-flex items-center gap-1 rounded-md bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700">
                  <CalendarX2 className="h-2.5 w-2.5" aria-hidden="true" />
                  {t("overlay.unavailable")}
                </span>
              ) : null}

              <div className="space-y-1.5">
                {cellShifts.map((shift) => {
                  const open = shift.staff_id == null;
                  const position = positionsById.get(shift.position_id);
                  const assignee =
                    shift.staff_id != null
                      ? staffById.get(shift.staff_id)?.name
                      : null;
                  return (
                    <button
                      key={shift.id}
                      type="button"
                      onClick={() => onEditShift(shift)}
                      className={`block w-full rounded-xl border px-2.5 py-2 text-left text-xs shadow-sm shadow-warm-900/5 transition hover:border-brand/30 ${
                        open
                          ? "border-amber-200 bg-amber-50"
                          : "border-warm-200 bg-white"
                      }`}
                    >
                      <span className="block font-semibold text-ink-950">
                        {formatTimeRange(shift.starts_at, shift.ends_at, locale)}
                      </span>
                      <span className="mt-0.5 flex items-center gap-1 text-[11px] text-ink-500">
                        {mode === "role" ? (
                          open ? (
                            <span className="font-medium text-amber-700">
                              {t("open")}
                            </span>
                          ) : (
                            assignee
                          )
                        ) : (
                          <>
                            <span
                              className="h-1.5 w-1.5 rounded-full bg-brand"
                              style={
                                position?.color_hex
                                  ? { backgroundColor: position.color_hex }
                                  : undefined
                              }
                              aria-hidden="true"
                            />
                            {position
                              ? translatePositionName(position.name, t)
                              : null}
                          </>
                        )}
                      </span>
                    </button>
                  );
                })}

                <button
                  type="button"
                  aria-label={addShiftAccessibleName(
                    t("addShiftFor"),
                    row.label,
                    activeDay,
                    locale,
                  )}
                  onClick={() =>
                    onCreateShift({
                      dayKey: activeDay,
                      staffId: row.staffId ?? null,
                      positionId: row.positionId ?? null,
                    })
                  }
                  className="flex min-h-[40px] w-full items-center justify-center gap-1 rounded-xl border border-dashed border-warm-200 py-2 text-[11px] font-medium text-ink-400 transition hover:border-brand/40 hover:text-brand"
                >
                  <Plus className="h-3 w-3" />
                  {t("addShift")}
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

"use client";

import { Chip } from "@nextui-org/react";
import { AlertTriangle, CalendarClock, Clock } from "lucide-react";
import type { LaborWarning } from "@/api/schedule";

export interface WarningLabels {
  /** "...{name}...{hours}..." — overtime, per staff. */
  overtime: string;
  /** "...{days}..." — schedule posted inside the lead window. */
  postedLate: string;
  /** "...{time}..." — a minor scheduled past the cutoff (HH:MM). */
  minorLate: string;
}

/** cutoff minute-of-day → "HH:MM" (business-TZ local time). */
function fmtTime(min: number): string {
  const safe = Math.max(0, min);
  return `${String(Math.floor(safe / 60)).padStart(2, "0")}:${String(safe % 60).padStart(2, "0")}`;
}

/**
 * Non-blocking compliance warnings (amber). Labels are keyed off the Go warning
 * `code` (labor/calculator.go) and interpolated with String.replace — no ICU.
 *
 * `posted_late` can optionally open Schedule settings so the scold has a next
 * step (adjust lead time) instead of a bare amber chip (#166 / #248).
 */
export function WarningChips({
  warnings,
  labels,
  staffNames,
  onPostedLateAction,
  postedLateActionLabel,
}: {
  warnings: LaborWarning[];
  labels: WarningLabels;
  /** staff_id → display name; unknown ids fall back to "#id". */
  staffNames?: Map<number, string>;
  /** When set, posted_late chips become a button that opens the fix path. */
  onPostedLateAction?: () => void;
  /** Accessible label for the posted_late action button. */
  postedLateActionLabel?: string;
}) {
  if (!warnings.length) return null;
  return (
    <div className="flex max-w-full min-w-0 flex-wrap gap-2" role="list">
      {warnings.map((w, i) => {
        let text: string;
        let Icon = AlertTriangle;
        if (w.code === "overtime") {
          Icon = Clock;
          const name =
            (w.staff_id != null ? staffNames?.get(w.staff_id) : undefined) ??
            `#${w.staff_id ?? ""}`;
          text = labels.overtime
            .replace("{name}", name)
            .replace("{hours}", String(Math.round(w.detail / 60)));
        } else if (w.code === "posted_late") {
          Icon = CalendarClock;
          text = labels.postedLate.replace("{days}", String(w.detail));
        } else {
          text = labels.minorLate.replace("{time}", fmtTime(w.detail));
        }
        const key = `${w.code}-${w.staff_id ?? "all"}-${i}`;
        const chipClassNames = {
          base: "h-auto max-w-full min-w-0 whitespace-normal",
          content: "whitespace-normal break-words",
        };
        if (w.code === "posted_late" && onPostedLateAction) {
          return (
            <button
              key={key}
              type="button"
              role="listitem"
              onClick={onPostedLateAction}
              aria-label={postedLateActionLabel || text}
              className="inline-flex max-w-full min-w-0"
            >
              <Chip
                startContent={<Icon size={14} aria-hidden="true" />}
                variant="flat"
                color="warning"
                size="sm"
                className="max-w-full cursor-pointer underline-offset-2 hover:underline"
                classNames={chipClassNames}
              >
                {text}
              </Chip>
            </button>
          );
        }
        return (
          <Chip
            key={key}
            role="listitem"
            startContent={<Icon size={14} aria-hidden="true" />}
            variant="flat"
            color="warning"
            size="sm"
            className="max-w-full"
            classNames={chipClassNames}
          >
            {text}
          </Chip>
        );
      })}
    </div>
  );
}

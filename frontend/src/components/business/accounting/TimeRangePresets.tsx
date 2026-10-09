import { useMemo } from "react";

export type RangePreset = "today" | "week" | "month" | "30d" | "ytd" | "custom";

interface TimeRangePresetsProps {
  active: RangePreset;
  onSelect: (preset: RangePreset, start: string, end: string) => void;
  labels: Record<Exclude<RangePreset, "custom">, string> & { custom: string };
  /** Accessible name for the preset group. Falls back to a generic English
   *  string only if the caller doesn't supply a localized one. */
  ariaLabel?: string;
}

function pad(value: number): string {
  return value.toString().padStart(2, "0");
}

function formatDate(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

// NOTE: presets are computed against the browser's local calendar (new Date()).
// This component isn't passed the business timezone, so near midnight a preset
// like "today" can differ from the business day. Aligning to business TZ needs
// a timezone prop threaded down from the dashboard — tracked in HANDOFF
// (R3-AC). Kept browser-local for now to avoid a partial fix.
function rangeFor(preset: Exclude<RangePreset, "custom">): {
  start: string;
  end: string;
} {
  const today = new Date();
  const end = formatDate(today);
  if (preset === "today") {
    return { start: end, end };
  }
  if (preset === "week") {
    const day = today.getDay();
    const diff = day === 0 ? 6 : day - 1; // Monday-anchored week
    const start = new Date(today);
    start.setDate(today.getDate() - diff);
    return { start: formatDate(start), end };
  }
  if (preset === "month") {
    const start = new Date(today.getFullYear(), today.getMonth(), 1);
    return { start: formatDate(start), end };
  }
  if (preset === "ytd") {
    const start = new Date(today.getFullYear(), 0, 1);
    return { start: formatDate(start), end };
  }
  const start = new Date(today);
  start.setDate(today.getDate() - 29);
  return { start: formatDate(start), end };
}

export default function TimeRangePresets({
  active,
  onSelect,
  labels,
  ariaLabel,
}: TimeRangePresetsProps) {
  const presets = useMemo(
    () =>
      (
        [
          "today",
          "week",
          "month",
          "30d",
          "ytd",
          "custom",
        ] as RangePreset[]
      ).map((preset) => ({ preset, label: labels[preset] })),
    [labels],
  );

  // role="group" + aria-pressed (not tablist/tab): these are toggle buttons
  // that set a date range, not a tab strip with panels, so tablist's arrow-key
  // navigation semantics don't apply. See R3-AC (LOW).
  return (
    <div
      role="group"
      aria-label={ariaLabel ?? "Time range presets"}
      className="flex flex-wrap items-center gap-1.5"
    >
      {presets.map(({ preset, label }) => {
        const isActive = preset === active;
        return (
          <button
            key={preset}
            type="button"
            aria-pressed={isActive}
            onClick={() => {
              if (preset === "custom") {
                onSelect("custom", "", "");
                return;
              }
              const range = rangeFor(preset);
              onSelect(preset, range.start, range.end);
            }}
            className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
              isActive
                ? "border-brand bg-brand/[0.08] text-brand"
                : "border-warm-200 bg-white text-ink-700 hover:bg-warm-50"
            }`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}

export function detectPreset(start: string, end: string): RangePreset {
  const today = formatDate(new Date());
  if (start === today && end === today) return "today";
  const ranges: Exclude<RangePreset, "custom">[] = ["week", "month", "30d", "ytd"];
  for (const preset of ranges) {
    const r = rangeFor(preset);
    if (r.start === start && r.end === end) return preset;
  }
  return "custom";
}


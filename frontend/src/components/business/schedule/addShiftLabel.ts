import { intlLocaleFor } from "@/utils/intlLocale";

/** Accessible name for a schedule-grid Add shift control (#448). */
export function addShiftAccessibleName(
  template: string,
  target: string,
  dayKey: string,
  locale: string,
): string {
  const [year, month, day] = dayKey.split("-").map(Number);
  const date = Number.isFinite(year)
    ? new Intl.DateTimeFormat(intlLocaleFor(locale), {
        weekday: "short",
        month: "short",
        day: "numeric",
      }).format(new Date(year, month - 1, day))
    : dayKey;
  if (template.includes("{target}")) {
    return template.replace("{target}", target).replace("{date}", date);
  }
  return `${template} ${target} ${date}`;
}

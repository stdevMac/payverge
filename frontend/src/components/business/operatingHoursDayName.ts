import { type Locale } from "@/i18n/config";
import { getTranslation } from "@/i18n/getTranslation";

/**
 * Convert a day-of-week number (0 = Sunday) to the operator locale's day name.
 */
export const getDayName = (
  dayOfWeek: number,
  locale: Locale = "en",
): string => {
  const dayKeys = [
    "sunday",
    "monday",
    "tuesday",
    "wednesday",
    "thursday",
    "friday",
    "saturday",
  ];
  const dayKey = dayKeys[dayOfWeek];

  if (dayKey) {
    const translationKey = `businessSettings.operatingHoursEditor.dayNames.${dayKey}`;
    const translation = getTranslation(translationKey, locale);
    return Array.isArray(translation)
      ? translation[0] || dayKey
      : (translation as string);
  }

  const unknownTranslation = getTranslation(
    "businessSettings.operatingHoursEditor.dayNames.unknown",
    locale,
  );
  return Array.isArray(unknownTranslation)
    ? unknownTranslation[0] || "Unknown"
    : (unknownTranslation as string);
};

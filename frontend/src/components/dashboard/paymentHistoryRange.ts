import type { Locale } from "@/i18n/config";
import { surfaceBackendError } from "@/utils/localizedError";
import { isReversedCustomRange } from "./isReversedCustomRange";

export { isReversedCustomRange };

/**
 * L6-9: when Personalizado is active, PeriodTabs must not highlight a preset.
 * Use a sentinel outside the real option keys (not "") so NextUI Tabs does not
 * auto-select the first chip and fire onSelectionChange.
 */
const PERIOD_TABS_CUSTOM_SENTINEL = "__custom__";

export function periodTabsValueForCustom(
  customActive: boolean,
  periodKey: string,
): string {
  return customActive ? PERIOD_TABS_CUSTOM_SENTINEL : periodKey;
}

/**
 * L6-9: client gate before fetch — returns localized reversed-range copy or null.
 */
export function customRangeClientError(
  customActive: boolean,
  start: string,
  end: string,
  reversedMessage: string,
): string | null {
  if (customActive && isReversedCustomRange(start, end)) {
    return reversedMessage;
  }
  return null;
}

/**
 * L6-9: honest server-error surface for payment history list failures.
 */
export function paymentHistoryListError(
  err: unknown,
  locale: Locale,
): string {
  return surfaceBackendError(err, locale);
}

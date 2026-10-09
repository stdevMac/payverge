/**
 * F4: NextUI Modal hardcodes aria-label="Close" on the visible X button
 * (see @nextui-org/modal getCloseButtonProps). Visually-hidden DismissButtons
 * from @react-aria/overlays use I18nProvider locale instead.
 *
 * Pair this helper's closeButtonProps with NextUIProvider locale (Providers)
 * so both the X button and Dismiss chrome speak the operator language.
 */
import { getChromeTranslation } from "@/i18n/operatorChromeCatalog";
import {
  isSupportedLocale,
  type Locale,
} from "@/i18n/localeRegistry";

/** Map Payverge operator locales to BCP-47 tags react-aria ships messages for. */
export function toReactAriaLocale(locale: string): string {
  const short = (locale || "en").toLowerCase().split(/[-_]/)[0];
  if (short === "es") return "es-ES";
  return "en-US";
}

function asOperatorLocale(locale: string): Locale {
  if (isSupportedLocale(locale)) return locale;
  const short = (locale || "en").toLowerCase().split(/[-_]/)[0];
  if (short === "es") return "es";
  return "en";
}

/** Localized aria-label for the visible modal close (X) control. */
export function modalCloseAriaLabel(locale: string): string {
  const label = getChromeTranslation("common.close", asOperatorLocale(locale));
  return typeof label === "string" && label.trim() ? label : "Close";
}

/**
 * Preferred aria-label object for any *hand-rolled* close control.
 * Note: NextUI Modal does **not** accept closeButtonProps — its X button
 * hardcodes "Close". Use ModalCloseAriaLocalizer / LocalizedModal instead.
 */
export function modalCloseButtonProps(locale: string): {
  "aria-label": string;
} {
  return { "aria-label": modalCloseAriaLabel(locale) };
}

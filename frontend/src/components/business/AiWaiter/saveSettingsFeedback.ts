import type { Locale } from "@/i18n/config";
import { surfaceBackendError } from "@/utils/localizedError";

/**
 * L4-4: Camarero IA "Guardar Configuración" outcome messages.
 * Success is a dedicated key; failure goes through {@link surfaceBackendError}.
 */
export function saveSettingsSuccessMessage(successCopy: string): string {
  return successCopy;
}

export function saveSettingsErrorMessage(
  err: unknown,
  locale: Locale,
  fallback: string,
): string {
  return surfaceBackendError(err, locale, fallback);
}

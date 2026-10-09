import type { Locale } from "@/i18n/config";
import { surfaceBackendError } from "@/utils/localizedError";

/**
 * L6-7: revenue CSV export with honest error surface.
 * On failure, resolves the message through {@link surfaceBackendError} (413 →
 * range-too-large) and hands it to the caller's toast/banner.
 */
export async function runRevenueExport(args: {
  exportFn: () => Promise<Blob>;
  locale: Locale;
  fallback: string;
  onError: (message: string) => void;
}): Promise<"ok" | "error"> {
  try {
    await args.exportFn();
    return "ok";
  } catch (error) {
    args.onError(surfaceBackendError(error, args.locale, args.fallback));
    return "error";
  }
}

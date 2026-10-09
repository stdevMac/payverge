import type { Locale } from "@/i18n/config";
import { surfaceBackendError } from "@/utils/localizedError";

/**
 * L4-14: thread rename with error-banner lifecycle.
 *
 * Always clears the banner before the request so a retry cannot leave a stale
 * failure message; clears again on success; surfaces backend detail on failure
 * via {@link surfaceBackendError}.
 *
 * Extracted so unit tests drive the same path the dashboard uses.
 */
export async function runDirectorRename(args: {
  rename: () => Promise<void>;
  locale: Locale;
  fallback: string;
  setError: (message: string | null) => void;
}): Promise<void> {
  args.setError(null);
  try {
    await args.rename();
    args.setError(null);
  } catch (err) {
    args.setError(surfaceBackendError(err, args.locale, args.fallback));
  }
}

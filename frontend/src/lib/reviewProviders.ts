import { PLUGIN } from "@/constants/plugins";

/** Review sources the Business Page Reviews tab can offer. */
type ReviewProviderId = "google" | "trustpilot";

export interface ReviewProvider {
  id: ReviewProviderId;
  /** Stable key for i18n / labels. */
  nameKey: string;
  /** Plugin name when the provider is plugin-backed; null for first-party. */
  pluginName: string | null;
}

/**
 * Review providers available for a business.
 *
 * - Google is first-party (fields on the business row), always offered.
 * - Trustpilot (and any future review plugins) ship only when the matching
 *   business plugin is enabled — never hardcode Trustpilot as absent or always
 *   present.
 */
export function reviewProvidersFromEnabledPlugins(
  enabledPluginNames: readonly string[],
): ReviewProvider[] {
  const enabled = new Set(
    enabledPluginNames.map((n) => n.trim().toLowerCase()).filter(Boolean),
  );

  const providers: ReviewProvider[] = [
    {
      id: "google",
      nameKey: "google",
      pluginName: null,
    },
  ];

  if (enabled.has(PLUGIN.trustpilot)) {
    providers.push({
      id: "trustpilot",
      nameKey: "trustpilot",
      pluginName: PLUGIN.trustpilot,
    });
  }

  return providers;
}

/** True when the given plugin name is a known review provider plugin. */
export function isReviewPluginName(name: string | undefined | null): boolean {
  if (!name) return false;
  return name.trim().toLowerCase() === PLUGIN.trustpilot;
}

import type { InstanceFeatures, InstanceInfo } from "./instanceInfo";

/**
 * Dashboard rails that only make sense when an optional integration is
 * configured on this install (GET /api/v1/instance features). They are hidden
 * from navigation and a deep link renders <FeatureUnavailable> instead.
 */
const INSTANCE_TAB_FEATURE: Readonly<
  Partial<Record<string, keyof InstanceFeatures>>
> = {
  "ai-waiter": "ai",
  "director-console": "ai",
  fiscal: "fiscal_ar",
};

/** Self-hosting docs per optional integration (public repository tree). */
const DOCS_BASE = "https://github.com/stdevMac/payverge/blob/main/docs/self-hosting";
export const FEATURE_DOCS_URL: Readonly<
  Partial<Record<keyof InstanceFeatures, string>>
> = {
  ai: `${DOCS_BASE}/ai.md`,
  email: `${DOCS_BASE}/email.md`,
  whatsapp: `${DOCS_BASE}/whatsapp.md`,
  telegram: "https://github.com/stdevMac/payverge/blob/main/docs/telegram-plugin-runbook.md",
  fiscal_ar: "https://github.com/stdevMac/payverge/tree/main/docs/fiscal",
};

/** The gated feature a tab needs, when the instance confirmed it is off. */
export function instanceOffFeatureForTab(
  tab: string,
  info: InstanceInfo | null,
): keyof InstanceFeatures | null {
  const feature = INSTANCE_TAB_FEATURE[tab];
  if (!feature || !info) return null;
  return info.features[feature] === false ? feature : null;
}

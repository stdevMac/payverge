import { trackEvent } from "@/utils/analytics";

export const ASSISTANT_EVENT_NAMES = [
  "assistant_opened",
  "assistant_closed",
  "assistant_message_sent",
  "assistant_retry_requested",
  "assistant_render_fallback",
  "assistant_link_clicked",
  "assistant_source_clicked",
  "assistant_action_clicked",
  "assistant_action_completed",
  "assistant_action_failed",
  "assistant_jump_control_shown",
  "assistant_jump_control_clicked",
  "assistant_user_scrolled_away",
  "assistant_feedback_submitted",
  "assistant_history_contract_mismatch",
] as const;

export type AssistantEventName = (typeof ASSISTANT_EVENT_NAMES)[number];
type AssistantSurface = "ops" | "waiter";
type AssistantContractVersion = "v1" | "v2";

export interface AssistantAnalyticsContext {
  surface: AssistantSurface;
  contract_version?: AssistantContractVersion;
  locale: string;
}

export interface AssistantEventProperties {
  surface?: AssistantSurface;
  contract_version?: AssistantContractVersion;
  locale?: string;
  action_kind?: string;
  source_kind?: string;
  destination_kind?: string;
  outcome?: string;
  message_size_bucket?: number;
  response_size_bucket?: number;
}

const eventNames = new Set<string>(ASSISTANT_EVENT_NAMES);
const surfaces = new Set(["ops", "waiter"]);
const contractVersions = new Set(["v1", "v2"]);
const actionKinds = new Set([
  "navigate",
  "external_link",
  "director_handoff",
  "add_cart_item",
  "capture_lead",
  "create_support_request",
]);
const sourceKinds = new Set([
  "payverge_page",
  "product_registry",
  "pricing_registry",
  "knowledge_entry",
  "dashboard_guide",
  "business_state",
  "menu_item",
  "bundle",
  "offer",
  "order_state",
  "support_record",
]);
const destinationKinds = new Set(["internal", "external"]);
const outcomes = new Set([
  "accepted",
  "rejected",
  "completed",
  "failed",
  "positive",
  "negative",
]);
const supportedLocales = new Map(
  [
    "ar",
    "da",
    "de",
    "en",
    "es",
    "es-AR",
    "fr",
    "hi",
    "it",
    "ja",
    "ko",
    "nl",
    "no",
    "pl",
    "pt",
    "ru",
    "sv",
    "th",
    "tr",
    "vi",
    "zh",
  ].map((locale) => [locale.toLowerCase(), locale]),
);

function enumValue(
  value: unknown,
  allowed: ReadonlySet<string>,
): string | undefined {
  return typeof value === "string" && allowed.has(value) ? value : undefined;
}

function normalizeLocale(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const candidate = value.trim().replaceAll("_", "-").toLowerCase();
  return supportedLocales.get(candidate);
}

function sizeBucket(value: unknown): string | undefined {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    return undefined;
  }
  if (value <= 80) return "0-80";
  if (value <= 300) return "81-300";
  if (value <= 1_000) return "301-1000";
  return "1001+";
}

function normalizeProperties(
  properties: AssistantEventProperties,
): Record<string, string> {
  const normalized: Record<string, string> = {};
  const surface = enumValue(properties.surface, surfaces);
  const contractVersion = enumValue(
    properties.contract_version,
    contractVersions,
  );
  const locale = normalizeLocale(properties.locale);
  const actionKind = enumValue(properties.action_kind, actionKinds);
  const sourceKind = enumValue(properties.source_kind, sourceKinds);
  const destinationKind = enumValue(
    properties.destination_kind,
    destinationKinds,
  );
  const outcome = enumValue(properties.outcome, outcomes);
  const messageSize = sizeBucket(properties.message_size_bucket);
  const responseSize = sizeBucket(properties.response_size_bucket);

  if (surface) normalized.surface = surface;
  if (contractVersion) normalized.contract_version = contractVersion;
  if (locale) normalized.locale = locale;
  if (actionKind) normalized.action_kind = actionKind;
  if (sourceKind) normalized.source_kind = sourceKind;
  if (destinationKind) normalized.destination_kind = destinationKind;
  if (outcome) normalized.outcome = outcome;
  if (messageSize) normalized.message_size_bucket = messageSize;
  if (responseSize) normalized.response_size_bucket = responseSize;

  return normalized;
}

export function trackAssistantEvent(
  name: AssistantEventName,
  properties: AssistantEventProperties = {},
): void {
  if (typeof window === "undefined" || !eventNames.has(name)) return;
  try {
    trackEvent(name, normalizeProperties(properties));
  } catch {
    // Analytics is best-effort and must never block the user's requested action.
  }
}

export function assistantEventProperties(
  context: AssistantAnalyticsContext,
  properties: Omit<
    AssistantEventProperties,
    "surface" | "contract_version" | "locale"
  > = {},
): AssistantEventProperties {
  return {
    ...properties,
    surface: context.surface,
    contract_version: context.contract_version,
    locale: context.locale,
  };
}

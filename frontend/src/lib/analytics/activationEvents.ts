import { hasAnalyticsConsent } from "./consentGate";

export const ACTIVATION_SCHEMA_VERSION = 1 as const;

export const ACTIVATION_EVENT_NAMES = [
  "registration_started",
  "registration_completed",
  "workspace_created",
  "onboarding_step_viewed",
  "onboarding_step_clicked",
  "menu_item_created",
  "table_created",
  "qr_previewed",
  "payment_configured",
  "staff_invited",
  "setup_completed",
  "test_order_completed",
  "activation_achieved",
  "first_paid_bill",
] as const;

type ActivationEventName = (typeof ACTIVATION_EVENT_NAMES)[number];
export type OptionalClientActivationEvent =
  | "registration_started"
  | "onboarding_step_viewed"
  | "onboarding_step_clicked";

const optionalClientEvents = new Set<ActivationEventName>([
  "registration_started",
  "onboarding_step_viewed",
  "onboarding_step_clicked",
]);

type ActivationDeviceClass =
  | "mobile"
  | "tablet"
  | "desktop"
  | "server"
  | "unknown";

export interface ActivationDimensions {
  locale: string;
  device_class: ActivationDeviceClass;
  acquisition_source?: string;
  acquisition_campaign?: string;
  onboarding_step?: string;
  elapsed_ms: number;
}

export interface ClientActivationEvent {
  name: OptionalClientActivationEvent;
  schema_version: typeof ACTIVATION_SCHEMA_VERSION;
  funnel_id: string;
  idempotency_key: string;
  dimensions: ActivationDimensions;
}

export interface ActivationStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const SAFE = /^[A-Za-z0-9][A-Za-z0-9._/-]{0,79}$/;
const CARD = /(?:\d[ -]*?){13,19}/;
const allowedTopLevel = new Set([
  "name",
  "schema_version",
  "funnel_id",
  "idempotency_key",
  "dimensions",
]);
const allowedDimensionKeys = new Set([
  "locale",
  "device_class",
  "acquisition_source",
  "acquisition_campaign",
  "onboarding_step",
  "elapsed_ms",
]);

export function validateActivationEvent(event: ClientActivationEvent): void {
  for (const key of Object.keys(event)) {
    if (!allowedTopLevel.has(key))
      throw new Error(`unsupported activation property: ${key}`);
  }
  if (!ACTIVATION_EVENT_NAMES.includes(event.name))
    throw new Error("unknown activation event");
  if (!optionalClientEvents.has(event.name))
    throw new Error(
      "authoritative activation event requires server confirmation",
    );
  if (event.schema_version !== ACTIVATION_SCHEMA_VERSION)
    throw new Error("unsupported activation schema version");
  if (!UUID.test(event.funnel_id)) throw new Error("invalid funnel id");
  const step = event.dimensions.onboarding_step ?? "global";
  if (
    event.idempotency_key !== `client:${event.funnel_id}:${event.name}:${step}`
  )
    throw new Error("invalid activation idempotency key");
  for (const key of Object.keys(event.dimensions)) {
    if (!allowedDimensionKeys.has(key))
      throw new Error(`unsupported activation dimension: ${key}`);
  }
  if (
    !Number.isSafeInteger(event.dimensions.elapsed_ms) ||
    event.dimensions.elapsed_ms < 0
  )
    throw new Error("invalid elapsed_ms");
  for (const [key, value] of Object.entries(event.dimensions)) {
    if (key === "elapsed_ms" || value === undefined) continue;
    if (
      typeof value !== "string" ||
      !SAFE.test(value) ||
      value.includes("@") ||
      CARD.test(value)
    ) {
      throw new Error(`unsafe activation dimension: ${key}`);
    }
  }
}

const FUNNEL_KEY = "payverge_activation_funnel_v1";
const SENT_PREFIX = "payverge_activation_sent_v1:";

function browserStorage(): ActivationStorage {
  return window.sessionStorage;
}

export function createActivationEmitter(
  options: {
    transport?: (event: ClientActivationEvent) => Promise<void>;
    storage?: ActivationStorage;
    hasConsent?: () => boolean;
    randomUUID?: () => string;
  } = {},
) {
  const storage = options.storage ?? browserStorage();
  const consent = options.hasConsent ?? hasAnalyticsConsent;
  const randomUUID = options.randomUUID ?? (() => crypto.randomUUID());
  const transport =
    options.transport ??
    (async (event) => {
      const response = await fetch("/api/v1/analytics/activation-event", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(event),
      });
      if (!response.ok)
        throw new Error(`activation event rejected: ${response.status}`);
    });
  const inflight = new Map<string, Promise<"sent" | "already_sent">>();

  const funnelID = () => {
    const existing = storage.getItem(FUNNEL_KEY);
    if (existing && UUID.test(existing)) return existing;
    const generated = randomUUID();
    if (!UUID.test(generated))
      throw new Error("randomUUID returned an invalid UUID");
    storage.setItem(FUNNEL_KEY, generated);
    return generated;
  };

  const emit = async (
    name: OptionalClientActivationEvent,
    dimensions: ActivationDimensions,
  ) => {
    if (!consent()) return "consent_denied" as const;
    const funnel = funnelID();
    const dimensionKey = dimensions.onboarding_step ?? "global";
    const idempotencyKey = `client:${funnel}:${name}:${dimensionKey}`;
    const sentKey = SENT_PREFIX + idempotencyKey;
    if (storage.getItem(sentKey) === "1") return "already_sent" as const;
    const existing = inflight.get(idempotencyKey);
    if (existing) return existing;

    const event: ClientActivationEvent = {
      name,
      schema_version: ACTIVATION_SCHEMA_VERSION,
      funnel_id: funnel,
      idempotency_key: idempotencyKey,
      dimensions,
    };
    validateActivationEvent(event);
    const request = transport(event)
      .then(() => {
        storage.setItem(sentKey, "1");
        return "sent" as const;
      })
      .finally(() => inflight.delete(idempotencyKey));
    inflight.set(idempotencyKey, request);
    return request;
  };

  return { emit, funnelID };
}

let defaultEmitter: ReturnType<typeof createActivationEmitter> | undefined;

export function trackOptionalActivationEvent(
  name: OptionalClientActivationEvent,
  dimensions: ActivationDimensions,
) {
  if (typeof window === "undefined")
    return Promise.resolve("consent_denied" as const);
  defaultEmitter ??= createActivationEmitter();
  return defaultEmitter.emit(name, dimensions);
}

export function safeActivationToken(
  value: string | null | undefined,
  fallback = "unknown",
) {
  const raw = (value ?? "").trim();
  const lower = raw.toLowerCase();
  if (
    raw.includes("@") ||
    CARD.test(raw) ||
    ["name=", "email=", "address=", "customer=", "payment=", "card="].some(
      (marker) => lower.includes(marker),
    )
  )
    return fallback;
  const normalized = raw.replace(/[^A-Za-z0-9._/-]+/g, "_").slice(0, 80);
  return normalized && SAFE.test(normalized) ? normalized : fallback;
}

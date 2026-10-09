import type {
  ErrorEvent,
  Event,
  EventHint,
  Log,
  TransactionEvent,
} from "@sentry/core";

import { getPublicConfig } from "../../config/publicConfig";

const FILTERED_VALUE = "[Filtered]";
const DEFAULT_TRACES_SAMPLE_RATE = 0.05;
// Session Replay is opt-in. An operator sets a sample rate via env; until
// then both rates stay 0 and the client never builds the replay integration.
const DEFAULT_REPLAYS_SESSION_SAMPLE_RATE = 0;
const DEFAULT_REPLAYS_ON_ERROR_SAMPLE_RATE = 0;

const SENSITIVE_VALUE_PATTERNS = [
  /\b0x[a-fA-F0-9]{40}\b/g,
  /\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/gi,
  /\b(?:basic|bearer)\s+[A-Za-z0-9._~+/=-]+/gi,
  /-----BEGIN [^-]*PRIVATE KEY-----[\s\S]*?(?:-----END [^-]*PRIVATE KEY-----)?/g,
  /\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g,
  /\b(?:ghp|github_pat|sk|rk|tok|whsec|xox[baprs])[_-][A-Za-z0-9_=-]+\b/gi,
] as const;

const SAFE_KEYS = new Set([
  "id",
  "user_id",
  "business_id",
  "restaurant_id",
  "order_id",
  "bill_id",
  "table_id",
  "request_id",
  "x_request_id",
  "event_id",
  "trace_id",
  "span_id",
  "parent_span_id",
  "transaction_id",
]);

const SENSITIVE_EXACT_KEYS = new Set([
  "address",
  "address_line",
  "address_line_1",
  "address_line_2",
  "authorization",
  "cf_connecting_ip",
  "client_ip",
  "cookie",
  "cookies",
  "customer_name",
  "delivery_address",
  "first_name",
  "forwarded",
  "ip_address",
  "last_name",
  "name",
  "notes",
  "payer_address",
  "postal_code",
  "query_string",
  "settlement_address",
  "street",
  "true_client_ip",
  "wallet_address",
  "x_forwarded_for",
  // The proxy's socket-peer stamp (src/lib/proxy/peerStamp.ts): the client
  // IP plus the per-process nonce that authenticates the stamp.
  "x_payverge_peer",
  "x_real_ip",
]);

const SENSITIVE_KEY_PARTS = new Set([
  "address",
  "auth",
  "card",
  "csrf",
  "cvc",
  "cvv",
  "email",
  "jwt",
  "name",
  "note",
  "password",
  "passwd",
  "phone",
  "postal",
  "secret",
  "session",
  "street",
  "token",
  "username",
]);

const URL_KEYS = new Set([
  "href",
  "path",
  "pathname",
  "request_url",
  "route",
  "transaction",
  "url",
]);

export interface FrontendSentryRuntimeConfig {
  dsn?: string;
  enabled: boolean;
  environment: string;
  release?: string;
  tracesSampleRate: number;
  replaysSessionSampleRate: number;
  replaysOnErrorSampleRate: number;
}

export function parseBoolean(value: string | undefined): boolean | undefined {
  const normalized = value?.trim().toLowerCase();

  if (!normalized) {
    return undefined;
  }

  if (["1", "true", "yes", "on"].includes(normalized)) {
    return true;
  }

  if (["0", "false", "no", "off"].includes(normalized)) {
    return false;
  }

  return undefined;
}

export function parseSampleRate(value: string | undefined, fallback: number): number {
  const normalized = value?.trim();

  if (!normalized) {
    return fallback;
  }

  const parsed = Number(normalized);

  if (!Number.isFinite(parsed) || parsed < 0 || parsed > 1) {
    return fallback;
  }

  return parsed;
}

export function getFrontendSentryRuntimeConfig(): FrontendSentryRuntimeConfig {
  // DSN, environment, enable flag and sample rates are runtime public config
  // (window.__PAYVERGE_ENV__ in the browser); only the release is build-time.
  const publicConfig = getPublicConfig();
  const dsn = normalizeEnvValue(publicConfig.sentryDsn);
  const environment = normalizeEnvironmentName(
    normalizeEnvValue(publicConfig.sentryEnvironment) ??
      process.env.NODE_ENV ??
      "development",
  );
  const explicitEnabled = parseBoolean(publicConfig.sentryEnabled);
  const environmentAllowsSentry = ["production", "staging"].includes(
    environment.toLowerCase(),
  );

  return {
    dsn,
    enabled: Boolean(dsn) && (explicitEnabled ?? environmentAllowsSentry),
    environment,
    release: normalizeEnvValue(process.env.NEXT_PUBLIC_SENTRY_RELEASE),
    tracesSampleRate: parseSampleRate(
      publicConfig.sentryTracesSampleRate,
      DEFAULT_TRACES_SAMPLE_RATE,
    ),
    replaysSessionSampleRate: parseSampleRate(
      publicConfig.sentryReplaysSessionSampleRate,
      DEFAULT_REPLAYS_SESSION_SAMPLE_RATE,
    ),
    replaysOnErrorSampleRate: parseSampleRate(
      publicConfig.sentryReplaysOnErrorSampleRate,
      DEFAULT_REPLAYS_ON_ERROR_SAMPLE_RATE,
    ),
  };
}

export function sanitizeUrl(rawUrl: string | undefined): string | undefined {
  if (!rawUrl) {
    return rawUrl;
  }

  const queryIndex = rawUrl.indexOf("?");
  const fragmentIndex = rawUrl.indexOf("#");
  const endIndex = [queryIndex, fragmentIndex]
    .filter((index) => index >= 0)
    .sort((left, right) => left - right)[0];

  const strippedUrl = endIndex === undefined ? rawUrl : rawUrl.slice(0, endIndex);
  const routeScrubbedUrl = scrubSensitiveRouteSegments(strippedUrl);

  return scrubSensitiveString(routeScrubbedUrl);
}

export function sanitizeObject<T>(value: T, parentKey = ""): T {
  const normalizedParentKey = normalizeKey(parentKey);

  if (typeof value === "string") {
    if (URL_KEYS.has(normalizedParentKey) || isUrlLikeString(value)) {
      return sanitizeUrl(value) as T;
    }

    return scrubSensitiveString(value) as T;
  }

  if (value === null || value === undefined || typeof value !== "object") {
    return value;
  }

  if (Array.isArray(value)) {
    return value.map((entry) => sanitizeObject(entry, parentKey)) as T;
  }

  if (!isPlainObject(value)) {
    return value;
  }

  const sanitized: Record<string, unknown> = {};

  for (const [key, entryValue] of Object.entries(value)) {
    const normalizedKey = normalizeKey(key);

    if (isSensitiveKey(normalizedKey)) {
      sanitized[key] = FILTERED_VALUE;
      continue;
    }

    sanitized[key] = sanitizeObject(entryValue, normalizedKey);
  }

  return sanitized as T;
}

export function scrubSentryEvent(event: ErrorEvent, hint?: EventHint): ErrorEvent | null {
  return scrubSentryPayload(event, hint);
}

export function scrubSentryTransaction(
  event: TransactionEvent,
  hint?: EventHint,
): TransactionEvent | null {
  return scrubSentryPayload(event, hint);
}

export function scrubSentryReplayRecordingEvent<TEvent>(event: TEvent): TEvent | null {
  if (isConsoleReplayBreadcrumb(event)) {
    return null;
  }

  return sanitizeObject(event);
}

export function scrubSentryLog(log: Log): Log | null {
  if (!log) {
    return log;
  }

  const scrubbed: Log = { ...log };

  if (typeof scrubbed.message === "string") {
    // Preserve the ParameterizedString brand while stripping sensitive values
    // (emails, bearer tokens, 0x addresses, keys) from the log body.
    scrubbed.message = scrubSensitiveString(scrubbed.message) as Log["message"];
  }

  if (scrubbed.attributes) {
    scrubbed.attributes = sanitizeObject(scrubbed.attributes);
  }

  return scrubbed;
}

function scrubSentryPayload<TEvent extends Event>(
  event: TEvent,
  hint?: EventHint,
): TEvent | null {
  void hint;

  const scrubbed = sanitizeObject(event);

  const safeUserId = getSafeUserId(event.user?.id);
  if (safeUserId !== undefined) {
    scrubbed.user = { id: safeUserId };
  } else {
    delete scrubbed.user;
  }

  if (event.request) {
    const request = sanitizeObject({ ...event.request });

    if (event.request.url) {
      request.url = sanitizeUrl(event.request.url);
    }

    if (event.request.headers) {
      request.headers = sanitizeObject(event.request.headers);
    }

    delete request.cookies;
    delete request.data;
    delete request.query_string;
    scrubbed.request = request;
  }

  if (event.extra) {
    scrubbed.extra = sanitizeObject(event.extra);
  }

  if (event.contexts) {
    scrubbed.contexts = sanitizeObject(event.contexts);
  }

  return scrubbed;
}

function normalizeEnvValue(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

// The wider codebase treats ENV=prod as production (see CLAUDE.md). Normalize
// the alias so both the Sentry gating decision and the reported `environment`
// tag agree on "production".
function normalizeEnvironmentName(value: string): string {
  return value.toLowerCase() === "prod" ? "production" : value;
}

function isConsoleReplayBreadcrumb(event: unknown): boolean {
  if (!isRecord(event) || !isRecord(event.data)) {
    return false;
  }

  const { data } = event;
  return (
    data.tag === "breadcrumb" &&
    isRecord(data.payload) &&
    data.payload.category === "console"
  );
}

function scrubSensitiveString(value: string): string {
  return SENSITIVE_VALUE_PATTERNS.reduce(
    (scrubbedValue, pattern) => scrubbedValue.replace(pattern, FILTERED_VALUE),
    value,
  );
}

function isUrlLikeString(value: string): boolean {
  return value.startsWith("/") || value.startsWith("//") || value.includes("://");
}

function scrubSensitiveRouteSegments(rawUrl: string): string {
  const { prefix, path } = splitUrlPath(rawUrl);
  const segments = path.split("/");
  const routeSegments = segments
    .map((segment, index) => ({
      index,
      value: normalizeRouteSegment(segment),
    }))
    .filter((segment) => segment.value);
  const redactIndexes = new Set<number>();
  const start = routeStartsWith(routeSegments, "api", "v1") ? 2 : 0;

  if (routeSegments[start]?.value === "t" && routeSegments[start + 1]) {
    redactIndexes.add(routeSegments[start + 1].index);
  }

  if (routeSegments[start]?.value === "table" && routeSegments[start + 1]) {
    redactIndexes.add(routeSegments[start + 1].index);
  }

  if (
    routeSegments[start]?.value === "guest" &&
    routeSegments[start + 1]?.value === "table" &&
    routeSegments[start + 2]
  ) {
    redactIndexes.add(routeSegments[start + 2].index);
  }

  if (
    routeSegments[start]?.value === "guest" &&
    routeSegments[start + 1]?.value === "bill" &&
    routeSegments[start + 2]
  ) {
    redactIndexes.add(routeSegments[start + 2].index);
  }

  if (routeSegments[start]?.value === "reservations" && routeSegments[start + 1]) {
    redactIndexes.add(routeSegments[start + 1].index);
  }

  if (
    routeSegments[start]?.value === "delivery" &&
    routeSegments[start + 1] &&
    routeSegments[start + 2]?.value === "track"
  ) {
    redactIndexes.add(routeSegments[start + 1].index);
  }

  if (redactIndexes.size === 0) {
    return rawUrl;
  }

  return (
    prefix +
    segments
      .map((segment, index) => (redactIndexes.has(index) ? FILTERED_VALUE : segment))
      .join("/")
  );
}

function splitUrlPath(rawUrl: string): { prefix: string; path: string } {
  const schemeIndex = rawUrl.indexOf("://");
  if (schemeIndex >= 0) {
    const authorityStart = schemeIndex + 3;
    const pathStart = rawUrl.indexOf("/", authorityStart);
    if (pathStart < 0) {
      return { prefix: rawUrl, path: "" };
    }
    return { prefix: rawUrl.slice(0, pathStart), path: rawUrl.slice(pathStart) };
  }

  if (rawUrl.startsWith("//")) {
    const pathStart = rawUrl.indexOf("/", 2);
    if (pathStart < 0) {
      return { prefix: rawUrl, path: "" };
    }
    return { prefix: rawUrl.slice(0, pathStart), path: rawUrl.slice(pathStart) };
  }

  return { prefix: "", path: rawUrl };
}

function normalizeRouteSegment(segment: string): string {
  try {
    return decodeURIComponent(segment).toLowerCase();
  } catch {
    return segment.toLowerCase();
  }
}

function routeStartsWith(
  routeSegments: Array<{ value: string }>,
  first: string,
  second: string,
): boolean {
  return routeSegments[0]?.value === first && routeSegments[1]?.value === second;
}

function isSensitiveValue(value: string): boolean {
  return scrubSensitiveString(value) !== value;
}

function isPlainObject(value: object): value is Record<string, unknown> {
  return Object.prototype.toString.call(value) === "[object Object]";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object";
}

function isSensitiveKey(normalizedKey: string): boolean {
  if (SAFE_KEYS.has(normalizedKey)) {
    return false;
  }

  if (SENSITIVE_EXACT_KEYS.has(normalizedKey)) {
    return true;
  }

  return normalizedKey.split("_").some((part) => SENSITIVE_KEY_PARTS.has(part));
}

function getSafeUserId(value: unknown): string | number | undefined {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : undefined;
  }

  if (typeof value !== "string") {
    return undefined;
  }

  const trimmed = value.trim();

  return trimmed && !isSensitiveValue(trimmed) ? trimmed : undefined;
}

function normalizeKey(key: string): string {
  return key
    .replace(/([a-z0-9])([A-Z])/g, "$1_$2")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

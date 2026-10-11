/**
 * Single source of truth for what each payment-plugin config editor submits to
 * the backend. The editors build their form state from these factories, and the
 * config-schema contract test (configFields.contract.test.ts) reads them back to
 * assert — against the backend's own GetConfigSchema fixtures — that:
 *   1. every REQUIRED schema field has a form input (so a plugin can't 400 on
 *      save forever, e.g. MercadoPago/PayPal `base_url`), and
 *   2. every submitted field is a real schema property or an explicitly declared
 *      UI-only field (so dead fields the backend never reads, e.g. the old
 *      Stripe `webhook_endpoint` / MercadoPago `webhook_url`, can't creep back).
 *
 * Because the editors and the test both derive from these factories, the test
 * genuinely fails on drift rather than restating a hardcoded list.
 */

/** Sentinel the backend returns for a secret that already exists server-side. */
export const MASKED_SECRET = "••••••••";

export function isMaskedSecret(value: unknown): boolean {
  return typeof value === "string" && value === MASKED_SECRET;
}

/**
 * A secret input is "satisfied" (Save may proceed) when it either still holds
 * the server-side mask sentinel — meaning "keep the stored secret" — or holds a
 * genuinely new value that passes the format validator. `validate` returns
 * true/false for a non-empty value, or null when there's nothing to check.
 */
export function isSecretSatisfied(
  value: string,
  validate: (v: string) => boolean | null,
): boolean {
  if (isMaskedSecret(value)) return true;
  return validate(value) === true;
}

/**
 * Whether to surface a format error for a secret input. The mask sentinel and
 * empty values never error — only a genuinely new value that fails validation.
 */
export function hasSecretFormatError(
  value: string,
  validate: (v: string) => boolean | null,
): boolean {
  if (!value || isMaskedSecret(value)) return false;
  return validate(value) === false;
}

export function buildStripeInitialConfig(config: Record<string, any>) {
  return {
    secret_key: config.secret_key || "",
    publishable_key: config.publishable_key || "",
    webhook_secret: config.webhook_secret || "",
    auto_capture:
      config.auto_capture !== undefined ? config.auto_capture : true,
    // UI-only: the backend derives live/test from the key prefix; this only
    // drives the toggle's display. Declared in STRIPE_UI_ONLY_FIELDS below.
    mode: config.mode === "live" ? "live" : "test",
    // OAuth connection fields. connection_mode / stripe_user_id / tokens are
    // schema properties; oauth_status and live_mode are server-written metadata
    // preserved on save so a full config replace does not wipe the OAuth link.
    connection_mode: config.connection_mode || "",
    stripe_user_id: config.stripe_user_id || "",
    access_token: config.access_token || "",
    refresh_token: config.refresh_token || "",
    oauth_status: config.oauth_status || "",
    live_mode: config.live_mode === true || config.live_mode === "true",
  };
}

/** Fields submitted by the editor that are intentionally absent from the schema. */
export const STRIPE_UI_ONLY_FIELDS = ["mode", "oauth_status", "live_mode"];

export function buildMercadoPagoInitialConfig(config: Record<string, any>) {
  return {
    access_token: config.access_token || "",
    public_key: config.public_key || "",
    webhook_secret: config.webhook_secret || "",
    base_url: config.base_url || "",
    country: config.country || "AR",
    auto_return: config.auto_return || "approved",
    installments: config.installments || 12,
    // OAuth connection fields. connection_mode + refresh_token are schema
    // properties; the rest are server-written metadata preserved on save so a
    // full config replace does not wipe the OAuth link.
    connection_mode: config.connection_mode || "",
    refresh_token: config.refresh_token || "",
    mp_user_id: config.mp_user_id || "",
    live_mode: config.live_mode === true || config.live_mode === "true",
    oauth_status: config.oauth_status || "",
    token_expires_at: config.token_expires_at || "",
    environment: config.environment || "",
  };
}

/** Fields submitted by the editor that are intentionally absent from the schema. */
export const MERCADOPAGO_UI_ONLY_FIELDS = [
  "mp_user_id",
  "live_mode",
  "oauth_status",
  "token_expires_at",
  "environment",
];

export function buildPayPalInitialConfig(config: Record<string, any>) {
  return {
    client_id: config.client_id || "",
    client_secret: config.client_secret || "",
    environment: config.environment || "sandbox",
    webhook_id: config.webhook_id || "",
    base_url: config.base_url || "",
    brand_name: config.brand_name || "",
    return_url: config.return_url || "",
    cancel_url: config.cancel_url || "",
  };
}

export const PAYPAL_UI_ONLY_FIELDS: string[] = [];

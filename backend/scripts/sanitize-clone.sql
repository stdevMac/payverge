-- sanitize-clone.sql — PII / secret scrub for a RESTORED PRODUCTION BACKUP CLONE.
--
-- Priority-4 gate (e): "sanitized production-backup clone". run-restore-drill.sh
-- restores the newest production dump into a THROWAWAY scratch Postgres container,
-- then runs THIS script before the clone is verified/used so that no real customer
-- PII, staff PII, wallet/financial identifiers, auth secrets, provider tokens, or
-- free-text that routinely embeds PII survive in the drill clone.
--
-- Scope: overwrites PII/secret COLUMNS only. Non-personal data (money amounts,
-- quantities, statuses, foreign keys, timestamps, public business/menu names) is
-- intentionally left intact so schema + invariant verification stays meaningful.
--
-- Redaction conventions:
--   * UNIQUE columns are scrubbed per-row (…|| id) so the scrub cannot violate a
--     unique constraint. This includes values that feed UNIQUE indexes via
--     triggers (e.g. user_sessions.previous_refresh_token → history.token_hash).
--   * Emails use the reserved, non-routable .invalid TLD (RFC 6761).
--   * Wallet/blockchain addresses -> zero address (const) or per-row when UNIQUE.
--   * Secrets (password/pin hashes, tokens, provider secrets) -> '[redacted]',
--     except when uniqueness requires per-row redaction (session/refresh tokens).
--   * Free-text that may embed PII -> '[redacted]'.
--   * JSON config blobs that may embed provider secrets -> '{"_redacted":true}'.
--   * Geolocation numerics -> 0.
--   * Pure secret tables with no non-secret value to preserve may be DELETEd
--     (user_session_refresh_history).
--
-- Portability: plain UPDATEs using only ANSI `||` concatenation and CAST(id AS TEXT),
-- so the same statements run under Postgres (the drill) and SQLite (the Go
-- scrub-logic test in internal/database/sanitize_clone_test.go).
--
-- Idempotent: re-running scrubs already-scrubbed rows to the same values.
-- Fail-loud: run under psql --set ON_ERROR_STOP=1; a missing table/column aborts.

BEGIN;

-- ===========================================================================
-- Identity / accounts
-- ===========================================================================
UPDATE users SET
  email    = 'redacted-' || CAST(id AS TEXT) || '@example.invalid',
  name     = '[redacted-name]',
  address  = NULL,
  username = '[redacted]',
  google_id = NULL;

UPDATE customers SET
  email                = 'redacted-' || CAST(id AS TEXT) || '@example.invalid',
  password_hash        = '[redacted]',
  name                 = '[redacted-name]',
  phone                = '[redacted-phone]',
  wallet_address       = '0x0000000000000000000000000000000000000000',
  verification_token   = '[redacted]',
  password_reset_token = '[redacted]';

UPDATE staff SET
  email                  = 'redacted-' || CAST(id AS TEXT) || '@example.invalid',
  name                   = '[redacted-name]',
  pin_hash               = '[redacted]',
  invited_by             = '[redacted]',
  permissions_updated_by = '[redacted]';

UPDATE businesses SET
  owner_name      = '[redacted-name]',
  phone           = '[redacted-phone]',
  email           = '[redacted]@example.invalid',
  owner_address   = '0x0000000000000000000000000000000000000000',
  settlement_addr = '0x0000000000000000000000000000000000000000',
  tipping_addr    = '0x0000000000000000000000000000000000000000',
  street          = NULL,
  city            = NULL,
  state           = NULL,
  postal_code     = NULL,
  country         = NULL,
  latitude        = 0,
  longitude       = 0;

-- ===========================================================================
-- Auth secrets / sessions
-- ===========================================================================
-- Refresh-token history stores secret token hashes under a global
-- UNIQUE(token_hash). Wipe it first so real hashes never survive the clone.
-- Updating user_sessions.previous_refresh_token also fires the deferred
-- capture_user_session_refresh_history trigger, which re-inserts history at
-- COMMIT — so previous_refresh_token (and refresh_token) MUST be scrubbed to
-- per-row unique values, same as session_token. A constant '[redacted]'
-- collides across sessions (unique_violation / "token collision").
DELETE FROM user_session_refresh_history;

UPDATE user_auths SET
  password_hash      = '[redacted]',
  provider_user_id   = '[redacted]',
  wallet_address     = '0x0000000000000000000000000000000000000000',
  verification_token = '[redacted]',
  reset_token        = '[redacted]';

ALTER TABLE user_sessions DISABLE TRIGGER trg_capture_user_session_refresh_history;

WITH numbered_refresh_history AS (
  SELECT
    ctid,
    ROW_NUMBER() OVER (ORDER BY session_id, rotated_at, token_hash) AS scrub_ordinal
  FROM user_session_refresh_history
)
UPDATE user_session_refresh_history AS history SET
  token_hash = 'redacted-refresh-history-' || CAST(numbered_refresh_history.scrub_ordinal AS TEXT)
FROM numbered_refresh_history
WHERE history.ctid = numbered_refresh_history.ctid;

UPDATE user_sessions SET
  session_token          = 'redacted-' || CAST(id AS TEXT),
  refresh_token          = 'redacted-refresh-' || CAST(id AS TEXT),
  previous_refresh_token = 'redacted-prev-' || CAST(id AS TEXT),
  address                = '0x0000000000000000000000000000000000000000',
  ip_address             = '0.0.0.0',
  user_agent             = '[redacted]';

ALTER TABLE user_sessions ENABLE TRIGGER trg_capture_user_session_refresh_history;

UPDATE auth_attempts SET
  principal = 'redacted-' || CAST(id AS TEXT);

UPDATE staff_invitations SET
  email      = '[redacted]@example.invalid',
  name       = '[redacted-name]',
  token      = 'redacted-' || CAST(id AS TEXT),
  invited_by = '[redacted]';

UPDATE staff_login_codes SET
  code = '[redacted]';

-- ===========================================================================
-- Customer contact / addresses / delivery
-- ===========================================================================
UPDATE customer_addresses SET
  street                = '[redacted]',
  apartment             = NULL,
  city                  = '[redacted]',
  state                 = NULL,
  postal_code           = NULL,
  country               = '[redacted]',
  formatted_address     = NULL,
  latitude              = 0,
  longitude             = 0,
  delivery_instructions = NULL,
  contact_name          = '[redacted-name]',
  contact_phone         = '[redacted-phone]';

UPDATE delivery_drivers SET
  name           = '[redacted-name]',
  phone          = '[redacted-phone]',
  email          = '[redacted]@example.invalid',
  license_number = '[redacted]',
  vehicle_type   = '[redacted]',
  vehicle_plate  = '[redacted]';

UPDATE delivery_orders SET
  customer_name              = '[redacted-name]',
  customer_phone             = '[redacted-phone]',
  customer_email             = '[redacted]@example.invalid',
  delivery_street            = NULL,
  delivery_apartment         = NULL,
  delivery_city              = NULL,
  delivery_state             = NULL,
  delivery_postal_code       = NULL,
  delivery_country           = NULL,
  delivery_formatted_address = NULL,
  pickup_latitude            = 0,
  pickup_longitude           = 0,
  dropoff_latitude           = 0,
  dropoff_longitude          = 0;

-- ===========================================================================
-- Marketing / CRM / growth
-- ===========================================================================
UPDATE customer_communications SET
  content = '[redacted]',
  subject = '[redacted]';

UPDATE customer_visits SET
  feedback = '[redacted]';

-- ===========================================================================
-- Provider / plugin / payment identifiers & secrets
-- ===========================================================================
UPDATE telegram_connection_tokens SET
  token_hash       = 'redacted-' || CAST(id AS TEXT),
  used_by_username = '[redacted]',
  created_ip       = '0.0.0.0';

UPDATE business_plugins SET
  config = '{"_redacted":true}';

UPDATE printers SET
  cloudprnt_token = 'redacted-' || CAST(id AS TEXT);

UPDATE payment_refund_destinations SET
  refund_address = '0x0000000000000000000000000000000000000000',
  signature_ref  = '[redacted]';

UPDATE payment_refunds SET
  verified_recipient = '0x0000000000000000000000000000000000000000',
  requested_by       = '[redacted]',
  approved_by        = '[redacted]',
  rejected_by        = '[redacted]',
  override_reason    = '[redacted]';

UPDATE alternative_payments SET
  participant_name = '[redacted-name]',
  participant_addr = '0x0000000000000000000000000000000000000000';

-- Platform catalog: scrub every is_secret row. Empty string (not
-- '[redacted]') so readers treat the credential as unset rather than sending a
-- placeholder to a provider.
-- (Avoid semicolons in these comments: the Go scrub test splits on them.)
UPDATE platform_settings SET
  value = ''
WHERE is_secret = true;

-- ===========================================================================
-- Bills / splits / fiscal customer identity
-- ===========================================================================
UPDATE bills SET
  fiscal_customer_name = '[redacted-name]';

UPDATE bill_split_shares SET
  display_name = '[redacted-name]';

-- ===========================================================================
-- AI / chat / assistant free-text (routinely embeds guest PII)
-- ===========================================================================
UPDATE ai_waiter_conversations SET
  claimed_by_name = '[redacted-name]';

UPDATE ai_waiter_messages SET
  content     = '[redacted]',
  author_name = '[redacted-name]',
  tool_calls  = '[redacted]';

UPDATE chat_messages SET
  sender_name = '[redacted-name]',
  content     = '[redacted]';

UPDATE director_console_messages SET
  content             = '[redacted]',
  structured_response = '{"_redacted":true}';

UPDATE ops_assistant_messages SET
  content             = '[redacted]',
  structured_response = '{"_redacted":true}';

UPDATE menu_wizard_messages SET
  content = '[redacted]';

UPDATE shift_notes SET
  content = '[redacted]';

UPDATE shoutouts SET
  message = '[redacted]';

-- ===========================================================================
-- Escalations / error logs (free-text + contact + external ids + metadata)
-- ===========================================================================
UPDATE escalations SET
  contact_email      = '[redacted]@example.invalid',
  issue              = '[redacted]',
  transcript_summary = '[redacted]',
  admin_notes        = '[redacted]';

-- additional_info is GORM serializer:json (map); must stay valid JSON.
-- Plain '[redacted]' starts as a JSON array and fails with
-- "invalid character 'r' looking for beginning of value" — which 500s
-- GET /admin/stats on every sanitized clone.
UPDATE error_logs SET
  error           = '[redacted]',
  message         = '[redacted]',
  stack           = '[redacted]',
  user_id         = '[redacted]',
  metadata        = '{"_redacted":true}',
  additional_info = '{"_redacted":true}';

COMMIT;

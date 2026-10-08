# Telegram Plugin Production Runbook

Last updated: 2026-08-11

## Scope

This runbook covers the Payverge Telegram notification plugin in production:

- dashboard connection flow
- Telegram webhook ingestion
- plugin notification outbox delivery
- worker operations
- rollout, monitoring, and incident response

## Required environment

Backend environment or CLI flags:

- `TELEGRAM_TOKEN` or `TELEGRAM_BOT_TOKEN`: bot token used for replies and outbound notifications.
- `TELEGRAM_BOT_USERNAME`: public bot username without relying on token introspection.
- `TELEGRAM_WEBHOOK_SECRET`: secret expected in `X-Telegram-Bot-Api-Secret-Token`.
- `TELEGRAM_CONNECTION_TOKEN_TTL_MINUTES`: connection link TTL. Allowed range is 5 to 60 minutes; default is 15.
- `TELEGRAM_NOTIFICATION_WORKER_CONCURRENCY`: number of outbox worker loops to run per backend instance. Defaults to `2`; values below `1` are clamped to `1`.
- `TELEGRAM_NOTIFICATION_EVENTS`: comma-separated allow-list of event types to enqueue, for phased rollout. Empty or `*` enables every Telegram event.

Enablement is credential-derived — there are no enable flags:

- the notification delivery worker runs iff `TELEGRAM_TOKEN` is set.
- the public webhook route registers iff `TELEGRAM_TOKEN` and `TELEGRAM_WEBHOOK_SECRET` are both set.

## Webhook setup

Configure the webhook after deployment. Set `PUBLIC_URL` in your shell to
the instance's public origin, the same value the backend uses:

```bash
curl -X POST "https://api.telegram.org/bot${TELEGRAM_TOKEN}/setWebhook" \
  -d "url=${PUBLIC_URL}/api/v1/webhooks/telegram" \
  -d "secret_token=${TELEGRAM_WEBHOOK_SECRET}" \
  -d "allowed_updates=[\"message\"]"
```

Verify webhook state:

```bash
curl "https://api.telegram.org/bot${TELEGRAM_TOKEN}/getWebhookInfo"
```

Expected webhook endpoint:

```text
POST /api/v1/webhooks/telegram
```

The endpoint rejects requests with an invalid webhook secret.

## Rotating webhook secret

1. Generate a new high-entropy secret.
2. Set `TELEGRAM_WEBHOOK_SECRET` on backend instances.
3. Deploy or restart backend instances.
4. Re-run `setWebhook` with the new `secret_token`.
5. Confirm `payverge_telegram_webhook_updates_total{status="unauthorized"}` does not spike after rotation.

During rotation, keep the window between backend deploy and Telegram `setWebhook` update as short as possible.

## Connection flow

1. Business enables the Telegram plugin.
2. Dashboard calls `POST /inside/businesses/:id/plugins/telegram/generate-token`.
3. Backend creates an opaque `pv_tg_` token, stores only its SHA-256 hash, revokes older pending tokens, and returns a `t.me` URL plus expiry.
4. User opens the URL and sends the prefilled `/start <token>` command to the bot.
5. Telegram webhook consumes the token exactly once.
6. Backend stores chat metadata in the business plugin config.
7. Dashboard polling observes `health=connected`.

Supported chat types:

- `private`
- `group`
- `supergroup`

Rejected chat type:

- `channel`

## Notification delivery path

Business events enqueue rows in `plugin_notification_deliveries`.

Current event types:

- `order.created`
- `order.status_changed`
- `payment.received`
- `reservation.created`
- `reservation.status_changed`
- `inventory.low_stock`
- `summary.daily`

The Telegram worker claims due rows for `plugin_name=telegram`, renders safe HTML, sends through Telegram, and updates delivery status.

Delivery statuses:

- `pending`: created and due for first delivery.
- `processing`: claimed by a worker.
- `delivered`: Telegram accepted the message.
- `retry`: transient failure; worker will retry after backoff or Telegram retry-after.
- `failed`: permanent failure or max attempts reached.
- `dropped`: not deliverable, for example disconnected chat or disabled notification type.

Retry backoff:

- attempt 1: 1 minute
- attempt 2: 5 minutes
- attempt 3: 15 minutes
- attempt 4: 1 hour
- attempt 5+: 6 hours or final failure when max attempts is reached

## User-facing status

Dashboard status values:

- `not_connected`: no chat connected and no pending valid token.
- `pending`: valid pending connection token exists.
- `connected`: chat connected and no recent sender error.
- `degraded`: chat connected but last send failed.
- `disabled`: reserved status for future plugin-level disablement.

Important config fields:

- `chat_id`
- `chat_type`
- `chat_title`
- `telegram_username`
- `is_connected`
- `connected_at`
- `last_sent_at`
- `last_test_sent_at`
- `last_error`
- `last_error_at`
- `failure_count`

## Metrics

Prometheus metrics:

- `payverge_telegram_connection_tokens_issued_total`
- `payverge_telegram_connection_tokens_consumed_total`
- `payverge_telegram_connection_failures_total{reason}`
- `payverge_telegram_webhook_updates_total{status}`
- `payverge_plugin_notification_enqueued_total{plugin,event_type}`
- `payverge_plugin_notification_delivered_total{plugin,event_type}`
- `payverge_plugin_notification_failed_total{plugin,event_type,reason}`
- `payverge_plugin_notification_retried_total{plugin,event_type,reason}`
- `payverge_plugin_notification_queue_depth{plugin}`
- `payverge_plugin_notification_delivery_latency_seconds{plugin,event_type}`
- `payverge_telegram_send_attempts_total{status,reason}`
- `payverge_telegram_send_duration_seconds{status,reason}`
- `payverge_telegram_connected_businesses`

Recommended alerts:

- Webhook unauthorized spike: `payverge_telegram_webhook_updates_total{status="unauthorized"}` increases unexpectedly.
- Webhook processing failures: `status="processing_failed"` above baseline.
- Queue backlog: `payverge_plugin_notification_queue_depth{plugin="telegram"}` remains above 100 for more than 10 minutes.
- Retry/failure spike: high rate of `payverge_plugin_notification_retried_total` or `failed_total`.
- Telegram rate limiting: `payverge_telegram_send_attempts_total{reason="telegram_rate_limited"}` increases.
- Degraded businesses: investigate configs with non-empty `last_error`.

Suggested dashboard panels:

- Connected businesses: `payverge_telegram_connected_businesses`
- Send success rate: rate of `payverge_telegram_send_attempts_total{status="sent"}`
- Send failures by reason: rate of `payverge_telegram_send_attempts_total{status!="sent"}`
- Queue depth: `payverge_plugin_notification_queue_depth{plugin="telegram"}`
- Delivery latency p95: histogram quantile for `payverge_plugin_notification_delivery_latency_seconds{plugin="telegram"}`
- Webhook updates by status: rate of `payverge_telegram_webhook_updates_total`

## Operational checks

After deploy:

1. Confirm backend starts with Telegram token and webhook secret.
2. Confirm webhook is configured with the production URL and secret.
3. Generate a connection link from a test business.
4. Connect via `/start <token>`.
5. Confirm dashboard moves from `pending` to `connected`.
6. Send a test notification.
7. Trigger a low-risk event and confirm one `plugin_notification_deliveries` row is delivered.
8. Check Prometheus counters for webhook accepted, token consumed, notification enqueued, and notification delivered.

Useful database checks:

```sql
SELECT business_id, expires_at, used_at, revoked_at
FROM telegram_connection_tokens
ORDER BY created_at DESC
LIMIT 20;
```

```sql
SELECT plugin_name, event_type, status, COUNT(*)
FROM plugin_notification_deliveries
GROUP BY plugin_name, event_type, status
ORDER BY plugin_name, event_type, status;
```

```sql
SELECT business_id, event_type, event_id, status, attempt_count, last_error_code, last_error_message
FROM plugin_notification_deliveries
WHERE plugin_name = 'telegram'
ORDER BY updated_at DESC
LIMIT 50;
```

## Incident response

### Users cannot connect Telegram

Check:

- `TELEGRAM_BOT_USERNAME` is correct.
- Webhook URL is configured and reachable.
- Webhook secret matches `TELEGRAM_WEBHOOK_SECRET`.
- Token is not expired.
- Chat type is not `channel`.
- Token generation was not rate-limited.

Relevant metrics:

- `payverge_telegram_connection_failures_total{reason}`
- `payverge_telegram_webhook_updates_total{status}`

Recovery:

1. Revoke pending token from dashboard.
2. Generate a new connection link.
3. Ask user to send the exact prefilled `/start` command.

### Notifications are delayed

Check:

- `TELEGRAM_TOKEN` is set (the delivery worker runs iff the token is set)
- worker logs for processing failures
- `payverge_plugin_notification_queue_depth{plugin="telegram"}`
- database rows in `retry` or `processing`

Recovery:

1. Confirm Telegram API is reachable from backend hosts.
2. Restart backend worker process if rows remain stuck in `processing`.
3. Inspect `last_error_code` for common provider failures.

### Telegram returns forbidden or chat not found

Cause:

- user blocked the bot
- group removed the bot
- chat migrated or ID changed

Recovery:

1. Dashboard will show degraded status.
2. Disconnect Telegram for the business.
3. Ask user to reconnect the bot.

### Rate limiting

Cause:

- Telegram returned 429 or retry-after.

Behavior:

- worker records `telegram_rate_limited`
- retry is scheduled using Telegram retry-after when available

Recovery:

1. Let retries drain naturally.
2. If rate limits persist, reduce worker concurrency or event volume.

### Disable Telegram globally

There are no enable/disable flags; enablement follows credentials.

To stop event enqueue during phased rollout:

```bash
TELEGRAM_NOTIFICATION_EVENTS=
```

To disable the webhook route, unset `TELEGRAM_WEBHOOK_SECRET` (the route only
registers when both `TELEGRAM_TOKEN` and the secret are set). Unsetting
`TELEGRAM_TOKEN` disables the delivery worker and the webhook together.

If the webhook route is disabled, also unset Telegram's webhook through the Telegram API to prevent repeated delivery attempts.

## Rollback

Application rollback:

1. Stop deliveries by unsetting `TELEGRAM_TOKEN` (the worker runs iff the token is set).
2. Deploy previous backend/frontend.
3. Keep database migrations in place unless explicitly rolling back schema in a controlled maintenance window.

Schema rollback:

Use the matching migration down file only if no production data must be preserved from:

- `telegram_connection_tokens`
- `telegram_update_receipts`
- `plugin_notification_deliveries`
- `plugin_notification_delivery_attempts`

Do not drop these tables during a normal application rollback.

# Alert Response Runbook

**When to use this runbook:** a Prometheus alert on a Payverge backend fired,
or you are writing alert rules and need to know what each signal means.

It covers four alert families: payment webhook failures, orphaned fiscal
authorizations (CAE), readiness, and dead-lettered deliveries. Each section
names the metric or probe, a starting PromQL expression, what the alert
means, and what to do. Tune the windows and thresholds to your traffic.

Metrics are served at `http://backend:8080/metrics` with
`Authorization: Bearer $METRICS_TOKEN`. The path is not routed through Caddy;
scrape it from a container on the compose `edge` network
([deploy/README.md](../../deploy/README.md#metrics-and-logs)). Counters reset
when the backend restarts, so alert on `increase()` or `rate()`, never on the
raw value.

Database checks below use:

```sh
docker compose exec postgres psql -U payverge -d payverge
```

---

## 1. Payment webhook failures

**Signal:** `payverge_payment_webhook_processing_failures_total{plugin, reason}`
counts payment-provider webhook events that were received but not processed.

```promql
sum by (plugin, reason) (
  increase(payverge_payment_webhook_processing_failures_total{reason!="capture_pending"}[15m])
) > 0
```

`reason="capture_pending"` is excluded on purpose: it is a refund or dispute
that arrived before its payment and is being retried (see
[payments.md](../self-hosting/payments.md#refunds-and-disputes-that-arrive-before-the-payment)).
When such an event is still unmatched after 48 hours it is acknowledged, a
payment-review alert appears in the dashboard (on the bill, or on the
restaurant when the event named no bill), and this counter increases. A
background sweeper does the same for an event the provider stopped
redelivering, so the alert fires either way:

```promql
increase(payverge_payment_webhook_unsupported_actions_total{action="capture_pending.expired"}[1h]) > 0
```

**What it means:** a provider (Stripe, PayPal, Mercado Pago) sent an event
and Payverge did not apply it. A payment may show as pending on a bill even
though the guest paid. A burst with a signature-related `reason` almost always
means the webhook secret in Payverge no longer matches the provider's.

**What to do:**

1. Read the failure reasons from the logs:
   `docker compose logs backend | grep -i webhook`.
2. List the failed events with a platform-admin token:
   `GET /api/v1/admin/webhooks/failed` (see
   [payments.md](../self-hosting/payments.md#checking-that-it-works)), or
   query them directly:

   ```sql
   SELECT id, provider, event_type, status, error, received_at
   FROM webhook_events
   WHERE status IN ('failed', 'processing')
   ORDER BY received_at DESC
   LIMIT 50;
   ```

   A row left in `processing` for more than a few minutes is also a failure.
   Rows in `retry_pending` are waiting for their payment event and are
   retried by the provider; they are not failures. They are expired with an
   alert after 48 hours.
3. Fix the cause (usually the secret; see
   [payments.md](../self-hosting/payments.md)). There is no retry endpoint:
   resend each event from the provider's dashboard, then acknowledge any
   event you deliberately skip with
   `POST /api/v1/admin/webhooks/:id/acknowledge`.
4. Check the affected bills against the provider's dashboard before telling
   staff a payment is missing.

---

## 2. Orphaned fiscal authorization (CAE)

**Signal:** `payverge_fiscal_orphaned_cae_total` counts ARCA (AFIP)
authorizations whose local receipt row could not be saved.

```promql
increase(payverge_fiscal_orphaned_cae_total[1h]) > 0
```

Page on the first occurrence. There is no safe threshold above zero.

**What it means:** ARCA issued a CAE, which consumed a legal invoice number,
but Payverge failed to record the receipt after its bounded retries. The job
is marked `failed_permanent` with `last_error_code = 'persist_failed_terminal'`
and is never retried on purpose: issuing again would request a new number and
create a duplicate legal invoice.

**What to do:**

1. Find the CAE and receipt number in the backend log line
   `orphaned CAE: authorized receipt could not be saved` (fields `job_id`,
   `cae`, `receipt_number`). The CAE is not a secret.
2. Find the job:

   ```sql
   SELECT id, business_id, bill_id, action, status, last_error_code,
          last_error_message, updated_at
   FROM fiscal_jobs
   WHERE last_error_code = 'persist_failed_terminal'
   ORDER BY updated_at DESC;
   ```

3. Fix why the save failed (database disk, connection limits, a constraint in
   `last_error_message`) before anything else.
4. **Do not** reset the job to `pending` or reissue the receipt from the
   dashboard. Confirm the authorization in ARCA's portal, then reconcile it by
   hand with the venue's accountant: record the CAE against the bill, or void
   it with a credit note in ARCA if the sale did not happen.

---

## 3. Readiness

**Signal:** `GET /api/v1/health/ready` returns `200 {"status":"ready"}` or
`503 {"status":"not ready"}`. There is no readiness metric; probe the endpoint
with the Prometheus blackbox exporter (or any HTTP monitor) and alert on the
probe:

```promql
probe_success{job="payverge-ready"} == 0
```

Add `up{job="payverge-backend"} == 0` for the scrape itself.

**What it means:** the backend is up but a dependency it needs is not: the
database, or a component checked at startup (JWT, plugins, network, email,
storage, proxy, AI budget, AI privacy). A `/health/live` that passes while
`/health/ready` fails means the process is fine and a dependency is not.

**What to do:**

1. Read the component matrix with the detail token:

   ```sh
   curl -s -H "Authorization: Bearer $HEALTH_DETAIL_TOKEN" \
     http://127.0.0.1:8080/api/v1/health/ready
   ```

   Each failed component carries a `code` (for example
   `email.api_key.missing`, `s3.public.missing`).
2. A `database` failure: check `docker compose ps postgres` and
   `docker compose logs postgres`, then disk space on the host.
3. A configuration code: fix `.env` (see
   [configuration.md](../self-hosting/configuration.md)) and run
   `docker compose up -d`.
4. If the backend exits at startup instead, read
   [troubleshooting.md](../self-hosting/troubleshooting.md), and
   [migration-dirty-recovery.md](migration-dirty-recovery.md) for a dirty
   migration.

---

## 4. Dead-lettered deliveries

Work that ran out of retries is kept as a dead letter, not dropped. Each queue
has its own signal:

| Queue | Signal | Dead state |
|---|---|---|
| Fiscal receipt delivery (artifact, email, print) | `payverge_fiscal_delivery_dead_letters_total{channel}` | `fiscal_delivery_tasks.status = 'dead'` |
| Transactional email outbox | `payverge_email_outbox_failed_total{template, reason}` | `email_outbox.status = 'failed'` |
| Plugin notifications (Telegram and others) | `payverge_plugin_notification_failed_total{plugin, event_type, reason}` | `plugin_notification_deliveries.status = 'failed'` |

```promql
sum by (channel) (increase(payverge_fiscal_delivery_dead_letters_total[1h])) > 0
```

```promql
sum by (template, reason) (increase(payverge_email_outbox_failed_total[1h])) > 0
```

```promql
sum by (plugin, reason) (increase(payverge_plugin_notification_failed_total[1h])) > 5
```

A growing backlog is the early warning before dead letters:

```promql
payverge_fiscal_delivery_oldest_age_seconds > 1800
```

```promql
payverge_email_outbox_queue_depth > 100
```

**What it means:** a receipt was authorized but never reached the guest or
printer, an email (receipt, reservation, password reset) was never sent, or a
staff notification was lost. The money and the legal receipt are unaffected.

**What to do:**

1. Read the reason. For fiscal deliveries:

   ```sql
   SELECT id, business_id, receipt_id, channel, attempts, last_error, dead_at
   FROM fiscal_delivery_tasks
   WHERE status = 'dead'
   ORDER BY dead_at DESC
   LIMIT 50;
   ```

   For email, the `reason` label and the backend log name the provider error;
   see [email.md](../self-hosting/email.md).
2. Fix the shared cause first: provider credentials, a suppressed recipient,
   a printer that is offline, or storage that rejects uploads.
3. Resend what matters. A fiscal receipt can be sent again with
   `POST /api/v1/inside/businesses/:id/fiscal/receipts/:receiptId/resend` (needs
   `fiscal:write`). Do not bulk-reset dead rows to `pending`: they would retry
   against the same failure and dead-letter again.

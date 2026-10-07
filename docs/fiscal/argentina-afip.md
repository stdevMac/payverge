# Argentina — AFIP/ARCA electronic invoicing (factura electrónica)

Payverge can issue real AFIP/ARCA‑authorized electronic invoices (factura A/B/C and
notas de crédito) for Argentine businesses. This guide is the operator + deployer
setup path. The fiscal engine is **dark by default** and only activates per business
once credentials are uploaded, validated, and the worker is enabled.

> **Audience:** restaurant operators in Argentina (and the engineer deploying the
> backend). On‑chain/crypto rails are unrelated to fiscal issuance — fiscal is a
> separate, AFIP‑facing subsystem.

---

## How it works (one paragraph)

When a bill is fully paid, the backend enqueues a `fiscal_jobs` row. A background
worker claims due jobs, resolves the business's per‑business AFIP provider
(decrypting the stored certificate), authenticates against **WSAA**, requests a
**CAE** via **WSFEv1** (`FECAESolicitar`), and persists an authorized
`fiscal_receipt` with the CAE, receipt number, and the official AFIP QR. Refunds
enqueue a **nota de crédito**. A status‑reconciliation path (`FECompConsultar`)
re‑checks receipts that failed transiently. Everything is idempotent and retried
with backoff; nothing is issued unless AFIP actually returns an authorization.

### Durable receipt delivery (Wave 4)

**Guest factura access (Wave A):** dine-in guests can receive the factura via (a) the optional email captured in the fiscal identity form at checkout (`fiscal_customer_email`, first link in the delivery recipient chain), (b) the printed thermal ticket with CAE/número/QR when a receipt print job runs after authorization, and (c) the guest bill thank-you page (`GET /guest/bill/:bill_token/fiscal-receipt` status JSON + `.../fiscal-receipt/pdf` stream).

Authorization and customer delivery are **decoupled**. When an issue receipt is
persisted as `authorized`, the same DB transaction inserts independent
`fiscal_delivery_tasks` rows for channels **`artifact`** (protected PDF/QR
upload), **`email`**, and **`print`**. A leased **fiscal delivery worker**
(`FISCAL_DELIVERY_WORKER_INTERVAL_SECONDS`, `FISCAL_DELIVERY_WORKER_CONCURRENCY`)
claims due tasks with `FOR UPDATE SKIP LOCKED`, executes the channel, and applies
bounded exponential backoff with jitter. Terminal failures go **`dead`** after
`max_attempts` (default 8) or on permanent validation errors.

**Operator recovery**

- Dashboard shows per-channel badges (pending / in progress / sent / failed).
- `POST .../fiscal/delivery-tasks/:taskId/retry` (`fiscal:retry`) requeues a dead
  channel with a new idempotency salt and writes a `fiscal_delivery_task_requeued`
  audit event. Double-click is safe.
- Full re-send (`.../receipts/:id/resend`) requeues all channels for that receipt.

**SLO / observability**

- Metrics: `payverge_fiscal_delivery_pending`,
  `payverge_fiscal_delivery_oldest_age_seconds`,
  `payverge_fiscal_delivery_*_total` / `_duration_seconds` by channel.
- Target: email/artifact delivery within a few minutes of authorization under
  healthy transports; page when oldest pending age exceeds ~30m or dead-letter
  rate spikes.
- A coarse `SweepUndeliveredReceipts` remains only to drain **legacy** rows that
  predate delivery tasks (skipped when any task exists for the receipt). Retire
  it when `CountLegacyUndeliveredAuthorizedReceipts == 0` in production.

---

## Prerequisites (operator, one‑time, on AFIP/ARCA)

1. **CUIT** of the business (responsable inscripto, monotributo, or exento).
2. **A digital certificate for web services.** In AFIP's *Administración de
   Certificados Digitales*, generate a CSR, obtain the `.crt`, and keep the matching
   private key (`.key`). This is the cert Payverge uses to sign the WSAA Login Ticket
   Request — it is **not** your AFIP website password.
3. **Authorize the web service** `wsfe` (Facturación Electrónica) for that
   certificate/computador fiscal in *Administrador de Relaciones de Clave Fiscal*.
4. **Register a Punto de Venta (point of sale)** of type *Web Services* in
   *Comprobantes en línea / ABM de Puntos de Venta*. Note the PoS number.

Without steps 3 and 4, WSAA login or `FECAESolicitar` will reject — validation
(below) surfaces the exact AFIP error.

---

## Operator setup in Payverge

1. **Fiscal → Settings**: choose mode, environment, enter CUIT, tax condition
   (`monotributo` / `responsable_inscripto` / `exento`), and the point of sale.
   - **Mode**: `off` (no issuance), `manual` (operator issues from the receipts
     page), `automatic_non_blocking` (issue on paid bill; a failure never blocks the
     guest).
   - **Environment**: `sandbox` (AFIP **homologación** / WSHOMO — use this first) or
     `production` (live AFIP).
2. **Fiscal → Credentials**: upload the `.crt` and `.key` (or a combined `.pem`).
   The key is encrypted at rest (AES‑GCM, see deployment below) and is **never**
   shown again — only a fingerprint + expiry are displayed.
3. **Validate**: press *Validate*. Payverge performs a live `FEDummy` health check +
   a WSAA login round‑trip and flips `setup_status` to **ready** on success, or
   shows the AFIP/transport error inline on failure (status stays not‑ready).
4. Once `ready`, paid bills issue automatically (in an automatic mode) or can be
   issued/credited from **Fiscal → Receipts**.

`setup_status` progression: `draft → credentials_set → ready`.

---

## Deployment / configuration (engineer)

Set these in the **root `.env`** (compose only forwards declared vars):

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `PLUGIN_SECRET_KEY` | **Yes, for fiscal** | — | 32‑byte key that encrypts uploaded certificates/keys at rest. If unset in production, credentials cannot be decrypted and **every fiscal job fails**. Shared with the plugin‑secrets subsystem. |
| `FISCAL_WORKER_INTERVAL_SECONDS` | No | `10` | Worker sweep interval. |
| `FISCAL_WSAA_URL` | No | AFIP host by env | Override the WSAA endpoint (testing/staging). |
| `FISCAL_WSFE_URL` | No | AFIP host by env | Override the WSFEv1 endpoint (testing/staging). |
| `FISCAL_AR_CF_ID_THRESHOLD_CENTS` | No | `1_000_000_000` (ARS 10,000,000, RG 5700/2025) | Factura B unidentified-consumidor-final identification threshold in int64 cents. Inclusive (`>=`). |

AFIP endpoints resolved automatically from the per‑business `environment`:

| | Homologación (sandbox) | Production |
|---|---|---|
| WSAA | `wsaahomo.afip.gov.ar` | `wsaa.afip.gov.ar` |
| WSFEv1 | `wswhomo.afip.gov.ar` | `servicios1.afip.gov.ar` |

### Rollout order (important)

1. Provision `PLUGIN_SECRET_KEY` (production: a real 32‑byte secret, not the dev
   fallback).
2. Deploy the **backend first** (the worker + routes), then the frontend.
3. The worker always runs — gate the rollout **per business** instead: keep each
   business's fiscal `environment` on **homologación** until the e2e (below) is
   green for at least one real test CUIT, then switch that business to production.
   Watch logs + the `payverge_fiscal_*` metrics.

> **Data‑compat note:** the receipt's `provider_receipt_id` stores the AFIP
> comprobante key (`type-pos-number`) used by status reconciliation. The feature is
> unreleased, so no legacy rows exist; if any pre‑release row ever stored a bare CAE
> there, a status check on it fails *closed* (terminal, never spurious‑authorize) —
> re‑issue rather than reconcile.

### Observability

Prometheus series (gated like the rest of `/metrics`):
- `payverge_fiscal_receipts_total{status}` — outcomes by terminal/transition status.
- `payverge_fiscal_job_attempts_total{action}` — attempts by action.
- `payverge_fiscal_jobs_queue_depth` — current due backlog.

Alert if `queue_depth` climbs steadily (worker stuck) or
`fiscal_receipts_total{status="failed_permanent"}` rises (systemic rejection).

---

## Homologación (sandbox) end‑to‑end — the production gate

Before enabling production issuance, run the build‑tagged e2e against WSHOMO with a
real test CUIT cert/key. It runs the full real flow — `FEDummy` → WSAA login →
`FECompUltimoAutorizado` → `FECAESolicitar` — and asserts a real CAE comes back.
**Do not enable `production` for any business until this is green.**

```bash
cd backend
FISCAL_HOMO_CUIT=20123456789 \
FISCAL_HOMO_POS=1 \
FISCAL_HOMO_CERT_PATH=/abs/path/test.crt \
FISCAL_HOMO_KEY_PATH=/abs/path/test.key \
go test -tags afip_homo -run TestAFIPHomologacionE2E -v ./internal/fiscal/providers/ar/
```

Without the `afip_homo` tag (and the env vars) the test is excluded from the normal
suite, so CI never needs AFIP connectivity.

---

## Troubleshooting

- **Validation fails with a WSAA error** → the certificate isn't authorized for
  `wsfe`, or clock skew, or wrong CUIT. Re‑check AFIP steps 2–3.
- **`FECAESolicitar` rejects (e.g. 10016)** → the point of sale isn't registered for
  web services, or a field is malformed. The rejection is recorded on the receipt as
  `failed_permanent` with the AFIP code/message.
- **Jobs pile up, nothing issues** → `PLUGIN_SECRET_KEY` is missing/invalid so
  credentials can't be decrypted (the startup log warns loudly), or the backend
  isn't running the current build (the worker always starts as of this version).
- **Receipt stuck `failed_retryable`** → a transient AFIP/transport error; the
  worker backs off and retries, and a status check reconciles via `FECompConsultar`.

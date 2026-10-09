# Email

Payverge sends transactional email: address verification, password resets,
staff invites, receipts, reservation confirmations and lifecycle notices. A
fresh install needs **no email account**. With nothing configured, mail is not
sent. The backend logs metadata only (template, recipient count and a hash)
and nothing goes to a third party.

To deliver real mail, point Payverge at any SMTP server, or at Resend or
Postmark.

## Quick start

| Goal | Set |
|---|---|
| Try Payverge, no mail delivery | nothing |
| Deliver through your own mailbox or relay | `EMAIL_PROVIDER=smtp`, `SMTP_HOST`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` |
| Deliver through Resend | `RESEND_API_KEY` (or `EMAIL_PROVIDER=resend` with `EMAIL_API_KEY`), `FROM_EMAIL`, `FROM_EMAIL_UPDATES` |
| Deliver through Postmark | `EMAIL_PROVIDER=postmark`, `EMAIL_API_KEY`, `FROM_EMAIL`, `FROM_EMAIL_UPDATES` |

After changing these values, restart the backend (`docker compose up -d backend`).
The startup log names the provider and the verification mode in use:

```
Email provider: smtp (EMAIL_VERIFICATION=required)
```

## Providers

| `EMAIL_PROVIDER` | Delivers? | Needs | Bounce handling |
|---|---|---|---|
| `log` | no | nothing | not applicable |
| `smtp` | yes | `SMTP_HOST` (plus credentials for most relays) | bounces go to the sender mailbox; see [Bounces and suppression](#bounces-and-suppression) |
| `resend` | yes | `EMAIL_API_KEY` or `RESEND_API_KEY` | webhook at `/api/v1/webhooks/email/resend` |
| `postmark` | yes | `EMAIL_API_KEY` | none |

### How the provider is chosen

1. If `EMAIL_PROVIDER` is set, it wins. Matching ignores case and surrounding
   spaces.
2. If it is empty and `RESEND_API_KEY` is set, the provider is `resend`.
3. Otherwise the provider is `log`.

Two settings do **not** select a provider on their own:

- `SMTP_HOST`. Set `EMAIL_PROVIDER=smtp` explicitly.
- `EMAIL_API_KEY`. It works for both Resend and Postmark, so it does not say
  which one you mean. Pair it with `EMAIL_PROVIDER=resend` or
  `EMAIL_PROVIDER=postmark`.

The same goes for `SMTP_USERNAME` and `SMTP_PASSWORD`. If any of these
delivery credentials is set while the provider resolves to `log`, the backend
treats it as a mistake rather than a choice:

- in production (`--production`), startup fails and names the variable;
- in development, the backend logs a warning, keeps using `log`, and treats
  `EMAIL_VERIFICATION=auto` as `required` (links are logged only when
  `EMAIL_LOG_CONTENT=true`), so a half-finished mail setup never opens
  unverified sign-up.

The flags `--email-provider` and `--email-api-key` mirror the two variables.
When a flag is non-empty it overrides
the matching variable.

The public `GET /api/v1/instance` endpoint returns the effective
`email_verification` mode. It also returns `features.email`, which is `false`
only for the `log` provider.

## Environment variables

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `EMAIL_PROVIDER` | empty, then inferred | all | `log`, `smtp`, `resend` or `postmark`. Any other value stops startup. |
| `EMAIL_LOG_CONTENT` | false | log | Also log recipients, subject, body preview and links. Off logs metadata only. |
| `EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION` | false | log | Required in production when `EMAIL_LOG_CONTENT=true`, or startup fails. |
| `EMAIL_VERIFICATION` | `auto` | all | `auto`, `required` or `off`. See [Verification modes](#verification-modes). Any other value stops startup. |
| `EMAIL_API_KEY` | empty | resend, postmark | Provider API key. Needs `EMAIL_PROVIDER` to be set. |
| `RESEND_API_KEY` | empty | resend | Alias of `EMAIL_API_KEY`. `EMAIL_API_KEY` wins when both are set. |
| `RESEND_WEBHOOK_SECRET` | empty | resend | Signing secret for the bounce and complaint webhook. |
| `SMTP_HOST` | empty | smtp | Relay hostname. Required for `smtp`. |
| `SMTP_PORT` | `587` | smtp | `587` uses STARTTLS and `465` uses implicit TLS. |
| `SMTP_USERNAME` | empty | smtp | When empty, no `AUTH` is attempted, which suits open internal relays. |
| `SMTP_PASSWORD` | empty | smtp | Used exactly as written: surrounding spaces are kept. |
| `SMTP_FROM` | empty | smtp | Replaces the From header and the envelope sender on every message. Set it to an address your relay accepts. |
| `SMTP_TLS` | `auto` | smtp | `auto`, `starttls`, `tls` or `none`. See [SMTP transport security](#smtp-transport-security). |
| `FROM_EMAIL` | `noreply@yourapp.com` in compose | all | Sender for transactional mail. Also `--from-email`. |
| `FROM_EMAIL_UPDATES` | `updates@yourapp.com` in compose | all | Sender for product and lifecycle mail. Also `--from-email-updates`. |

If both `FROM_EMAIL` and the flag are empty, the sender falls back to the
first available of these:

1. `SMTP_FROM`;
2. `noreply@` followed by the host from `PUBLIC_URL` (a leading `www.` is
   dropped);
3. `noreply@localhost`.

An empty `FROM_EMAIL_UPDATES` copies `FROM_EMAIL`.

With `smtp`, set `SMTP_FROM` and the sender variables no longer matter. With
`resend` and `postmark`, `FROM_EMAIL` and `FROM_EMAIL_UPDATES` must use a domain
you have verified with that provider.

### Per-tenant sending limits

Mail a business triggers (receipts, staff invites, booking replies) counts
against daily budgets before the provider is called; a refused send returns
`email_budget_exceeded`. "Daily" is a 24-hour window per counter that opens
with its first counted send, not a UTC calendar day. A negative value means
unlimited and `0` blocks the scope. `.env.example` explains each cap.

| Variable | Default | Scope |
|---|---|---|
| `EMAIL_TENANT_DAILY_CAP` / `_DEMO_` | 1000 / 20 | One business, all mail, by tier (standard or demo). |
| `EMAIL_TENANT_RECIPIENT_DAILY_CAP` | 20 | One business mailing one address. |
| `EMAIL_TENANT_RECIPIENT_GLOBAL_ALL_DAILY_CAP` | 50 | One address, all businesses together, every purpose. Stops many businesses from each mailing one victim. Payment and fiscal receipts count against a separate key with the same cap, so other mail cannot crowd them out. |
| `EMAIL_TENANT_RESERVATION_DAILY_CAP` | 200 (clamped to half the tier cap) | Anonymous guest-booking lane, per business. |
| `EMAIL_TENANT_RESERVATION_RECIPIENT_DAILY_CAP` | 3 | Guest-booking lane, one business to one address. |
| `EMAIL_TENANT_RECIPIENT_GLOBAL_DAILY_CAP` | 10 | Guest-booking lane, one address across all businesses. |
| `EMAIL_TENANT_DEDUPE_MINUTES` | 10 | An identical message to the same recipients inside the window is sent once. |

Keep the all-purpose cap above both the per-business recipient cap and the
lane cap, so neither anonymous bookings nor one venue can exhaust it alone.
The counters live in the database (`auth_attempts`), so they hold across
replicas.

The all-purpose cap has a denial side. With `REGISTRATION_MODE=open`, a few
throwaway businesses (20 sends each to one address) can use up an address's
daily 50, and receipts or staff invites to that address from other venues are
then refused until the next UTC day. On an open-registration instance,
consider `REGISTRATION_MODE=invite` (or `closed`), or raise the cap if your
legitimate venues share many recipients. A closed-registration instance does
not have this exposure.

## Verification modes

`EMAIL_VERIFICATION` controls whether a new email/password account must click
a verification link before it can sign in.

| Value | Behaviour |
|---|---|
| `auto` (default) | `off` with the `log` provider, `required` with any provider that delivers. A delivery credential set without `EMAIL_PROVIDER`, or a development fallback to `log` (see [Missing or invalid configuration](#missing-or-invalid-configuration)), resolves to `required`. |
| `required` | Sign-up sends a verification link. Sign-in is refused with 403 until the link is used. |
| `off` | Sign-up signs the new account in straight away. No verification email is sent, and the address is **not** marked verified. |

The default `auto` means:

- **A box that cannot send mail does not lock its users out.** With the `log`
  provider, verification is off, so sign-up works with no mail setup.
- **A box that can send mail verifies addresses.** Once you configure `smtp`,
  `resend` or `postmark`, new accounts must confirm their address.

You can force either behaviour:

- `EMAIL_VERIFICATION=required` with `EMAIL_PROVIDER=log` keeps verification
  on. Set `EMAIL_LOG_CONTENT=true` and the verification links appear in the
  backend log. This is useful when testing the full sign-up flow.
- `EMAIL_VERIFICATION=off` with a delivering provider lets anyone sign up with
  an address they do not control. Use it only on private or invite-only
  instances.

### What `off` does not vouch for

Off mode lets an account sign in without proving it owns its address. It does
not pretend the address was proven, because anyone could have registered it
first. An account whose email/password credential was never verified is
therefore:

- **not merged by Google sign-in.** When someone signs in with Google using
  the same address, Payverge drops the unproven password only if the account
  was never used: no session, linked wallet or other sign-in method, and no
  workspace except one a seller's onboarding link created together with the
  account. Any account that was used, which includes every off-mode sign-up
  since sign-up signs it in, gets a `409` instead. With verification `off`,
  keep signing in with the password and link Google from the account settings.
  With verification `required`, open the verification link sent to the
  address (signing in with the password sends a fresh one). After the address
  is verified, Google sign-in attaches to the account as usual.
- **not promoted by `DEFAULT_ADMIN_EMAIL`.** That legacy bootstrap never
  promotes an existing non-admin account, verified or not, and logs why. To
  make the address an administrator, set `ADMIN_EMAIL`/`ADMIN_PASSWORD` (it
  takes the account over: the password is replaced, other sign-in methods are
  unlinked and sessions are signed out; an account with a linked wallet is
  refused) or run `server admin create`. See [Administration](admin.md).

To verify an address on a box without mail delivery, request a link with
`POST /api/v1/auth/email/resend-verification` and the body
`{"email": "you@example.com"}`, then open the verification link from the
backend log (see [The `log` provider](#the-log-provider); this needs
`EMAIL_LOG_CONTENT=true`). This works in every mode. A password reset
(`POST /api/v1/auth/password/reset-request`, or "Forgot password" on the
sign-in page) also writes its link to the log when content logging is on.

### Changing the mode on a running instance

- **`required` to `off`**: accounts still waiting for verification can sign
  in with the correct password. They stay unverified; nothing is marked
  verified on their behalf. A wrong password is still refused with 401. A
  sign-in address left behind by an email change gets a 403.
- **`off` to `required`**: accounts created while verification was off were
  never verified, so they now need to verify too. Their next sign-in is
  refused with 403 and sends a fresh verification link; existing sessions of
  those accounts stop working until the link is used. Accounts that verified
  through a link keep working.

The mode applies to email/password accounts. Google sign-in, wallet sign-in
and staff PIN sign-in are not affected.

## The `log` provider

The `log` provider never opens a network connection. Each email becomes one
log line at `info` level. By default that line is metadata only: the template
name (when the message has one), the recipient count and a short hash of the
addresses, plus an attachment count when there are attachments. Recipients,
sender, subject, body and links are not logged.

Set `EMAIL_LOG_CONTENT=true` to also log recipients, subject, a body preview
and links. That is useful for local evaluation of verification, password-reset
and invite flows. In production, startup fails unless
`EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION=true` is set as well.

To read them:

```bash
docker compose logs backend | grep 'EMAIL_PROVIDER=log'
```

At startup the backend logs a warning that mail is not being delivered.

> **Note:** with `EMAIL_LOG_CONTENT=true` the log holds recipient addresses and
> one-time links. If you ship backend logs to a shared aggregator, leave
> content logging off, or treat those logs as secrets.

## SMTP

The `smtp` provider uses Go's `net/smtp` and works with any standard
submission server: a mailbox provider, Amazon SES, Mailgun, Postfix or a local
relay.

### SMTP transport security

| `SMTP_TLS` | Behaviour |
|---|---|
| `auto` or empty | Port `465` uses implicit TLS. Every other port requires STARTTLS, so a relay on `2525` or `25` that does not offer it fails instead of sending in cleartext. Only a loopback host (`localhost`, `127.0.0.1`, `::1`) on a port other than `587` upgrades just when offered. |
| `starttls` | Requires STARTTLS on any port. Sending fails if the server does not offer it. |
| `tls` | Implicit TLS (SMTPS) on any port. |
| `none` | Never upgrades. Use this only for a trusted relay on the same host or Docker network (for example a Postfix container on port `25` without TLS). |

Server certificates are always verified against the system trust store. If the
relay uses a self-signed certificate, set `SMTP_TLS=none` on a private network.
Custom certificate authorities are not supported yet.

With `SMTP_USERNAME` set, the provider authenticates with `PLAIN`, `LOGIN` or
`CRAM-MD5`, whichever the server offers, in that order. `net/smtp` refuses to
send `PLAIN` or `LOGIN` credentials over an unencrypted connection to any host
except loopback. Credentials over `SMTP_TLS=none` therefore work only against
`localhost`.

### Failures and retries

Emails go through a database outbox and are retried with backoff. The SMTP
reply code decides whether a failure is retried:

| Reply | Classified as | Retried? |
|---|---|---|
| connection error or timeout before the message is sent | `timeout` / `upstream` | yes |
| connection lost after the whole message was sent, before the final reply | `outcome_unknown` | no. The relay may have accepted it, and SMTP has no idempotency key, so a retry could deliver it twice. |
| `4xx` (for example `421`, `451`, `452`) | `upstream` | yes |
| `530`, `534`, `535`, `538` | `authentication` | no. Fix the credentials. |
| other `5xx` (for example `550`, `554`) | `rejected` | no |

Each send carries a `Message-ID` derived from its outbox idempotency key, so
the rare re-send after a crash between delivery and bookkeeping reuses the
same `Message-ID`. Some mailbox providers collapse such duplicates; Resend
deduplicates on the key itself.

## Bounces and suppression

Payverge keeps its own suppression list in the `email_suppressions` table. It
is checked before **every** send, whatever the provider. A suppressed address
is skipped without contacting the provider.

| Provider | How addresses reach the list |
|---|---|
| `resend` | Hard bounces, complaints and provider suppressions, delivered by the webhook at `POST /api/v1/webhooks/email/resend`. Set `RESEND_WEBHOOK_SECRET` to the signing secret from the Resend dashboard. |
| `postmark` | Not automatically. |
| `smtp` | Not automatically. A `5xx` refusal at send time is recorded as `rejected` and not retried. Bounces that arrive later go to the `SMTP_FROM` mailbox. |
| `log` | Not applicable. |

The webhook route exists only when the provider is `resend`. With any other
provider, `/api/v1/webhooks/email/resend` returns 404.

SMTP `550` refusals are deliberately not auto-suppressed: relays also use
`550` for policy refusals, such as an unverified sender, and those would
wrongly block valid recipients. To suppress an address by hand:

```bash
docker compose exec postgres psql -U payverge -d payverge -c \
  "INSERT INTO email_suppressions (email, reason, provider, suppressed_at, last_event_at)
   VALUES (lower('someone@example.com'), 'manual', 'smtp', now(), now())
   ON CONFLICT (email) DO NOTHING;"
```

To lift it, delete the row.

## Missing or invalid configuration

| Situation | Development | Production (`--production`, the compose default) |
|---|---|---|
| Nothing configured | `log` provider, warning | `log` provider, warning |
| Unknown `EMAIL_PROVIDER` or `EMAIL_VERIFICATION` value | startup fails | startup fails |
| `EMAIL_PROVIDER=smtp` without `SMTP_HOST` | falls back to `log`, warning; `auto` verification stays `required` | startup fails |
| `EMAIL_PROVIDER=resend` or `postmark` without a key | falls back to `log`, warning; `auto` verification stays `required` | startup fails |
| `EMAIL_API_KEY`, `SMTP_HOST`, `SMTP_USERNAME` or `SMTP_PASSWORD` set without `EMAIL_PROVIDER` | `log` provider, warning; `auto` verification is `required` | startup fails |
| `EMAIL_PROVIDER=smtp` with an invalid `SMTP_PORT` or `SMTP_TLS` | startup fails | startup fails |

An explicitly chosen provider never falls back silently to an unconfigured
API: an empty key never reaches Resend or Postmark.

## Testing delivery

The backend image ships a one-shot probe. It sends a single message through
the configured provider and exits with an error if the provider refuses it:

```bash
docker compose exec -e EMAIL_HEALTHCHECK_TO=you@example.com backend /app/email-smoke
```

It resolves the provider the same way the server does. It refuses to run with
the `log` provider, because nothing would be delivered. It sends from
`FROM_EMAIL`, falling back to `SMTP_FROM` for `smtp`. The bundled
`docker-compose.yml` passes `EMAIL_PROVIDER`, `FROM_EMAIL` and the provider
settings to the backend container as environment variables, so no extra `-e`
flags are needed.

## Recipes

### Gmail or Google Workspace

Create an [app password](https://support.google.com/accounts/answer/185833),
which requires two-step verification.

```env
EMAIL_PROVIDER=smtp
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=you@gmail.com
SMTP_PASSWORD=your-16-char-app-password
SMTP_FROM=Your Restaurant <you@gmail.com>
```

Gmail limits sending to roughly 500 messages a day on personal accounts. The
From address must be the account itself or a configured alias.

### Amazon SES

Use SES **SMTP credentials**, which differ from IAM access keys, and a
verified identity:

```env
EMAIL_PROVIDER=smtp
SMTP_HOST=email-smtp.eu-west-1.amazonaws.com
SMTP_PORT=587
SMTP_USERNAME=AKIA...
SMTP_PASSWORD=...
SMTP_FROM=noreply@your-domain.com
```

Mailgun, Postmark SMTP, SendGrid SMTP, Brevo, Fastmail and most other services
follow the same pattern on port 587.

### Catching all mail locally with Mailpit

For development, or to inspect what Payverge sends, run
[Mailpit](https://mailpit.axllent.org/) next to the stack, for example in a
`docker-compose.override.yml`:

```yaml
services:
  mailpit:
    image: axllent/mailpit
    ports:
      - "127.0.0.1:8025:8025"
```

Then set:

```env
EMAIL_PROVIDER=smtp
SMTP_HOST=mailpit
SMTP_PORT=1025
SMTP_TLS=none
SMTP_FROM=noreply@localhost
```

Open http://127.0.0.1:8025 to read every message. Under `auto`, verification
is now `required`, so the verification links arrive in Mailpit.

### Resend

```env
RESEND_API_KEY=re_...
RESEND_WEBHOOK_SECRET=whsec_...
FROM_EMAIL=noreply@your-domain.com
FROM_EMAIL_UPDATES=updates@your-domain.com
```

In the Resend dashboard, add a webhook pointing to
`https://<your API host>/api/v1/webhooks/email/resend`. Subscribe it to the
bounced, complained, suppressed, delivered, delivery-delayed and failed events.

## See also

- Every email variable, with defaults: [configuration.md](configuration.md#email).
- Emails that do not arrive: [troubleshooting.md](troubleshooting.md#signing-in).

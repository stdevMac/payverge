# Payverge Admin MCP

A small, dependency-free [Model Context Protocol](https://modelcontextprotocol.io) server for a Payverge
instance. It lets an AI agent (Claude Code, Claude Desktop, Codex, Cursor, or any MCP client) do two jobs:

- **Set up a restaurant.** 24 configuration tools cover the business profile and settings, the menu
  (categories, items, bulk JSON import), tables and their QR links, staff invitations and roles, payment
  plugin status, AI settings, reservation settings, opening hours, and instance status.
- **Diagnose a platform.** 9 operator tools read health, error logs, failed webhooks and fiscal jobs, and
  can acknowledge or requeue them.

Every tool calls an HTTP endpoint the backend already serves. Nothing here needs a backend change, a
database connection or shell access to the server. The server speaks JSON-RPC over stdio (MCP protocol
`2025-11-25`) and runs on Node 20.18 or newer, with no `npm install`.

For how this fits with the in-app AI and the coding-agent skills, see
[docs/agents/README.md](../../docs/agents/README.md).

## Quick start

```bash
# 1. Point at your instance and sign in as the restaurant owner (the password is read from env, never argv).
export PAYVERGE_API_BASE_URL=https://pos.example.com/api/v1
export PAYVERGE_OWNER_EMAIL=owner@pos.example.com
read -rs PAYVERGE_OWNER_PASSWORD && export PAYVERGE_OWNER_PASSWORD

# 2. Register it with Claude Code (user scope, so the secrets stay out of the repository).
claude mcp add --transport stdio --scope user \
  --env PAYVERGE_API_BASE_URL="$PAYVERGE_API_BASE_URL" \
  --env PAYVERGE_OWNER_EMAIL="$PAYVERGE_OWNER_EMAIL" \
  --env PAYVERGE_OWNER_PASSWORD="$PAYVERGE_OWNER_PASSWORD" \
  payverge -- node /absolute/path/to/payverge/tools/payverge-admin-mcp/bin/payverge-admin-mcp.mjs
```

Then ask the agent something like "check my Payverge instance and tell me what is left to set up". A
typical first session runs:

1. `payverge_instance_status`: is the server up, which features (AI, email, WhatsApp, ...) are on, and how
   this MCP server is authenticated. Needs no credentials.
2. `payverge_list_businesses`: which businesses the account can manage.
3. `payverge_get_setup_status`: the dashboard's first-run checklist and what is missing.
4. The relevant write tools, each first as a preview (`dry_run` defaults to `true`), then applied with
   `dry_run: false` once you agree with the preview.

The [configure-restaurant skill](../../.claude/skills/configure-restaurant/SKILL.md) walks an agent
through a full setup with these tools.

## Authentication

The backend has three auth surfaces, and the MCP server can hold credentials for two of them.

| Surface | Routes | Credential in this MCP server | Tools |
|---|---|---|---|
| Public | `GET /api/v1/instance`, `/health/live`, `/health/ready` | none | `payverge_instance_status` (and the provider check inside the AI tools) |
| Owner session | `/api/v1/inside/*` | `PAYVERGE_OWNER_EMAIL` + `PAYVERGE_OWNER_PASSWORD`, or `PAYVERGE_OWNER_TOKEN` | the 23 other configuration tools |
| Platform admin | `/api/v1/admin/*` | `PAYVERGE_ADMIN_MCP_TOKEN` | the 9 operator tools |

The tools that appear in `tools/list` follow what is configured. With no credentials, all 24
configuration tools are listed, and every one except `payverge_instance_status` answers with an
`owner_auth_not_configured` error that says what to set. The admin tools are listed only when an admin
token is set. In total you get 24 tools, or 33 with an admin token.

### Owner email and password (recommended)

Set `PAYVERGE_OWNER_EMAIL` and `PAYVERGE_OWNER_PASSWORD`; set both or neither. The server signs in with
`POST /api/v1/auth/login` on the first tool call that needs it, then works like this:

- **Re-login.** The backend issues a 15-minute JWT. Its refresh token is a browser cookie, so the MCP
  server signs in again when the JWT is within 60 seconds of expiry, or when the backend answers 401. A
  request is retried once with the new token.
- **One login at a time.** Concurrent tool calls share one sign-in.
- **No lockout loops.** Wrong credentials (401) and a lockout (429) stop all further sign-ins until you
  restart the MCP server, because every failed attempt counts toward the account lockout. The error says
  so and points at the admin CLI's `reset-password` command.
- **Unverified email.** An unverified address is reported as `owner_email_unverified`. The verification
  link the backend returns is never echoed.
- **No secrets in output.** Neither the password nor the token appears in any output or error. The email
  is shown redacted (`o***@example.com`).

The account must be the business owner. Staff accounts sign in with an emailed one-time code, which an
unattended process cannot complete.

### Session token (`PAYVERGE_OWNER_TOKEN`)

Paste a session JWT, for example from the browser's dashboard session. The token is used as is and never
refreshed, so it stops working after 15 minutes (`owner_token_expired`). It is meant for short, supervised
sessions. A **manager's** token works for most tools, with some limits:

- `UpdateBusiness` drops owner-only fields (AI settings, payout wallets), and `payverge_update_ai_settings`
  reports them in `skipped_fields`.
- Routes need the manager's RBAC permissions, listed in the catalogue below.

When both an email/password and a token are set, the email and password win.

## Platform admin token

The 9 operator tools read `/api/v1/admin/*`, which accepts a dedicated bearer token.

1. Generate the token:

   ```bash
   openssl rand -hex 32
   ```

2. On the backend, store only its SHA-256 hash (preferred). `docker-compose.yml` forwards all three
   variables:

   ```bash
   TOKEN="<openssl-token>"
   printf "%s" "$TOKEN" | shasum -a 256 | awk '{print $1}'
   ```

   ```env
   PAYVERGE_ADMIN_MCP_TOKEN_SHA256=<sha256-of-token>
   PAYVERGE_ADMIN_MCP_ALLOWED_IPS=127.0.0.1/32,::1/128
   ```

   For local development you can set the raw token on the backend instead
   (`PAYVERGE_ADMIN_MCP_TOKEN=<openssl-token>`).

3. Give the MCP server the raw token as `PAYVERGE_ADMIN_MCP_TOKEN`. `PAYVERGE_ADMIN_TOKEN`, a short-lived
   admin session JWT, is accepted as a fallback for local testing only.

`PAYVERGE_ADMIN_MCP_ALLOWED_IPS` takes comma-separated IPs or CIDRs and defaults to loopback only. The
backend answers 401 for a wrong token (`admin_unauthorized`) and 403 for a caller outside the allow-list
(`admin_forbidden`). To reach a remote API, add the public IP of the machine that runs this MCP server.

Behind a reverse proxy, the backend reads the real client address from `X-Forwarded-For` only when
the proxy connects from a range in `TRUSTED_PROXIES`. Unset, it trusts loopback only; the `deploy/`
compose file sets it for its bundled Caddy, and an explicit value may hold only private or loopback
CIDRs. The allow-list is only as reliable as that setting. A loopback address counts only for a direct
local connection with no forwarding headers, so a request relayed by a proxy never matches the
loopback default. Leave
`TRUSTED_PLATFORM` blank whatever `EDGE` is. With the default `EDGE=none` (no CDN) a non-empty value
fails production startup with `proxy.platform.unexpected`. With `EDGE=cloudflare` it fails with
`cloudflare.platform.unexpected`: Caddy checks Cloudflare's addresses and forwards the client IP, and
the backend trusts only Caddy. When you test against a port published by Docker, the backend may see
the Docker gateway address instead of `127.0.0.1`. In that case, add the address the backend actually
sees to `PAYVERGE_ADMIN_MCP_ALLOWED_IPS`.

## Least privilege

- **Scope to the businesses the agent should touch.** Set `PAYVERGE_MCP_BUSINESS_IDS=12,14`. Other
  businesses are then filtered out of `payverge_list_businesses`, every per-business tool refuses them
  with `business_not_allowed`, and `payverge_create_business` is refused. The two admin tools that read
  one business, `payverge_inspect_business` and `payverge_diagnose_issue` with a `businessId`, refuse
  them too, and need the numeric id. The allow-list is a guard in this MCP server, not a backend
  permission: the session or token it holds can still reach everything that account can.
- **Start read-only.** Set `PAYVERGE_MCP_READ_ONLY=true` to explore or audit. Previews still work, and
  every apply is refused with `read_only_mode`.
- **Use the narrowest account that does the job.** If the restaurant runs on its own owner account, use
  that account rather than one that owns many businesses. For short supervised changes, a manager's
  `PAYVERGE_OWNER_TOKEN` cannot touch AI settings or payout wallets and expires in 15 minutes.
- **Keep the platform admin token for diagnostics.** It is instance-wide. The allow-list covers only the
  two admin tools that read one business: the health snapshot, error logs, failed webhooks and fiscal
  jobs span every tenant, and the webhook and fiscal write tools act on any of them. Leave it unset
  unless you need it, keep the backend's
  `PAYVERGE_ADMIN_MCP_ALLOWED_IPS` on loopback or a single address, and store only the SHA-256 on the
  server.
- **Pass secrets through the environment.** Never pass them as command-line arguments, which show up in
  process listings and shell history. Never commit them: use user-scoped MCP config, or `${VAR}`
  references in a project `.mcp.json`, as shown below.
- **Know what a sign-in leaves behind.** Each sign-in creates a backend session: a 15-minute access JWT,
  which is the only token the MCP server keeps (in memory), plus a refresh token that is valid for 7 days.
  The refresh token arrives as a browser cookie, and the MCP server discards it. The session row stays
  until the refresh token expires, and the hourly cleanup removes it a day after that, so a long agent
  session leaves several rows for about 8 days. After removing the MCP server from a client, or if its
  config may have leaked, end every session for the account and change the password in one step:
  `/app/server admin reset-password --email ... --password-stdin`, run with
  `docker compose exec -T backend` (see `docs/self-hosting/admin.md`).
- **Nothing reads secrets.** No tool reads plugin credentials or prints payout wallet addresses (only
  the booleans `settlement_configured` and `tipping_configured`). Staff invitation links stay redacted
  unless you pass `reveal_invitation_url: true`.

## Environment

| Variable | Default | Purpose |
|---|---|---|
| `PAYVERGE_API_BASE_URL` | `http://localhost:8080/api/v1` | Backend API base, ending in `/api/v1`. |
| `PAYVERGE_PUBLIC_URL` | API base minus `/api/v1` | Guest-facing origin used to build table QR links (`<PUBLIC_URL>/t/<code>`). Set it when the frontend runs on a different origin, as in local dev (`http://localhost:3000`). |
| `PAYVERGE_OWNER_EMAIL` | | Owner sign-in email (with `PAYVERGE_OWNER_PASSWORD`). |
| `PAYVERGE_OWNER_PASSWORD` | | Owner password. |
| `PAYVERGE_OWNER_TOKEN` | | Alternative to email/password: a session JWT, used as is (15 minutes). |
| `PAYVERGE_ADMIN_MCP_TOKEN` | | Platform admin token for the operator tools. |
| `PAYVERGE_ADMIN_TOKEN` | | Fallback admin session JWT, for local testing only. |
| `PAYVERGE_MCP_READ_ONLY` | `false` | `true`/`1`/`yes`/`on` refuses every apply; previews still work. |
| `PAYVERGE_MCP_BUSINESS_IDS` | | Comma-separated business ids the configuration tools, `payverge_inspect_business` and `payverge_diagnose_issue` may touch. The other admin tools are instance-wide. The two admin tools check the same id they fetch: a call that names the business twice (`id` and `businessId`) with different values, or with a blank one, fails with `invalid_arguments`. |
| `PAYVERGE_MCP_ALLOW_INSECURE_HTTP` | `false` | Credentials (admin token, owner password or token) are only sent to an `https://` API base or a loopback host (`localhost`, `*.localhost`, `127.x`, `::1`). Set `true` to allow plain `http://` on a trusted private network, such as `http://backend:8080/api/v1` inside a Compose network. |
| `PAYVERGE_MCP_REQUEST_TIMEOUT_MS` | `30000` | Per-request timeout. |
| `PAYVERGE_MCP_WEBHOOK_SECRET` | | Webhook receiver only: shared secret. |
| `PAYVERGE_MCP_WEBHOOK_HOST` | `127.0.0.1` | Webhook receiver only: bind address. |
| `PAYVERGE_MCP_WEBHOOK_PORT` | `3977` | Webhook receiver only: port. |

## Conventions

- **Validated input.** Inputs are checked against each tool's JSON schema before any request is made.
  Unknown fields are rejected, and a bad input returns `invalid_arguments` with one line per problem.
- **Preview first.** Every write tool defaults to `dry_run: true` and returns the exact request it would
  send, a field-by-field `diff` where it applies, and `warnings`. Call it again with `dry_run: false` to
  apply. `dryRun` is accepted as an alias.
- **Destructive changes need `confirm`.** `payverge_import_menu` with `mode: "replace"` deletes every
  current category and item, and `payverge_set_opening_hours` with `mode: "replace"` clears every weekday
  you leave out (its preview lists them in `clears`), so both also need `confirm: true`. Without it, the
  call fails with `confirmation_required`.
- **Sanitization review.** When the backend would drop fields from a menu write (for example an unknown
  allergen or an out-of-range price), it saves nothing, and every menu tool (category and item writes and
  both import modes) fails with `sanitization_review_required`; `error.details` holds the backend's
  report. Fix the input, or pass `confirm_sanitization: true` to save without the dropped fields.
- **Optimistic locking.** Menu writes carry the menu `version`. On `MENU_VERSION_CONFLICT` the tool
  re-reads the menu and retries once; a second conflict returns `conflict`.
- **Units.** Prices are major units of the business currency (`12.5` is 12.50). Tax and service rates
  are percents (`21` is 21%). Weekdays are names (`monday` .. `sunday`); times are `HH:MM`, 24-hour.
- **Partial progress.** Bulk tools (`payverge_import_menu`, `payverge_bulk_create_tables`) report what
  was done before a failure in `partial`, so a retry can skip it.
- **Structured errors.** Errors come back as MCP tool errors (`isError: true`) with
  `structuredContent.error`:

  ```json
  {
    "error": {
      "code": "conflict",
      "message": "Menu was modified by another user. Please refresh and try again.",
      "hint": "The menu changed twice while writing. Re-read it with payverge_get_menu and retry.",
      "status": 409,
      "backend_code": "MENU_VERSION_CONFLICT",
      "request": "PUT /inside/businesses/12/menu/items",
      "request_id": "3f1c..."
    }
  }
  ```

  Search the backend logs for `request_id`: `docker compose logs backend | grep <request_id>`.

| Error code | Meaning |
|---|---|
| `invalid_arguments` | Input failed schema or semantic validation; `details` lists each problem. |
| `owner_auth_not_configured` | No owner credentials are set. |
| `owner_invalid_credentials`, `owner_login_locked` | Sign-in rejected or the account is locked; no retries until restart. |
| `owner_email_unverified`, `owner_login_forbidden` | Sign-in refused (unverified email, or a disabled account). |
| `owner_login_failed`, `owner_login_unreachable` | Server error or network failure during sign-in; retried on the next call. |
| `owner_token_expired`, `owner_token_rejected` | The static `PAYVERGE_OWNER_TOKEN` expired or was refused. |
| `business_not_allowed` | The business is outside `PAYVERGE_MCP_BUSINESS_IDS`, or an admin tool got a business id that is not numeric while the allow-list is set. |
| `read_only_mode` | `PAYVERGE_MCP_READ_ONLY` is set and the call tried to apply. |
| `confirmation_required` | A destructive change needs `confirm: true`. |
| `sanitization_review_required` | The backend would drop fields; see the details. |
| `not_found`, `ambiguous_category`, `ambiguous_item` | The referenced business, category, item or staff member does not exist or matches more than one row. |
| `import_stopped`, `bulk_create_stopped` | A bulk write failed part-way; see `partial`. |
| `invalid_request` (400), `unauthorized` (401), `ai_budget_exceeded`, `payment_required` (402), `business_suspended`, `business_closed`, `forbidden` (403), `conflict` (409), `rate_limited` (429), `ai_not_configured`, `runtime_control_disabled` (with `control`), `service_unavailable` (503), `backend_error` (5xx), `backend_unreachable` | HTTP failures from the backend, each with a hint. |
| `admin_auth_not_configured`, `admin_unauthorized`, `admin_forbidden` | Operator tools: no admin token, wrong token, or caller IP outside `PAYVERGE_ADMIN_MCP_ALLOWED_IPS`. |

## Configuration tools

All paths are under `/api/v1`. "Permission" is the RBAC permission a manager token needs; the owner
always has it.

| Tool | Writes | Endpoint(s) | Permission |
|---|---|---|---|
| `payverge_instance_status` | no | `GET /instance`, `/health/live`, `/health/ready` | public |
| `payverge_list_businesses` | no | `GET /inside/businesses` | owner or staff |
| `payverge_get_business_profile` | no | `GET /inside/businesses/:id` | owner or staff |
| `payverge_get_setup_status` | no | `GET /inside/businesses/:id/setup-status` | owner or staff |
| `payverge_create_business` | yes | `POST /inside/businesses` (with `Idempotency-Key`) | any signed-in account |
| `payverge_update_business_profile` | yes | `PUT /inside/businesses/:id` (partial; address merged) | `business:settings` |
| `payverge_get_ai_settings` | no | `GET /inside/businesses/:id`, `GET /instance` | owner or staff |
| `payverge_update_ai_settings` | yes | `PUT /inside/businesses/:id` (`ai_*` fields) | owner only |
| `payverge_get_menu` | no | `GET /inside/businesses/:id/menu` | `menu:read` |
| `payverge_add_menu_category` | yes | `POST /inside/businesses/:id/menu/categories` | `menu:categories` |
| `payverge_update_menu_category` | yes | `PUT /inside/businesses/:id/menu/category/:category_id` | `menu:categories` |
| `payverge_add_menu_item` | yes | `POST /inside/businesses/:id/menu/items` | `menu:items` |
| `payverge_update_menu_item` | yes | `PUT /inside/businesses/:id/menu/items` | `menu:items` |
| `payverge_import_menu` | yes (replace is destructive) | append: the two `POST` routes above, per row; replace: `POST /inside/businesses/:id/menu` | `menu:categories`, `menu:items`, `menu:write` |
| `payverge_list_tables` | no | `GET /inside/businesses/:id/tables` | `tables:read` |
| `payverge_bulk_create_tables` | yes | `POST /inside/businesses/:id/tables`, once per table | `tables:create` |
| `payverge_list_staff` | no | `GET /inside/businesses/:id/staff` | `staff:read` |
| `payverge_invite_staff` | yes | `POST /inside/businesses/:id/staff/invite` | `staff:invite` |
| `payverge_change_staff_role` | yes | `PUT /inside/businesses/:id/staff/:staffId/role` | `staff:role` |
| `payverge_payment_plugin_status` | no | `GET /inside/businesses/:id/plugins` (+ `GET /inside/plugins` for the catalogue) | `plugins:read` |
| `payverge_get_reservation_settings` | no | `GET /inside/businesses/:id/reservations/settings` | `reservations:read` |
| `payverge_update_reservation_settings` | yes | `PUT /inside/businesses/:id/reservations/settings` | `reservations:settings` |
| `payverge_get_opening_hours` | no | `GET /inside/businesses/:id/operating-hours` | `settings:read` |
| `payverge_set_opening_hours` | yes (replace is destructive) | `PUT /inside/businesses/:id/operating-hours` | `settings:write` |

Notes:

- `payverge_get_menu` returns both the stored `manual_available` flag and the effective `sellable_now`,
  which also accounts for inventory. `payverge_update_menu_item` writes `manual_available`.
- `payverge_update_menu_category` re-sends the category's items unchanged. The backend's category update
  has no `sort_order` field, so a category with a stored `sort_order` resets to 0. The tool warns when
  that will happen.
- `payverge_list_tables` and `payverge_bulk_create_tables` return each table's absolute `guest_url`
  (`<PAYVERGE_PUBLIC_URL>/t/<table_code>`). That is the link the printed QR code opens.
- `payverge_payment_plugin_status` reports installed and enabled plugins, their last status and error,
  and whether payments are ready. Connecting a provider (Stripe, MercadoPago, ...) needs its credentials
  or an OAuth flow, so it stays in the dashboard.
- AI tools report whether the server has an LLM provider (`features.ai` from `GET /instance`). Turning on
  the AI waiter for a business does nothing visible until a provider is configured; see
  [docs/agents/README.md](../../docs/agents/README.md#enable-the-in-app-ai).

## Operator tools (platform admin token)

| Tool | Writes | Endpoint(s) |
|---|---|---|
| `payverge_health_snapshot` | no | `GET /admin/system/health` (+ optional errors, failed webhooks, fiscal summary) |
| `payverge_list_error_logs` | no | `GET /admin/errors` |
| `payverge_list_failed_webhooks` | no | `GET /admin/webhooks/failed` |
| `payverge_ack_failed_webhook` | yes | `POST /admin/webhooks/:id/acknowledge` |
| `payverge_fiscal_summary` | no | `GET /admin/fiscal/summary` |
| `payverge_list_fiscal_jobs` | no | `GET /admin/fiscal/jobs` |
| `payverge_requeue_fiscal_job` | yes | `POST /admin/fiscal/jobs/:id/requeue` |
| `payverge_inspect_business` | no | `GET /admin/businesses/:id/detail` |
| `payverge_diagnose_issue` | no | the reads above, combined into one diagnosis with signals |

The write tools also default to a dry run (`dryRun: true`, alias `dry_run`) and honour
`PAYVERGE_MCP_READ_ONLY`. Applying either one needs `confirm: true` together with `dry_run: false`; acknowledging a webhook also needs a non-empty `reason`, and requeuing a `failed_permanent` job needs `allow_permanent: true`.

## Client configuration

Always use an absolute path to `bin/payverge-admin-mcp.mjs`. If `node` is not on the `PATH` the client
uses, give the absolute path to `node` as well.

### Claude Code

User scope keeps the credentials in your own Claude config, outside the repository:

```bash
claude mcp add --transport stdio --scope user \
  --env PAYVERGE_API_BASE_URL=https://pos.example.com/api/v1 \
  --env PAYVERGE_OWNER_EMAIL="$PAYVERGE_OWNER_EMAIL" \
  --env PAYVERGE_OWNER_PASSWORD="$PAYVERGE_OWNER_PASSWORD" \
  --env PAYVERGE_MCP_BUSINESS_IDS=12 \
  payverge -- node /absolute/path/to/payverge/tools/payverge-admin-mcp/bin/payverge-admin-mcp.mjs

claude mcp list   # payverge should show as connected
```

To share the server with a team, put a `.mcp.json` in **your own** project (not in the Payverge
repository; its root is a closed set of files). Claude Code expands `${VAR}` from each user's
environment, so no secret is committed:

```json
{
  "mcpServers": {
    "payverge": {
      "type": "stdio",
      "command": "node",
      "args": ["/absolute/path/to/payverge/tools/payverge-admin-mcp/bin/payverge-admin-mcp.mjs"],
      "env": {
        "PAYVERGE_API_BASE_URL": "${PAYVERGE_API_BASE_URL:-http://localhost:8080/api/v1}",
        "PAYVERGE_PUBLIC_URL": "${PAYVERGE_PUBLIC_URL:-}",
        "PAYVERGE_OWNER_EMAIL": "${PAYVERGE_OWNER_EMAIL}",
        "PAYVERGE_OWNER_PASSWORD": "${PAYVERGE_OWNER_PASSWORD}",
        "PAYVERGE_MCP_READ_ONLY": "${PAYVERGE_MCP_READ_ONLY:-false}"
      }
    }
  }
}
```

### Claude Desktop

Edit `claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/`, Windows:
`%APPDATA%\Claude\`) and restart Claude Desktop. This file stores the values in plain text, so restrict
its permissions (`chmod 600`) and prefer a read-only or business-scoped setup:

```json
{
  "mcpServers": {
    "payverge": {
      "command": "node",
      "args": ["/absolute/path/to/payverge/tools/payverge-admin-mcp/bin/payverge-admin-mcp.mjs"],
      "env": {
        "PAYVERGE_API_BASE_URL": "https://pos.example.com/api/v1",
        "PAYVERGE_OWNER_EMAIL": "owner@pos.example.com",
        "PAYVERGE_OWNER_PASSWORD": "<owner password>",
        "PAYVERGE_MCP_BUSINESS_IDS": "12"
      }
    }
  }
}
```

### Codex and other clients

```bash
codex mcp add payverge \
  --env PAYVERGE_API_BASE_URL=https://pos.example.com/api/v1 \
  --env PAYVERGE_OWNER_EMAIL="$PAYVERGE_OWNER_EMAIL" \
  --env PAYVERGE_OWNER_PASSWORD="$PAYVERGE_OWNER_PASSWORD" \
  -- node /absolute/path/to/payverge/tools/payverge-admin-mcp/bin/payverge-admin-mcp.mjs
```

Any MCP client that can launch a stdio server works the same way: command `node`, the absolute script
path as the only argument, and the variables above in its environment.

### Local development

A source checkout started from the repository root with
`docker compose --env-file .env up -d --build` publishes the backend on `http://127.0.0.1:8080` and the
frontend on `http://127.0.0.1:3000`, on loopback only:

```bash
cd tools/payverge-admin-mcp
PAYVERGE_API_BASE_URL=http://127.0.0.1:8080/api/v1 \
PAYVERGE_PUBLIC_URL=http://127.0.0.1:3000 \
PAYVERGE_OWNER_EMAIL=owner@example.test \
PAYVERGE_OWNER_PASSWORD="$PAYVERGE_OWNER_PASSWORD" \
node ./bin/payverge-admin-mcp.mjs
```

A `deploy/` local trial (`bash install.sh --domain localhost`, see "Local trial" in `deploy/README.md`)
serves everything through Caddy on one HTTPS origin. Use `PAYVERGE_API_BASE_URL=https://localhost/api/v1`
and `PAYVERGE_PUBLIC_URL=https://localhost` (add `:8443` if you passed `--https-port 8443`). Caddy signs
`localhost` with its own CA, so export that CA as the section describes and start the server with
`NODE_EXTRA_CA_CERTS=/path/to/payverge-local-ca.crt`.

## Webhook receiver

`bin/payverge-admin-webhooks.mjs` is a small HTTP receiver for local automations. When the backend or a
monitor flags an issue, it runs the same diagnosis as `payverge_diagnose_issue`. It needs the platform
admin token.

```bash
cd tools/payverge-admin-mcp
PAYVERGE_API_BASE_URL=https://pos.example.com/api/v1 \
PAYVERGE_ADMIN_MCP_TOKEN="$TOKEN" \
PAYVERGE_MCP_WEBHOOK_SECRET="$LOCAL_SHARED_SECRET" \
node ./bin/payverge-admin-webhooks.mjs
```

Endpoint:

```text
POST /webhooks/backend-flag
Header: x-payverge-mcp-secret: <PAYVERGE_MCP_WEBHOOK_SECRET>
```

Payload fields:

```json
{
  "type": "backend.flagged",
  "businessId": 42,
  "component": "checkout",
  "source": "backend",
  "severity": "warning",
  "message": "failed_webhooks queue is non-empty"
}
```

The response is `{ "ok": true, "diagnosis": ... }`. `PAYVERGE_MCP_BUSINESS_IDS` applies here too: a
`businessId` outside the allow-list is refused with 403 before any business is read.

## Verify locally

```bash
cd tools/payverge-admin-mcp
npm run check   # node --check on every source file
npm test        # unit tests against an in-memory mock of the Payverge API (test/mockBackend.mjs)
npm run smoke   # starts a fake API, launches the MCP server over stdio, exercises admin and config tools
```

None of these need `npm install` or a running backend. The smoke test checks that the owner session signs
in exactly once, and that a preview never writes.

## Adding a tool

1. Pick an existing endpoint. Wrap only what the backend already serves; if the endpoint you need does not
   exist, open an issue that names the route to add.
2. Add an entry to the `TOOLS` table in `src/configTools.mjs`. Every entry needs a closed `inputSchema`
   (`additionalProperties: false`) and annotations. Write tools also need the dry-run properties and
   `mutating: true`, and per-business tools need `business: true` so the allow-list applies.
3. Return a preview for `dry_run`, and throw `ToolError` with a `hint` for anything the caller can fix.
4. Add tests to `test/configTools.test.mjs` with `mockBackend()`. They should assert the requests that
   reach the backend, and that a dry run makes no writes.
5. If you add a source file, add it to the `check` script in `package.json`.

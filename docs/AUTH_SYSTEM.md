# Authentication

How people sign in to Payverge, what the server issues when they do, and how a
session ends. The operator settings (first admin, password resets, who may sign
up) are in [self-hosting/admin.md](self-hosting/admin.md); this page explains
the mechanism behind them.

## Who signs in

| Identity | Sign-in methods | Token `type` | Cookies | Middleware |
|---|---|---|---|---|
| Operator (restaurant owner, platform admin) | Email and password, Google, wallet (SIWE), the public demo's one-click login | `user` (email, Google, demo), `web3` (wallet) | `session_token`, `refresh_token` | `AuthenticationMiddleware`, `AuthenticationAdminMiddleware`, `HybridAuthenticationMiddleware` |
| Staff member of one business | Invitation link, emailed login code, Google | `staff` | `staff_token`, `refresh_token` | `StaffAuthenticationMiddleware`, `HybridAuthenticationMiddleware` |
| Guest with a customer account | Email and password | `customer` | `customer_token`, `customer_refresh_token` | `CustomerAuthenticationMiddleware` |

The `/api/v1/inside` routes use `HybridAuthenticationMiddleware`, which accepts
operator and staff tokens and rejects customer tokens. A staff token only works
for the business it was issued for. Permissions inside a business come from
RBAC (`RoleBasedAccessMiddleware`, `backend/internal/server/rbac.go`): the owner
holds every permission, and staff roles are `manager`, `server`, `host` and
`kitchen`.

## Tokens and cookies

- Access tokens are HS256 JWTs signed with `JWT_SECRET_KEY`, valid for 15
  minutes. The key must be at least 32 characters or the backend refuses to
  start; in production it also rejects known placeholder values.
- Every token carries a `type` claim, and each middleware accepts only its own
  types, so a customer or staff token cannot pass for an operator token even
  though all are signed with the same key. Every token also carries a
  `session_id` (see below).
- Cookies are `HttpOnly` and `Path=/`. They are `Secure` in production mode or
  when `PUBLIC_URL` starts with `https://`, and scoped to `COOKIE_DOMAIN` when
  that is set. All are `SameSite=Lax` except `customer_refresh_token`, which is
  `Strict`.
- An `Authorization: Bearer <token>` header is accepted too, and wins over a
  cookie.
- A state-changing request (anything but GET, HEAD, OPTIONS) that carries an
  auth cookie must come from a trusted origin (`PUBLIC_URL` or
  `ALLOWED_ORIGINS`); otherwise it gets `403 origin_not_allowed`
  (`middleware.RequireTrustedOriginForMutations`).

## Sessions, refresh and revocation

Every sign-in creates a row in `user_sessions` holding SHA-256 hashes of the
access token and of a random refresh token. The token's `session_id` points at
that row, and each authenticated request checks that the row exists, is not
revoked or expired, and matches the token. Signing out therefore takes effect
at once, not when the 15-minute token expires.

**Refresh.** The refresh cookie lasts 7 days. Operators and staff refresh with
`POST /api/v1/auth/refresh`, customers with `POST /api/v1/customer/refresh`.
Each refresh rotates the refresh token, issues a new access token and extends
the session by 24 hours. If a rotated-out refresh token comes back within 90
seconds (two tabs refreshing at once), the request gets `401
AUTH_REFRESH_ROTATED` and nothing is revoked. Later than that it is treated as
stolen: every session of that kind (operator, staff or customer) for the
account is revoked. The frontend refreshes every 12 minutes and after any 401,
with one refresh shared across tabs (`frontend/src/utils/refreshAuth.ts`).

**What ends sessions:**

| Event | Sessions revoked |
|---|---|
| Sign-out | That session |
| Password reset by email, or `admin reset-password` on the server | Every operator session of the account |
| `ADMIN_EMAIL` or `admin create` taking over an existing account | Every operator session of the account |
| Operator requests deletion of their account | Every operator session of the account |
| Customer deletes their account | Every customer session; the account is anonymised |
| Staff member removed, deactivated, or their role or permissions changed | Every staff session of that member; a token already issued fails on its next request (see [Staff](#staff)) |
| Platform admin demoted | Admin routes refuse the old token at once; every operator session of the account is revoked the next time it reaches a business route |
| An email or Google login no longer counts as verified (for example after `EMAIL_VERIFICATION` becomes `required`, or the email changes) | That session, on its next request |
| Refresh-token reuse (above) | Every session of that kind for the account |

An hourly job deletes session rows that were revoked or expired more than 24
hours earlier.

## Operators

### Email and password

`POST /api/v1/auth/register`, `/login` and `/logout`; `GET /api/v1/auth/me`.

- Passwords are hashed with bcrypt and need at least 8 characters. The
  platform admin password policy is stricter (see
  [admin.md](self-hosting/admin.md#1-first-admin)).
- After 5 failed logins for one email within 15 minutes, that email is locked
  for 1 minute; each further lockout doubles, up to 1 hour. The lockout lives
  in the `auth_attempts` table, so it holds across restarts and replicas.
- Each IP may register at most `REGISTRATION_RATE_LIMIT_PER_HOUR` accounts per
  hour (default 5).
- Email verification follows `EMAIL_VERIFICATION` (`auto`, `required` or
  `off`; see [email.md](self-hosting/email.md#verification-modes)). When it is
  required, login answers `403 requires_email_verification` until the address
  is verified.
- Verification links last 24 hours and password-reset links 1 hour. Both are
  single use, stored only as SHA-256 hashes in `user_auths`, and limited to 3
  per hour per email.

### Google

Set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` (both, or Google sign-in is
off). Register `<APP_BASE_URL or PUBLIC_URL>/api/v1/auth/google/callback` as the
redirect URI in the Google console.

- `GET /api/v1/auth/google` starts the flow. Its `state` is single use, expires
  after 10 minutes and must match the `oauth_state` cookie. The state is held
  in the backend process's memory, so with several replicas the callback must
  reach the replica that started the flow.
- Google must report the email as verified, except when a signed-in user is
  linking Google to their own account.
- Google's access token is used once to read the profile and is not stored.

### Wallet (Sign-In with Ethereum)

`POST /api/v1/auth/challenge` returns a nonce; the wallet signs an EIP-4361
message and `POST /api/v1/auth/signin` verifies it.

- The message's domain and URI must match the `PUBLIC_URL` host or an
  `ALLOWED_ORIGINS` entry. Nonces expire after 5 minutes and work once.
- A new address creates an operator account, subject to `REGISTRATION_MODE`.
- A signed-in operator can link a wallet with `POST /api/v1/auth/wallet/link`
  (the same SIWE checks) and remove it with `DELETE /api/v1/auth/wallet/unlink`.
- Challenges are tied to one backend process; see
  [admin.md](self-hosting/admin.md#wallet-sign-in-on-one-backend-replica) for
  what that means with several replicas.

### Public demo

`POST /api/v1/auth/demo/login` with `role` set to `owner`, `kitchen` or
`waiter` signs in to the seeded demo restaurant without an account. It answers
404 unless `DEMO_MODE` is on, never issues an admin session, and every demo
session stops working once `DEMO_MODE` is turned off.

### Platform admin

A platform admin is an operator whose `users.role` is `admin`.
`AuthenticationAdminMiddleware` reads the role from the database on every
request rather than trusting the token. The first admin comes from
`ADMIN_EMAIL` and `ADMIN_PASSWORD` or the `admin create` command; see
[admin.md](self-hosting/admin.md). The admin MCP server authenticates with its
own token, `PAYVERGE_ADMIN_MCP_TOKEN`, accepted only on an allowlisted set of
admin routes; see
[tools/payverge-admin-mcp/README.md](../tools/payverge-admin-mcp/README.md).

### Who may sign up

`REGISTRATION_MODE` applies to every way of creating an operator account
(email, Google and wallet):

| Value | Effect |
|---|---|
| `invite` (default) | A new account needs a valid invite code |
| `open` | Anyone may create an account |
| `closed` | No new accounts; existing users still sign in |

An unknown value stops a production server from starting and counts as
`closed` elsewhere. `DEMO_MODE` forces `closed`. The frontend reads the mode
from `GET /api/v1/platform/registration-mode`. Invites and the bootstrap admin
are covered in
[admin.md](self-hosting/admin.md#4-who-may-sign-up-registration_mode).

## Staff

- **Invitation.** An owner or a staff member with `staff:invite` invites by
  email. The link (`/staff/accept-invitation?token=…`) is valid for 7 days;
  accepting it creates the staff member and signs them in.
- **Login code.** `POST /api/v1/staff/request-login-code` emails a 6-digit code
  that is valid for 10 minutes; `POST /api/v1/staff/verify-login-code` signs in
  with it. At most 3 codes are sent per hour per address. Five wrong codes from
  one client for one email within 15 minutes block that client. Twenty failures
  for one email lock all of that email's staff memberships for 15 minutes and
  void the outstanding codes.
- **Google.** `GET /api/v1/auth/google/staff` signs in a staff member whose
  verified Google email matches their staff record.
- **Several businesses.** When one email is staff at more than one business,
  the code or Google step returns a single-use selection token that is valid
  for 5 minutes, and the person picks the business. Each staff token covers
  exactly one business.
- **Live checks.** A staff token carries the member's role and an
  `authz_version`. Every request reloads the staff row and rejects the token
  with 401 if the member was removed or deactivated, or if `authz_version`
  changed. Removing a member, deactivating them, or changing their role or
  permissions increments it.

`GET /api/v1/staff/profile` and `POST /api/v1/staff/logout` are the
staff-only routes.

## Guest customer accounts

`POST /api/v1/crm/register` creates a customer account (email, name, password
of at least 8 characters, bcrypt) without signing in; `POST /api/v1/crm/login`
signs in. `GET /api/v1/customer/session-info`, `POST /api/v1/customer/refresh`
and `POST /api/v1/customer/logout` manage the session.
`DELETE /api/v1/customer/account` anonymises the account and revokes its
sessions.

## Rate limits

| Limiter | Default | Settings | Covers |
|---|---|---|---|
| Auth | 10 requests per minute per IP, burst 3 | `AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE`, `AUTH_RATE_LIMIT_BURST` | Wallet challenge and sign-in, register, login, demo login, password reset, email verification, both refresh routes, customer register and login |
| Staff auth | 5 requests per minute per IP, burst 2 | `STAFF_AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE`, `STAFF_AUTH_RATE_LIMIT_BURST` | Invitation preview and acceptance, login-code request and verification |

## Where the code is

| Concern | Files |
|---|---|
| Routes | `backend/cmd/app/main.go` (`/api/v1/auth`, `/api/v1/staff`, `/api/v1/crm`, `/api/v1/customer`) |
| Email, Google, wallet linking, password reset, bootstrap admin, demo login | `backend/internal/auth/` |
| Token minting and verification | `backend/internal/server/jwt.go` |
| Middleware, live staff and admin checks | `backend/internal/server/middleware.go`, `live_user_role.go` |
| Wallet sign-in | `backend/internal/server/auth_handlers.go`, `siwe_message.go`, `backend/internal/logic/challenge.go` |
| Refresh | `backend/internal/server/refresh_handler.go` |
| Sessions and revocation | `backend/internal/session/store.go` |
| Staff invitations and login codes | `backend/internal/server/staff_handlers.go`, `backend/internal/auth/staff_oauth.go` |
| Customer accounts | `backend/internal/crm/` |
| Signup policy | `backend/internal/config/registration.go`, `backend/internal/config/email.go` |
| Cookies | `backend/internal/utils/cookie.go` |
| Frontend | `frontend/src/providers/HybridAuthProvider.tsx`, `frontend/src/contexts/CustomerAuthContext.tsx`, `frontend/src/utils/refreshAuth.ts` |

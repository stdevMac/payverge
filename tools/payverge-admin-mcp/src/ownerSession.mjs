// Owner session for the configuration tools.
//
// Business configuration routes live under /api/v1/inside/businesses/:id and
// require an owner (or manager) session JWT; the admin MCP token is only
// accepted on /api/v1/admin/*. The backend issues that JWT from
// POST /api/v1/auth/login {email, password}. It is valid for 15 minutes and
// its refresh token is a cookie that only browsers can use, so this module
// re-logs in when the JWT is about to expire or the backend answers 401.
//
// Safety rules:
//   - one login in flight at a time (concurrent tool calls share it);
//   - wrong credentials (401) and lockouts (429) are sticky: the session stops
//     retrying, because every failed attempt counts towards the per-account
//     lockout on the backend;
//   - the password and tokens never appear in errors.

import { MCP_SERVER_VERSION } from "./mcpProtocol.mjs";

const DEFAULT_TIMEOUT_MS = 30000;
const EXPIRY_SKEW_SECONDS = 60;

class OwnerSessionError extends Error {
  constructor(message, { code, status, hint, requestId } = {}) {
    super(message);
    this.name = "OwnerSessionError";
    this.code = code;
    this.status = status;
    this.hint = hint;
    this.requestId = requestId;
  }

  toJSON() {
    return {
      name: this.name,
      message: this.message,
      code: this.code,
      status: this.status,
      hint: this.hint,
      request_id: this.requestId,
    };
  }
}

export function decodeJwtExpiry(token) {
  const parts = String(token ?? "").split(".");
  if (parts.length !== 3) return undefined;
  try {
    const payload = JSON.parse(Buffer.from(parts[1], "base64url").toString("utf8"));
    return typeof payload.exp === "number" ? payload.exp : undefined;
  } catch {
    return undefined;
  }
}

/**
 * @param {object} options
 * @param {string} options.baseUrl  API base, e.g. https://pos.example.com/api/v1
 * @param {string} [options.email]
 * @param {string} [options.password]
 * @param {string} [options.staticToken]  PAYVERGE_OWNER_TOKEN (no re-login)
 */
export function createOwnerSession({
  baseUrl,
  email,
  password,
  staticToken,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  fetchImpl = globalThis.fetch,
  now = () => Date.now(),
} = {}) {
  if (!baseUrl) throw new Error("PAYVERGE_API_BASE_URL is required");
  const hasPassword = Boolean(email && password);
  if (!hasPassword && !staticToken) {
    throw new Error("owner auth needs PAYVERGE_OWNER_EMAIL + PAYVERGE_OWNER_PASSWORD, or PAYVERGE_OWNER_TOKEN");
  }
  const loginUrl = `${String(baseUrl).replace(/\/+$/, "")}/auth/login`;
  const mode = hasPassword ? "email_password" : "static_token";

  let token = hasPassword ? undefined : staticToken;
  let expiresAt = hasPassword ? undefined : decodeJwtExpiry(staticToken);
  let inflight;
  let fatal;
  let logins = 0;

  const nowSeconds = () => Math.floor(now() / 1000);
  const fresh = () => token && (expiresAt === undefined || expiresAt - EXPIRY_SKEW_SECONDS > nowSeconds());

  async function login() {
    logins += 1;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    let response;
    try {
      response = await fetchImpl(loginUrl, {
        method: "POST",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
          "user-agent": `payverge-admin-mcp/${MCP_SERVER_VERSION}`,
        },
        body: JSON.stringify({ email, password }),
        signal: controller.signal,
      });
    } catch (err) {
      throw new OwnerSessionError(
        err?.name === "AbortError"
          ? `owner login timed out after ${timeoutMs}ms`
          : `owner login request failed: ${err?.message ?? String(err)}`,
        { code: "owner_login_unreachable", hint: "Check PAYVERGE_API_BASE_URL and that the backend is up (GET /api/v1/health/live)." },
      );
    } finally {
      clearTimeout(timer);
    }

    let body = {};
    try {
      body = JSON.parse((await response.text()) || "{}");
    } catch {
      body = {};
    }
    const requestId = response.headers?.get?.("x-request-id") ?? undefined;

    if (response.ok && typeof body.token === "string" && body.token) {
      token = body.token;
      expiresAt = decodeJwtExpiry(body.token);
      return token;
    }

    // Never echo the backend body verbatim: a 403 for an unverified email can
    // carry a verification link.
    const backendMessage = typeof body.error === "string" ? body.error : undefined;
    if (response.status === 401) {
      fatal = new OwnerSessionError("owner login rejected: invalid email or password", {
        code: "owner_invalid_credentials",
        status: 401,
        requestId,
        hint: "Fix PAYVERGE_OWNER_EMAIL / PAYVERGE_OWNER_PASSWORD and restart the MCP server. The server does not retry, because repeated failures lock the account. An admin can reset the password with `docker compose exec -T backend /app/server admin reset-password --email <email> --password-stdin`.",
      });
      throw fatal;
    }
    if (response.status === 429) {
      fatal = new OwnerSessionError(`owner login rate-limited: ${backendMessage ?? "too many attempts"}`, {
        code: "owner_login_locked",
        status: 429,
        requestId,
        hint: "Wait for the lockout window to pass, then restart the MCP server.",
      });
      throw fatal;
    }
    if (response.status === 403) {
      const unverified = Boolean(body?.params?.requires_email_verification);
      fatal = new OwnerSessionError(
        unverified ? "owner login refused: email address not verified" : `owner login refused: ${backendMessage ?? "forbidden"}`,
        {
          code: unverified ? "owner_email_unverified" : "owner_login_forbidden",
          status: 403,
          requestId,
          hint: unverified
            ? "Verify the address from the email Payverge sent, or use the ADMIN_EMAIL bootstrap account, which is created verified."
            : "The account cannot sign in (deleted or blocked). Use another owner account.",
        },
      );
      throw fatal;
    }
    throw new OwnerSessionError(`owner login failed with ${response.status}${backendMessage ? `: ${backendMessage}` : ""}`, {
      code: "owner_login_failed",
      status: response.status,
      requestId,
      hint: "Check backend logs for this request id.",
    });
  }

  return {
    mode,

    async getToken() {
      if (fatal) throw fatal;
      if (fresh()) return token;
      if (!hasPassword) {
        throw new OwnerSessionError("PAYVERGE_OWNER_TOKEN has expired", {
          code: "owner_token_expired",
          hint: "Owner JWTs last 15 minutes. Prefer PAYVERGE_OWNER_EMAIL + PAYVERGE_OWNER_PASSWORD so the server can re-login.",
        });
      }
      if (!inflight) {
        inflight = login().finally(() => {
          inflight = undefined;
        });
      }
      return inflight;
    },

    invalidate(rejected) {
      if (!hasPassword) {
        fatal = new OwnerSessionError("PAYVERGE_OWNER_TOKEN was rejected by the backend", {
          code: "owner_token_rejected",
          status: 401,
          hint: "Mint a fresh token (sign in again) or switch to PAYVERGE_OWNER_EMAIL + PAYVERGE_OWNER_PASSWORD.",
        });
        return;
      }
      if (rejected === undefined || rejected === token) {
        token = undefined;
        expiresAt = undefined;
      }
    },

    describe() {
      return {
        mode,
        email: hasPassword ? redactEmail(email) : undefined,
        has_token: Boolean(token),
        expires_at: expiresAt ? new Date(expiresAt * 1000).toISOString() : undefined,
        logins,
        blocked: fatal ? fatal.code : undefined,
      };
    },
  };
}

export function redactEmail(value) {
  const [local, domain] = String(value ?? "").split("@");
  if (!domain) return "***";
  return `${local.slice(0, 1)}***@${domain}`;
}

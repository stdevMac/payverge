import { MCP_SERVER_VERSION } from "./mcpProtocol.mjs";

const DEFAULT_TIMEOUT_MS = 30000;
const USER_AGENT = `payverge-admin-mcp/${MCP_SERVER_VERSION}`;

export class PayvergeAdminClientError extends Error {
  constructor(message, { status, body, method, path, requestId, code } = {}) {
    super(message);
    this.name = "PayvergeAdminClientError";
    this.status = status;
    this.body = body;
    this.method = method;
    this.path = path;
    this.requestId = requestId;
    this.code = code ?? (body && typeof body === "object" ? body.code : undefined);
  }

  toJSON() {
    return {
      name: this.name,
      message: this.message,
      status: this.status,
      code: this.code,
      body: this.body,
      method: this.method,
      path: this.path,
      request_id: this.requestId,
    };
  }
}

/**
 * Thin JSON client for the Payverge HTTP API.
 *
 * Auth is one of:
 *   - token:          a static bearer token (admin MCP token, or an owner JWT);
 *   - tokenProvider:  { getToken(): Promise<string>, invalidate(token): void }
 *                     (the owner session); a 401 invalidates the token and the
 *                     request is retried once with a fresh one;
 *   - anonymous:true  for public routes (/instance, /health/*).
 *
 * Credentials never appear in thrown errors or their JSON form.
 */
export class PayvergeAdminClient {
  constructor({
    baseUrl,
    token,
    tokenProvider,
    anonymous = false,
    label = "Payverge admin API",
    timeoutMs = DEFAULT_TIMEOUT_MS,
    fetchImpl = globalThis.fetch,
  } = {}) {
    if (!baseUrl) {
      throw new Error("PAYVERGE_API_BASE_URL is required");
    }
    if (!token && !tokenProvider && !anonymous) {
      throw new Error("PAYVERGE_ADMIN_TOKEN is required");
    }
    if (typeof fetchImpl !== "function") {
      throw new Error("fetch implementation is required");
    }

    this.baseUrl = String(baseUrl).replace(/\/+$/, "");
    this.token = token;
    this.tokenProvider = tokenProvider;
    this.anonymous = Boolean(anonymous) && !token && !tokenProvider;
    this.label = label;
    this.timeoutMs = timeoutMs;
    this.fetchImpl = fetchImpl;
  }

  async get(path, query = {}) {
    return this.request("GET", path, { query });
  }

  async post(path, body = {}, options = {}) {
    return this.request("POST", path, { body, ...options });
  }

  async put(path, body = {}, options = {}) {
    return this.request("PUT", path, { body, ...options });
  }

  async patch(path, body = {}, options = {}) {
    return this.request("PATCH", path, { body, ...options });
  }

  async delete(path, body, options = {}) {
    return this.request("DELETE", path, { body, ...options });
  }

  async request(method, path, { query, body, headers: extraHeaders } = {}) {
    const token = await this.resolveToken();
    try {
      return await this.send(method, path, { query, body, extraHeaders, token });
    } catch (err) {
      if (
        err instanceof PayvergeAdminClientError &&
        err.status === 401 &&
        this.tokenProvider &&
        typeof this.tokenProvider.invalidate === "function"
      ) {
        // The owner JWT is short-lived (15 min); a 401 usually means it expired
        // between the exp check and the request. Re-login once, then give up.
        this.tokenProvider.invalidate(token);
        const fresh = await this.resolveToken();
        if (fresh && fresh !== token) {
          return this.send(method, path, { query, body, extraHeaders, token: fresh });
        }
      }
      throw err;
    }
  }

  async resolveToken() {
    if (this.anonymous) return undefined;
    if (this.tokenProvider) return this.tokenProvider.getToken();
    return this.token;
  }

  async send(method, path, { query, body, extraHeaders, token }) {
    const url = this.buildUrl(path, query);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const headers = {
        accept: "application/json",
        "user-agent": USER_AGENT,
        ...(extraHeaders ?? {}),
      };
      if (token) headers.authorization = `Bearer ${token}`;
      let requestBody;
      if (method !== "GET" && body !== undefined) {
        headers["content-type"] = "application/json";
        requestBody = JSON.stringify(body);
      }

      const response = await this.fetchImpl(url, {
        method,
        headers,
        body: requestBody,
        signal: controller.signal,
      });
      const parsed = await parseResponseBody(response);

      if (!response.ok) {
        throw new PayvergeAdminClientError(
          `${this.label} ${method} ${path} failed with ${response.status}`,
          {
            status: response.status,
            body: parsed,
            method,
            path,
            requestId: response.headers?.get?.("x-request-id") ?? undefined,
          },
        );
      }

      return parsed;
    } catch (err) {
      if (err instanceof PayvergeAdminClientError) {
        throw err;
      }
      if (err?.name === "AbortError") {
        throw new PayvergeAdminClientError(
          `${this.label} ${method} ${path} timed out after ${this.timeoutMs}ms`,
          { method, path },
        );
      }
      throw new PayvergeAdminClientError(
        `${this.label} ${method} ${path} request failed: ${err?.message ?? String(err)}`,
        { method, path },
      );
    } finally {
      clearTimeout(timer);
    }
  }

  buildUrl(path, query = {}) {
    const normalizedPath = path.startsWith("/") ? path : `/${path}`;
    const url = new URL(`${this.baseUrl}${normalizedPath}`);

    for (const [key, value] of Object.entries(query ?? {})) {
      if (value === undefined || value === null || value === "") continue;
      url.searchParams.set(key, String(value));
    }

    return url;
  }
}

async function parseResponseBody(response) {
  const text = await response.text();
  if (!text) return {};

  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) {
    return { body: text };
  }

  try {
    return JSON.parse(text);
  } catch {
    return { body: text };
  }
}

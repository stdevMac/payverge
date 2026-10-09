// In-memory Payverge API for tests: a fetch implementation that routes
// "METHOD /path" (relative to /api/v1) to handlers and records every request.
//
//   const backend = mockBackend({
//     "GET /inside/businesses": () => [{ id: 1 }],
//     "PUT /inside/businesses/1": (req) => ({ ...req.body }),
//     "GET /instance": { status: 404, body: { error: "nope" } },
//   });
//   createRuntime(config, { fetchImpl: backend.fetch });
//
// A handler may be a value (sent as a 200 JSON body), a function returning a
// value, or a function/value of the form { status, body, headers }. Handlers
// receive { method, path, query, headers, body } with body already parsed.

export const API_BASE = "http://payverge.localhost/api/v1";

export function mockBackend(routes = {}) {
  const table = { ...routes };
  const requests = [];

  async function fetchImpl(url, init = {}) {
    const parsed = new URL(String(url));
    const path = parsed.pathname.replace(/^\/api\/v1/, "");
    const method = (init.method ?? "GET").toUpperCase();
    const headers = normalizeHeaders(init.headers);
    let body;
    if (typeof init.body === "string" && init.body.length > 0) {
      body = JSON.parse(init.body);
    }
    const request = { method, path, query: Object.fromEntries(parsed.searchParams), headers, body };
    requests.push(request);

    const key = `${method} ${path}`;
    if (!(key in table)) {
      return jsonResponse(404, { error: `mock backend has no route for ${key}` });
    }
    let handler = table[key];
    if (Array.isArray(handler) && handler.queue === true) {
      handler = handler.length > 1 ? handler.shift() : handler[0];
    }
    const produced = typeof handler === "function" ? await handler(request) : handler;
    if (produced && typeof produced === "object" && produced.__response === true) {
      return jsonResponse(produced.status, produced.body, produced.headers);
    }
    return jsonResponse(200, produced);
  }

  return {
    fetch: fetchImpl,
    requests,
    routes: table,
    /** Requests that reached the backend, as "METHOD /path" strings. */
    calls() {
      return requests.map((request) => `${request.method} ${request.path}`);
    },
    writes() {
      return requests.filter((request) => request.method !== "GET" && request.path !== "/auth/login");
    },
    set(key, handler) {
      table[key] = handler;
    },
  };
}

/** A non-200 (or header-carrying) reply from a route handler. */
export function reply(status, body = {}, headers = {}) {
  return { __response: true, status, body, headers };
}

/** Successive answers for one route: the last one repeats. */
export function sequence(...handlers) {
  const queue = [...handlers];
  queue.queue = true;
  return queue;
}

/** A JWT-shaped token whose payload carries exp (seconds since epoch). */
export function fakeJwt(exp, extra = {}) {
  const encode = (value) => Buffer.from(JSON.stringify(value)).toString("base64url");
  return `${encode({ alg: "HS256", typ: "JWT" })}.${encode({ exp, ...extra })}.signature`;
}

export function loginRoute({ email = "owner@example.test", password = "correct horse", ttlSeconds = 900, now = () => Date.now() } = {}) {
  let issued = 0;
  return (request) => {
    if (request.body?.email !== email || request.body?.password !== password) {
      return reply(401, { error: "Invalid email or password" }, { "x-request-id": "req-login-401" });
    }
    issued += 1;
    return { token: fakeJwt(Math.floor(now() / 1000) + ttlSeconds, { n: issued }), user: { email } };
  };
}

function normalizeHeaders(headers) {
  const out = {};
  for (const [key, value] of Object.entries(headers ?? {})) out[key.toLowerCase()] = value;
  return out;
}

function jsonResponse(status, body, headers = {}) {
  const text = body === undefined ? "" : JSON.stringify(body);
  const lower = { "content-type": "application/json" };
  for (const [key, value] of Object.entries(headers)) lower[key.toLowerCase()] = value;
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (name) => lower[String(name).toLowerCase()] ?? null },
    text: async () => text,
  };
}

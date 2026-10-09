/**
 * Same-origin reverse proxy from the Next.js server to the Go backend.
 *
 * Self-hosted installs publish ONE origin (PUBLIC_URL): the browser calls
 * `${PUBLIC_URL}/api/v1/*` and loads `${PUBLIC_URL}/media/*`, and the Next
 * server forwards those to BACKEND_INTERNAL_URL on the private network. That
 * removes CORS, a second TLS hostname and cookie-domain configuration from a
 * fresh install. When BACKEND_INTERNAL_URL is unset the proxy is off (404),
 * so deployments that route /api at the edge (Caddy) are unaffected.
 *
 * Contract (see docs/self-hosting/frontend-config.md):
 *   - every method; bodies stream both ways, nothing is buffered (SSE works);
 *   - request bodies are capped at 15 MB (413), upstream calls at 300 s (504),
 *     except text/event-stream responses, which may stay open indefinitely;
 *   - cookies, Authorization and all end-to-end headers pass through;
 *     X-Request-Id is forwarded (an id is minted when the client sent none or
 *     an unsafe one);
 *   - client identity is the proxy's call, not the client's: X-Forwarded-For
 *     is the stamped socket peer (peerStamp.ts), and inbound client-IP
 *     headers are dropped, unless that peer is in FRONTEND_TRUSTED_PROXIES
 *     (then the edge's chain is kept and the peer appended);
 *   - hop-by-hop headers (RFC 9110 §7.6.1) are stripped in both directions;
 *   - redirects are never followed; a Location pointing at the private
 *     backend origin is rewritten to a same-origin relative path;
 *   - the upstream URL must keep the backend origin and the route prefix, so
 *     a crafted path can never make the proxy call another host.
 */

import { PEER_STAMP_HEADER, readPeerStamp } from "./peerStamp";
import { isTrustedProxy } from "./trustedProxies";

const MAX_PROXY_BODY_BYTES = 15 * 1024 * 1024;
const PROXY_TIMEOUT_MS = 300_000;

const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "proxy-connection",
  "te",
  "trailer",
  "trailers",
  "transfer-encoding",
  "upgrade",
]);

/**
 * Headers that name the client's address. Gin reads X-Forwarded-For and
 * X-Real-IP (and CF-Connecting-IP under TRUSTED_PLATFORM); the rest are
 * dropped too so no backend or plugin ever sees a client-chosen value.
 */
const CLIENT_IDENTITY_HEADERS = [
  "x-forwarded-for",
  "x-real-ip",
  "forwarded",
  "cf-connecting-ip",
  "true-client-ip",
  "x-client-ip",
  "x-cluster-client-ip",
  "fastly-client-ip",
] as const;

/** Request headers the proxy owns or that must not reach the backend. */
const REQUEST_DROP = new Set<string>([
  "host",
  "accept-encoding",
  ...CLIENT_IDENTITY_HEADERS,
  "x-forwarded-proto",
  "x-forwarded-host",
  "x-forwarded-port",
  "x-request-id",
  "expect",
  PEER_STAMP_HEADER,
]);

/** Next.js internal request headers; meaningless (or risky) upstream. */
const NEXT_INTERNAL_PREFIXES = ["x-middleware-", "x-invoke-", "x-nextjs-"];

const NULL_BODY_STATUSES = new Set([101, 103, 204, 205, 304]);
const BODYLESS_METHODS = new Set(["GET", "HEAD"]);
const REQUEST_ID_RE = /^[A-Za-z0-9._:-]{1,128}$/;

type EnvLike = Readonly<Record<string, string | undefined>>;

export interface ProxyOptions {
  /** Path prefix this route serves, e.g. "/api/v1/" or "/media/". */
  readonly prefix: string;
  /** Methods the route accepts; others get 405. Default: all. */
  readonly methods?: readonly string[];
  readonly env?: EnvLike;
  readonly maxBodyBytes?: number;
  readonly timeoutMs?: number;
  /** Injected for tests. */
  readonly fetchImpl?: typeof fetch;
}

/** Parsed BACKEND_INTERNAL_URL, or null when the proxy is disabled. */
export function backendTarget(env: EnvLike = process.env): URL | null {
  const raw = (env.BACKEND_INTERNAL_URL ?? "").trim();
  if (!raw) return null;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return null;
  if (url.username || url.password || url.search || url.hash) return null;
  return url;
}

function json(status: number, error: string, extra?: HeadersInit): Response {
  const headers = new Headers(extra);
  headers.set("content-type", "application/json; charset=utf-8");
  headers.set("cache-control", "no-store");
  return new Response(JSON.stringify({ error }), { status, headers });
}

function connectionTokens(headers: Headers): Set<string> {
  const tokens = new Set<string>();
  for (const token of (headers.get("connection") ?? "").split(",")) {
    const name = token.trim().toLowerCase();
    if (name) tokens.add(name);
  }
  return tokens;
}

function isHopByHop(name: string, named: Set<string>): boolean {
  return HOP_BY_HOP.has(name) || named.has(name);
}

function firstToken(value: string | null): string {
  return (value ?? "").split(",")[0].trim();
}

/** Append the edge's own address to its X-Forwarded-For chain. */
function appendHop(chain: string | null, peer: string): string {
  const hops = (chain ?? "")
    .split(",")
    .map((hop) => hop.trim())
    .filter(Boolean);
  // Next fills a missing X-Forwarded-For with this same peer (`??=`).
  if (hops.length === 0 || (hops.length === 1 && hops[0] === peer)) return peer;
  return `${hops.join(", ")}, ${peer}`;
}

/** Build the headers sent to the backend. */
function buildUpstreamRequestHeaders(
  incoming: Headers,
  requestUrl: URL,
  env: EnvLike = process.env,
): Headers {
  const named = connectionTokens(incoming);
  const out = new Headers();
  incoming.forEach((value, rawName) => {
    const name = rawName.toLowerCase();
    if (isHopByHop(name, named) || REQUEST_DROP.has(name)) return;
    if (NEXT_INTERNAL_PREFIXES.some((prefix) => name.startsWith(prefix))) return;
    out.append(name, value);
  });

  // Client identity. In a route handler a client-sent X-Forwarded-For and the
  // one Next fills from the socket (`??=`) look alike, so only the stamped
  // socket peer counts. A peer in FRONTEND_TRUSTED_PROXIES is an edge that
  // owns these headers; anyone else gets exactly its socket address. Without
  // a stamp (boot, internal image-optimizer fetches) nothing is forwarded and
  // the backend sees the frontend's own address.
  const peer = readPeerStamp(incoming);
  const edge = peer !== null && isTrustedProxy(peer.address, env);
  if (edge) {
    for (const name of CLIENT_IDENTITY_HEADERS) {
      const value = incoming.get(name);
      if (value) out.set(name, value);
    }
    out.set(
      "x-forwarded-for",
      appendHop(incoming.get("x-forwarded-for"), peer.address),
    );
  } else if (peer) {
    out.set("x-forwarded-for", peer.address);
  }

  // Not request.url's scheme: the Next server never terminates TLS, so Next
  // can only have taken an https there from a client X-Forwarded-Proto.
  const proto =
    (edge ? firstToken(incoming.get("x-forwarded-proto")) : "") ||
    peer?.protocol ||
    "http";
  out.set("x-forwarded-proto", proto);

  const host =
    (edge ? firstToken(incoming.get("x-forwarded-host")) : "") ||
    incoming.get("host") ||
    requestUrl.host;
  out.set("x-forwarded-host", host);

  const requestId = (incoming.get("x-request-id") ?? "").trim();
  out.set(
    "x-request-id",
    REQUEST_ID_RE.test(requestId) ? requestId : crypto.randomUUID(),
  );

  // undici transparently decodes compressed bodies but keeps the
  // Content-Encoding header; ask for identity and let Next compress (or not)
  // for the client.
  out.set("accept-encoding", "identity");
  return out;
}

/** Build the headers returned to the browser. */
function buildDownstreamResponseHeaders(
  upstream: Response,
  backend: URL,
): Headers {
  const named = connectionTokens(upstream.headers);
  const out = new Headers();
  upstream.headers.forEach((value, rawName) => {
    const name = rawName.toLowerCase();
    if (isHopByHop(name, named) || name === "set-cookie") return;
    out.append(name, value);
  });
  for (const cookie of upstream.headers.getSetCookie()) {
    out.append("set-cookie", cookie);
  }

  // The body we hand on is already decoded by fetch.
  if (out.has("content-encoding")) {
    out.delete("content-encoding");
    out.delete("content-length");
  }

  const location = out.get("location");
  if (location) {
    try {
      const target = new URL(location, backend);
      if (target.origin === backend.origin) {
        out.set("location", `${target.pathname}${target.search}${target.hash}`);
      }
    } catch {
      // leave unparsable Location untouched
    }
  }

  if ((out.get("content-type") ?? "").toLowerCase().startsWith("text/event-stream")) {
    // `no-transform` keeps Next's gzip (and most edges) from buffering events.
    out.set("cache-control", "no-cache, no-transform");
    out.set("x-accel-buffering", "no");
    out.delete("content-length");
  }
  return out;
}

/**
 * Resolve the upstream URL for a request, or null when the path is outside
 * the route prefix or would leave the backend origin.
 */
export function upstreamUrlFor(
  requestUrl: URL,
  backend: URL,
  prefix: string,
): URL | null {
  const pathname = requestUrl.pathname;
  if (!pathname.startsWith(prefix)) return null;
  const base = backend.href.replace(/\/+$/, "");
  let upstream: URL;
  try {
    upstream = new URL(`${base}${pathname}${requestUrl.search}`);
  } catch {
    return null;
  }
  if (upstream.origin !== backend.origin) return null;
  const basePath = backend.pathname.replace(/\/+$/, "");
  if (!upstream.pathname.startsWith(`${basePath}${prefix}`)) return null;
  return upstream;
}

class BodyTooLargeError extends Error {
  constructor() {
    super("request body too large");
    this.name = "BodyTooLargeError";
  }
}

function limitBody(
  body: ReadableStream<Uint8Array>,
  maxBytes: number,
  onTooLarge: () => void,
): ReadableStream<Uint8Array> {
  let seen = 0;
  return body.pipeThrough(
    new TransformStream<Uint8Array, Uint8Array>({
      transform(chunk, controller) {
        seen += chunk.byteLength;
        if (seen > maxBytes) {
          onTooLarge();
          controller.error(new BodyTooLargeError());
          return;
        }
        controller.enqueue(chunk);
      },
    }),
  );
}

/** Pass the upstream body through, clearing the timeout once it is done. */
function releaseOnEnd(
  body: ReadableStream<Uint8Array>,
  release: () => void,
): ReadableStream<Uint8Array> {
  return body.pipeThrough(
    new TransformStream<Uint8Array, Uint8Array>({
      flush() {
        release();
      },
    }),
  );
}

function anySignal(signals: AbortSignal[]): AbortSignal {
  const live = signals.filter(Boolean);
  if (typeof AbortSignal.any === "function") return AbortSignal.any(live);
  const controller = new AbortController();
  for (const signal of live) {
    if (signal.aborted) {
      controller.abort(signal.reason);
      break;
    }
    signal.addEventListener("abort", () => controller.abort(signal.reason), {
      once: true,
    });
  }
  return controller.signal;
}

/** Forward one request to the backend and return the streamed response. */
export async function proxyToBackend(
  request: Request,
  options: ProxyOptions,
): Promise<Response> {
  const backend = backendTarget(options.env);
  if (!backend) return json(404, "Not found");

  const method = request.method.toUpperCase();
  if (options.methods && !options.methods.includes(method)) {
    return json(405, "Method not allowed", { allow: options.methods.join(", ") });
  }

  const requestUrl = new URL(request.url);
  const upstreamUrl = upstreamUrlFor(requestUrl, backend, options.prefix);
  if (!upstreamUrl) return json(404, "Not found");

  const maxBytes = options.maxBodyBytes ?? MAX_PROXY_BODY_BYTES;
  const declared = request.headers.get("content-length");
  if (declared !== null) {
    const length = Number(declared);
    if (!Number.isFinite(length) || length < 0) {
      return json(400, "Invalid Content-Length");
    }
    if (length > maxBytes) return json(413, "Request body too large");
  }

  let tooLarge = false;
  let body: ReadableStream<Uint8Array> | undefined;
  if (!BODYLESS_METHODS.has(method) && request.body) {
    body =
      declared === null
        ? limitBody(request.body, maxBytes, () => {
            tooLarge = true;
          })
        : request.body;
  }

  const timeout = new AbortController();
  const timer = setTimeout(
    () => timeout.abort(new Error("upstream timeout")),
    options.timeoutMs ?? PROXY_TIMEOUT_MS,
  );
  // A client that abandons a body leaves the timer armed; never let that keep
  // the process alive (it only aborts an already-finished call).
  (timer as { unref?: () => void }).unref?.();
  const release = () => clearTimeout(timer);
  const signal = anySignal(
    request.signal ? [request.signal, timeout.signal] : [timeout.signal],
  );

  const init: RequestInit & { duplex?: "half" } = {
    method,
    headers: buildUpstreamRequestHeaders(request.headers, requestUrl, options.env),
    redirect: "manual",
    cache: "no-store",
    signal,
  };
  if (body) {
    init.body = body;
    init.duplex = "half";
  }

  let upstream: Response;
  try {
    upstream = await (options.fetchImpl ?? fetch)(upstreamUrl, init);
  } catch {
    release();
    if (tooLarge) return json(413, "Request body too large");
    if (timeout.signal.aborted) return json(504, "Upstream timeout");
    return json(502, "Bad gateway");
  }

  const headers = buildDownstreamResponseHeaders(upstream, backend);
  const isEventStream = (headers.get("content-type") ?? "")
    .toLowerCase()
    .startsWith("text/event-stream");

  if (
    method === "HEAD" ||
    NULL_BODY_STATUSES.has(upstream.status) ||
    !upstream.body
  ) {
    release();
    return new Response(null, {
      status: upstream.status,
      statusText: upstream.statusText,
      headers,
    });
  }

  // Server-sent events legitimately outlive any request timeout; they end
  // when either side disconnects (request.signal aborts the upstream call).
  if (isEventStream) release();

  return new Response(
    isEventStream ? upstream.body : releaseOnEnd(upstream.body, release),
    { status: upstream.status, statusText: upstream.statusText, headers },
  );
}

/** Route handlers for every method, bound to one prefix. */
export function createProxyHandlers(options: ProxyOptions) {
  const handler = (request: Request) => proxyToBackend(request, options);
  return {
    GET: handler,
    HEAD: handler,
    POST: handler,
    PUT: handler,
    PATCH: handler,
    DELETE: handler,
    OPTIONS: handler,
  };
}

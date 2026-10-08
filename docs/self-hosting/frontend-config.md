# Frontend runtime configuration

One frontend image serves any origin. The Next.js server reads its settings
from the **container environment at request time**, and the browser gets the
public subset through `window.__PAYVERGE_ENV__`. Changing `PUBLIC_URL`, the
API location, Sentry, analytics or media hosts needs a restart, not a rebuild.

Source of truth: `frontend/src/config/publicConfig.ts` (browser-visible
values) and `frontend/src/config/serverConfig.ts` (server-only values).

## How values reach the browser

1. On the server, `getPublicConfig()` resolves each key per call: the
   canonical runtime name first, then the legacy aliases, then the value
   inlined at build time (old-style images only), then the default.
2. The root layout (`src/app/layout.tsx`) serializes the resolved values into
   a nonce'd inline script, `window.__PAYVERGE_ENV__ = Object.freeze({...})`.
   It is the first body script, so it runs before any page client module
   evaluates: those chunks are required from flight data that streams after
   it. Then it dispatches a `payverge:public-env` event on `window`.
   - `instrumentation-client.ts` is the exception. Next evaluates it from its
     async main chunk, which can run before the parser reaches the config
     script. It therefore initializes Sentry through `onPublicEnvReady()`:
     immediately when the object is present, otherwise on the ready event, or
     at the latest on `DOMContentLoaded` (build-time fallback values).
   - Anything else that must run that early has to use `onPublicEnvReady()`
     too; a module-level `getPublicConfig()` there would see build-time
     defaults.
   - The payload is built by iterating the whitelist (`PUBLIC_ENV_KEYS`), so
     no other environment variable can leak into the page.
   - JSON is escaped against `</script>`, `&` and the JS line separators.
3. In the browser, `getPublicConfig()` reads that object and ignores any key
   that is not on the whitelist.

Invalid values never crash the server: they fall back to defaults. At
startup, `src/instrumentation.ts` logs each problem by key and rule
(`PUBLIC_URL: missing`, `API_URL: HTTPS required in production`), never the
value.

**Lint guard.** A `no-restricted-syntax` rule in `frontend/eslint.config.mjs`
forbids literal `process.env.NEXT_PUBLIC_*` reads outside `publicConfig.ts`.
The four build-time keys listed below are the only exceptions.

## Classification of `NEXT_PUBLIC_*`

### Runtime

Set these on the frontend container. The legacy `NEXT_PUBLIC_*` names still
work as runtime aliases, and as build-time fallbacks for images built the old
way.

| Key (browser) | Runtime env names, first non-empty wins | Default | Notes |
|---|---|---|---|
| `PUBLIC_URL` | `PUBLIC_URL`, `NEXT_PUBLIC_PUBLIC_URL` | `http://localhost:3000` | Canonical origin. Used for canonicals, hreflang, sitemap, robots, feeds, JSON-LD, QR/table links, storefront links and security.txt. Only the origin is kept; a path or query is ignored, and the startup log warns about it. When unset or invalid, the server uses the default and the browser uses the page's own origin. Client components that render it use `useSiteUrl()` (`src/hooks/useSiteUrl.ts`), which hydrates with the server's value and then switches, so the two never mismatch. |
| `API_URL` | `API_URL`, `NEXT_PUBLIC_API_URL` | `/api/v1` (same origin) in production or when `BACKEND_INTERNAL_URL` is set; `http://localhost:8080/api/v1` in `next dev` | Leave it unset unless the API really lives on another origin. |
| `SUPPORT_EMAIL` | `SUPPORT_EMAIL`, `NEXT_PUBLIC_SUPPORT_EMAIL` | empty | No vendor fallback. Feeds the contact mailbox in `src/config/brand.ts` (contact page, registration-success help). When unset, contact affordances are hidden. |
| `NETWORK` | `NETWORK`, `NEXT_PUBLIC_NETWORK` | `baseSepolia` | `base` or `baseSepolia`. |
| `RPC_URL` | `PUBLIC_RPC_URL`, `NEXT_PUBLIC_RPC_URL` | public RPC for the chain | Deliberately **not** `RPC_URL`: the backend's `RPC_URL` may embed a paid provider key. |
| `MAINTENANCE_MODE` | `MAINTENANCE_MODE`, `NEXT_PUBLIC_MAINTENANCE_MODE` | `false` | |
| `VAPID_PUBLIC_KEY` | `VAPID_PUBLIC_KEY`, `NEXT_PUBLIC_VAPID_PUBLIC_KEY` | empty | Web-push public key (public by design). |
| `MEDIA_ORIGINS` | `MEDIA_ORIGINS`, `NEXT_PUBLIC_MEDIA_ORIGINS` | empty | Comma-separated https origins for uploads served from S3/CDN. Feeds the CSP `img-src`, AI-image trust and the marketing media proxy allowlist. |
| `LOGO_URL` | `LOGO_URL` (server-only) | empty | The same value the backend serves as `/api/v1/instance` `logo_url`. The frontend reads it only to add the logo's origin to the CSP `img-src`; give the frontend container the same value as the backend. |
| `SENTRY_DSN` | `FRONTEND_SENTRY_DSN`, `NEXT_PUBLIC_SENTRY_DSN` | empty (off) | `FRONTEND_` prefix so the backend's `SENTRY_DSN` is never published. |
| `SENTRY_ENVIRONMENT`, `SENTRY_ENABLED`, `SENTRY_TRACES_SAMPLE_RATE`, `SENTRY_REPLAYS_SESSION_SAMPLE_RATE`, `SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE` | `FRONTEND_SENTRY_*`, `NEXT_PUBLIC_SENTRY_*` | empty | |
| `POSTHOG_KEY`, `POSTHOG_HOST` | `POSTHOG_KEY`/`POSTHOG_HOST`, `NEXT_PUBLIC_POSTHOG_*` | empty | Plumbed through config and the CSP. The PostHog client itself is not wired yet (`src/lib/analytics/consentGate.ts`). |
| `LIFI_INTEGRATOR` | `LIFI_INTEGRATOR`, `NEXT_PUBLIC_LIFI_INTEGRATOR` | `Payverge` | Cross-chain widget integrator id. |

`APP_BASE_URL` is **not** an alias of `PUBLIC_URL`. The backend uses it for
its own API origin, so with a shared env file it would point canonicals at the
API.

### Build-time only

These describe the image itself. They stay inlined, and are set by the release
pipeline.

| Variable | Used for |
|---|---|
| `NEXT_PUBLIC_RELEASE_SHA` | `<meta name="payverge-build">`, `window.__PAYVERGE_BUILD__` |
| `NEXT_PUBLIC_VERSION` | same |
| `NEXT_PUBLIC_BUILD_TIMESTAMP` | same, plus the sitemap `lastModified` for static routes |
| `NEXT_PUBLIC_SENTRY_RELEASE` | Sentry release tag (matches uploaded source maps) |

### Dropped

| Variable | Why |
|---|---|
| `NEXT_PUBLIC_ALLOW_INSECURE_URLS` | No longer read. Plain `http` is accepted automatically for loopback and private-network hosts (see [LAN installs](#lan-installs-plain-http)). |
| `NEXT_PUBLIC_USDC_ADDRESS` | No consumer. USDC addresses are fixed per chain in `src/lib/lifi/config.ts`. The plan listed it as runtime, but there is nothing to wire. |
| `SITEMAP_SKIP_REMOTE` (build arg) | No consumer. The sitemap is rendered per request, so there is no build-time fetch to skip. |

All three were removed from `frontend/Dockerfile`. CI and compose files that
still pass them only trigger an "unused build arg" warning.

## Server-only settings

`src/config/serverConfig.ts` and the proxy read these per request. They are
never sent to the browser.

| Variable | Default | Effect |
|---|---|---|
| `BACKEND_INTERNAL_URL` | unset (proxy off) | Private backend origin, e.g. `http://backend:8080`. Enables the same-origin proxy and the server-side API base. |
| `INTERNAL_API_URL` | unset | Full private API base. Takes precedence for server-side fetches. |
| `SEO_INDEXING` | `false` | `true` lifts the robots.txt `Disallow: /` and the root `noindex` meta, and lets `sitemap.xml` list pages and storefronts (it is empty until then). A fresh instance is never crawled by accident. |
| `SEO_TWITTER_HANDLE` | empty | `twitter:creator` and Organization `sameAs`. |
| `CLOUDFLARE_INSIGHTS` | unset | `true`/`false` explicitly allows or forbids the Cloudflare Web Analytics beacon in the CSP. |
| `EDGE` | `none` | `EDGE=cloudflare` enables the beacon when `CLOUDFLARE_INSIGHTS` is unset. |
| `SECURITY_EMAIL` | empty | security.txt contact. Falls back to `SUPPORT_EMAIL`, and the route 404s when both are empty. |
| `FRONTEND_TRUSTED_PROXIES` | empty (trust nothing) | Comma-separated IPs/CIDRs of the edge directly in front of the Next server, e.g. `10.0.0.0/8, fd00::/8`. Only a request from one of these peers may pass its `X-Forwarded-For`, `X-Real-IP`, `Forwarded`, `CF-Connecting-IP`, `X-Forwarded-Proto` and `X-Forwarded-Host` through the proxy. Invalid and `/0` entries are ignored with a boot warning that names the position, never the value. |

Server-side API base order (`getServerApiUrl`):

1. `INTERNAL_API_URL`
2. `BACKEND_INTERNAL_URL` + `/api/v1`
3. an absolute `API_URL`
4. in production, `PUBLIC_URL` + `/api/v1` (a hairpin through the public edge)
5. nothing: server-side fetches are skipped

Build-time-only build switches:

- `SENTRY_BUILD_PLUGIN=1` enables the Sentry webpack plugin (tunnel route
  `/monitoring`, release injection, source-map upload with `SENTRY_ORG`,
  `SENTRY_FRONTEND_PROJECT` and the `sentry_auth_token` build secret). It is
  also enabled when `NEXT_PUBLIC_SENTRY_DSN` is set at build time, which is the
  legacy behaviour.
- Without the plugin, browser Sentry still works at runtime from
  `FRONTEND_SENTRY_DSN`, reporting directly to the DSN host, which the CSP
  allows.

## Same-origin proxy

Routes: `src/app/api/v1/[...path]/route.ts` and `src/app/media/[...path]/route.ts`.
Implementation: `src/lib/proxy/backendProxy.ts`.

When `BACKEND_INTERNAL_URL` is set, the Next server forwards
`${PUBLIC_URL}/api/v1/*` and `${PUBLIC_URL}/media/*` to the backend over the
private network. When it is unset, both routes answer 404 and the edge (Caddy)
must route them.

Contract:

- **Methods.**
  - `/api/v1/*` accepts every method.
  - `/media/*` accepts only `GET` and `HEAD`.
- **Streaming.** Bodies stream in both directions and nothing is buffered, so
  SSE works.
  - Middleware does not match `/api/*` or `/media/*`, so it never buffers a
    request body.
- **Limits.**
  - Request bodies are capped at 15 MB; larger ones get 413.
  - Upstream calls time out at 300 s with 504. A `text/event-stream` response
    may stay open indefinitely.
- **Headers.**
  - Cookies, `Authorization` and all end-to-end headers pass through.
  - Client identity headers sent by the browser are dropped:
    `X-Forwarded-For`, `X-Real-IP`, `Forwarded`, `CF-Connecting-IP`,
    `True-Client-IP` and similar. `X-Forwarded-For` is set to the request's
    TCP peer. `X-Forwarded-Proto` and `X-Forwarded-Host` come from the socket
    and the `Host` header. A peer listed in `FRONTEND_TRUSTED_PROXIES` is an
    edge: its identity headers are kept and the peer is appended to its
    `X-Forwarded-For` chain. See "Client IP trust" below.
  - `X-Request-Id` is forwarded, or minted when it is missing or unsafe.
  - Hop-by-hop headers (RFC 9110 §7.6.1) and Next-internal headers are
    stripped.
- **Redirects.**
  - Redirects are never followed.
  - A `Location` header that points at the private backend origin is
    rewritten to a same-origin relative path.
- **Origin pinning.** The upstream URL must keep the backend origin and the
  route prefix, so a crafted path cannot reach another host.

Topologies:

| Topology | `/api/v1/*`, `/media/*` | Set |
|---|---|---|
| Caddy / reverse proxy in front (deploy compose) | routed to the backend by Caddy | `PUBLIC_URL`, `BACKEND_INTERNAL_URL` (recommended) |
| Single public port on the frontend (Render, Railway, bare compose) | Next proxy | `PUBLIC_URL`, `BACKEND_INTERNAL_URL`; on a PaaS also `FRONTEND_TRUSTED_PROXIES` set to the platform router's private range, and the backend `TRUSTED_PROXIES` set to the frontend's address **plus** that same router range (see "Client IP trust") |
| Split origin (API on its own host) | browser calls `API_URL` directly | `PUBLIC_URL`, `API_URL=https://api.example.com/api/v1` |

An edge must route only `/api/v1/*` and `/media/*` to the backend, never all
of `/api/*`. The frontend serves its own routes under `/api`:

- `/api/health`: the offline banner (`ConnectivityContext`) polls it with
  `HEAD`. Sent to the backend it answers 404 and the banner never clears.
- `/api/marketing-media`: the marketing image redirector.

`/.well-known/security.txt` is also a frontend route (see below), so it
belongs with everything else on the frontend.

Keep `BACKEND_INTERNAL_URL` set even behind Caddy:

- **Optimized `/media` images need it.** The next/image optimizer fetches a
  same-origin `src` such as `/media/...` by dispatching it inside the Next
  server, not through the edge. Caddy never sees that request. Without
  `BACKEND_INTERNAL_URL`, the Next `/media` route answers 404 and every
  optimized upload fails to load (`/_next/image` returns 400). Plain
  `<img>` tags and `unoptimized` images still work, because the browser
  fetches them through Caddy.
- Server components and route handlers use it for API reads. Without it they
  fall back to `INTERNAL_API_URL`, an absolute `API_URL`, or in production a
  hairpin through `PUBLIC_URL` (see the order above).

A production install with a same-origin `API_URL` and no
`BACKEND_INTERNAL_URL` logs `BACKEND_INTERNAL_URL: missing (...)` at boot.

### Client IP trust

The backend rate-limits by client IP, which Gin reads from `X-Forwarded-For`
(walking it from the right) when the immediate peer is in `TRUSTED_PROXIES`.
For proxied requests that peer is the frontend, so the frontend decides what
the backend sees.

Route handlers only get a web `Request`, never the socket, and Next keeps a
client-sent `X-Forwarded-For` unchanged instead of appending the socket
address. So the frontend records the peer itself:

- At boot, `src/instrumentation.ts` subscribes to Node's
  `http.server.request.start` diagnostics channel. Every incoming request is
  stamped with its socket address before Next reads any header, under a
  header keyed by a random per-process nonce. A client can send that header,
  but it cannot know the nonce, and the stamp overwrites it anyway.
- The proxy (`src/lib/proxy/backendProxy.ts`) reads the stamp:
  - **Peer not in `FRONTEND_TRUSTED_PROXIES` (the default).** Every
    client identity header is dropped, and `X-Forwarded-For` is exactly the
    peer address. A browser cannot pick the IP it is rate-limited on.
  - **Peer in `FRONTEND_TRUSTED_PROXIES`.** The edge's
    `X-Forwarded-For` chain is kept and the peer appended, and its
    `X-Real-IP`, `CF-Connecting-IP`, `X-Forwarded-Proto` and
    `X-Forwarded-Host` pass through.
  - **No valid stamp** (a request in the moment before instrumentation ran,
    or one the next/image optimizer dispatches internally). No
    `X-Forwarded-For` is sent, so the backend sees the frontend's own
    address. This fails toward a shared limit, never a forged one.

Setting it up:

- **Next on the public port (bare compose, a LAN install).** Leave
  `FRONTEND_TRUSTED_PROXIES` empty. Add the frontend's network to the
  backend `TRUSTED_PROXIES` so the backend accepts the peer the frontend
  sends.
- **A PaaS router or reverse proxy in front of Next (Render, Railway, Caddy
  forwarding everything to the frontend).** Two settings, one on each
  service:
  - Frontend: set `FRONTEND_TRUSTED_PROXIES` to that router's private
    range, so the proxy keeps the router's `X-Forwarded-For` chain.
  - Backend: set `TRUSTED_PROXIES` to the frontend's address **and** the
    same router range. The proxy appends the router (its TCP peer) to the
    chain, and Gin walks the chain from the right, skipping only the hops
    it trusts. If the backend trusts only the frontend, the router's
    address is the first untrusted hop and becomes every client's IP.

  Missing either setting puts all clients in one rate-limit and audit
  bucket keyed by the router's address. A worked example, with the client
  at `203.0.113.7`, the router at `10.0.4.2` and the frontend at
  `10.0.9.5`:

  | Hop | Sees peer | Sends `X-Forwarded-For` |
  |---|---|---|
  | Router | `203.0.113.7` | `203.0.113.7` |
  | Next proxy (`FRONTEND_TRUSTED_PROXIES=10.0.0.0/16`) | `10.0.4.2` (trusted) | `203.0.113.7, 10.0.4.2` |
  | Backend (`TRUSTED_PROXIES=10.0.0.0/16`) | `10.0.9.5` (trusted) | Gin skips `10.0.4.2` (trusted), `ClientIP()` = `203.0.113.7` |

  With backend `TRUSTED_PROXIES=10.0.9.5/32` instead, Gin stops at
  `10.0.4.2` and every client shares that address.
- **Caddy routing `/api/v1/*` and `/media/*` straight to the backend (deploy
  compose).** Browser API calls never reach the Next proxy. Gin sees Caddy
  as the peer and reads Caddy's chain. The Next `/media` route still serves
  the image optimizer's internal fetches, which carry no client IP.

The default trusts nothing on purpose. With Docker's userland proxy, a
published port shows every client as the bridge gateway (`172.x.0.1`). That
address is inside most private ranges people would list, and so are guests
on a restaurant LAN. Only list addresses that are always an edge you
control.

The plan's motivation applies here: same-origin is effectively mandatory,
because backend cookies are host-only with `SameSite=Lax`, and PaaS default
domains sit on the Public Suffix List.

## Middleware and CSP

`src/middleware.ts` runs on the **Node.js runtime** (`config.runtime =
"nodejs"`), so it reads the runtime environment on every request. It mints a
nonce and sets a `Content-Security-Policy` built by `src/lib/security/csp.ts`
from:

| Directive | Sources |
|---|---|
| `script-src` | `'self'`, the nonce, `'strict-dynamic'`; plus `'unsafe-eval'` in dev only, and `static.cloudflareinsights.com` when Cloudflare Insights is enabled |
| `img-src` | `'self' data: blob:`, the hosts in `src/config/imageOrigins.json`, `MEDIA_ORIGINS`, the `LOGO_URL` origin, and the `API_URL` origin when the API is split-origin |
| `connect-src` | `'self'`, the `API_URL` origin (plus both loopback spellings when it is local), the RPC origins for both chains, the Sentry DSN host, `POSTHOG_HOST`, and the Cloudflare beacon when enabled |
| everything else | static: `default-src 'self'`, `frame-ancestors 'none'`, `object-src 'none'`, `base-uri 'self'`, `form-action 'self'`, `worker-src 'self' blob:`; `style-src 'self' 'unsafe-inline'` (NextUI) |

No deployment-specific host is baked into the policy. Cloudflare Insights is
off unless configured.

## Images

`frontend/next.config.mjs` `images` is **build-time**: Next freezes it into the
image.

- `localPatterns` is deliberately unset. Every same-origin path is
  optimizable, including `/media/**` uploads served through the proxy or Caddy.
- `remotePatterns` comes from `src/config/imageOrigins.json`. It holds only
  generic hosts: Google avatars, and Unsplash photos used by demo data. The
  same manifest feeds the CSP, so the two cannot drift.
- Uploads served from a deployment's own S3/CDN host go in `MEDIA_ORIGINS` at
  runtime. That host is allowed by the CSP, but its images render
  **unoptimized** unless it is also added to `imageOrigins.json` and the image
  is rebuilt. Components that render uploaded media (storefront banners, menu
  photos, logos, partner icons) check `canOptimizeImageSrc()` from
  `src/config/imageOrigins.ts` and pass `unoptimized` for any other host,
  because the optimizer would reject it and the image would never load.
- The recommended setup is local storage, or a storage driver that emits
  root-relative `/media/...` URLs. Those need no image configuration at all.

## LAN installs (plain http)

In production builds, absolute URLs must be `https`, except for these hosts:

- loopback: `localhost`, `*.localhost`, `127.0.0.0/8`, `::1`, `0.0.0.0`;
- private networks: `10/8`, `172.16/12`, `192.168/16`, IPv6 ULA `fc00::/7`, and
  `*.local`, `*.lan`, `*.internal`, `*.home.arpa`.

A restaurant can run `PUBLIC_URL=http://192.168.1.20:3000`, and QR codes,
canonicals and links will point at that address. A plain-http public hostname
is rejected, logged, and replaced by the default.

## SEO and metadata

- `robots.txt`, `sitemap.xml` and `/.well-known/security.txt` are
  `force-dynamic` route handlers. They read `PUBLIC_URL` on every request.
- `robots.txt` returns `Disallow: /` unless `SEO_INDEXING=true`. The root
  layout adds `noindex` under the same rule.
- Page metadata calls `getSiteUrl()`/`absoluteSiteUrl()`.
  - The root layout awaits `headers()`, so pages render dynamically.
  - Metadata objects declared at module scope are evaluated once per server
    process, after the container environment is set. A restart picks up a new
    `PUBLIC_URL`.
- Open Graph and Twitter images are root-relative. They resolve against
  `metadataBase`, which equals `PUBLIC_URL`.
- Organization and WebSite JSON-LD are built per request from `PUBLIC_URL`.
  `sameAs` is filled only from `SEO_TWITTER_HANDLE`.

## security.txt

`src/app/.well-known/security.txt/route.ts` serves RFC 9116 with:

- `Contact: mailto:` set to `SECURITY_EMAIL`, falling back to `SUPPORT_EMAIL`;
- `Canonical: ${PUBLIC_URL}/.well-known/security.txt`;
- `Preferred-Languages`;
- `Expires` set 180 days ahead, stable within a UTC day, so always under one
  year.

When neither email is configured, the route returns 404 rather than naming
someone else's inbox. The static `public/.well-known/security.txt` was removed.

The backend has no security.txt route, so this frontend route is the only
source. A reverse proxy must send `/.well-known/security.txt` on the public
site to the frontend. An API-only hostname that should also publish one
needs its own rule at the edge.

## Minimal configurations

Single host behind Caddy, or on a single port:

```env
PUBLIC_URL=https://pos.example.com
BACKEND_INTERNAL_URL=http://backend:8080
SUPPORT_EMAIL=help@example.com      # optional
SECURITY_EMAIL=security@example.com # optional, enables security.txt
```

Restaurant LAN, no TLS:

```env
PUBLIC_URL=http://192.168.1.20:3000
BACKEND_INTERNAL_URL=http://backend:8080
```

A public production site that wants search traffic adds `SEO_INDEXING=true`.
It also adds `MEDIA_ORIGINS=https://cdn.example.com` when uploads live on a
CDN.

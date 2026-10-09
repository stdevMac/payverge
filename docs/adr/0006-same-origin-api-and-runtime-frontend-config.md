# 0006. Same-origin API and runtime frontend configuration

- Status: Accepted
- Date: 2026-10-03 (records a decision already in effect)

## Context

Next.js inlines `NEXT_PUBLIC_*` variables at build time. Configured that way,
every deployment needs its own frontend image, built with its own domain and
API URL baked in. That does not work for a published image that anyone can
run. Separately, serving the API from a different origin than the app means
CORS preflights, cross-site cookie rules and two TLS names to manage.

## Decision

- **One public origin.** An install is reached at `PUBLIC_URL`. The browser
  calls the API at the relative path `/api/v1` and loads uploaded files from
  `/media/*` on that same origin.
- **Two ways to route it.** Either the reverse proxy sends `/api/*` and
  `/media/*` to the backend, or `BACKEND_INTERNAL_URL` is set and the Next.js
  server proxies them itself, streaming bodies and event streams.
- **Runtime configuration.** The frontend server reads its environment per
  request and writes a whitelisted public subset into the page as
  `window.__PAYVERGE_ENV__`, from a nonce'd inline script. Only a few
  build-identity values remain build-time.
- **Runtime capabilities.** `GET /api/v1/instance` tells the frontend the
  product name, branding, billing and registration modes and which optional
  features are available. The frontend is meant to hide what the instance
  does not offer; that gating is not built yet, so today an unavailable
  feature stays visible and its routes answer with an error.

## Consequences

- **Easier:** one frontend image for everyone; changing the domain is a
  restart, not a rebuild. Cookies are first-party: operator sessions are
  `SameSite=Lax` and the diner refresh cookie is `SameSite=Strict`. No CORS
  in the common setup.
- **Easier:** a minimal setup needs a TLS proxy in front of the frontend
  only.
- **Harder:** the Next.js proxy adds a hop and must be kept transparent for
  uploads, event streams and redirects. Code that must run before the config
  script uses a ready callback.
- **Accepted:** an API on another origin is still possible (`API_URL`), but it
  is not the default, and it brings back CORS configuration.

See [architecture/overview.md](../architecture/overview.md#one-origin) and
[self-hosting/frontend-config.md](../self-hosting/frontend-config.md).

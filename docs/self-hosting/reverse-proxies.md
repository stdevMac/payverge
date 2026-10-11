# Reverse proxies and client IPs

Payverge rate-limits logins, payments and the public API per client IP, and
writes that IP to the audit log. Both only work if every hop between the
visitor and the backend passes the real address along, and if each hop
believes only the hops it should. A mistake here fails one of two ways:

- **Too little trust.** Every visitor appears to come from one proxy address
  and shares one rate limit. A busy dinner service locks everyone out.
- **Too much trust.** A visitor writes its own `X-Forwarded-For` and picks the
  IP it is limited on, which makes the limits useless.

The variables are listed in
[configuration.md](configuration.md#edge-tls-and-proxy-trust).

## The default stack

```text
visitor ──TLS──▶ Caddy ──▶ backend   (/api/v1/*, /media/*, AI routes)
                       └─▶ frontend ──▶ backend   (pages, server-side calls)
```

Caddy is the only thing published on the host (ports 80 and 443). It resolves
the client address once, removes every client-sent `X-Real-IP`,
`CF-Connecting-IP`, `True-Client-IP`, `X-Client-IP` and `Forwarded` header,
and sends the apps exactly one address in `X-Forwarded-For`. The backend and
frontend are not published on the host.

Pick the row that matches what sits in front of Caddy:

| In front of Caddy | Set in `.env` | Guide |
|---|---|---|
| Nothing (Caddy faces the internet) | nothing | – |
| Cloudflare proxy (orange cloud) | `EDGE=cloudflare` | [deploy/README.md](../../deploy/README.md#cloudflare) |
| A load balancer or proxy that forwards HTTPS | `TRUSTED_PROXY_CIDRS=<its source addresses>` | [deploy/README.md](../../deploy/README.md#other-proxies-and-load-balancers) |
| nginx, Apache or Traefik on the same host, terminating TLS | `TLS_TERMINATION=proxy`, `HTTP_PORT=127.0.0.1:8080`, `TRUSTED_PROXY_CIDRS=<edge gateway>/32` | [deploy/README.md](../../deploy/README.md#behind-another-web-server) |

## Caddy: `TRUSTED_PROXY_CIDRS` (strict mode)

When `TRUSTED_PROXY_CIDRS` is set, Caddy believes `X-Forwarded-For` only from
those addresses, and reads it in **strict mode**: from the right, skipping
only addresses in the list, and taking the first one that is not. A proxy
usually appends the address it saw to whatever header the client sent, so
the leftmost entry is attacker-controlled; strict mode never reaches it.

`deploy/docker-compose.yml` turns strict mode on only when the list is set:

```yaml
CADDY_TRUSTED_PROXIES_STRICT_OPTION: ${TRUSTED_PROXY_CIDRS:+trusted_proxies_strict}
```

With the list empty, Caddy trusts no one and uses the TCP peer address.

Keep the list as narrow as the proxy's real source addresses. A broad range
such as `10.0.0.0/8` on a shared cloud network lets anything else on that
network spoof client IPs.

## `TRUSTED_PROXIES` on the backend

The backend (Gin) reads `X-Forwarded-For` only when the TCP peer is in
`TRUSTED_PROXIES`, a comma-separated list of CIDRs or IPs. Its peers in the
deploy stack are Caddy and the frontend, both on the `payverge_edge` Docker
network (`<project>_edge` when `COMPOSE_PROJECT_NAME` is set; `docker network ls --filter label=com.docker.compose.network=edge` prints the exact name).

**Default.** An unset `TRUSTED_PROXIES` means loopback only,
`127.0.0.0/8,::1`. That is right for a backend run bare behind a proxy on the
same host, and safe everywhere else: nothing on the network can forge a
client IP. When an `X-Forwarded-For` arrives from a private, non-loopback peer
while `TRUSTED_PROXIES` is unset, the backend logs a `[SECURITY]` warning once,
naming the variable to set; until you set it, every client behind that proxy
shares the proxy's address for rate limiting.

**Bundled stack.** `deploy/docker-compose.yml` gives the edge network a fixed
subnet, `EDGE_SUBNET` (default `172.30.0.0/24`), and sets
`TRUSTED_PROXIES=${EDGE_SUBNET},127.0.0.1`: that subnet plus loopback, and no
other private range. This is safe because the backend publishes no host port
and its only peers on that network are Caddy and the frontend, which both
overwrite `X-Forwarded-For` with the client address they verified. If the
range is taken on the host, set another one before the first start:

```sh
# in .env
EDGE_SUBNET=172.31.0.0/24
docker compose up -d
```

`TRUSTED_PROXIES` and `FRONTEND_TRUSTED_PROXIES` follow it unless you set them
yourself.

`TRUSTED_PLATFORM=cloudflare` makes Gin read `CF-Connecting-IP` directly. The
deploy compose forces it empty because Caddy already resolved the address;
set it only when the backend runs without Caddy, directly behind Cloudflare.

## `FRONTEND_TRUSTED_PROXIES` on the frontend

The frontend forwards some requests to the backend server-side. Unless the
peer it received the request from is in `FRONTEND_TRUSTED_PROXIES`, it drops
every client identity header and sends exactly that peer's address, so a
browser cannot choose its IP. In the deploy stack the peer is Caddy, so
`FRONTEND_TRUSTED_PROXIES` must cover Caddy's address. The bundled compose
sets it to the same `EDGE_SUBNET` as the backend (without loopback); change
both together. Left empty, the frontend trusts no one and these requests are
limited per Caddy address, which errs toward a shared limit rather than a
forged one.
Details: [frontend-config.md](frontend-config.md#client-ip-trust).

## Checking it

Run with the stack up (not run for these docs; needs a running instance):

`CLIENT_IP_DEBUG=true` makes the backend log a `[ClientIPDebug]` line with
the resolved `client_ip`, the TCP peer, `X-Forwarded-For` and
`CF-Connecting-IP` for every sign-up attempt (`POST /api/v1/auth/register`).
An empty body is enough; the request is rejected after the line is logged.
Not run for these docs (needs a running instance with a public address).

```sh
echo CLIENT_IP_DEBUG=true >> .env && docker compose up -d backend
curl -s -o /dev/null -X POST https://pay.example.com/api/v1/auth/register
# A forged header must not change the result:
curl -s -o /dev/null -X POST -H 'X-Forwarded-For: 203.0.113.9' https://pay.example.com/api/v1/auth/register
docker compose logs --since 2m backend | grep ClientIPDebug
```

`client_ip` must be your own public IP both times, never a `172.x`/`10.x`
address and never `203.0.113.9`. Remove `CLIENT_IP_DEBUG` from `.env` and run
`docker compose up -d` afterwards. The sign-up limiter counts these requests.

If every request shows Docker's gateway address even with Caddy facing the
internet, Docker is forwarding through its userland proxy; see
[troubleshooting.md](troubleshooting.md#every-visitor-has-the-same-ip).

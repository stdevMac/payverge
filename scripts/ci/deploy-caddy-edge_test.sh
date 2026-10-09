#!/usr/bin/env bash
# Behaviour test for the self-host edge in deploy/: runs the stock Caddy image
# with deploy/Caddyfile (EDGE=none) and deploy/Caddyfile.cloudflare
# (EDGE=cloudflare) in front of a stub that echoes what it receives, and
# checks:
#   - routing: /api/v1/* and /media/* reach the backend, the rest the frontend;
#   - a client-sent X-Forwarded-For, CF-Connecting-IP, X-Real-IP or Forwarded
#     never reaches the apps (the client is not inside a trusted range);
#   - with TRUSTED_PROXY_CIDRS set (a load balancer that appends to
#     X-Forwarded-For), the apps get the right-most untrusted address, never a
#     value the client put in front (trusted_proxies_strict);
#   - edge security headers are present, each one added only when the app
#     did not send it (one app header never drops the others), Server is
#     removed, and the edge's Strict-Transport-Security (HSTS_POLICY) replaces
#     the apps' own;
#   - request bodies over 15 MB are refused with 413;
#   - plain HTTP redirects to HTTPS;
#   - the apps are always told X-Forwarded-Proto: https;
#   - TLS_TERMINATION=proxy (the caddy entrypoint from docker-compose.yml):
#     plain HTTP for DOMAIN with no redirect and no HTTPS listener, the client
#     address from the trusted server in front, the same headers; and the
#     entrypoint refuses it with EDGE=cloudflare or an unknown value;
#   - both Caddyfiles validate for DOMAIN=localhost and a public name.
#
# Needs docker, curl and awk. Binds 127.0.0.1 only.
#   CADDY_IMAGE           image to test (default: the one deploy/docker-compose.yml pins)
#   EDGE_TEST_PREFIX      container/network name prefix (default payverge-edge-test)
#   EDGE_TEST_PORT_BASE   first of eight host ports (default 28440)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEPLOY="$ROOT/deploy"
PINNED_IMAGE="$(sed -n 's/^[[:space:]]*image:[[:space:]]*\(caddy:[^[:space:]]*\).*/\1/p' "$DEPLOY/docker-compose.yml" | head -n 1)"
IMAGE="${CADDY_IMAGE:-${PINNED_IMAGE:-caddy:2}}"
PREFIX="${EDGE_TEST_PREFIX:-payverge-edge-test}-$$"
BASE="${EDGE_TEST_PORT_BASE:-28440}"
NET="$PREFIX-net"
STUB="$PREFIX-stub"
EDGE="$PREFIX-edge"
CF="$PREFIX-cf"
LB="$PREFIX-lb"
PX="$PREFIX-px"
WORK="$(mktemp -d)"

cleanup() {
	docker rm -f "$STUB" "$EDGE" "$CF" "$LB" "$PX" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

fail() {
	echo "deploy-caddy-edge: FAIL: $*" >&2
	for c in "$EDGE" "$CF" "$LB" "$PX"; do
		docker logs --tail 20 "$c" >&2 2>/dev/null || true
	done
	exit 1
}
pass() { echo "deploy-caddy-edge: ok: $*"; }

caddy_files=(
	-v "$DEPLOY/Caddyfile:/etc/caddy/Caddyfile:ro"
	-v "$DEPLOY/Caddyfile.cloudflare:/etc/caddy/Caddyfile.cloudflare:ro"
	-v "$DEPLOY/payverge.caddy:/etc/caddy/payverge.caddy:ro"
	-v "$DEPLOY/cloudflare-cidrs.caddy:/etc/caddy/cloudflare-cidrs.caddy:ro"
)

# 1. Both Caddyfiles validate, with and without the optional settings.
for domain in localhost example.com; do
	for file in Caddyfile Caddyfile.cloudflare; do
		docker run --rm "${caddy_files[@]}" \
			-e DOMAIN="$domain" -e CADDY_ACME_EMAIL_OPTION="email ops@example.com" \
			-e CADDY_TRUSTED_PROXIES_OPTION="trusted_proxies static 10.0.0.0/8" \
			-e CADDY_TRUSTED_PROXIES_STRICT_OPTION="trusted_proxies_strict" \
			-e CADDY_HSTS="max-age=63072000; includeSubDomains; preload" \
			"$IMAGE" caddy validate --config "/etc/caddy/$file" --adapter caddyfile >/dev/null 2>&1 ||
			fail "caddy validate $file with DOMAIN=$domain"
		docker run --rm "${caddy_files[@]}" -e DOMAIN="$domain" \
			"$IMAGE" caddy validate --config "/etc/caddy/$file" --adapter caddyfile >/dev/null 2>&1 ||
			fail "caddy validate $file with DOMAIN=$domain and no optional settings"
	done
done
pass "Caddyfile and Caddyfile.cloudflare validate"

# The caddy service's entrypoint script, as the container receives it.
ENTRYPOINT="$(awk '/^  caddy:$/ {c=1} c && /^      - \|$/ {p=1; next} p && /^    [a-z_]+:/ {exit} p {sub(/^        /, ""); gsub(/\$\$/, "$"); print}' "$DEPLOY/docker-compose.yml")"
[[ $ENTRYPOINT == *"exec caddy run"* ]] || fail "could not read the caddy entrypoint from docker-compose.yml"
entrypoint() { # entrypoint [docker run args...] -- run it in the pinned image
	docker run --rm "${caddy_files[@]}" -e DOMAIN=localhost "$@" --entrypoint /bin/sh "$IMAGE" -ec "$ENTRYPOINT"
}
rc=0
out=$(entrypoint -e EDGE=cloudflare -e TLS_TERMINATION=proxy 2>&1) || rc=$?
[[ $rc == 64 && $out == *"needs EDGE=none"* ]] || fail "TLS_TERMINATION=proxy with EDGE=cloudflare must exit 64, got $rc: $out"
rc=0
out=$(entrypoint -e TLS_TERMINATION=nginx 2>&1) || rc=$?
[[ $rc == 64 && $out == *"must be caddy or proxy"* ]] || fail "an unknown TLS_TERMINATION must exit 64, got $rc: $out"
pass "the entrypoint refuses TLS_TERMINATION=proxy with EDGE=cloudflare, and unknown values"

# 2. Stub apps: one Caddy answering as backend (:8080) and frontend (:3000).
cat >"$WORK/stub.Caddyfile" <<'EOF'
{
	admin off
	auto_https off
}
:8080 {
	# Read the whole body before answering, like the real upload handlers, so
	# the edge's body limit is exercised.
	@upload {
		path /api/v1/upload
		expression `{http.request.body} != "-"`
	}
	respond @upload "backend upload" 200
	respond "backend path={path} proto=[{header.X-Forwarded-Proto}] xff=[{header.X-Forwarded-For}] cf=[{header.CF-Connecting-IP}] real=[{header.X-Real-IP}] fwd=[{header.Forwarded}]" 200
}
:3000 {
	# The apps send their own HSTS (with includeSubDomains); the edge must
	# replace it with HSTS_POLICY.
	header Strict-Transport-Security "max-age=31536000; includeSubDomains"
	# A page with its own framing policy and none of the other defaults: the
	# edge must keep this value and still add the other four.
	header /framed X-Frame-Options "SAMEORIGIN"
	respond "frontend path={path} proto=[{header.X-Forwarded-Proto}] xff=[{header.X-Forwarded-For}] cf=[{header.CF-Connecting-IP}] real=[{header.X-Real-IP}] fwd=[{header.Forwarded}]" 200
}
EOF
docker network create "$NET" >/dev/null
docker run -d --name "$STUB" --network "$NET" --network-alias backend --network-alias frontend \
	-v "$WORK/stub.Caddyfile:/etc/caddy/Caddyfile:ro" "$IMAGE" >/dev/null

start_edge() { # start_edge NAME CONFIG HTTP_PORT HTTPS_PORT [docker run args...]
	local name=$1 config=$2 http=$3 https=$4
	shift 4
	docker run -d --name "$name" --network "$NET" \
		-p "127.0.0.1:$http:80" -p "127.0.0.1:$https:443" \
		"${caddy_files[@]}" -e DOMAIN=localhost "$@" \
		"$IMAGE" caddy run --config "/etc/caddy/$config" --adapter caddyfile >/dev/null
}
start_edge "$EDGE" Caddyfile "$BASE" "$((BASE + 1))"
start_edge "$CF" Caddyfile.cloudflare "$((BASE + 2))" "$((BASE + 3))"
# EDGE=none behind a load balancer: what docker-compose.yml passes for
# TRUSTED_PROXY_CIDRS="<cidrs>" (and HSTS_POLICY). The host's connections reach
# the container from a private address, so private_ranges stands in for the
# load balancer's range.
start_edge "$LB" Caddyfile "$((BASE + 4))" "$((BASE + 5))" \
	-e CADDY_TRUSTED_PROXIES_OPTION="trusted_proxies static private_ranges" \
	-e CADDY_TRUSTED_PROXIES_STRICT_OPTION="trusted_proxies_strict" \
	-e CADDY_HSTS="max-age=63072000; includeSubDomains"
# TLS_TERMINATION=proxy, started through the compose entrypoint. The test's
# curl plays the web server in front; it reaches the container from a private
# address, which private_ranges stands in for.
docker run -d --name "$PX" --network "$NET" \
	-p "127.0.0.1:$((BASE + 6)):80" -p "127.0.0.1:$((BASE + 7)):443" \
	"${caddy_files[@]}" -e DOMAIN=localhost -e EDGE=none -e TLS_TERMINATION=proxy \
	-e CADDY_TRUSTED_PROXIES_OPTION="trusted_proxies static private_ranges" \
	-e CADDY_TRUSTED_PROXIES_STRICT_OPTION="trusted_proxies_strict" \
	--entrypoint /bin/sh "$IMAGE" -ec "$ENTRYPOINT" >/dev/null

HTTPS="$((BASE + 1))"
CF_HTTPS="$((BASE + 3))"
LB_HTTPS="$((BASE + 5))"
get() { # get PORT PATH [curl args...]
	local port=$1 path=$2
	shift 2
	curl -sk --max-time 10 --resolve "localhost:$port:127.0.0.1" "$@" "https://localhost:$port$path"
}

for _ in $(seq 1 30); do
	if get "$HTTPS" /api/v1/health/live -o /dev/null && get "$CF_HTTPS" / -o /dev/null && get "$LB_HTTPS" / -o /dev/null; then
		break
	fi
	sleep 1
done
get "$HTTPS" /api/v1/health/live -o /dev/null || fail "edge did not answer on https://localhost:$HTTPS"

# 3. Routing.
[[ $(get "$HTTPS" /api/v1/health/live) == "backend path=/api/v1/health/live "* ]] || fail "/api/v1/* must reach the backend"
[[ $(get "$HTTPS" /api/v1/businesses/7/ai/menu-wizard) == "backend "* ]] || fail "AI routes must reach the backend"
[[ $(get "$HTTPS" /media/menu/dish.webp) == "backend "* ]] || fail "/media/* must reach the backend"
[[ $(get "$HTTPS" /dashboard) == "frontend path=/dashboard "* ]] || fail "other paths must reach the frontend"
[[ $(get "$HTTPS" /api/health) == "frontend "* ]] || fail "/api/health (Next.js) must reach the frontend"
[[ $(get "$HTTPS" /api/v1x) == "frontend "* ]] || fail "/api/v1x must not match the backend prefix"
pass "routing"

# 4. Client-chosen addresses never reach the apps.
spoof=(-H 'X-Forwarded-For: 203.0.113.9' -H 'CF-Connecting-IP: 203.0.113.9' -H 'X-Real-IP: 203.0.113.9'
	-H 'Forwarded: for=203.0.113.9' -H 'True-Client-IP: 203.0.113.9' -H 'X-Client-IP: 203.0.113.9')
body=$(get "$HTTPS" /api/v1/me "${spoof[@]}")
[[ $body == *"xff=["*"]"* && $body != *203.0.113.9* ]] || fail "a spoofed client-IP header reached the backend: $body"
[[ $body =~ xff=\[[0-9a-f.:]+\]\ cf=\[\]\ real=\[\]\ fwd=\[\] ]] || fail "backend must receive exactly one client address and no other client-IP header: $body"
body=$(get "$HTTPS" / "${spoof[@]}")
[[ $body != *203.0.113.9* ]] || fail "a spoofed client-IP header reached the frontend: $body"
[[ $body == *" cf=[] real=[] fwd=[]" ]] || fail "the frontend must not receive CF-Connecting-IP, X-Real-IP or Forwarded: $body"
body=$(get "$CF_HTTPS" /api/v1/me -H 'CF-Connecting-IP: 198.51.100.7' -H 'X-Forwarded-For: 198.51.100.7' -H 'X-Real-IP: 198.51.100.7')
[[ $body != *198.51.100.7* ]] || fail "EDGE=cloudflare trusted CF-Connecting-IP from a non-Cloudflare peer: $body"
[[ $body == *" cf=[] real=[] fwd=[]" ]] || fail "EDGE=cloudflare must remove CF-Connecting-IP and X-Real-IP before the apps: $body"
pass "client-IP headers from untrusted peers are ignored and removed"

# 4b. Behind a trusted load balancer that appends to X-Forwarded-For, the apps
# get the right-most address that is not a trusted proxy. 203.0.113.9 is what
# the client sent; 198.51.100.20 is what the load balancer appended.
body=$(get "$LB_HTTPS" /api/v1/me -H 'X-Forwarded-For: 203.0.113.9, 198.51.100.20')
[[ $body == *"xff=[198.51.100.20] cf=[]"* ]] || fail "TRUSTED_PROXY_CIDRS: a client-prepended X-Forwarded-For entry won (want 198.51.100.20): $body"
body=$(get "$LB_HTTPS" / -H 'X-Forwarded-For: 203.0.113.9, 198.51.100.20, 10.1.2.3')
[[ $body == *"xff=[198.51.100.20] cf=[]"* ]] || fail "TRUSTED_PROXY_CIDRS: trusted hops must be skipped from the right (want 198.51.100.20): $body"
body=$(get "$LB_HTTPS" /api/v1/me -H 'X-Forwarded-For: 198.51.100.20')
[[ $body == *"xff=[198.51.100.20] cf=[]"* ]] || fail "TRUSTED_PROXY_CIDRS: the load balancer's client address must reach the backend: $body"
pass "TRUSTED_PROXY_CIDRS reads X-Forwarded-For right to left (trusted_proxies_strict)"

# 5. Security headers. The frontend stub sends HSTS with includeSubDomains;
# the edge replaces it with HSTS_POLICY (default: DOMAIN only).
defaults=("x-content-type-options: nosniff" "x-frame-options: DENY" "referrer-policy: strict-origin-when-cross-origin"
	"x-xss-protection: 1; mode=block" "permissions-policy: accelerometer=")
headers=$(get "$HTTPS" / -D - -o /dev/null | tr -d '\r')
for h in "${defaults[@]}"; do
	grep -qi "^$h" <<<"$headers" || fail "missing header '$h'"
done
# Each default is decided on its own: an app that sends one of them keeps its
# value, and the edge still adds the others.
headers=$(get "$HTTPS" /framed -D - -o /dev/null | tr -d '\r')
[[ $(grep -ic '^x-frame-options:' <<<"$headers") == 1 ]] || fail "exactly one X-Frame-Options expected: $headers"
grep -qix 'x-frame-options: SAMEORIGIN' <<<"$headers" || fail "the app's own X-Frame-Options must win: $headers"
for h in "${defaults[@]}"; do
	[[ $h == x-frame-options:* ]] && continue
	grep -qi "^$h" <<<"$headers" || fail "an app header must not drop the default '$h': $headers"
done
[[ $(grep -ic '^strict-transport-security:' <<<"$headers") == 1 ]] || fail "exactly one Strict-Transport-Security header expected: $headers"
grep -qix 'strict-transport-security: max-age=31536000' <<<"$headers" || fail "default HSTS must be max-age=31536000 without includeSubDomains: $headers"
headers_api=$(get "$HTTPS" /api/v1/health/live -D - -o /dev/null | tr -d '\r')
grep -qix 'strict-transport-security: max-age=31536000' <<<"$headers_api" || fail "backend responses must carry the edge HSTS: $headers_api"
! grep -qi '^server:' <<<"$headers" || fail "Server header must be removed"
headers=$(get "$LB_HTTPS" / -D - -o /dev/null | tr -d '\r')
grep -qix 'strict-transport-security: max-age=63072000; includeSubDomains' <<<"$headers" || fail "HSTS_POLICY must set the HSTS value: $headers"
pass "security headers (per-header defaults; HSTS from HSTS_POLICY replaces the apps' own)"

# 6. Bodies over 15 MB are refused.
code=$(head -c 16000000 /dev/zero | get "$HTTPS" /api/v1/upload -o /dev/null -w '%{http_code}' -X POST --data-binary @- -H 'Content-Type: application/octet-stream')
[[ $code == 413 ]] || fail "a 16 MB body must get 413, got $code"
code=$(head -c 1000000 /dev/zero | get "$HTTPS" /api/v1/upload -o /dev/null -w '%{http_code}' -X POST --data-binary @- -H 'Content-Type: application/octet-stream')
[[ $code == 200 ]] || fail "a 1 MB body must pass, got $code"
pass "15 MB request body limit"

# 7. HTTP redirects to HTTPS.
code=$(curl -s -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 10 --resolve "localhost:$BASE:127.0.0.1" "http://localhost:$BASE/dashboard")
[[ $code == "308 https://localhost/dashboard" ]] || fail "HTTP must redirect to HTTPS, got '$code'"
pass "HTTP to HTTPS redirect"

# 8. The apps are told the public scheme.
[[ $(get "$HTTPS" /api/v1/me) == "backend path=/api/v1/me proto=[https] "* ]] || fail "the backend must get X-Forwarded-Proto: https"
pass "X-Forwarded-Proto: https"

# 9. TLS_TERMINATION=proxy: the server in front terminates TLS and forwards
# plain HTTP; it appended 198.51.100.20 to what the client sent.
PX_HTTP="$((BASE + 6))"
px() { # px PATH [curl args...]
	local path=$1
	shift
	curl -s --max-time 10 --resolve "localhost:$PX_HTTP:127.0.0.1" "$@" "http://localhost:$PX_HTTP$path"
}
for _ in $(seq 1 30); do
	px / -o /dev/null && break
	sleep 1
done
code=$(px /dashboard -o /dev/null -w '%{http_code}')
[[ $code == 200 ]] || fail "TLS_TERMINATION=proxy must serve plain HTTP without a redirect, got $code"
body=$(px /api/v1/me -H 'X-Forwarded-For: 203.0.113.9, 198.51.100.20')
[[ $body == "backend path=/api/v1/me proto=[https] xff=[198.51.100.20] cf=[]"* ]] || fail "TLS_TERMINATION=proxy backend: $body"
body=$(px /dashboard -H 'X-Forwarded-For: 198.51.100.20' -H 'X-Real-IP: 203.0.113.9')
[[ $body == "frontend path=/dashboard proto=[https] xff=[198.51.100.20] cf=[] real=[] fwd=[]" ]] || fail "TLS_TERMINATION=proxy frontend: $body"
headers=$(px / -D - -o /dev/null | tr -d '\r')
for h in "${defaults[@]}"; do
	grep -qi "^$h" <<<"$headers" || fail "TLS_TERMINATION=proxy: missing header '$h'"
done
grep -qix 'strict-transport-security: max-age=31536000' <<<"$headers" || fail "TLS_TERMINATION=proxy must still send HSTS: $headers"
! curl -sk --max-time 5 --resolve "localhost:$((BASE + 7)):127.0.0.1" -o /dev/null "https://localhost:$((BASE + 7))/" ||
	fail "TLS_TERMINATION=proxy must not serve HTTPS"
pass "TLS_TERMINATION=proxy"

echo "deploy-caddy-edge: all checks passed ($IMAGE)"

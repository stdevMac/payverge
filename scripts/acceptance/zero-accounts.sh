#!/usr/bin/env bash
# Zero-accounts acceptance test: a restaurant runs end to end on a fresh
# self-hosted stack with no third-party account of any kind.
#
#   scripts/acceptance/zero-accounts.sh            # build, boot, test, tear down
#   scripts/acceptance/zero-accounts.sh --keep     # leave the stack running
#   scripts/acceptance/zero-accounts.sh --target http://127.0.0.1:8080 \
#       --backend-log backend.log                  # test an instance already running
#
# See scripts/acceptance/README.md for what each step proves.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HERE="$ROOT/scripts/acceptance"

KEEP=0
TARGET=""
BACKEND_LOG=""
RESTART_CMD=""
NO_BUILD=0
PROJECT="${ACCEPTANCE_PROJECT:-oss-acceptance-$$}"
HTTP_PORT="${ACCEPTANCE_HTTP_PORT:-28080}"
HTTPS_PORT="${ACCEPTANCE_HTTPS_PORT:-28443}"
READY_TIMEOUT="${ACCEPTANCE_READY_TIMEOUT:-1800}"
SSE_TIMEOUT="${ACCEPTANCE_SSE_TIMEOUT:-10}"

usage() {
  sed -n '2,11p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  cat <<'EOF'
Options:
  --keep               do not tear the stack down at the end
  --no-build           reuse existing payverge-*:oss-accept images
  --project NAME       compose project name (default oss-acceptance-<pid>)
  --target URL         skip compose; test the backend at URL (API steps only).
                       Needs ADMIN_EMAIL and ADMIN_PASSWORD in the environment.
  --backend-log FILE   with --target: the backend's log, for the email check
  --restart-cmd CMD    with --target: command that restarts the backend, for
                       the persistence check (skipped when not given)
Environment: ACCEPTANCE_HTTP_PORT (28080), ACCEPTANCE_HTTPS_PORT (28443),
ACCEPTANCE_READY_TIMEOUT seconds (1800, includes the image build),
ACCEPTANCE_SSE_TIMEOUT seconds (10).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep) KEEP=1 ;;
    --no-build) NO_BUILD=1 ;;
    --project) PROJECT="$2"; shift ;;
    --target) TARGET="${2%/}"; shift ;;
    --backend-log) BACKEND_LOG="$2"; shift ;;
    --restart-cmd) RESTART_CMD="$2"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

for tool in curl jq openssl; do
  command -v "$tool" >/dev/null || { echo "FAIL: $tool is required" >&2; exit 2; }
done

WORK="$(mktemp -d "${TMPDIR:-/tmp}/payverge-acceptance.XXXXXX")"
STEP="setup"
SSE_PID=""

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }
fail() {
  printf '\nFAIL [%s]: %s\n' "$STEP" "$*" >&2
  exit 1
}
step() { STEP="$1"; log "== $1"; }

# ---- compose helpers ---------------------------------------------------------
# Compose gives shell variables precedence over --env-file, so a developer's or
# runner's exported OPENROUTER_API_KEY, EMAIL_PROVIDER, REGISTRATION_MODE, ... would
# leak into the "zero accounts" stack (and hand a real key to a throwaway
# container). Run compose with only what the Docker CLI itself needs.
compose_env=(env -i "PATH=$PATH" "HOME=$HOME")
for var in DOCKER_HOST DOCKER_CONTEXT DOCKER_CONFIG DOCKER_CERT_PATH DOCKER_TLS_VERIFY \
  DOCKER_BUILDKIT BUILDX_BUILDER TMPDIR XDG_RUNTIME_DIR; do
  [[ -n "${!var:-}" ]] && compose_env+=("$var=${!var}")
done
compose() {
  "${compose_env[@]}" docker compose -p "$PROJECT" --env-file "$WORK/.env" \
    -f "$ROOT/deploy/docker-compose.yml" \
    -f "$ROOT/deploy/docker-compose.build.yml" \
    -f "$HERE/compose.acceptance.yml" "$@"
}

cleanup() {
  local rc=$?
  [[ -n "$SSE_PID" ]] && kill "$SSE_PID" 2>/dev/null || true
  if [[ -z "$TARGET" && -f "$WORK/.env" ]]; then
    if [[ $rc -ne 0 ]]; then
      log "backend log tail (secrets are not logged):"
      compose logs --no-color --tail 60 backend 2>/dev/null || true
    fi
    if [[ $KEEP -eq 1 ]]; then
      log "--keep: stack left running. Stop it with:"
      log "  docker compose -p $PROJECT --env-file $WORK/.env -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml -f scripts/acceptance/compose.acceptance.yml down -v"
      return
    fi
    log "tearing down compose project $PROJECT"
    compose down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK"
  if [[ $rc -eq 0 ]]; then log "PASS: zero-accounts acceptance"; fi
}
trap cleanup EXIT

backend_logs() {
  if [[ -n "$TARGET" ]]; then
    [[ -n "$BACKEND_LOG" ]] && cat "$BACKEND_LOG"
  else
    compose logs --no-color backend 2>/dev/null
  fi
}

# ---- HTTP helpers ------------------------------------------------------------
CURL=(curl -sS --max-time 30)
TOKEN=""

# api METHOD PATH EXPECTED_STATUS [curl args...]; body lands in $WORK/body.
# EXPECTED_STATUS may list alternatives: "200|201".
api() {
  local method="$1" path="$2" want="$3"
  shift 3
  local auth=()
  [[ -n "$TOKEN" ]] && auth=(-H "Authorization: Bearer $TOKEN")
  local code
  code="$("${CURL[@]}" -o "$WORK/body" -w '%{http_code}' -X "$method" "${auth[@]}" "$@" "$BASE$path")" ||
    fail "$method $path: request failed"
  if [[ ! "$code" =~ ^($want)$ ]]; then
    fail "$method $path: HTTP $code, want $want: $(head -c 400 "$WORK/body")"
  fi
}
json() { jq -r "$1" "$WORK/body"; }
expect() { # expect JQ_FILTER DESCRIPTION: the filter must print true
  [[ "$(jq -r "$1" "$WORK/body")" == "true" ]] || fail "$2: $(head -c 400 "$WORK/body")"
}

wait_ready() {
  local deadline=$((SECONDS + $1)) code
  while ((SECONDS < deadline)); do
    code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' "$BASE/api/v1/health/ready" 2>/dev/null || true)"
    [[ "$code" == 200 ]] && return 0
    sleep 3
  done
  fail "/api/v1/health/ready did not return 200 within $1s (last: ${code:-none})"
}

# ---- 1. boot -----------------------------------------------------------------
if [[ -n "$TARGET" ]]; then
  BASE="$TARGET"
  : "${ADMIN_EMAIL:?--target needs ADMIN_EMAIL}" "${ADMIN_PASSWORD:?--target needs ADMIN_PASSWORD}"
  step "target $BASE"
  wait_ready 120
else
  command -v docker >/dev/null || fail "docker is required"
  docker compose version >/dev/null || fail "the docker compose plugin is required"
  step "generate .env (zero third-party accounts)"
  ADMIN_EMAIL="admin@acceptance.test"
  ADMIN_PASSWORD="$(openssl rand -hex 16)"
  umask 077
  cat >"$WORK/.env" <<EOF
DOMAIN=localhost
PUBLIC_URL=https://localhost:$HTTPS_PORT
HTTP_PORT=$HTTP_PORT
HTTPS_PORT=$HTTPS_PORT
ADMIN_EMAIL=$ADMIN_EMAIL
ADMIN_PASSWORD=$ADMIN_PASSWORD
DB_PASSWORD=$(openssl rand -hex 24)
JWT_SECRET_KEY=$(openssl rand -base64 48 | tr -d "\n")
PLUGIN_SECRET_KEY=$(openssl rand -hex 32)
PAYVERGE_VERSION=oss-accept
COMPOSE_PROFILES=
EMAIL_PROVIDER=log
# Not the default 172.30.0.0/24: a stack already running on this host has it.
EDGE_SUBNET=${ACCEPT_EDGE_SUBNET:-172.30.254.0/24}
EOF
  BASE="https://localhost:$HTTPS_PORT"
  # Caddy serves localhost from its own local CA; the trust store does not
  # know it, so TLS verification is off for this loopback-only test.
  CURL+=(-k)
  if [[ $NO_BUILD -eq 0 ]]; then
    step "build images from source (backend, then frontend)"
    compose build backend
    compose build frontend
  fi
  step "docker compose up -d (project $PROJECT)"
  compose up -d --no-build
  step "wait for /api/v1/health/ready"
  wait_ready "$READY_TIMEOUT"
fi

# ---- 2. instance contract + admin login --------------------------------------
step "GET /api/v1/instance"
api GET /api/v1/instance 200
expect '(.public_url|type)=="string" and (.registration_mode|type)=="string" and (.features|type)=="object" and (.demo|type)=="object"' "instance shape"
expect 'has("billing_mode")|not' "instance must not report a billing mode"
expect '.features.ai==false and .features.email==false' "no AI or email provider should be configured"
if [[ -z "$TARGET" ]]; then
  expect ".public_url==\"https://localhost:$HTTPS_PORT\"" "public_url should echo PUBLIC_URL"
fi
log "instance: $(jq -c '{public_url,registration_mode,email_verification}' "$WORK/body")"

step "admin login (ADMIN_EMAIL/ADMIN_PASSWORD bootstrap)"
login() {
  api POST /api/v1/auth/login 200 -H 'Content-Type: application/json' \
    --data-binary "$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" '{email:$e,password:$p}')"
  TOKEN="$(json .token)"
  [[ -n "$TOKEN" && "$TOKEN" != null ]] || fail "login returned no token"
}
login
log "logged in as user $(json .user_id)"

# ---- 3. restaurant setup -----------------------------------------------------
step "create a business (no plan step, no paywall)"
api POST /api/v1/inside/businesses '200|201' -H 'Content-Type: application/json' \
  -H "Idempotency-Key: acceptance-$PROJECT" \
  --data-binary '{"name":"Acceptance Bistro","currency":"USD"}'
api GET /api/v1/inside/businesses 200
BIZ="$(jq -r '[.. | objects | select(has("business_id") and .name=="Acceptance Bistro")][0].id' "$WORK/body")"
[[ "$BIZ" =~ ^[0-9]+$ ]] || fail "business not listed: $(head -c 300 "$WORK/body")"
expect "[.. | objects | select(.id==$BIZ and has(\"is_active\"))][0].is_active==true" "a new business should be active"
log "business id $BIZ"

step "upload a menu photo (local storage driver)"
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xcf\xc0\xf0\x1f\x00\x05\x00\x01\xff\x89\x99=\x1d\x00\x00\x00\x00IEND\xaeB`\x82' >"$WORK/photo.png"
api POST "/api/v1/inside/businesses/$BIZ/uploads" '200|201' -F "file=@$WORK/photo.png;type=image/png" -F folder=menu
PHOTO_URL="$(json .location)"
PHOTO_PATH="/${PHOTO_URL#*://*/}"
[[ "$PHOTO_PATH" == /media/* ]] || fail "upload location is not under /media: $PHOTO_URL"

check_photo() {
  local code
  code="$("${CURL[@]}" -o "$WORK/photo.got" -w '%{http_code}' "$BASE$PHOTO_PATH")" || fail "GET $PHOTO_PATH failed"
  [[ "$code" == 200 ]] || fail "GET $PHOTO_PATH: HTTP $code"
  cmp -s "$WORK/photo.png" "$WORK/photo.got" || fail "GET $PHOTO_PATH returned different bytes"
}
check_photo
log "photo served at $PHOTO_PATH"

step "menu category + item"
api POST "/api/v1/inside/businesses/$BIZ/menu/categories" '200|201' -H 'Content-Type: application/json' \
  --data-binary '{"name":"Mains"}'
CAT="$(json .category.id)"
VERSION="$(json .version)"
api POST "/api/v1/inside/businesses/$BIZ/menu/items" '200|201' -H 'Content-Type: application/json' \
  --data-binary "$(jq -n --arg c "$CAT" --argjson v "$VERSION" --arg img "$PHOTO_URL" \
    '{version:$v,category_id:$c,item:{name:"Steak Frites",description:"Acceptance dish",price:24.5,is_available:true,image:$img,images:[$img]}}')"
ITEM="$(json .item.id)"
log "category $CAT, item $ITEM"

step "create a table"
api POST "/api/v1/inside/businesses/$BIZ/tables" '200|201' -H 'Content-Type: application/json' \
  --data-binary '{"name":"T1","capacity":4}'
api GET "/api/v1/inside/businesses/$BIZ/tables" 200
CODE="$(jq -r '[.. | objects | select(has("table_code") and .name=="T1")][0].table_code' "$WORK/body")"
[[ -n "$CODE" && "$CODE" != null ]] || fail "table T1 not listed: $(head -c 300 "$WORK/body")"
log "table code $CODE"

# ---- 4. guest side -----------------------------------------------------------
step "guest QR target and menu via public routes"
api GET "/api/v1/guest/table/$CODE" 200
expect '.business.name=="Acceptance Bistro"' "guest table should name the business"
api GET "/api/v1/guest/table/$CODE/menu" 200
expect "[.categories[].items[] | select(.id==\"$ITEM\" and .price==24.5 and .image==\"$PHOTO_URL\")] | length==1" "guest menu should list the item with its photo"

step "open the staff SSE stream, then place a guest order"
"${CURL[@]}" -N --max-time $((SSE_TIMEOUT + 30)) \
  -H "Authorization: Bearer $TOKEN" -H 'Accept: text/event-stream' \
  "$BASE/api/v1/inside/businesses/$BIZ/events" >"$WORK/sse" 2>/dev/null &
SSE_PID=$!
for _ in $(seq 1 20); do grep -q '^event: connected' "$WORK/sse" 2>/dev/null && break; sleep 0.5; done
grep -q '^event: connected' "$WORK/sse" || fail "SSE stream did not connect"

api POST "/api/v1/guest/table/$CODE/order" '200|201' -H 'Content-Type: application/json' \
  -H "X-Request-Id: acceptance-$PROJECT-order-1" \
  --data-binary "$(jq -n --arg id "$ITEM" '{items:[{menu_item_name:"Steak Frites",menu_item_id:$id,quantity:2,price:24.5}],notes:"acceptance"}')"
ORDER="$(json .order.id)"
BILL="$(json .bill.id)"
expect '.bill.total_amount==49' "bill total should be 2 x 24.50"
log "order $ORDER on bill $BILL"

deadline=$((SECONDS + SSE_TIMEOUT))
until grep -q '^event: order.created' "$WORK/sse"; do
  ((SECONDS < deadline)) || fail "no order.created SSE event within ${SSE_TIMEOUT}s"
  sleep 0.5
done
kill "$SSE_PID" 2>/dev/null || true
SSE_PID=""
log "order.created arrived on the SSE stream"

step "order is on the staff/KDS orders list"
api GET "/api/v1/inside/businesses/$BIZ/orders" 200
expect "[.orders[] | select(.id==$ORDER)] | length==1" "staff orders list should include the guest order"

# ---- 5. settle ---------------------------------------------------------------
step "settle the bill in cash"
mails_before="$(backend_logs | grep -c 'provider=log' || true)"
api POST "/api/v1/inside/businesses/$BIZ/cash-register/sessions" '200|201' -H 'Content-Type: application/json' \
  --data-binary '{"opening_float":"100.00"}'
api POST "/api/v1/inside/bills/$BILL/alternative-payment" '200|201' -H 'Content-Type: application/json' \
  -H "Idempotency-Key: acceptance-$PROJECT-pay-1" \
  --data-binary '{"amount":"49.00","payment_method":"cash","business_confirmation":true}'
expect '.payment_breakdown.is_complete==true and .payment_breakdown.remaining==0' "cash should settle the bill"
api GET "/api/v1/inside/bills/$BILL" 200
expect '((.bill // .).status)=="paid" or ((.bill // .).status)=="closed"' "bill should be paid"

step "email goes to the log provider"
# Dine-in receipts go only to an address already bound to the bill (anti
# relay); an unbound address is refused. The settlement itself sends the
# owner's transactional mail, which the log provider writes to the log.
BILL_TOKEN="$(json '(.bill // .).public_token')"
api POST "/api/v1/guest/bill/$BILL_TOKEN/email-receipt" 403 -H 'Content-Type: application/json' \
  --data-binary '{"email":"stranger@acceptance.test"}'
if [[ -n "$TARGET" && -z "$BACKEND_LOG" ]]; then
  log "skipped: no --backend-log given"
else
  # The settlement-triggered mail is the owner's "First order completed"
  # notice (template milestone_first_order); the log provider records its
  # template name (metadata only, no recipients or body).
  deadline=$((SECONDS + 20))
  until backend_logs | grep -q 'milestone_first_order'; do
    ((SECONDS < deadline)) || fail "the settlement email (milestone_first_order) never reached the log provider"
    sleep 1
  done
  # observed_provider.go logs outcome=failed / "email delivery failed".
  if backend_logs | grep -E 'email delivery failed|outcome=failed' | grep -q 'provider=log'; then
    fail "the log provider recorded a failed delivery"
  fi
  log "log provider delivered the settlement email ($(($(backend_logs | grep -c 'provider=log') - mails_before)) log-provider email(s) since settlement)"
fi

# ---- 6. frontend smoke -------------------------------------------------------
if [[ -z "$TARGET" ]]; then
  step "frontend: / serves the venue, invite links reach /dashboard"
  # "/" (and the locale roots) serve the published venue or a directory of
  # venues (200), or redirect to the operator sign-in while nothing is
  # published. An invite link on "/" always reaches /dashboard with its query,
  # and the removed SaaS sales pages are gone (404).
  for page in / /es; do
    loc="$("${CURL[@]}" -o /dev/null -w '%{http_code} %{redirect_url}' "$BASE$page")" || fail "GET $page failed"
    case "$loc" in
      "200 "*) ;;
      "307 https://localhost:$HTTPS_PORT/dashboard" | "308 https://localhost:$HTTPS_PORT/dashboard") ;;
      *) fail "GET $page: want 200 or a redirect to /dashboard, got $loc" ;;
    esac
    log "GET $page -> $loc"
  done
  page="/?invite_code=acceptance"
  want="/dashboard?invite_code=acceptance"
  loc="$("${CURL[@]}" -o /dev/null -w '%{http_code} %{redirect_url}' "$BASE$page")" || fail "GET $page failed"
  [[ "$loc" == "307 https://localhost:$HTTPS_PORT$want" || "$loc" == "308 https://localhost:$HTTPS_PORT$want" ]] ||
    fail "GET $page: want a redirect to $want, got $loc"
  log "GET $page -> ${loc#* }"
  code="$("${CURL[@]}" -o /dev/null -w '%{http_code}' "$BASE/pricing")" || fail "GET /pricing failed"
  [[ "$code" == 404 ]] || fail "GET /pricing: want 404 (no SaaS sales pages), got $code"
  log "GET /pricing 404"

  step "frontend: runtime config injected, no upstream domain"
  for page in /dashboard /staff/login "/t/$CODE"; do
    code="$("${CURL[@]}" -o "$WORK/page.html" -w '%{http_code}' "$BASE$page")" || fail "GET $page failed"
    [[ "$code" == 200 ]] || fail "GET $page: HTTP $code"
    grep -q 'window.__PAYVERGE_ENV__' "$WORK/page.html" || fail "GET $page: window.__PAYVERGE_ENV__ missing"
    grep -q "https://localhost:$HTTPS_PORT" "$WORK/page.html" || fail "GET $page: PUBLIC_URL not in the runtime config"
    if grep -qi 'payverge\.io' "$WORK/page.html"; then
      fail "GET $page: HTML mentions payverge.io: $(grep -oi '.\{40\}payverge\.io.\{40\}' "$WORK/page.html" | head -3)"
    fi
    log "GET $page 200"
  done
fi

# ---- 7. upgrade / restart keeps data -----------------------------------------
verify_persisted() {
  login
  api GET /api/v1/inside/businesses 200
  expect "[.. | objects | select(.id==$BIZ and .name==\"Acceptance Bistro\")] | length>=1" "business lost after $1"
  api GET "/api/v1/guest/table/$CODE/menu" 200
  expect "[.categories[].items[] | select(.id==\"$ITEM\")] | length==1" "menu item lost after $1"
  api GET "/api/v1/inside/businesses/$BIZ/orders" 200
  expect "[.orders[] | select(.id==$ORDER)] | length==1" "order lost after $1"
  api GET "/api/v1/inside/bills/$BILL" 200
  expect '((.bill // .).status)=="paid" or ((.bill // .).status)=="closed"' "bill state lost after $1"
  check_photo
  log "data and photo intact after $1"
}

if [[ -z "$TARGET" ]]; then
  # A real upgrade pulls new images and recreates every container, so only
  # named volumes survive. Recreate (and down/up without -v) to prove the
  # data and the photo live in volumes, not in container filesystems.
  step "upgrade: recreate every container (compose up -d --force-recreate)"
  compose up -d --no-build --force-recreate
  wait_ready 300
  verify_persisted "compose up -d --force-recreate"
  step "upgrade: compose down (keeping volumes) then up -d"
  compose down --remove-orphans
  compose up -d --no-build
  wait_ready 300
  verify_persisted "compose down + up -d"
  step "restart every service"
  compose restart
  wait_ready 300
  verify_persisted "compose restart"
elif [[ -n "$RESTART_CMD" ]]; then
  step "restart the backend ($RESTART_CMD)"
  bash -c "$RESTART_CMD"
  wait_ready 300
  verify_persisted "backend restart"
else
  log "persistence check skipped: --target without --restart-cmd"
fi

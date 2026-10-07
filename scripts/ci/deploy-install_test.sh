#!/usr/bin/env bash
# shellcheck disable=SC2015 # `[[ ... ]] && pass || fail` is the assertion style of this file
# Hermetic test of deploy/install.sh: no Docker daemon, no network.
#
# A stub `docker` (and curl, getent, host, ss, lsof) on PATH records every
# call and answers from environment variables, and install.sh runs in place
# in a throwaway copy of the repository layout (deploy/ next to a
# backend/Dockerfile). Covered:
#
#   - a failing `docker compose up -d` stops with the reason and the service
#     state, not silently;
#   - any other failing command is named by the ERR trap;
#   - a domain that does not resolve yet is a warning, not an exit;
#   - the generated admin password is shown and removed from .env only when
#     the backend says it created (or took over) the account; refused,
#     ignored and unknown outcomes are handled without losing the password;
#   - an upgrade of an existing database takes a backup set first, with the
#     installed version, and changes nothing when that backup fails; it then
#     prints that exact set and the commands that restore it;
#   - a re-run recreates caddy after `up -d` so a replaced Caddyfile is
#     loaded (a fresh install does not need to);
#   - BACKUP_UID/BACKUP_GID are written for a non-root installer;
#   - with TLS_TERMINATION=proxy (another web server in front), the installer
#     checks Caddy over plain HTTP on HTTP_PORT (which may carry a bind
#     address), skips the certificate hints and says where to forward, and
#     refuses HTTP_PORT/HTTPS_PORT without a bind address (they would publish
#     plain HTTP on every interface, around the proxy);
#   - --dry-run fails without an admin email and gives up on a hung daemon;
#   - TRUSTED_PROXY_CIDRS must be space-separated IPs/CIDRs no broader than /8 (IPv4) or /16 (IPv6);
#   - a localhost trial publishes Caddy on 127.0.0.1 only, --http-port and
#     --https-port take ADDR:PORT, and COMPOSE_PROJECT_NAME is kept in .env;
#   - an in-place install with no --version pins PAYVERGE_VERSION to the
#     newest release (the curl stub's v9.8.7), never latest; --build pins
#     local and does not call releases/latest;
#   - the public check probes the frontend (/) through Caddy as well as
#     /api/v1/health/live. A failing frontend probe still prints the
#     "Payverge is running" summary and the admin password, then exits
#     non-zero. Probes that pass leave the fresh-install cases at exit 0;
#   - a release download (install.sh outside a checkout) verifies SHA256SUMS
#     with cosign when the stub is on PATH: a passing verify-blob proceeds,
#     a failing one installs nothing, and --require-signature with no cosign
#     exits before any download.
#
# Usage: scripts/ci/deploy-install_test.sh
set -Eeuo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/payverge-install-test.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

failures=0
pass() { printf 'ok   - %s\n' "$*"; }
fail() {
	printf 'FAIL - %s\n' "$*" >&2
	failures=$((failures + 1))
}

# --- stubs ---------------------------------------------------------------------

STUBS=$WORK/bin
mkdir -p "$STUBS"
cat >"$STUBS/docker" <<'EOF'
#!/usr/bin/env bash
# Stub docker. Every call is appended to $STUB_LOG.
printf '%s\n' "$*" >>"$STUB_LOG"
case "$1 $2" in
"compose version") echo "2.29.1"; exit 0 ;;
"info "* | "info")
	# STUB_DOCKER_HANG=1: a wedged daemon that never answers.
	[[ ${STUB_DOCKER_HANG:-0} == 1 ]] && exec sleep 60
	exit 0
	;;
"version "*) echo "27.0.0"; exit 0 ;;
"volume ls") [[ ${STUB_DB_VOLUME:-0} == 1 ]] && echo "payverge_db"; exit 0 ;;
"ps "*) exit 0 ;;
esac
[[ $1 == compose ]] || { echo "stub docker: unexpected: $*" >&2; exit 99; }
shift
case "$*" in
"pull --quiet") exit 0 ;;
"up -d --remove-orphans" | "up -d --build --remove-orphans")
	[[ ${STUB_UP_STATUS:-0} == 0 ]] || echo "Error response from daemon: Bind for 0.0.0.0:443 failed: port is already allocated" >&2
	exit "${STUB_UP_STATUS:-0}"
	;;
"up -d backend") exit 0 ;;
"up -d --force-recreate --no-deps caddy") exit "${STUB_CADDY_STATUS:-0}" ;;
"ps -a") echo "NAME STATE"; echo "payverge-caddy-1 created"; exit 0 ;;
"ps -a --format {{.State}} backend") echo "running"; exit 0 ;;
"logs --tail 20") echo "stub: recent logs"; exit 0 ;;
"logs --tail 40 backend") echo "stub: backend logs"; exit 0 ;;
"logs --no-log-prefix backend") cat "${STUB_BACKEND_LOG:-/dev/null}"; exit 0 ;;
"exec -T caddy wget "*) exit 0 ;;
"run --rm -T backup once")
	# Records the version the backup set would be stamped with.
	echo "backup-version=$(sed -n 's/^PAYVERGE_VERSION=//p' .env | tail -n 1)" >>"$STUB_LOG"
	# What backup.sh logs; STUB_BACKUP_LOG=quiet leaves the set name out.
	if [[ ${STUB_BACKUP_LOG:-} != quiet && ${STUB_BACKUP_STATUS:-0} == 0 ]]; then
		echo "2026-10-03T09:15:00Z backup: set complete: daily/20261003T091500Z (12K)"
	fi
	exit "${STUB_BACKUP_STATUS:-0}"
	;;
esac
echo "stub docker compose: unexpected: $*" >&2
exit 98
EOF
cat >"$STUBS/curl" <<'EOF'
#!/bin/sh
# Logs every call. Prints the newest-release URL when an argument contains
# url_effective. Exits 22 when an argument matches the glob in STUB_CURL_FAIL
# (default: none). With STUB_CURL_DIR, `curl -o FILE URL` copies
# DIR/$(basename URL) onto FILE (skipped for -o /dev/null).
echo "curl $*" >>"$STUB_LOG"
url=
out=
prev=
for arg in "$@"; do
	if [ "$prev" = "-o" ]; then
		out=$arg
	fi
	prev=$arg
	case $arg in
	*url_effective*)
		printf '%s\n' "https://github.com/stdevMac/payverge/releases/tag/v9.8.7"
		;;
	http://* | https://*) url=$arg ;;
	esac
done
if [ -n "${STUB_CURL_FAIL:-}" ]; then
	for arg in "$@"; do
		# The variable is a glob on purpose (STUB_CURL_FAIL='https://host:443/').
		# shellcheck disable=SC2254
		case $arg in
		$STUB_CURL_FAIL) exit 22 ;;
		esac
	done
fi
if [ -n "$out" ] && [ "$out" != "/dev/null" ] && [ -n "${STUB_CURL_DIR:-}" ] && [ -n "$url" ]; then
	base=$(basename "$url")
	if [ ! -f "$STUB_CURL_DIR/$base" ]; then
		echo "stub curl: no fixture for $url" >&2
		exit 22
	fi
	cp "$STUB_CURL_DIR/$base" "$out" || exit 22
fi
exit 0
EOF
printf '#!/bin/sh\nexit 2\n' >"$STUBS/getent"
# shellcheck disable=SC2016 # $1 is the stub's own argument, not this script's
printf '#!/bin/sh\necho "Host $1 not found: 3(NXDOMAIN)"\nexit 1\n' >"$STUBS/host"
printf '#!/bin/sh\nexit 0\n' >"$STUBS/ss"
printf '#!/bin/sh\nexit 1\n' >"$STUBS/lsof"
chmod +x "$STUBS"/*

# new_checkout NAME: a fresh copy of deploy/ laid out like a clone.
new_checkout() {
	local dir=$WORK/$1
	mkdir -p "$dir/backend"
	: >"$dir/backend/Dockerfile"
	cp -R "$REPO/deploy" "$dir/deploy"
	rm -f "$dir/deploy/.env"
	rm -rf "$dir/deploy/backups"
	printf '%s' "$dir/deploy"
}

# new_installed NAME: deploy/ with no sibling backend/Dockerfile, so install.sh
# treats itself as an earlier install and downloads a release.
new_installed() {
	local dir=$WORK/$1/deploy
	mkdir -p "$dir"
	cp -R "$REPO/deploy"/. "$dir/"
	rm -f "$dir/.env"
	rm -rf "$dir/backups"
	printf '%s' "$dir"
}

# path_without_cosign: $PATH minus any directory that ships a cosign binary.
path_without_cosign() {
	local tail="" part
	while IFS= read -r part; do
		[[ -n $part && ! -x $part/cosign ]] || continue
		tail=${tail:+$tail:}$part
	done < <(printf '%s\n' "$PATH" | tr ':' '\n')
	printf '%s' "$tail"
}

# release_fixture VERSION DEST: payverge-deploy-<v>.tar.gz (prefix
# payverge-deploy-<v>/), a matching SHA256SUMS and a bundle file.
release_fixture() {
	local v=$1 dest=$2 stage hash
	stage=$WORK/stage-$v
	rm -rf "$stage"
	mkdir -p "$stage/payverge-deploy-$v" "$dest"
	cp -R "$REPO/deploy"/. "$stage/payverge-deploy-$v/"
	rm -f "$stage/payverge-deploy-$v/.env"
	rm -rf "$stage/payverge-deploy-$v/backups"
	COPYFILE_DISABLE=1 tar -czf "$dest/payverge-deploy-$v.tar.gz" -C "$stage" "payverge-deploy-$v"
	if command -v sha256sum >/dev/null 2>&1; then
		hash=$(sha256sum "$dest/payverge-deploy-$v.tar.gz" | awk 'NR==1 { print $1 }')
	else
		hash=$(shasum -a 256 "$dest/payverge-deploy-$v.tar.gz" | awk 'NR==1 { print $1 }')
	fi
	printf '%s  %s\n' "$hash" "payverge-deploy-$v.tar.gz" >"$dest/SHA256SUMS"
	printf '%s\n' '{"mediaType":"application/vnd.dev.sigstore.bundle+json"}' >"$dest/SHA256SUMS.sigstore.json"
}

# install DIR [ARGS...]: run install.sh in DIR with the stubs; output in $out,
# exit status in $status, docker calls in $calls.
install() {
	local dir=$1
	shift
	: >"$WORK/calls"
	status=0
	# STUB_PATH_TAIL replaces the inherited PATH after the stubs (used to hide
	# a real cosign). Unset, this is the same PATH="$STUBS:$PATH" as before.
	out=$(env PATH="$STUBS:${STUB_PATH_TAIL:-$PATH}" STUB_LOG="$WORK/calls" PAYVERGE_READY_TIMEOUT=30 \
		bash "$dir/install.sh" "$@" 2>&1 </dev/null) || status=$?
	calls=$(cat "$WORK/calls")
}

env_value() { sed -n "s/^$1=//p" "$2" | tail -n 1; }

expect_contains() { # expect_contains NAME HAYSTACK NEEDLE
	if [[ $2 == *"$3"* ]]; then pass "$1"; else
		fail "$1: missing '$3'"
		printf '%s\n' "$2" | sed 's/^/    | /' >&2
	fi
}
expect_absent() { # expect_absent NAME HAYSTACK NEEDLE
	if [[ $2 != *"$3"* ]]; then pass "$1"; else fail "$1: unexpected '$3'"; fi
}

FRESH=(--yes --domain pos.example.invalid --admin-email owner@example.com --no-demo)
log_line() { printf '{"level":"%s","msg":"%s","time":"2026-10-03T00:00:00Z"}\n' "$1" "$2"; }

# --- 1. docker compose up fails --------------------------------------------------

dir=$(new_checkout up-fails)
STUB_UP_STATUS=1 install "$dir" "${FRESH[@]}"
[[ $status -ne 0 ]] && pass "up failure: non-zero exit ($status)" || fail "up failure: exit status 0"
expect_contains "up failure: reason" "$out" "docker compose up -d failed (exit 1)"
expect_contains "up failure: compose error shown" "$out" "port is already allocated"
expect_contains "up failure: service state shown" "$out" "payverge-caddy-1 created"
expect_absent "up failure: no success banner" "$out" "Payverge is running"
expect_contains "unresolvable domain: only a warning" "$out" "pos.example.invalid does not resolve yet"
export STUB_UP_STATUS=0

# --- 2. bootstrap outcomes -------------------------------------------------------

# bootstrap NAME LOGLINE: fresh install whose backend logged LOGLINE.
bootstrap() {
	dir=$(new_checkout "$1")
	printf '%s\n' "$(log_line info 'Registration mode: invite')" "$2" >"$WORK/$1.log"
	STUB_BACKEND_LOG="$WORK/$1.log" install "$dir" "${FRESH[@]}"
	env_file=$dir/.env
	generated=$(grep -c '^# ADMIN_PASSWORD was generated' "$env_file" || true)
}

bootstrap created "$(log_line info 'Bootstrap admin o***@example.com created (user id 1)')"
[[ $status -eq 0 ]] && pass "created: exit 0" || fail "created: exit $status"
pw=$(sed -n 's/^  Password     \([A-Za-z0-9-]*\)$/\1/p' <<<"$out")
[[ $pw =~ ^[A-Za-z0-9]{5}(-[A-Za-z0-9]{5}){3}$ ]] && pass "created: password shown" || fail "created: no password in output"
[[ -z $(env_value ADMIN_PASSWORD "$env_file") && $generated == 0 ]] && pass "created: password removed from .env" || fail "created: password still in .env"
expect_contains "created: backend restarted without it" "$calls" "compose up -d backend"
if grep -Eq '^curl .* https://pos\.example\.invalid:443/$' <<<"$calls"; then
	pass "created: frontend probed"
else
	fail "created: frontend not probed"
fi
expect_absent "fresh install: caddy not recreated" "$calls" "force-recreate"
[[ $(env_value PLUGIN_SECRET_KEY "$env_file") =~ ^[0-9a-f]{64}$ ]] && pass "fresh .env: PLUGIN_SECRET_KEY is 64 hex characters" || fail "fresh .env: PLUGIN_SECRET_KEY is not hex"
[[ $(env_value MINIO_ROOT_PASSWORD "$env_file") =~ ^[0-9a-f]{48}$ ]] && pass "fresh .env: MINIO_ROOT_PASSWORD generated" || fail "fresh .env: MINIO_ROOT_PASSWORD missing"
if [[ $(id -u) -ne 0 ]]; then
	[[ $(env_value BACKUP_UID "$env_file") == "$(id -u)" && $(env_value BACKUP_GID "$env_file") == "$(id -g)" ]] &&
		pass "fresh .env: BACKUP_UID/BACKUP_GID are the installer's" || fail "fresh .env: BACKUP_UID/BACKUP_GID missing"
fi

bootstrap took-over "$(log_line warning 'Bootstrap admin o***@example.com matched an existing non-admin account (user id 7); took it over: password set from ADMIN_PASSWORD, 1 other sign-in method(s) unlinked, all sessions signed out, promoted to admin.')"
expect_contains "took over: warned" "$out" "already had a (non-admin) account"
[[ -n $(sed -n 's/^  Password     \([A-Za-z0-9]\{5\}-.*\)$/\1/p' <<<"$out") ]] && pass "took over: password shown" || fail "took over: password not shown"
[[ -z $(env_value ADMIN_PASSWORD "$env_file") ]] && pass "took over: password removed" || fail "took over: password kept"

for refusal in \
	"Bootstrap admin NOT created: ADMIN_PASSWORD rejected: too short. Choose a unique passphrase of at least 12 characters and restart." \
	"Bootstrap admin NOT applied: ADMIN_EMAIL belongs to an existing non-admin account with a linked wallet, which is never promoted automatically. Nothing was changed." \
	"Bootstrap admin NOT applied: another active account holds an email/password login for ADMIN_EMAIL, so an admin with that address could not sign in. Nothing was changed." \
	"Bootstrap admin for o***@example.com failed: connection reset"; do
	name="refused ($(cut -c1-28 <<<"$refusal"))"
	bootstrap refused "$(log_line error "$refusal")"
	[[ $status -eq 0 ]] && pass "$name: exit 0" || fail "$name: exit $status"
	expect_contains "$name: warned with the backend's reason" "$out" "${refusal:0:40}"
	expect_contains "$name: summary says not set" "$out" "Password     not set"
	[[ -n $(env_value ADMIN_PASSWORD "$env_file") && $generated == 1 ]] && pass "$name: password kept for the retry" || fail "$name: password lost"
	expect_absent "$name: backend not restarted" "$calls" "compose up -d backend"
	rm -rf "$WORK/refused"
done

bootstrap ignored "$(log_line info 'ADMIN_PASSWORD ignored: o***@example.com already exists and its password is never overwritten at boot.')"
expect_contains "ignored: warned" "$out" "its password was NOT changed"
expect_contains "ignored: summary unchanged" "$out" "Password     unchanged"
[[ -z $(env_value ADMIN_PASSWORD "$env_file") ]] && pass "ignored: password removed" || fail "ignored: password kept"

bootstrap silent "$(log_line info 'Server listening on :8080')"
expect_contains "no outcome: warned" "$out" "does not say whether it created the admin account"
[[ -n $(env_value ADMIN_PASSWORD "$env_file") ]] && pass "no outcome: password kept" || fail "no outcome: password lost"
expect_absent "no outcome: password not shown as set" "$out" "It is shown only"

# A refusal followed by a later success (restart after a fix) counts as success.
bootstrap retried "$(log_line error 'Bootstrap admin NOT created: ADMIN_PASSWORD rejected: too short.')
$(log_line info 'Bootstrap admin o***@example.com created (user id 2)')"
[[ -z $(env_value ADMIN_PASSWORD "$env_file") ]] && pass "newest outcome wins" || fail "newest outcome ignored"

# --- 3. upgrades take a backup first --------------------------------------------

# upgrade_checkout NAME: an existing install pinned to 1.0.0 without BACKUP_UID.
upgrade_checkout() {
	dir=$(new_checkout "$1")
	STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}" --version 1.0.0 >/dev/null
	sed -i.bak '/^BACKUP_UID=/d;/^BACKUP_GID=/d' "$dir/.env" && rm -f "$dir/.env.bak"
}

upgrade_checkout upgrade
STUB_DB_VOLUME=1 install "$dir" --yes --version 1.1.0
[[ $status -eq 0 ]] && pass "upgrade: exit 0" || fail "upgrade: exit $status"
expect_contains "upgrade: backup taken" "$calls" "compose run --rm -T backup once"
expect_contains "upgrade: backup stamped with the installed version" "$calls" "backup-version=1.0.0"
first_backup=$(grep -n 'backup once' <<<"$calls" | head -n 1 | cut -d: -f1)
first_up=$(grep -n '^compose up' <<<"$calls" | head -n 1 | cut -d: -f1)
((first_backup < first_up)) && pass "upgrade: backup before up" || fail "upgrade: backup after up"
[[ $(env_value PAYVERGE_VERSION "$dir/.env") == 1.1.0 ]] && pass "upgrade: version set" || fail "upgrade: version not set"
if [[ $(id -u) -ne 0 ]]; then
	[[ $(env_value BACKUP_UID "$dir/.env") == "$(id -u)" ]] && pass "upgrade: BACKUP_UID added to an old .env" || fail "upgrade: BACKUP_UID not added"
fi
expect_contains "upgrade: rollback names the pre-upgrade set" "$out" "pre-upgrade backup set: daily/20261003T091500Z (version 1.0.0)"
expect_contains "upgrade: rollback restores that exact set" "$out" "docker compose --profile restore run --rm restore daily/20261003T091500Z --yes"
expect_contains "upgrade: rollback pins the previous version" "$out" "set PAYVERGE_VERSION=1.0.0 in .env"
expect_absent "upgrade: no 'newest set' advice" "$out" "restore the newest set"
expect_contains "upgrade: caddy recreated for the new Caddyfile" "$calls" "compose up -d --force-recreate --no-deps caddy"
recreate=$(grep -n 'force-recreate --no-deps caddy' <<<"$calls" | head -n 1 | cut -d: -f1 || true)
((${recreate:-0} > first_up)) && pass "upgrade: caddy recreated after up -d" || fail "upgrade: caddy recreated before up -d"

upgrade_checkout unnamed-set
STUB_DB_VOLUME=1 STUB_BACKUP_LOG=quiet install "$dir" --yes --version 1.1.0
[[ $status -eq 0 ]] && pass "unnamed set: exit 0" || fail "unnamed set: exit $status"
expect_contains "unnamed set: warns to pick the pre-upgrade set" "$out" "could not read the name of the pre-upgrade backup set"
expect_absent "unnamed set: no restore command with a guessed name" "$out" "run --rm restore daily/"

upgrade_checkout caddy-fails
STUB_DB_VOLUME=1 STUB_CADDY_STATUS=1 install "$dir" --yes --version 1.1.0
[[ $status -ne 0 ]] && pass "caddy recreate failure: non-zero exit" || fail "caddy recreate failure: exit 0"
expect_contains "caddy recreate failure: explained" "$out" "recreating caddy with the new Caddyfile failed (exit 1)"

upgrade_checkout backup-fails
STUB_DB_VOLUME=1 STUB_BACKUP_STATUS=1 install "$dir" --yes --version 1.1.0
[[ $status -ne 0 ]] && pass "failed backup: non-zero exit" || fail "failed backup: exit 0"
expect_contains "failed backup: explained" "$out" "the backup before the upgrade failed, so nothing was changed"
[[ $(env_value PAYVERGE_VERSION "$dir/.env") == 1.0.0 ]] && pass "failed backup: version unchanged" || fail "failed backup: version changed"
expect_absent "failed backup: nothing started" "$calls" "compose up"
expect_absent "failed backup: no ERR-trap noise" "$out" "install.sh stopped at line"

upgrade_checkout no-backup
STUB_DB_VOLUME=1 install "$dir" --yes --version 1.1.0 --no-backup
expect_absent "--no-backup: no backup" "$calls" "backup once"
expect_contains "--no-backup: warned" "$out" "upgrading without a backup set first"

upgrade_checkout same-version
STUB_DB_VOLUME=1 install "$dir" --yes
expect_absent "re-run at the same version: no backup" "$calls" "backup once"

upgrade_checkout fresh-volume
STUB_DB_VOLUME=0 install "$dir" --yes --version 1.1.0
expect_absent "no database yet: no backup" "$calls" "backup once"

# --- 4. TLS_TERMINATION=proxy ---------------------------------------------------

dir=$(new_checkout default-edge)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}"
expect_contains "default edge: HTTPS probed" "$calls" "https://pos.example.invalid:443/api/v1/health/live"
expect_absent "default edge: no proxy note" "$out" "TLS_TERMINATION=proxy"

printf 'TLS_TERMINATION=proxy\nHTTP_PORT=127.0.0.1:8080\nHTTPS_PORT=127.0.0.1:8443\n' >>"$dir/.env"
STUB_DB_VOLUME=1 install "$dir" --yes
[[ $status -eq 0 ]] && pass "proxy: exit 0" || fail "proxy: exit $status"
expect_contains "proxy: Caddy probed over HTTP on the bound port" "$calls" "--resolve pos.example.invalid:8080:127.0.0.1 http://pos.example.invalid:8080/api/v1/health/live"
expect_absent "proxy: no HTTPS probe" "$calls" "https://"
expect_contains "proxy: says where to forward" "$out" "forwards it to http://127.0.0.1:8080"
expect_contains "proxy: next step names the address" "$out" "must forward https://pos.example.invalid to http://127.0.0.1:8080"
expect_absent "proxy: no certificate hint" "$out" "a public certificate needs ports 80 and 443"
expect_absent "proxy: no DNS hint" "$out" "does not resolve yet"
expect_contains "proxy: URL stays https://DOMAIN" "$out" "URL          https://pos.example.invalid"

# A front proxy with Caddy's ports left on every interface would let clients
# skip the proxy (and its TLS) entirely, so the installer refuses it.
for ports in 'HTTP_PORT=80' 'HTTP_PORT=8080' '' 'HTTP_PORT=127.0.0.1:8080\nHTTPS_PORT=8443'; do
	sed -i.bak -e '/^HTTP_PORT=/d' -e '/^HTTPS_PORT=/d' "$dir/.env"
	[[ -z $ports ]] || printf '%b\n' "$ports" >>"$dir/.env"
	STUB_DB_VOLUME=1 install "$dir" --yes
	label=${ports//\\n/ }
	[[ $status -ne 0 ]] && pass "proxy: refuses unbound ports (${label:-defaults})" || fail "proxy: accepted unbound ports (${label:-defaults})"
	expect_contains "proxy: unbound ports (${label:-defaults}) names the fix" "$out" "HTTP_PORT=127.0.0.1:8080"
	expect_absent "proxy: unbound ports (${label:-defaults}) starts nothing" "$calls" "compose up"
done
sed -i.bak -e '/^HTTP_PORT=/d' -e '/^HTTPS_PORT=/d' "$dir/.env"
printf 'HTTP_PORT=0.0.0.0:8080\nHTTPS_PORT=0.0.0.0:8443\n' >>"$dir/.env"
STUB_DB_VOLUME=1 install "$dir" --yes
[[ $status -eq 0 ]] && pass "proxy: an explicit 0.0.0.0 bind is accepted" || { fail "proxy: explicit 0.0.0.0 bind refused"; printf '%s\n' "$out" >&2; }

# --- 4b. TRUSTED_PROXY_CIDRS validation -----------------------------------------

dir=$(new_checkout proxy-cidrs)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}"
for bad in '10.0.0.0/8,172.16.0.0/12' '0.0.0.0/0' '::/0' '0.0.0.0/1 128.0.0.0/1' '::/1 8000::/1' '10.0.0.0/7' 'fc00::/15' '10.0.0.0/33' '300.1.1.1' 'example.com' '10.0.0.0/8 }' ''; do
	sed -i.bak '/^TRUSTED_PROXY_CIDRS=/d' "$dir/.env"
	printf 'TRUSTED_PROXY_CIDRS="%s"\n' "$bad" >>"$dir/.env"
	STUB_DB_VOLUME=1 STUB_BACKEND_LOG=/dev/null install "$dir" --yes
	if [[ -z $bad ]]; then
		[[ $status -eq 0 ]] && pass "proxy cidrs: empty is accepted" || fail "proxy cidrs: empty refused"
	else
		[[ $status -ne 0 ]] && pass "proxy cidrs: refuses '$bad'" || fail "proxy cidrs: accepted '$bad'"
		expect_contains "proxy cidrs: '$bad' names the format" "$out" "space-separated IPv4/IPv6"
	fi
done
sed -i.bak '/^TRUSTED_PROXY_CIDRS=/d' "$dir/.env"
printf 'TRUSTED_PROXY_CIDRS="10.0.0.0/24 172.18.0.1 fd00::/64 private_ranges"\n' >>"$dir/.env"
STUB_DB_VOLUME=1 STUB_BACKEND_LOG=/dev/null install "$dir" --yes
[[ $status -eq 0 ]] && pass "proxy cidrs: valid list accepted" || { fail "proxy cidrs: valid list refused"; printf '%s\n' "$out" >&2; }

# --- 4c. dry run --------------------------------------------------------------------

dir=$(new_checkout dry-no-email)
install "$dir" --yes --dry-run --domain localhost
[[ $status -ne 0 ]] && pass "dry run without admin email: non-zero exit ($status)" || fail "dry run without admin email: exit 0"
expect_contains "dry run without admin email: says why" "$out" "no admin email"
[[ ! -e $dir/.env ]] && pass "dry run: no .env written" || fail "dry run: .env written"

dir=$(new_checkout dry-hung-docker)
started=$SECONDS
STUB_DOCKER_HANG=1 PAYVERGE_DOCKER_TIMEOUT=2 install "$dir" --yes --dry-run --domain localhost --admin-email owner@example.com
elapsed=$((SECONDS - started))
[[ $status -eq 0 ]] && pass "dry run, hung daemon: exit 0" || { fail "dry run, hung daemon: exit $status"; printf '%s\n' "$out" >&2; }
((elapsed < 30)) && pass "dry run, hung daemon: gave up after the timeout (${elapsed}s)" || fail "dry run, hung daemon: blocked ${elapsed}s"
expect_contains "dry run, hung daemon: warned" "$out" "cannot talk to the Docker daemon"
expect_contains "dry run, hung daemon: finished" "$out" "Dry run complete"

# --- 4d. ports and project name ------------------------------------------------------

dir=$(new_checkout trial-loopback)
STUB_BACKEND_LOG=/dev/null install "$dir" --yes --domain localhost --admin-email owner@example.com --no-demo
[[ $status -eq 0 ]] && pass "trial: installs" || { fail "trial: exit $status"; printf '%s\n' "$out" >&2; }
[[ $(env_value HTTP_PORT "$dir/.env") == 127.0.0.1:80 ]] && pass "trial: HTTP on loopback" || fail "trial: HTTP_PORT=$(env_value HTTP_PORT "$dir/.env")"
[[ $(env_value HTTPS_PORT "$dir/.env") == 127.0.0.1:443 ]] && pass "trial: HTTPS on loopback" || fail "trial: HTTPS_PORT=$(env_value HTTPS_PORT "$dir/.env")"
[[ -z $(env_value PUBLIC_URL "$dir/.env") ]] && pass "trial: default port needs no PUBLIC_URL" || fail "trial: PUBLIC_URL set"

dir=$(new_checkout trial-ports)
STUB_BACKEND_LOG=/dev/null install "$dir" --yes --domain localhost --admin-email owner@example.com --no-demo --http-port 8080 --https-port 0.0.0.0:8443
[[ $(env_value HTTP_PORT "$dir/.env") == 127.0.0.1:8080 ]] && pass "trial ports: bare port bound to loopback" || fail "trial ports: HTTP_PORT=$(env_value HTTP_PORT "$dir/.env")"
[[ $(env_value HTTPS_PORT "$dir/.env") == 0.0.0.0:8443 ]] && pass "trial ports: explicit bind kept" || fail "trial ports: HTTPS_PORT=$(env_value HTTPS_PORT "$dir/.env")"
[[ $(env_value PUBLIC_URL "$dir/.env") == https://localhost:8443 ]] && pass "trial ports: PUBLIC_URL uses the port only" || fail "trial ports: PUBLIC_URL=$(env_value PUBLIC_URL "$dir/.env")"

dir=$(new_checkout demo-home)
STUB_BACKEND_LOG=/dev/null install "$dir" --yes --domain localhost --admin-email owner@example.com --demo
[[ $status -eq 0 ]] && pass "demo: installs" || { fail "demo: exit $status"; printf '%s\n' "$out" >&2; }
[[ $(env_value DEMO_DATA "$dir/.env") == true ]] && pass "demo: DEMO_DATA set" || fail "demo: DEMO_DATA=$(env_value DEMO_DATA "$dir/.env")"
[[ $(env_value PRIMARY_VENUE "$dir/.env") == bodegon-mesa-larga ]] && pass "demo: storefront served at /" || fail "demo: PRIMARY_VENUE=$(env_value PRIMARY_VENUE "$dir/.env")"
dir=$(new_checkout no-demo-home)
STUB_BACKEND_LOG=/dev/null install "$dir" --yes --domain localhost --admin-email owner@example.com --no-demo
[[ -z $(env_value PRIMARY_VENUE "$dir/.env") ]] && pass "no demo: PRIMARY_VENUE left unset" || fail "no demo: PRIMARY_VENUE=$(env_value PRIMARY_VENUE "$dir/.env")"

dir=$(new_checkout public-bind)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}" --http-port 127.0.0.1:8080 --https-port '[::1]:8443'
[[ $status -eq 0 ]] && pass "ADDR:PORT accepted" || { fail "ADDR:PORT refused"; printf '%s\n' "$out" >&2; }
[[ $(env_value HTTPS_PORT "$dir/.env") == '[::1]:8443' ]] && pass "ADDR:PORT: IPv6 bind written" || fail "ADDR:PORT: HTTPS_PORT=$(env_value HTTPS_PORT "$dir/.env")"
dir=$(new_checkout public-default)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}"
[[ -z $(env_value HTTP_PORT "$dir/.env")$(env_value HTTPS_PORT "$dir/.env") ]] && pass "public domain: ports left at the compose default" || fail "public domain: bind written"
for bad in 'abc' '0' '70000' 'localhost:8080' '1.2.3.4:' ':8080'; do
	dir=$(new_checkout "bad-port-${bad//[^a-z0-9]/_}")
	install "$dir" "${FRESH[@]}" --https-port "$bad"
	[[ $status -ne 0 ]] && pass "refuses --https-port '$bad'" || fail "accepted --https-port '$bad'"
done

dir=$(new_checkout project-name)
COMPOSE_PROJECT_NAME=pv-test STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}"
[[ $(env_value COMPOSE_PROJECT_NAME "$dir/.env") == pv-test ]] && pass "project name: saved to .env" || fail "project name: not saved"
STUB_DB_VOLUME=1 STUB_BACKEND_LOG=/dev/null install "$dir" --yes
expect_contains "project name: re-run uses the saved name" "$calls" "label=com.docker.compose.project=pv-test"
dir=$(new_checkout project-name-bad)
COMPOSE_PROJECT_NAME='Bad Name' install "$dir" "${FRESH[@]}"
[[ $status -ne 0 ]] && pass "project name: refuses an invalid name" || fail "project name: accepted 'Bad Name'"

# --- 5. ERR trap -----------------------------------------------------------------

dir=$(new_checkout err-trap)
: >"$dir/backups" # a file where the backups directory belongs
install "$dir" "${FRESH[@]}"
[[ $status -ne 0 ]] && pass "ERR trap: non-zero exit" || fail "ERR trap: exit 0"
expect_contains "ERR trap: names the failed command" "$out" "install.sh stopped at line"
expect_contains "ERR trap: command shown" "$out" 'mkdir -p'

# --- 6. in-place version pin ----------------------------------------------------

dir=$(new_checkout in-place-latest)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}"
[[ $status -eq 0 ]] && pass "in-place, no --version: exit 0" || { fail "in-place, no --version: exit $status"; printf '%s\n' "$out" >&2; }
[[ $(env_value PAYVERGE_VERSION "$dir/.env") == 9.8.7 ]] && pass "in-place, no --version: PAYVERGE_VERSION=9.8.7" || fail "in-place, no --version: PAYVERGE_VERSION=$(env_value PAYVERGE_VERSION "$dir/.env")"
expect_contains "in-place, no --version: looked up the newest release" "$calls" "releases/latest"

dir=$(new_checkout in-place-build)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}" --build
[[ $status -eq 0 ]] && pass "in-place --build: exit 0" || { fail "in-place --build: exit $status"; printf '%s\n' "$out" >&2; }
[[ $(env_value PAYVERGE_VERSION "$dir/.env") == local ]] && pass "in-place --build: PAYVERGE_VERSION=local" || fail "in-place --build: PAYVERGE_VERSION=$(env_value PAYVERGE_VERSION "$dir/.env")"
expect_absent "in-place --build: no releases/latest curl" "$calls" "releases/latest"

# A leftover PAYVERGE_VERSION=local is not a pin once the images are not built.
dir=$(new_checkout local-not-building)
STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}" --version 1.4.0 >/dev/null
sed -i.bak 's/^PAYVERGE_VERSION=.*/PAYVERGE_VERSION=local/' "$dir/.env" && rm -f "$dir/.env.bak"
STUB_DB_VOLUME=1 STUB_BACKEND_LOG=/dev/null install "$dir" --yes
[[ $(env_value PAYVERGE_VERSION "$dir/.env") == 9.8.7 ]] && pass "local without a build: resolved to the newest release" || fail "local without a build: PAYVERGE_VERSION=$(env_value PAYVERGE_VERSION "$dir/.env")"

# --- 7. public probe ----------------------------------------------------------------

dir=$(new_checkout probe-frontend)
printf '%s\n' "$(log_line info 'Bootstrap admin o***@example.com created (user id 1)')" >"$WORK/probe-frontend.log"
# The root URL fails; /api/v1/health/live does not. One try, no 2-minute wait.
PAYVERGE_PROBE_ATTEMPTS=1 STUB_BACKEND_LOG="$WORK/probe-frontend.log" STUB_CURL_FAIL='https://pos.example.invalid:443/' install "$dir" "${FRESH[@]}"
[[ $status -ne 0 ]] && pass "frontend probe: non-zero exit ($status)" || fail "frontend probe: exit 0"
expect_contains "frontend probe: summary still printed" "$out" "Payverge is running"
pw=$(sed -n 's/^  Password     \([A-Za-z0-9-]*\)$/\1/p' <<<"$out")
[[ $pw =~ ^[A-Za-z0-9]{5}(-[A-Za-z0-9]{5}){3}$ ]] && pass "frontend probe: admin password shown" || fail "frontend probe: admin password not shown"
expect_contains "frontend probe: names the frontend" "$out" "frontend /"
expect_absent "frontend probe: backend passed" "$out" "backend /api/v1/health/live does not answer"
# Probes that pass are the fresh-install cases above (created: exit 0).

# --- 8. release signature -----------------------------------------------------------

release_files=$WORK/release-files
release_fixture 9.8.7 "$release_files"
cat >"$STUBS/cosign" <<'EOF'
#!/bin/sh
echo "cosign $*" >>"$STUB_LOG"
exit "${STUB_COSIGN_STATUS:-0}"
EOF
chmod +x "$STUBS/cosign"

dir=$(new_installed sig-ok)
STUB_CURL_DIR=$release_files STUB_COSIGN_STATUS=0 STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}" --version 9.8.7
[[ $status -eq 0 ]] && pass "signature ok: exit 0" || { fail "signature ok: exit $status"; printf '%s\n' "$out" >&2; }
expect_contains "signature ok: verify-blob" "$calls" "verify-blob"
expect_contains "signature ok: certificate identity" "$calls" "--certificate-identity https://github.com/stdevMac/payverge/.github/workflows/release.yml@refs/heads/main"
expect_contains "signature ok: bundle" "$calls" "--bundle SHA256SUMS.sigstore.json"
expect_contains "signature ok: stack started" "$calls" "up -d"
expect_contains "signature ok: noted" "$out" "signature verified (cosign)"

dir=$(new_installed sig-bad)
STUB_CURL_DIR=$release_files STUB_COSIGN_STATUS=1 STUB_BACKEND_LOG=/dev/null install "$dir" "${FRESH[@]}" --version 9.8.7
[[ $status -ne 0 ]] && pass "signature bad: non-zero exit ($status)" || fail "signature bad: exit 0"
expect_contains "signature bad: mentions the signature" "$out" "signature check failed"
expect_absent "signature bad: nothing started" "$calls" "up -d"

rm -f "$STUBS/cosign"
dir=$(new_installed sig-required)
STUB_PATH_TAIL=$(path_without_cosign) STUB_CURL_DIR=$release_files STUB_BACKEND_LOG=/dev/null \
	install "$dir" "${FRESH[@]}" --version 9.8.7 --require-signature
[[ $status -ne 0 ]] && pass "require-signature, no cosign: non-zero exit ($status)" || fail "require-signature, no cosign: exit 0"
expect_contains "require-signature, no cosign: says to install cosign" "$out" "https://docs.sigstore.dev/cosign/system_config/installation/"
expect_contains "require-signature, no cosign: names the flag" "$out" "--require-signature"
expect_absent "require-signature, no cosign: no download" "$calls" "curl "
expect_absent "require-signature, no cosign: nothing started" "$calls" "up -d"

if ((failures > 0)); then
	printf '\ndeploy-install: %d check(s) failed\n' "$failures" >&2
	exit 1
fi
printf '\ndeploy-install: all checks passed\n'

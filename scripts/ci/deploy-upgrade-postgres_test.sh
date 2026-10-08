#!/usr/bin/env bash
# shellcheck disable=SC2015 # `[[ ... ]] && pass || fail` is the assertion style of this file
# Hermetic test of deploy/upgrade-postgres.sh and deploy/postgres/pg18-guard.sh:
# no Docker daemon. A stub `docker` on PATH records every call and answers
# from environment variables. Covered:
#
#   - no legacy db volume, an empty one, or a pgdata volume that already holds
#     PostgreSQL 18: nothing is stopped or changed (exit 0);
#   - --dry-run prints the plan and changes nothing; without a terminal and
#     without --yes the script refuses before stopping anything;
#   - a release before PostgreSQL 18 (no pgdata mount) is refused;
#   - the happy path: compose down, PostgreSQL 15 on the db volume, row
#     counts and pg_dumpall into backups/pg18-upgrade-*/, PostgreSQL 18 on a
#     freshly created pgdata volume (labelled for compose), restore, the
#     superuser password cleared again, ANALYZE, counts compared, compose up;
#   - a row-count mismatch or an unexpected restore error fails, removes the
#     new pgdata volume and never removes the legacy db volume;
#   - the guard: `check` exits 3 only when legacy data exists and PostgreSQL
#     18 data does not, and 4 when 15 has run on the legacy volume since the
#     upgrade recorded its pg_control checksum (a rollback: pgdata is stale);
#     otherwise it hands over to docker-entrypoint.sh.
#
# Usage: scripts/ci/deploy-upgrade-postgres_test.sh
set -Eeuo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/payverge-pgupgrade-test.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

failures=0
pass() { printf 'ok   - %s\n' "$*"; }
fail() {
	printf 'FAIL - %s\n' "$*" >&2
	failures=$((failures + 1))
}

STUBS=$WORK/bin
mkdir -p "$STUBS"
cat >"$STUBS/docker" <<'EOF'
#!/usr/bin/env bash
# Stub docker. Every call is appended to $STUB_LOG.
printf '%s\n' "$*" >>"$STUB_LOG"
args=" $* "
case "$1 $2" in
"volume inspect")
	[[ " ${STUB_VOLUMES:-} " == *" $3 "* ]] && exit 0
	exit 1
	;;
"volume create" | "volume rm") exit 0 ;;
"compose down" | "compose up") exit 0 ;;
esac
case $1 in
stop | rm | logs) exit 0 ;;
run)
	if [[ $args == *" --rm "* ]]; then
		# The stale-pgdata check (pgdata already holds 18) and the pg_control
		# checksum taken after the dump.
		if [[ $args == *"echo stale"* ]]; then
			[[ ${STUB_STALE:-0} == 1 ]] && echo stale
			exit 0
		fi
		[[ $args == *"cksum </old/global/pg_control"* ]] && { echo "${STUB_CONTROL_CKSUM:-1234567 8192}"; exit 0; }
		[[ $args == *":/old:ro "* ]] && printf '%s\n' "${STUB_LEGACY_VERSION:-}"
		[[ $args == *":/new:ro "* ]] && printf '%s\n' "${STUB_NEW_VERSION:-}"
		exit 0
	fi
	exit 0
	;;
exec)
	[[ $args == *" pg_isready "* ]] && exit 0
	[[ $args == *" vacuumdb "* ]] && exit 0
	[[ $args == *" pg_dumpall "* ]] && { echo "-- stub dump"; exit 0; }
	user=""
	prev=""
	for a in "$@"; do
		[[ $prev == -U ]] && user=$a
		prev=$a
	done
	if [[ $args == *"SELECT rolsuper"* ]]; then
		[[ $user == "${STUB_SUPERUSER:-postgres}" ]] && { echo t; exit 0; }
		echo "psql: error: role \"$user\" does not exist" >&2
		exit 2
	fi
	[[ $args == *"rolpassword IS NOT NULL"* ]] && { echo f; exit 0; }
	[[ $args == *"SELECT datname"* ]] && { printf 'payverge\npostgres\n'; exit 0; }
	[[ $args == *"ALTER ROLE"* ]] && exit 0
	if [[ $args == *" ON_ERROR_STOP=0 "* ]]; then
		cat >/dev/null
		echo "psql:<stdin>:20: ERROR:  role \"${STUB_SUPERUSER:-postgres}\" already exists" >&2
		[[ ${STUB_RESTORE_ERROR:-0} == 1 ]] && echo 'psql:<stdin>:99: ERROR:  relation "bills" does not exist' >&2
		if [[ ${STUB_RESTORE_ERROR:-0} == fatal ]]; then
			echo 'psql:<stdin>:99: FATAL:  terminating connection due to administrator command' >&2
			exit 2
		fi
		exit 0
	fi
	if [[ $args == *" ON_ERROR_STOP=1 "* ]]; then
		cat >/dev/null
		db=${args##*-d }
		db=${db%% *}
		[[ $db == postgres ]] && exit 0
		echo "public.bills|3"
		if [[ $args == *"-pgupgrade-new "* && ${STUB_COUNT_MISMATCH:-0} == 1 ]]; then
			echo "public.users|1"
		else
			echo "public.users|2"
		fi
		exit 0
	fi
	;;
esac
echo "stub docker: unexpected: $*" >&2
exit 99
EOF
chmod +x "$STUBS/docker"

# A minimal install directory: the release's compose file and the script.
new_install() { # new_install NAME -> prints the directory
	local dir=$WORK/$1
	mkdir -p "$dir"
	cp "$REPO/deploy/docker-compose.yml" "$REPO/deploy/upgrade-postgres.sh" "$dir/"
	printf 'DB_USER=payverge\n' >"$dir/.env"
	printf '%s' "$dir"
}

run_upgrade() { # run_upgrade DIR ARGS... (env STUB_* set by the caller)
	local dir=$1
	shift
	STUB_LOG=$dir/docker.log PATH="$STUBS:$PATH" bash "$dir/upgrade-postgres.sh" --dir "$dir" "$@" \
		>"$dir/out.txt" 2>&1 </dev/null
}

stopped() { grep -q '^compose down' "$1/docker.log" 2>/dev/null; }

# --- nothing to do -------------------------------------------------------------

d=$(new_install no-legacy)
status=0
STUB_VOLUMES="" run_upgrade "$d" --yes || status=$?
[[ $status == 0 ]] && grep -q 'Nothing to upgrade' "$d/out.txt" && ! stopped "$d" &&
	pass "no db volume: nothing to upgrade, nothing stopped" ||
	fail "no db volume: status $status, $(cat "$d/out.txt")"

d=$(new_install empty-legacy)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION="" run_upgrade "$d" --yes || status=$?
[[ $status == 0 ]] && grep -q 'holds no database' "$d/out.txt" && ! stopped "$d" &&
	pass "empty db volume (a fresh PostgreSQL 18 install): nothing to upgrade" ||
	fail "empty db volume: status $status, $(cat "$d/out.txt")"

d=$(new_install already)
status=0
STUB_VOLUMES="payverge_db payverge_pgdata" STUB_LEGACY_VERSION=15 STUB_NEW_VERSION=18 run_upgrade "$d" --yes || status=$?
[[ $status == 0 ]] && grep -q 'already holds a PostgreSQL 18 database' "$d/out.txt" && ! stopped "$d" &&
	pass "pgdata already holds PostgreSQL 18: nothing to upgrade, rollback copy named" ||
	fail "already upgraded: status $status, $(cat "$d/out.txt")"

d=$(new_install stale)
status=0
STUB_VOLUMES="payverge_db payverge_pgdata" STUB_LEGACY_VERSION=15 STUB_NEW_VERSION=18 STUB_STALE=1 run_upgrade "$d" --yes || status=$?
[[ $status != 0 ]] && grep -q 'rollback' "$d/out.txt" && grep -q 'docker volume rm payverge_pgdata' "$d/out.txt" && ! stopped "$d" &&
	pass "pgdata older than a rollback is refused as stale, with the volume to remove" ||
	fail "stale pgdata: status $status, $(cat "$d/out.txt")"

d=$(new_install pg16)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=16 run_upgrade "$d" --yes || status=$?
[[ $status != 0 ]] && grep -q 'from 15 only' "$d/out.txt" && ! stopped "$d" &&
	pass "a legacy major other than 15 is refused before anything stops" ||
	fail "legacy 16: status $status, $(cat "$d/out.txt")"

d=$(new_install old-release)
sed -i.bak 's|pgdata:/var/lib/postgresql|db:/var/lib/postgresql/data|' "$d/docker-compose.yml"
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 run_upgrade "$d" --yes || status=$?
[[ $status != 0 ]] && grep -q 'before PostgreSQL 18' "$d/out.txt" && ! stopped "$d" &&
	pass "release files from before PostgreSQL 18 are refused" ||
	fail "old release files: status $status, $(cat "$d/out.txt")"

d=$(new_install dry-run)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 run_upgrade "$d" --dry-run || status=$?
[[ $status == 0 ]] && grep -q 'Dry run: nothing was changed' "$d/out.txt" && ! stopped "$d" &&
	! grep -qE '^(run -d|volume (create|rm))' "$d/docker.log" &&
	pass "--dry-run prints the plan and changes nothing" ||
	fail "--dry-run: status $status, $(cat "$d/out.txt")"

d=$(new_install no-tty)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 run_upgrade "$d" || status=$?
[[ $status != 0 ]] && grep -q 're-run with --yes' "$d/out.txt" && ! stopped "$d" &&
	pass "without a terminal and --yes it refuses before stopping anything" ||
	fail "no tty: status $status, $(cat "$d/out.txt")"

# --- happy path ----------------------------------------------------------------

d=$(new_install happy)
status=0
STUB_VOLUMES="payverge_db payverge_pgdata" STUB_LEGACY_VERSION=15 STUB_NEW_VERSION="" run_upgrade "$d" --yes || status=$?
log=$d/docker.log
if [[ $status == 0 ]]; then
	pass "happy path exits 0"
else
	fail "happy path: status $status, $(cat "$d/out.txt")"
fi
order=$(grep -nE '^(compose down|run -d --name payverge-pgupgrade-old|exec payverge-pgupgrade-old pg_dumpall|volume create|run -d --name payverge-pgupgrade-new|exec payverge-pgupgrade-new vacuumdb|compose up -d)' "$log" | cut -d: -f2- | awk '{print $1, $2, $3, $4}')
want=$'compose down  \nrun -d --name payverge-pgupgrade-old\nexec payverge-pgupgrade-old pg_dumpall -U\nvolume create --label com.docker.compose.project=payverge\nrun -d --name payverge-pgupgrade-new\nexec payverge-pgupgrade-new vacuumdb -U\ncompose up -d '
[[ $order == "$want" ]] && pass "steps run in order: down, dump with 15, new volume, restore with 18, analyze, up" ||
	fail "step order:"$'\n'"$order"
grep -q -- '-v payverge_db:/var/lib/postgresql/data postgres:15\.19-alpine@sha256:' "$log" &&
	pass "PostgreSQL 15 (pinned) runs on the legacy volume at its old path" || fail "old server mount/image: $(grep 'pgupgrade-old' "$log" | head -n 2)"
grep -q -- '-v payverge_pgdata:/var/lib/postgresql postgres:18\.' "$log" &&
	pass "PostgreSQL 18 runs on pgdata at /var/lib/postgresql" || fail "new server mount/image: $(grep 'run -d --name payverge-pgupgrade-new' "$log")"
grep -q -- '--label com.docker.compose.volume=pgdata payverge_pgdata' "$log" &&
	pass "the new volume carries the compose labels" || fail "volume labels: $(grep 'volume create' "$log")"
grep -q -- '-e POSTGRES_PASSWORD -v' "$log" && ! grep -qE 'POSTGRES_PASSWORD=[^ ]' "$log" &&
	pass "the throwaway superuser password never appears in argv" || fail "password in argv"
grep -q 'ALTER ROLE "postgres" PASSWORD NULL' "$log" &&
	pass "the superuser's password is cleared again, as init-app-role.sql left it" || fail "no ALTER ROLE PASSWORD NULL"
dumps=("$d"/backups/pg18-upgrade-*/pg15-dumpall.sql.gz)
[[ -f ${dumps[0]} ]] && cmp -s "$(dirname "${dumps[0]}")/row-counts-pg15.txt" "$(dirname "${dumps[0]}")/row-counts-pg18.txt" &&
	grep -q '^payverge|public.users|2$' "$(dirname "${dumps[0]}")/row-counts-pg15.txt" &&
	pass "dump and per-database row counts are kept in backups/pg18-upgrade-*/" || fail "dump dir: $(ls -R "$d/backups" 2>&1)"
! grep -q '^volume rm payverge_db' "$log" && pass "the legacy db volume is never removed" || fail "legacy volume removed"
grep -q 'payverge-pg15-control.cksum.* sh 1234567 8192$' "$log" &&
	grep -q -- '-v payverge_pgdata:/new ' "$log" &&
	pass "the PostgreSQL 15 pg_control checksum is recorded in pgdata for the guard" ||
	fail "control checksum marker: $(grep 'cksum' "$log")"

d=$(new_install no-start)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 run_upgrade "$d" --yes --no-start || status=$?
[[ $status == 0 ]] && ! grep -q '^compose up' "$d/docker.log" && grep -q 'docker compose up -d' "$d/out.txt" &&
	pass "--no-start leaves the stack stopped and says how to start it" || fail "--no-start: status $status"

# --- failures ------------------------------------------------------------------

d=$(new_install mismatch)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 STUB_COUNT_MISMATCH=1 run_upgrade "$d" --yes || status=$?
[[ $status != 0 ]] && grep -q 'row counts differ' "$d/out.txt" && grep -q '^volume rm payverge_pgdata' "$d/docker.log" &&
	! grep -q '^volume rm payverge_db' "$d/docker.log" && ! grep -q '^compose up' "$d/docker.log" &&
	pass "a row-count mismatch fails, removes pgdata, keeps db and does not start the stack" ||
	fail "mismatch: status $status, $(cat "$d/out.txt")"

d=$(new_install restore-error)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 STUB_RESTORE_ERROR=1 run_upgrade "$d" --yes || status=$?
[[ $status != 0 ]] && grep -q 'restore reported errors' "$d/out.txt" && grep -q 'relation "bills"' "$d/out.txt" &&
	grep -q '^volume rm payverge_pgdata' "$d/docker.log" && ! grep -q '^volume rm payverge_db' "$d/docker.log" &&
	pass "an unexpected restore error fails and names it; the expected 'role exists' does not" ||
	fail "restore error: status $status, $(cat "$d/out.txt")"

d=$(new_install restore-fatal)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 STUB_RESTORE_ERROR=fatal run_upgrade "$d" --yes || status=$?
[[ $status != 0 ]] && grep -q 'restore reported errors' "$d/out.txt" && grep -q 'FATAL:  terminating connection' "$d/out.txt" &&
	grep -q 'exited with status 2' "$d/out.txt" && grep -q '^volume rm payverge_pgdata' "$d/docker.log" &&
	pass "a FATAL restore (lost connection, non-zero psql) fails" ||
	fail "restore fatal: status $status, $(cat "$d/out.txt")"

d=$(new_install old-style-superuser)
status=0
STUB_VOLUMES="payverge_db" STUB_LEGACY_VERSION=15 STUB_SUPERUSER=payverge run_upgrade "$d" --yes || status=$?
[[ $status == 0 ]] && grep -q 'POSTGRES_USER=payverge' "$d/docker.log" &&
	pass "an install whose superuser is DB_USER is found and kept as the superuser" ||
	fail "DB_USER superuser: status $status, $(cat "$d/out.txt")"

# --- the guard -----------------------------------------------------------------

guard=$REPO/deploy/postgres/pg18-guard.sh
g=$WORK/guard
mkdir -p "$g/legacy" "$g/new/18/docker" "$g/bin"
printf '#!/bin/sh\necho "entrypoint $*"\n' >"$g/bin/docker-entrypoint.sh"
chmod +x "$g/bin/docker-entrypoint.sh"
guard_run() { PATH="$g/bin:$PATH" PAYVERGE_PGDATA_DIR=$g/new/18/docker PAYVERGE_LEGACY_PGDATA_DIR=$g/legacy sh "$guard" "$@"; }

status=0
guard_run check >/dev/null || status=$?
[[ $status == 0 ]] && pass "guard check: no data anywhere (fresh install) -> 0" || fail "guard fresh: $status"
echo 15 >"$g/legacy/PG_VERSION"
status=0
out=$(guard_run check) || status=$?
[[ $status == 3 && $out == *"PostgreSQL 15"* ]] && pass "guard check: legacy 15 data, no 18 data -> 3" || fail "guard legacy: $status $out"
out=$(timeout 2 sh -c 'PATH="$1/bin:$PATH" PAYVERGE_PGDATA_DIR=$1/new/18/docker PAYVERGE_LEGACY_PGDATA_DIR=$1/legacy sh "$2" postgres 2>&1' _ "$g" "$guard" || true)
[[ $out == *"upgrade-postgres.sh"* && $out != *"entrypoint"* ]] &&
	pass "guard: with only legacy data it explains, idles and never starts postgres" || fail "guard idle: $out"
echo 18 >"$g/new/18/docker/PG_VERSION"
status=0
guard_run check >/dev/null || status=$?
out=$(guard_run postgres)
[[ $status == 0 && $out == "entrypoint postgres" ]] &&
	pass "guard: once pgdata holds 18 data it hands over to docker-entrypoint.sh" || fail "guard upgraded: $status $out"

# The upgrade's marker: the checksum of 15's pg_control when it was dumped.
mkdir -p "$g/legacy/global"
printf 'control-at-upgrade' >"$g/legacy/global/pg_control"
cksum <"$g/legacy/global/pg_control" | awk '{print $1, $2}' >"$g/new/payverge-pg15-control.cksum"
guard_run_m() { PAYVERGE_UPGRADE_MARKER=$g/new/payverge-pg15-control.cksum guard_run "$@"; }
status=0
guard_run_m check >/dev/null || status=$?
out=$(guard_run_m postgres)
[[ $status == 0 && $out == "entrypoint postgres" ]] &&
	pass "guard: pg_control unchanged since the upgrade -> starts 18" || fail "guard marker match: $status $out"
printf 'control-after-rollback' >"$g/legacy/global/pg_control"
status=0
out=$(guard_run_m check) || status=$?
[[ $status == 4 && $out == *"stale-pgdata"* ]] && pass "guard check: 15 ran after the upgrade (rollback) -> 4" || fail "guard stale: $status $out"
out=$(timeout 2 sh -c 'PATH="$1/bin:$PATH" PAYVERGE_PGDATA_DIR=$1/new/18/docker PAYVERGE_LEGACY_PGDATA_DIR=$1/legacy PAYVERGE_UPGRADE_MARKER=$1/new/payverge-pg15-control.cksum sh "$2" postgres 2>&1' _ "$g" "$guard" || true)
[[ $out == *"stale"* && $out == *"_pgdata"* && $out != *"entrypoint"* ]] &&
	pass "guard: with stale pgdata it explains, idles and never starts postgres" || fail "guard stale idle: $out"
rm "$g/new/payverge-pg15-control.cksum"
status=0
guard_run_m check >/dev/null || status=$?
[[ $status == 0 ]] && pass "guard: no marker (pgdata not made by the upgrade script) -> 0" || fail "guard no marker: $status"

if ((failures)); then
	printf '\n%d check(s) failed\n' "$failures" >&2
	exit 1
fi
printf '\nall checks passed\n'

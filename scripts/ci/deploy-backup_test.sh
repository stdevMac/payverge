#!/usr/bin/env bash
# Round trip of deploy/backup/backup.sh and restore.sh against a real,
# throwaway PostgreSQL server (initdb + pg_ctl on 127.0.0.1, a high port, trust
# auth, deleted afterwards). No Docker. Covered:
#
#   - a set holds a pg_dump, the uploads and the MinIO data (without MinIO's
#     tmp/ and multipart/), a MANIFEST and SHA256SUMS, and becomes the weekly
#     and monthly copy;
#   - the sets are handed to BACKUP_UID:BACKUP_GID, and only when those are set;
#   - restore puts the database, the uploads and the MinIO data back exactly;
#   - restore refuses while something else is connected, and refuses a set
#     whose checksums do not match;
#   - a dump that passes its checksums but fails pg_restore, or an archive
#     that fails to unpack, leaves the live database and volumes untouched and
#     no temporary database or staging directory behind;
#   - an archive that fails once (a file changed while tar read it) is retried;
#   - an archive that keeps failing leaves a set without it, named under
#     archives_failed, not promoted, with valid checksums, and a non-zero exit;
#     restoring that set warns and leaves that volume alone;
#   - a set of a database with no schema yet (a fresh install whose backend is
#     still migrating) stays daily only;
#   - a set of a bare genesis baseline (schema_migrations present but empty,
#     migration version 0) is promoted and records schema_version=0;
#   - bad BACKUP_UID and BACKUP_ARCHIVE_ATTEMPTS values are refused;
#   - every run records "ok <epoch>" or "failed <epoch>" in .status, and
#     `backup.sh health` (the compose healthcheck) passes only for an ok or
#     pending status younger than BACKUP_MAX_AGE_HOURS (default 26).
#
# The scripts run with this host's sh, tar and PostgreSQL client instead of the
# container's busybox and postgres:15 client. `sha256sum` (busybox's -s flag),
# `chown` (recorded, not applied: the test is not root), `sleep` (no wait
# between retries) and a `tar` that can fail on purpose are shims on PATH.
#
# Usage: scripts/ci/deploy-backup_test.sh
# Without PostgreSQL server binaries (initdb, pg_ctl) on PATH, in PG_BINDIR or
# in /usr/lib/postgresql/*/bin it skips, unless DEPLOY_BACKUP_TEST_REQUIRE_PG=1.
set -Eeuo pipefail

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
BACKUP_SH=$REPO/deploy/backup/backup.sh
RESTORE_SH=$REPO/deploy/backup/restore.sh

find_pg_bindir() {
	local dir
	if [[ -n ${PG_BINDIR:-} ]]; then
		echo "$PG_BINDIR"
		return 0
	fi
	if command -v initdb >/dev/null 2>&1 && command -v pg_ctl >/dev/null 2>&1; then
		dirname "$(command -v initdb)"
		return 0
	fi
	# Debian/Ubuntu (GitHub runners): the newest installed major version.
	local found=""
	for dir in /usr/lib/postgresql/*/bin; do
		[[ -x $dir/initdb ]] && found=$dir
	done
	[[ -n $found ]] || return 1
	echo "$found"
}

if ! PG_BIN=$(find_pg_bindir); then
	if [[ ${DEPLOY_BACKUP_TEST_REQUIRE_PG:-0} == 1 ]]; then
		echo "deploy-backup: no PostgreSQL server binaries found (set PG_BINDIR)" >&2
		exit 1
	fi
	echo "deploy-backup: SKIP, no PostgreSQL server binaries (initdb, pg_ctl) found"
	exit 0
fi
for tool in initdb pg_ctl pg_dump pg_restore psql createdb; do
	[[ -x $PG_BIN/$tool ]] || {
		echo "deploy-backup: $PG_BIN/$tool is missing" >&2
		exit 1
	}
done
export PATH="$PG_BIN:$PATH"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/payverge-backup-test.XXXXXX")
PGDATA_DIR=$WORK/pgdata
cleanup() {
	pg_ctl -D "$PGDATA_DIR" -m immediate stop >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

failures=0
pass() { printf 'ok   - %s\n' "$*"; }
fail() {
	printf 'FAIL - %s\n' "$*" >&2
	failures=$((failures + 1))
}
check() {
	local name=$1
	shift
	if "$@"; then pass "$name"; else fail "$name"; fi
}
contains() { grep -qF -- "$2" "$1"; }

# --- throwaway PostgreSQL ---------------------------------------------------------

initdb -D "$PGDATA_DIR" -U postgres --auth=trust -E UTF8 --no-sync >"$WORK/initdb.log" 2>&1 ||
	{
		cat "$WORK/initdb.log" >&2
		exit 1
	}
started=0
for offset in 0 1 2 3 4 5 6 7; do
	PORT=$((54400 + ($$ % 400) + offset * 7))
	if pg_ctl -D "$PGDATA_DIR" -l "$WORK/postgres.log" -w -t 30 \
		-o "-p $PORT -c listen_addresses=127.0.0.1 -c unix_socket_directories='' -c fsync=off" start >/dev/null 2>&1; then
		started=1
		break
	fi
done
[[ $started == 1 ]] || {
	cat "$WORK/postgres.log" >&2
	echo "deploy-backup: could not start PostgreSQL" >&2
	exit 1
}
export PGHOST=127.0.0.1 PGPORT=$PORT PGUSER=payverge PGDATABASE=payverge
unset PGPASSWORD PGSERVICE PGSSLMODE
# The role deploy/postgres/init-app-role.sql gives the stack: backup.sh and
# restore.sh log in as an owner that is not a superuser, so the test does too.
psql -X -q -U postgres -d postgres -v ON_ERROR_STOP=1 \
	-c "CREATE ROLE payverge LOGIN NOSUPERUSER NOCREATEROLE CREATEDB NOREPLICATION NOBYPASSRLS"
createdb payverge
sql() { psql -X -q -At -v ON_ERROR_STOP=1 -c "$1"; }
[[ $(sql "SELECT rolsuper FROM pg_roles WHERE rolname = current_user") == f ]] ||
	{
		echo "deploy-backup: the test role must not be a superuser" >&2
		exit 1
	}
# pg_trgm: the one extension the genesis schema creates, as the owner (a
# trusted extension), so the dump restores it without a superuser.
sql "CREATE EXTENSION pg_trgm"
sql "CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL);
     INSERT INTO schema_migrations VALUES (221, false);
     CREATE TABLE bills (id serial PRIMARY KEY, note text NOT NULL, cents bigint NOT NULL);
     CREATE INDEX bills_note_trgm ON bills USING gin (note gin_trgm_ops);
     INSERT INTO bills (note, cents) VALUES ('table 4', 4250), ('table 9', 1999), ('ñandú', 100);"
db_state() { sql "SELECT string_agg(id || ':' || note || ':' || cents, ',' ORDER BY id) FROM bills"; }
DB_ORIGINAL=$(db_state)

# --- volumes and shims -------------------------------------------------------------

STORAGE=$WORK/vol/storage
MINIO=$WORK/vol/minio
mkdir -p "$STORAGE/receipts" "$MINIO/payverge-public/logo.png" "$MINIO/.minio.sys/config" \
	"$MINIO/.minio.sys/tmp/abc" "$MINIO/.minio.sys/multipart/def"
printf 'receipt 1\n' >"$STORAGE/receipts/r1.pdf"
printf 'logo bytes\n' >"$STORAGE/logo.png"
printf 'xl meta\n' >"$MINIO/payverge-public/logo.png/xl.meta"
printf '{"version":"1"}\n' >"$MINIO/.minio.sys/config/config.json"
printf 'half an upload\n' >"$MINIO/.minio.sys/tmp/abc/part"
printf 'a multipart part\n' >"$MINIO/.minio.sys/multipart/def/part.1"
cp -R "$STORAGE" "$WORK/storage.orig"
cp -R "$MINIO" "$WORK/minio.orig"

REAL_TAR=$(command -v tar)
STUBS=$WORK/bin
mkdir -p "$STUBS"
cat >"$STUBS/sha256sum" <<'EOF'
#!/bin/sh
# busybox `sha256sum -c -s` == shasum -a 256 -c -s
exec shasum -a 256 "$@"
EOF
cat >"$STUBS/chown" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$CHOWN_LOG"
EOF
cat >"$STUBS/sleep" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$STUBS/tar" <<'EOF'
#!/bin/sh
# Fails the first TAR_FAIL_TIMES archives of TAR_FAIL_SRC the way tar does when
# a file changes while it is read; everything else is the real tar.
if [ "$1" = "-C" ] && [ -n "${TAR_FAIL_SRC:-}" ] && [ "$2" = "$TAR_FAIL_SRC" ]; then
	n=$(cat "$TAR_FAIL_COUNT" 2>/dev/null || echo 0)
	if [ "$n" -lt "${TAR_FAIL_TIMES:-0}" ]; then
		echo $((n + 1)) >"$TAR_FAIL_COUNT"
		echo "tar: ./receipts/r1.pdf: file changed as we read it" >&2
		exit 1
	fi
fi
exec "$REAL_TAR" "$@"
EOF
chmod +x "$STUBS"/*
export REAL_TAR CHOWN_LOG=$WORK/chown.log TAR_FAIL_COUNT=$WORK/tar-fail-count

# run_backup ROOT [VAR=VALUE...]: backup.sh once into ROOT; output in $WORK/out.
run_backup() {
	local root=$1 status=0
	shift
	: >"$CHOWN_LOG"
	rm -f "$TAR_FAIL_COUNT"
	env PATH="$STUBS:$PATH" BACKUP_ROOT="$root" STORAGE_SRC="$STORAGE" MINIO_SRC="$MINIO" \
		PAYVERGE_VERSION=1.2.3 "$@" sh "$BACKUP_SH" once >"$WORK/out" 2>&1 || status=$?
	return "$status"
}

# run_restore ROOT ARG...: restore.sh into the test volumes; output in $WORK/out.
run_restore() {
	local root=$1 status=0
	shift
	: >"$CHOWN_LOG"
	env PATH="$STUBS:$PATH" BACKUP_ROOT="$root" STORAGE_DST="$STORAGE" MINIO_DST="$MINIO" \
		sh "$RESTORE_SH" "$@" >"$WORK/out" 2>&1 || status=$?
	return "$status"
}

only_set() { find "$1/daily" -mindepth 1 -maxdepth 1 -type d | head -n 1; }
show_out() { sed 's/^/    | /' "$WORK/out" >&2; }

# --- 1. a complete set ---------------------------------------------------------------

ROOT1=$WORK/backups1
if run_backup "$ROOT1" BACKUP_UID="$(id -u)" BACKUP_GID="$(id -g)"; then
	pass "complete: exit 0"
else
	fail "complete: exit 0"
	show_out
fi
SET1=$(only_set "$ROOT1")
for f in db.dump storage.tar.gz minio.tar.gz MANIFEST SHA256SUMS; do
	check "complete: has $f" test -s "$SET1/$f"
done
check "complete: MANIFEST names the version" contains "$SET1/MANIFEST" "payverge_version=1.2.3"
check "complete: MANIFEST has the schema version" contains "$SET1/MANIFEST" "schema_version=221"
check "complete: MANIFEST lists no failed archive" bash -c "! grep -q archives_failed '$SET1/MANIFEST'"
check "complete: checksums verify" bash -c "cd '$SET1' && shasum -a 256 -c -s SHA256SUMS"
check "complete: checksums cover all three payloads" bash -c \
	"grep -q ' db.dump\$' '$SET1/SHA256SUMS' && grep -q ' storage.tar.gz\$' '$SET1/SHA256SUMS' && grep -q ' minio.tar.gz\$' '$SET1/SHA256SUMS'"
"$REAL_TAR" -tzf "$SET1/minio.tar.gz" >"$WORK/minio.list"
check "complete: MinIO objects archived" contains "$WORK/minio.list" "payverge-public/logo.png/xl.meta"
check "complete: MinIO config archived" contains "$WORK/minio.list" ".minio.sys/config/config.json"
check "complete: MinIO tmp/ and multipart/ left out" bash -c "! grep -qE 'minio.sys/(tmp|multipart)' '$WORK/minio.list'"
check "complete: promoted to weekly" test -n "$(ls "$ROOT1/weekly")"
check "complete: promoted to monthly" test -n "$(ls "$ROOT1/monthly")"
check "complete: no partial set left" test -z "$(find "$ROOT1" -maxdepth 1 -name '.partial-*')"
check "complete: sets handed to BACKUP_UID:BACKUP_GID" \
	contains "$CHOWN_LOG" "-R $(id -u):$(id -g) $ROOT1/daily $ROOT1/weekly $ROOT1/monthly"

# --- 2. restore puts everything back ----------------------------------------------------

sql "UPDATE bills SET cents = 0 WHERE id = 1; DELETE FROM bills WHERE id = 2; INSERT INTO bills (note, cents) VALUES ('after', 1);"
rm "$STORAGE/logo.png"
printf 'written after the backup\n' >"$STORAGE/new.txt"
printf 'changed\n' >"$MINIO/payverge-public/logo.png/xl.meta"
if run_restore "$ROOT1" latest --yes; then
	pass "restore: exit 0"
else
	fail "restore: exit 0"
	show_out
fi
check "restore: database is back" test "$(db_state)" = "$DB_ORIGINAL"
check "restore: pg_trgm restored without a superuser" test "$(sql "SELECT count(*) FROM pg_extension WHERE extname = 'pg_trgm'")" = 1
check "restore: the restored database belongs to the app role" \
	test "$(sql "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()")" = payverge
check "restore: uploads are back" diff -r "$WORK/storage.orig" "$STORAGE"
check "restore: MinIO objects are back" diff -r "$WORK/minio.orig/payverge-public" "$MINIO/payverge-public"
check "restore: MinIO config is back" diff -r "$WORK/minio.orig/.minio.sys/config" "$MINIO/.minio.sys/config"
check "restore: files given to the backend user" contains "$CHOWN_LOG" "-R 65532:65532 $STORAGE"
check "restore: MinIO files given to the backend user" contains "$CHOWN_LOG" "-R 65532:65532 $MINIO"
no_leftovers() {
	[[ $(sql "SELECT count(*) FROM pg_database WHERE datname IN ('payverge_restore', 'payverge_pre_restore')") == 0 ]] &&
		[[ ! -e $STORAGE/.payverge-restore-staging && ! -e $MINIO/.payverge-restore-staging ]]
}
check "restore: no temporary database or staging directory left" no_leftovers

# --- 3. restore refuses while connected, and refuses a corrupt set -------------------------

psql -X -q -c "SELECT pg_sleep(60)" >/dev/null 2>&1 &
holder=$!
for _ in $(seq 1 50); do
	[[ $(sql "SELECT count(*) FROM pg_stat_activity WHERE datname = 'payverge' AND query LIKE '%pg_sleep%' AND pid <> pg_backend_pid()") -ge 1 ]] && break
	/bin/sleep 0.2
done
sql "UPDATE bills SET note = 'kept' WHERE id = 3"
if run_restore "$ROOT1" latest --yes; then
	fail "connected: refused"
else
	check "connected: refused with the reason" contains "$WORK/out" "other connection(s) to 'payverge'"
fi
check "connected: database untouched" test "$(sql "SELECT note FROM bills WHERE id = 3")" = "kept"
psql -X -q -At -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'payverge'" >/dev/null
wait "$holder" 2>/dev/null || true

cp -R "$SET1" "$ROOT1/daily/corrupt"
printf 'x' >>"$ROOT1/daily/corrupt/db.dump"
if run_restore "$ROOT1" daily/corrupt --yes; then
	fail "corrupt: refused"
else
	check "corrupt: refused on the checksum" contains "$WORK/out" "checksum mismatch"
fi
check "corrupt: database untouched" test "$(sql "SELECT note FROM bills WHERE id = 3")" = "kept"
rm -rf "$ROOT1/daily/corrupt"

# A set whose checksums match but whose dump is not a valid archive (written
# corrupt, or a disk error before the sums were taken): restore must fail
# without touching the live data.
printf 'live marker\n' >"$STORAGE/live-marker.txt"
printf 'live marker\n' >"$MINIO/live-marker.txt"
bad_set() { # bad_set NAME FILE: a copy of SET1 with FILE garbled and re-summed
	cp -R "$SET1" "$ROOT1/daily/$1"
	printf 'not a pg_dump archive\n' >"$ROOT1/daily/$1/$2"
	(cd "$ROOT1/daily/$1" && shasum -a 256 db.dump storage.tar.gz minio.tar.gz >SHA256SUMS.new && mv SHA256SUMS.new SHA256SUMS)
}
for garbled in db.dump minio.tar.gz; do
	name="bad $garbled"
	bad_set bad "$garbled"
	if run_restore "$ROOT1" daily/bad --yes; then
		fail "$name: restore fails"
	else
		pass "$name: restore fails"
		check "$name: says the live data was not changed" contains "$WORK/out" "the live database and volumes were not changed"
	fi
	check "$name: checksums were valid (not the checksum refusal)" bash -c "! grep -q 'checksum mismatch' '$WORK/out'"
	check "$name: live database untouched" test "$(sql "SELECT note FROM bills WHERE id = 3")" = "kept"
	check "$name: live uploads untouched" test -f "$STORAGE/live-marker.txt"
	check "$name: live MinIO data untouched" test -f "$MINIO/live-marker.txt"
	check "$name: uploads not partly replaced" diff -r -x live-marker.txt "$WORK/storage.orig" "$STORAGE"
	check "$name: no temporary database or staging directory left" no_leftovers
	rm -rf "$ROOT1/daily/bad"
done
rm -f "$STORAGE/live-marker.txt" "$MINIO/live-marker.txt"

# --- 4. a failed archive is retried ---------------------------------------------------------

ROOT2=$WORK/backups2
if run_backup "$ROOT2" TAR_FAIL_SRC="$STORAGE" TAR_FAIL_TIMES=1; then
	pass "retry: exit 0"
else
	fail "retry: exit 0"
	show_out
fi
SET2=$(only_set "$ROOT2")
check "retry: logged the second attempt" contains "$WORK/out" "changed while it was archived; trying again (1/3)"
check "retry: uploads archived" test -s "$SET2/storage.tar.gz"
check "retry: MANIFEST lists no failed archive" bash -c "! grep -q archives_failed '$SET2/MANIFEST'"
check "retry: promoted" test -n "$(ls "$ROOT2/weekly")"
check "retry: no hand-over without BACKUP_UID" test ! -s "$CHOWN_LOG"

# --- 5. an archive that keeps failing --------------------------------------------------------

ROOT3=$WORK/backups3
if run_backup "$ROOT3" TAR_FAIL_SRC="$MINIO" TAR_FAIL_TIMES=99 BACKUP_ARCHIVE_ATTEMPTS=2 \
	BACKUP_UID="$(id -u)" BACKUP_GID="$(id -g)"; then
	fail "failing: exit non-zero"
else
	pass "failing: exit non-zero"
fi
SET3=$(only_set "$ROOT3")
check "failing: set written" test -s "$SET3/db.dump"
check "failing: logged the give-up" contains "$WORK/out" "could not archive $MINIO after 2 attempts"
check "failing: MANIFEST names the missing archive" contains "$SET3/MANIFEST" "archives_failed=minio"
check "failing: no MinIO archive" test ! -e "$SET3/minio.tar.gz"
check "failing: uploads still archived" test -s "$SET3/storage.tar.gz"
check "failing: checksums verify" bash -c "cd '$SET3' && shasum -a 256 -c -s SHA256SUMS"
check "failing: not promoted to weekly" test -z "$(ls "$ROOT3/weekly")"
check "failing: not promoted to monthly" test -z "$(ls "$ROOT3/monthly")"
check "failing: no partial set left" test -z "$(find "$ROOT3" -maxdepth 1 -name '.partial-*')"
check "failing: sets still handed over" contains "$CHOWN_LOG" "-R $(id -u):$(id -g) $ROOT3/daily"

printf 'changed after the set\n' >"$MINIO/payverge-public/logo.png/xl.meta"
sql "UPDATE bills SET note = 'later' WHERE id = 3"
if run_restore "$ROOT3" latest --yes; then
	pass "failing restore: exit 0"
else
	fail "failing restore: exit 0"
	show_out
fi
check "failing restore: warns about the missing volume" contains "$WORK/out" "this set was taken without: minio"
check "failing restore: database restored" test "$(sql "SELECT note FROM bills WHERE id = 3")" = "kept"
check "failing restore: MinIO left as it was" contains "$MINIO/payverge-public/logo.png/xl.meta" "changed after the set"
check "failing restore: uploads restored" diff -r "$WORK/storage.orig" "$STORAGE"

# --- 6. a database with no schema yet is not promoted ------------------------------------

# A fresh install: the backup container takes its first set while the backend
# is still creating the schema. That set stays daily only, or it would occupy
# the weekly and monthly slots (first set of the period wins) for months.
createdb payverge_fresh
ROOT5=$WORK/backups5
if run_backup "$ROOT5" PGDATABASE=payverge_fresh; then
	pass "no schema: exit 0"
else
	fail "no schema: exit 0"
	show_out
fi
SET5=$(only_set "$ROOT5")
check "no schema: daily set written" test -f "$SET5/db.dump"
check "no schema: MANIFEST says unknown" contains "$SET5/MANIFEST" "schema_version=unknown"
check "no schema: not promoted to weekly" test -z "$(ls "$ROOT5/weekly")"
check "no schema: not promoted to monthly" test -z "$(ls "$ROOT5/monthly")"
check "no schema: says why" contains "$WORK/out" "the database has no schema yet"

# --- 6b. a bare genesis baseline (version 0, empty ledger) is promoted ------------------

# The genesis baseline is migration version 0: it creates schema_migrations but
# adds no row until migration 000001 ships. That is a complete schema, so the
# set is kept as the weekly and monthly copy and the MANIFEST says version 0.
createdb payverge_genesis
psql -X -q -At -v ON_ERROR_STOP=1 -d payverge_genesis -c \
	"CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL);
	 CREATE TABLE businesses (id serial PRIMARY KEY, name text NOT NULL);"
ROOT6=$WORK/backups6
if run_backup "$ROOT6" PGDATABASE=payverge_genesis; then
	pass "genesis: exit 0"
else
	fail "genesis: exit 0"
	show_out
fi
SET6=$(only_set "$ROOT6")
check "genesis: daily set written" test -f "$SET6/db.dump"
check "genesis: MANIFEST says schema version 0" contains "$SET6/MANIFEST" "schema_version=0"
check "genesis: promoted to weekly" test -n "$(ls "$ROOT6/weekly")"
check "genesis: promoted to monthly" test -n "$(ls "$ROOT6/monthly")"
if contains "$WORK/out" "no schema yet"; then
	fail "genesis: not reported as schemaless"
	show_out
else
	pass "genesis: not reported as schemaless"
fi

# --- 7. settings are validated -----------------------------------------------------------------

ROOT4=$WORK/backups4
if run_backup "$ROOT4" BACKUP_UID=payverge; then
	fail "validate: BACKUP_UID=payverge refused"
else
	check "validate: BACKUP_UID=payverge refused" contains "$WORK/out" "BACKUP_UID must be a numeric user id"
fi
if run_backup "$ROOT4" BACKUP_ARCHIVE_ATTEMPTS=0; then
	fail "validate: BACKUP_ARCHIVE_ATTEMPTS=0 refused"
else
	check "validate: BACKUP_ARCHIVE_ATTEMPTS=0 refused" contains "$WORK/out" "BACKUP_ARCHIVE_ATTEMPTS must be at least 1"
fi
check "validate: nothing written" test ! -e "$ROOT4"

# --- 8. status file and healthcheck --------------------------------------------------------------

# run_health ROOT [VAR=VALUE...]: backup.sh health; output in $WORK/out.
run_health() {
	local root=$1 status=0
	shift
	env BACKUP_ROOT="$root" "$@" sh "$BACKUP_SH" health >"$WORK/out" 2>&1 || status=$?
	return "$status"
}
status_line() { cat "$1/.status" 2>/dev/null || echo missing; }
now=$(date -u +%s)
within_a_minute() { # within_a_minute ROOT STATE
	local state at
	read -r state at <"$1/.status" || return 1
	[[ $state == "$2" && $at =~ ^[0-9]+$ ]] && ((at >= now - 60 && at <= now + 600))
}

check "status: a complete run records ok" within_a_minute "$ROOT1" ok
check "status: a failed run records failed" within_a_minute "$ROOT3" failed
check "status: written as one line" test "$(wc -l <"$ROOT1/.status" | tr -d ' ')" = 1
check "status: no temporary file left" test -z "$(find "$ROOT1" -maxdepth 1 -name '.status.tmp.*')"

if run_health "$ROOT1"; then
	check "health: ok and fresh passes" contains "$WORK/out" "backup ok"
else
	fail "health: ok and fresh passes ($(cat "$WORK/out"))"
fi
if run_health "$ROOT3"; then
	fail "health: a failed run fails"
else
	check "health: a failed run fails" contains "$WORK/out" "the last backup failed"
fi
ROOT7=$WORK/backups7
mkdir -p "$ROOT7"
if run_health "$ROOT7"; then
	fail "health: no status fails"
else
	check "health: no status fails" contains "$WORK/out" "no backup status yet"
fi
printf 'ok %s\n' $((now - 27 * 3600)) >"$ROOT7/.status"
if run_health "$ROOT7"; then
	fail "health: ok older than 26h fails"
else
	check "health: ok older than 26h fails" contains "$WORK/out" "no successful backup for 27 hours"
fi
if run_health "$ROOT7" BACKUP_MAX_AGE_HOURS=48; then
	pass "health: BACKUP_MAX_AGE_HOURS=48 accepts a 27h-old set"
else
	fail "health: BACKUP_MAX_AGE_HOURS=48 accepts a 27h-old set ($(cat "$WORK/out"))"
fi
printf 'pending %s\n' $((now - 3600)) >"$ROOT7/.status"
if run_health "$ROOT7"; then
	pass "health: a fresh pending status passes"
else
	fail "health: a fresh pending status passes ($(cat "$WORK/out"))"
fi
printf 'pending %s\n' $((now - 30 * 3600)) >"$ROOT7/.status"
if run_health "$ROOT7"; then
	fail "health: a pending status older than 26h fails"
else
	pass "health: a pending status older than 26h fails"
fi
printf 'garbage\n' >"$ROOT7/.status"
if run_health "$ROOT7"; then
	fail "health: an unreadable status fails"
else
	check "health: an unreadable status fails" contains "$WORK/out" "unreadable backup status"
fi
# A later good run replaces failed with ok. Sets are named by the UTC second,
# so wait for a second that no earlier ROOT3 set used; on a fast runner the
# failing run above can land in the same second and the backup rightly refuses
# to overwrite an existing set.
while [[ -e "$ROOT3/daily/$(date -u +%Y%m%dT%H%M%SZ)" ]]; do sleep 0.2; done
if run_backup "$ROOT3"; then
	check "status: a good run after a failure records ok" within_a_minute "$ROOT3" ok
else
	fail "status: second run into ROOT3 exits 0 ($(status_line "$ROOT3"))"
	show_out
fi

if ((failures > 0)); then
	echo "deploy-backup: $failures check(s) failed" >&2
	exit 1
fi
echo "deploy-backup: all checks passed (PostgreSQL $(pg_ctl --version | awk '{print $3}'))"

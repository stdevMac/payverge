#!/bin/sh
# Payverge backup job. Runs inside the postgres:18-alpine image as the
# `backup` service of deploy/docker-compose.yml (profile: backup).
#
#   backup.sh daemon   take a set now if none exists, then one every day at
#                      BACKUP_TIME (HH:MM, container time zone; TZ, default UTC)
#   backup.sh once     take one set now, then prune
#   backup.sh health   exit 0 when the last run succeeded less than
#                      BACKUP_MAX_AGE_HOURS (default 26) ago; the compose
#                      healthcheck of the backup service
#
# Every run records its outcome in /backups/.status, one line written
# atomically: "ok <epoch>" or "failed <epoch>" (UTC seconds when it finished).
# The daemon writes "pending <epoch>" when it starts and no status exists yet,
# so a new install is healthy until its first set is due. A failed run, or no
# successful one for BACKUP_MAX_AGE_HOURS, turns the container unhealthy
# (docker compose ps), which host monitoring can alert on.
#
# A set is one directory, written to a temporary name and renamed into place
# only when complete:
#
#   /backups/daily/<YYYYmmddTHHMMSSZ>/
#     db.dump          pg_dump custom format (compressed; restore with pg_restore)
#     storage.tar.gz   the uploads volume (STORAGE_DRIVER=local)
#     minio.tar.gz     the MinIO volume, only when the minio profile holds data
#     MANIFEST         creation time, database, schema version, image version
#     SHA256SUMS       checksums, verified again by restore.sh
#
# The apps keep running during a backup. The database dump is a consistent
# snapshot. The uploads and MinIO volumes are copied file by file: tar is
# retried when a file changes or disappears while it is read, and an object
# written during the backup may be missing from the set. When an archive still
# fails, the set is completed without it, MANIFEST names it under
# archives_failed, the set is not kept as a weekly or monthly copy, and the run
# exits non-zero.
#
# Retention: the newest BACKUP_KEEP_DAILY sets (default 7) stay in daily/.
# The first set of each ISO week is also kept in weekly/<YYYY-Www> (default 4)
# and the first set of each month in monthly/<YYYY-mm> (default 6). Weekly and
# monthly copies are hard links when the file system allows it, so they cost
# no extra space.
#
# Environment: PGHOST, PGPORT, PGUSER, PGPASSWORD, PGDATABASE (libpq), and
# BACKUP_TIME, BACKUP_KEEP_DAILY, BACKUP_KEEP_WEEKLY, BACKUP_KEEP_MONTHLY.
# BACKUP_UID and BACKUP_GID (optional): the host user and group that get the
# sets after each run, so that user can read and copy them without root.
set -eu
umask 077

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
STORAGE_SRC=${STORAGE_SRC:-/data/storage}
MINIO_SRC=${MINIO_SRC:-/data/minio}
KEEP_DAILY=${BACKUP_KEEP_DAILY:-7}
KEEP_WEEKLY=${BACKUP_KEEP_WEEKLY:-4}
KEEP_MONTHLY=${BACKUP_KEEP_MONTHLY:-6}
BACKUP_TIME=${BACKUP_TIME:-03:30}
BACKUP_UID=${BACKUP_UID:-}
BACKUP_GID=${BACKUP_GID:-$BACKUP_UID}
ARCHIVE_ATTEMPTS=${BACKUP_ARCHIVE_ATTEMPTS:-3}
STATUS_FILE=$BACKUP_ROOT/.status
MAX_AGE_HOURS=${BACKUP_MAX_AGE_HOURS:-26}

log() {
	printf '%s backup: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

die() {
	log "ERROR: $*" >&2
	exit 1
}

is_uint() {
	case $1 in
	'' | *[!0-9]*) return 1 ;;
	*) return 0 ;;
	esac
}

validate_settings() {
	for pair in "BACKUP_KEEP_DAILY=$KEEP_DAILY" "BACKUP_KEEP_WEEKLY=$KEEP_WEEKLY" "BACKUP_KEEP_MONTHLY=$KEEP_MONTHLY"; do
		is_uint "${pair#*=}" || die "${pair%%=*} must be a whole number (got '${pair#*=}')"
	done
	[ "$KEEP_DAILY" -ge 1 ] || die "BACKUP_KEEP_DAILY must be at least 1"
	if [ -n "$BACKUP_UID" ]; then
		is_uint "$BACKUP_UID" || die "BACKUP_UID must be a numeric user id (got '$BACKUP_UID')"
		is_uint "$BACKUP_GID" || die "BACKUP_GID must be a numeric group id (got '$BACKUP_GID')"
	fi
	is_uint "$ARCHIVE_ATTEMPTS" && [ "$ARCHIVE_ATTEMPTS" -ge 1 ] || die "BACKUP_ARCHIVE_ATTEMPTS must be at least 1"
	case $BACKUP_TIME in
	[0-1][0-9]:[0-5][0-9] | 2[0-3]:[0-5][0-9]) ;;
	*) die "BACKUP_TIME must be HH:MM in 24-hour time (got '$BACKUP_TIME')" ;;
	esac
	: "${PGDATABASE:?PGDATABASE is not set}"
	: "${PGUSER:?PGUSER is not set}"
}

dir_has_entries() {
	[ -d "$1" ] && [ -n "$(find "$1" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]
}

# keep_newest DIR N: delete all but the N newest entries of DIR (names sort by
# time). N=0 empties the tier.
keep_newest() {
	tier=$1
	keep=$2
	[ -d "$tier" ] || return 0
	find "$tier" -mindepth 1 -maxdepth 1 -exec basename {} \; | sort -r | tail -n +"$((keep + 1))" | while IFS= read -r old; do
		[ -n "$old" ] || continue
		log "pruning $(basename "$tier")/$old"
		rm -rf "${tier:?}/$old"
	done
}

# promote SET TIER KEY: keep SET as TIER/KEY unless that slot already exists.
promote() {
	set_dir=$1
	slot="$BACKUP_ROOT/$2/$3"
	[ -e "$slot" ] && return 0
	mkdir -p "$slot.tmp"
	for file in "$set_dir"/*; do
		ln "$file" "$slot.tmp/" 2>/dev/null || cp -p "$file" "$slot.tmp/"
	done
	mv "$slot.tmp" "$slot"
	log "kept as $2/$3"
}

# archive NAME SRC [TAR OPTION...]: write $work/NAME.tar.gz from SRC. tar
# fails when a file shrinks or disappears while it is read (the apps keep
# writing), so it is tried ARCHIVE_ATTEMPTS times before giving up.
archive() {
	name=$1
	src=$2
	shift 2
	attempt=1
	while :; do
		if tar -C "$src" "$@" -czf "$work/$name.tar.gz" .; then
			return 0
		fi
		rm -f "$work/$name.tar.gz"
		if [ "$attempt" -ge "$ARCHIVE_ATTEMPTS" ]; then
			log "ERROR: could not archive $src after $attempt attempts" >&2
			return 1
		fi
		log "$src changed while it was archived; trying again ($attempt/$ARCHIVE_ATTEMPTS)"
		attempt=$((attempt + 1))
		sleep 5
	done
}

# hand_over: give every set to BACKUP_UID:BACKUP_GID, so the host user who
# installed Payverge can list and copy them. Modes stay 0700/0600.
hand_over() {
	[ -n "$BACKUP_UID" ] || return 0
	chown -R "$BACKUP_UID:$BACKUP_GID" "$BACKUP_ROOT/daily" "$BACKUP_ROOT/weekly" "$BACKUP_ROOT/monthly" ||
		log "WARNING: could not give the sets to $BACKUP_UID:$BACKUP_GID (the backup service needs the CHOWN capability)" >&2
}

take_backup() {
	stamp=$(date -u +%Y%m%dT%H%M%SZ)
	mkdir -p "$BACKUP_ROOT/daily" "$BACKUP_ROOT/weekly" "$BACKUP_ROOT/monthly"
	# Leftovers of an interrupted run are never valid sets.
	find "$BACKUP_ROOT" -mindepth 1 -maxdepth 2 -name '.partial-*' -exec rm -rf {} +
	find "$BACKUP_ROOT/weekly" "$BACKUP_ROOT/monthly" -mindepth 1 -maxdepth 1 -name '*.tmp' -exec rm -rf {} +

	work="$BACKUP_ROOT/.partial-$stamp"
	final="$BACKUP_ROOT/daily/$stamp"
	[ -e "$final" ] && die "set $final already exists"
	mkdir -p "$work"

	log "dumping database '$PGDATABASE' from $PGHOST"
	pg_dump --format=custom --compress=6 --no-owner --no-privileges --file="$work/db.dump" "$PGDATABASE"
	pg_restore --list "$work/db.dump" >/dev/null || die "pg_dump output is not readable by pg_restore"

	# Always written, even when empty, so a restore puts uploads back to
	# exactly this point in time.
	failed=""
	[ -d "$STORAGE_SRC" ] || die "uploads volume is not mounted at $STORAGE_SRC"
	log "archiving uploads ($STORAGE_SRC)"
	archive storage "$STORAGE_SRC" || failed="storage"

	if dir_has_entries "$MINIO_SRC"; then
		# tmp/ and multipart/ hold uploads in progress, which MinIO discards
		# when it restarts anyway.
		log "archiving MinIO data ($MINIO_SRC)"
		archive minio "$MINIO_SRC" --exclude=.minio.sys/tmp --exclude=.minio.sys/multipart ||
			failed="${failed:+$failed }minio"
	fi

	schema_version=$(read_schema_version)
	server_version=$(psql -X -At -c 'SHOW server_version' "$PGDATABASE" 2>/dev/null || true)
	{
		echo "format=payverge-backup/1"
		echo "created=$stamp"
		echo "database=$PGDATABASE"
		echo "schema_version=${schema_version:-unknown}"
		echo "postgres_version=${server_version:-unknown}"
		echo "payverge_version=${PAYVERGE_VERSION:-unknown}"
		[ -z "$failed" ] || echo "archives_failed=$failed"
	} >"$work/MANIFEST"
	(
		cd "$work"
		set -- db.dump
		[ ! -f storage.tar.gz ] || set -- "$@" storage.tar.gz
		[ ! -f minio.tar.gz ] || set -- "$@" minio.tar.gz
		sha256sum "$@" MANIFEST >SHA256SUMS
	)

	mv "$work" "$final"
	if [ -n "$failed" ]; then
		log "ERROR: set daily/$stamp has the database but not: $failed ($(du -sh "$final" | cut -f1)); it is not kept as a weekly or monthly copy" >&2
	elif [ -z "$schema_version" ]; then
		# The backend has not created the schema yet (a fresh install whose
		# first migrations are still running). Weekly and monthly slots keep
		# the first set of their period, so an empty one would sit there for
		# months in place of a real backup.
		log "set complete: daily/$stamp ($(du -sh "$final" | cut -f1)); the database has no schema yet, so it is not kept as a weekly or monthly copy"
	else
		log "set complete: daily/$stamp ($(du -sh "$final" | cut -f1))"
		promote "$final" weekly "$(date -u +%G-W%V)"
		promote "$final" monthly "$(date -u +%Y-%m)"
	fi

	keep_newest "$BACKUP_ROOT/daily" "$KEEP_DAILY"
	keep_newest "$BACKUP_ROOT/weekly" "$KEEP_WEEKLY"
	keep_newest "$BACKUP_ROOT/monthly" "$KEEP_MONTHLY"
	hand_over
	[ -z "$failed" ] || exit 1
}

# has_schema: the backend applies the genesis baseline in one transaction, and
# that baseline creates public.schema_migrations, so the table existing means
# the whole schema is there. The ledger records the baseline's migration head;
# a head-0 baseline (no numbered migrations) leaves it empty.
has_schema() {
	[ "$(psql -X -At -c "SELECT to_regclass('public.schema_migrations') IS NOT NULL" "$PGDATABASE" 2>/dev/null || true)" = "t" ]
}

# read_schema_version: the migration version (0 for a bare genesis baseline),
# or nothing when the database has no schema yet.
read_schema_version() {
	has_schema || return 0
	psql -X -At -c 'SELECT coalesce(max(version), 0) FROM public.schema_migrations' "$PGDATABASE" 2>/dev/null || true
}

seconds_until() {
	target_h=${1%%:*}
	target_m=${1#*:}
	now_h=$(date +%H)
	now_m=$(date +%M)
	now_s=$(date +%S)
	# Strip one leading zero so "08" is not read as an octal number.
	now=$((${now_h#0} * 3600 + ${now_m#0} * 60 + ${now_s#0}))
	target=$((${target_h#0} * 3600 + ${target_m#0} * 60))
	delay=$(((target - now + 86400) % 86400))
	[ "$delay" -gt 0 ] || delay=86400
	echo "$delay"
}

# wait_for_schema: on a fresh install this container starts while the backend
# is still creating the schema. Wait for it (SCHEMA_WAIT_SECONDS, default 600)
# so the first set holds the migrated database, not an empty one.
wait_for_schema() {
	waited=0
	limit=${SCHEMA_WAIT_SECONDS:-600}
	while :; do
		! has_schema || return 0
		if [ "$waited" -ge "$limit" ]; then
			log "the database still has no schema after ${limit}s; taking the first set anyway"
			return 0
		fi
		[ "$waited" -gt 0 ] || log "waiting for the backend to create the database schema before the first set"
		sleep 10
		waited=$((waited + 10))
	done
}

# write_status STATE: replace the status file in one rename.
write_status() {
	mkdir -p "$BACKUP_ROOT"
	printf '%s %s\n' "$1" "$(date -u +%s)" >"$STATUS_FILE.tmp.$$"
	chmod 644 "$STATUS_FILE.tmp.$$"
	mv -f "$STATUS_FILE.tmp.$$" "$STATUS_FILE"
}

# record_outcome: the EXIT trap of `once`; keeps the run's exit status.
record_outcome() {
	outcome=$?
	trap - EXIT
	if [ "$outcome" -eq 0 ]; then
		write_status ok
	else
		write_status failed || true
	fi
	exit "$outcome"
}

# check_health: the healthcheck. Prints why it is unhealthy.
check_health() {
	is_uint "$MAX_AGE_HOURS" && [ "$MAX_AGE_HOURS" -ge 1 ] || {
		echo "BACKUP_MAX_AGE_HOURS must be a whole number of hours (got '$MAX_AGE_HOURS')"
		return 1
	}
	if [ ! -f "$STATUS_FILE" ]; then
		echo "no backup status yet ($STATUS_FILE)"
		return 1
	fi
	read -r state at <"$STATUS_FILE" || true
	if ! is_uint "${at:-}"; then
		echo "unreadable backup status: $(cat "$STATUS_FILE")"
		return 1
	fi
	age=$(($(date -u +%s) - at))
	case $state in
	ok | pending) ;;
	failed)
		echo "the last backup failed $((age / 60)) minutes ago; see: docker compose logs backup"
		return 1
		;;
	*)
		echo "unknown backup status '$state'"
		return 1
		;;
	esac
	if [ "$age" -ge $((MAX_AGE_HOURS * 3600)) ]; then
		echo "no successful backup for $((age / 3600)) hours (status $state; limit ${MAX_AGE_HOURS}h)"
		return 1
	fi
	echo "backup $state $((age / 60)) minutes ago"
}

run_once_isolated() {
	# A fresh shell keeps `set -e` effective inside the backup even though the
	# daemon loop checks its exit status.
	if /bin/sh "$0" once; then
		return 0
	fi
	log "ERROR: backup failed; the next attempt is at $BACKUP_TIME" >&2
	return 0
}

mode=${1:-once}
if [ "$mode" = health ]; then
	check_health
	exit
fi
validate_settings
case $mode in
once)
	# Record the outcome however the run ends: die, a failed archive, or a
	# command failing under set -e.
	trap record_outcome EXIT
	take_backup
	;;
daemon)
	log "daily backups at $BACKUP_TIME (TZ=${TZ:-UTC}); keeping $KEEP_DAILY daily, $KEEP_WEEKLY weekly, $KEEP_MONTHLY monthly in $BACKUP_ROOT"
	[ -f "$STATUS_FILE" ] || write_status pending
	if ! dir_has_entries "$BACKUP_ROOT/daily"; then
		wait_for_schema
		log "no backup yet; taking the first one now"
		run_once_isolated
	fi
	while :; do
		delay=$(seconds_until "$BACKUP_TIME")
		log "next backup in ${delay}s"
		sleep "$delay" &
		# Wait in the background so `docker compose stop` (SIGTERM) ends the
		# sleep immediately instead of after the 10 s kill timeout.
		trap 'kill $! 2>/dev/null; exit 0' TERM INT
		wait $!
		trap - TERM INT
		run_once_isolated
	done
	;;
*)
	die "usage: backup.sh once|daemon|health"
	;;
esac

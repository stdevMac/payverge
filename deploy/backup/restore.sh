#!/bin/sh
# Payverge restore job. Runs inside the postgres:15-alpine image as the
# `restore` service of deploy/docker-compose.yml (profile: restore):
#
#   docker compose stop backend frontend          # (and minio, if you use it)
#   docker compose --profile restore run --rm restore latest --yes
#   docker compose up -d
#
# Usage: restore.sh <set> --yes [--skip-storage]
#   <set>           latest (newest daily set), daily/<stamp>, weekly/<YYYY-Www>,
#                   monthly/<YYYY-mm>, or a bare <stamp> from daily/
#   --yes           required: the database and the uploads are REPLACED
#   --skip-storage  restore the database only
#
# What it does:
#   1. verifies the set against its SHA256SUMS
#   2. refuses while anything else is connected to the database
#   3. pg_restore --exit-on-error into a temporary database (<db>_restore)
#   4. unpacks the uploads (and MinIO data, when the set has it) into a staging
#      directory inside each volume (.payverge-restore-staging)
#   5. only when 3 and 4 both succeeded: renames the live database to
#      <db>_pre_restore and the temporary one to <db> in one transaction,
#      swaps each volume's contents for its staging directory, gives the files
#      back to the backend user (uid 65532), and drops <db>_pre_restore
#   A corrupt dump or archive therefore leaves the live database and volumes
#   as they were.
#
# The backend applies any newer migrations when it starts. It refuses a schema
# newer than its own binary, so restore with the image version named in the
# set's MANIFEST (PAYVERGE_VERSION in .env) or newer.
set -eu

BACKUP_ROOT=${BACKUP_ROOT:-/backups}
STORAGE_DST=${STORAGE_DST:-/data/storage}
MINIO_DST=${MINIO_DST:-/data/minio}
# The backend image's user (distroless nonroot).
APP_UID=${APP_UID:-65532}

log() {
	printf '%s restore: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

die() {
	log "ERROR: $*" >&2
	exit 1
}

usage() {
	sed -n '9,14p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 64
}

target=""
confirmed=0
skip_storage=0
for arg in "$@"; do
	case $arg in
	--yes) confirmed=1 ;;
	--skip-storage) skip_storage=1 ;;
	-h | --help) usage ;;
	-*) die "unknown option $arg" ;;
	*)
		[ -z "$target" ] || die "only one set may be named"
		target=$arg
		;;
	esac
done
[ -n "$target" ] || usage
[ "$confirmed" -eq 1 ] || die "restore REPLACES the database and uploads; re-run with --yes"
: "${PGDATABASE:?PGDATABASE is not set}"

newest_in() {
	[ -d "$1" ] || return 0
	find "$1" -mindepth 1 -maxdepth 1 -type d ! -name '.*' -exec basename {} \; | sort -r | head -n 1
}

case $target in
latest)
	name=$(newest_in "$BACKUP_ROOT/daily")
	[ -n "$name" ] || die "no sets in $BACKUP_ROOT/daily"
	set_dir="$BACKUP_ROOT/daily/$name"
	;;
daily/* | weekly/* | monthly/*)
	set_dir="$BACKUP_ROOT/$target"
	;;
*/* | .*)
	die "set names look like latest, daily/<stamp>, weekly/<YYYY-Www> or monthly/<YYYY-mm>"
	;;
*)
	set_dir="$BACKUP_ROOT/daily/$target"
	;;
esac
case $set_dir in
*/../* | */..) die "set names must not contain .." ;;
esac
[ -d "$set_dir" ] || die "no backup set at $set_dir"
[ -f "$set_dir/db.dump" ] || die "$set_dir has no db.dump"

log "verifying ${set_dir#"$BACKUP_ROOT"/}"
(cd "$set_dir" && sha256sum -c -s SHA256SUMS) || die "checksum mismatch in $set_dir; pick another set"
if [ -f "$set_dir/MANIFEST" ]; then
	sed 's/^/  /' "$set_dir/MANIFEST"
	incomplete=$(sed -n 's/^archives_failed=//p' "$set_dir/MANIFEST")
	if [ -n "$incomplete" ]; then
		log "WARNING: this set was taken without: $incomplete. Those volumes are left as they are; pick an older set to restore them too."
	fi
fi

tmpdb="${PGDATABASE}_restore"
olddb="${PGDATABASE}_pre_restore"
STAGING_NAME=.payverge-restore-staging

# other_connections DB: sessions on DB other than this one.
other_connections() {
	psql -X -At -d postgres -v ON_ERROR_STOP=1 -v dbname="$1" <<'SQL'
SELECT count(*) FROM pg_stat_activity
WHERE datname = :'dbname' AND pid <> pg_backend_pid();
SQL
}

others=$(other_connections "$PGDATABASE")
if [ "${others:-0}" -ne 0 ]; then
	die "$others other connection(s) to '$PGDATABASE'; run 'docker compose stop backend frontend' first"
fi
leftover=$(psql -X -At -d postgres -v ON_ERROR_STOP=1 -v olddb="$olddb" <<'SQL'
SELECT count(*) FROM pg_database WHERE datname = :'olddb';
SQL
)
if [ "${leftover:-0}" -ne 0 ]; then
	die "a database '$olddb' is left from an earlier restore that did not finish; it may hold the data from before that restore. Check it, then drop it (DROP DATABASE \"$olddb\") and run the restore again"
fi

# The volumes this set restores.
restore_storage=0
restore_minio=0
if [ "$skip_storage" -eq 1 ]; then
	log "skipping uploads (--skip-storage)"
else
	if [ -f "$set_dir/storage.tar.gz" ]; then
		restore_storage=1
	else
		log "set has no storage.tar.gz; uploads left unchanged"
	fi
	[ ! -f "$set_dir/minio.tar.gz" ] || restore_minio=1
fi

# Until the swap, a failure removes the temporaries and leaves the live data.
swapped=0
abort_cleanup() {
	status=$?
	[ "$swapped" -eq 0 ] || exit "$status"
	psql -X -q -d postgres -v ON_ERROR_STOP=1 -v tmpdb="$tmpdb" >/dev/null 2>&1 <<'SQL' || true
DROP DATABASE IF EXISTS :"tmpdb" WITH (FORCE);
SQL
	for dest in "$STORAGE_DST" "$MINIO_DST"; do
		[ ! -d "$dest/$STAGING_NAME" ] || rm -rf "${dest:?}/$STAGING_NAME"
	done
	exit "$status"
}
trap abort_cleanup EXIT

log "restoring database into '$tmpdb'"
psql -X -q -d postgres -v ON_ERROR_STOP=1 -v tmpdb="$tmpdb" <<'SQL'
DROP DATABASE IF EXISTS :"tmpdb" WITH (FORCE);
CREATE DATABASE :"tmpdb";
SQL
pg_restore --exit-on-error --no-owner --no-privileges --dbname="$tmpdb" "$set_dir/db.dump" ||
	die "pg_restore failed; the live database and volumes were not changed"

# stage_volume ARCHIVE DEST: unpack ARCHIVE into DEST/.payverge-restore-staging
# (same filesystem as DEST, so the swap moves entries instead of copying).
stage_volume() {
	archive=$1
	dest=$2
	[ -d "$dest" ] || die "$dest is not mounted; the live database and volumes were not changed"
	rm -rf "${dest:?}/$STAGING_NAME"
	mkdir "$dest/$STAGING_NAME"
	tar -C "$dest/$STAGING_NAME" -xzf "$archive" --numeric-owner ||
		die "unpacking $archive failed; the live database and volumes were not changed"
}

# swap_volume DEST: replace DEST's contents with its staging directory and
# hand the files to the application user.
# Called as `swap_volume DEST || die`, where set -e does not apply, so every
# step checks its own status.
swap_volume() {
	dest=$1
	find "$dest" -mindepth 1 -maxdepth 1 ! -name "$STAGING_NAME" -exec rm -rf {} + || return 1
	find "$dest/$STAGING_NAME" -mindepth 1 -maxdepth 1 -exec mv {} "$dest"/ \; || return 1
	rmdir "$dest/$STAGING_NAME" || return 1
	chown -R "$APP_UID:$APP_UID" "$dest" || return 1
	chmod 0700 "$dest"
}

if [ "$restore_storage" -eq 1 ]; then
	log "unpacking uploads"
	stage_volume "$set_dir/storage.tar.gz" "$STORAGE_DST"
fi
if [ "$restore_minio" -eq 1 ]; then
	log "unpacking MinIO data"
	stage_volume "$set_dir/minio.tar.gz" "$MINIO_DST"
fi

# A backend may have reconnected while the dump was restored.
others=$(other_connections "$PGDATABASE")
if [ "${others:-0}" -ne 0 ]; then
	die "$others other connection(s) to '$PGDATABASE' appeared during the restore; the live database and volumes were not changed. Run 'docker compose stop backend frontend' and restore again"
fi

log "swapping in the restored database"
psql -X -q -d postgres -v ON_ERROR_STOP=1 -v dbname="$PGDATABASE" -v tmpdb="$tmpdb" -v olddb="$olddb" <<'SQL' ||
BEGIN;
ALTER DATABASE :"dbname" RENAME TO :"olddb";
ALTER DATABASE :"tmpdb" RENAME TO :"dbname";
COMMIT;
SQL
	die "renaming the restored database into place failed; the live database and volumes were not changed"
swapped=1

swap_failed() {
	die "replacing $1 failed after the database was swapped; the previous database is kept as '$olddb'. Once the cause is fixed, drop it (DROP DATABASE \"$olddb\") if you do not need it, then restore this set again: the restore refuses while '$olddb' exists"
}
if [ "$restore_storage" -eq 1 ]; then
	log "restoring uploads"
	swap_volume "$STORAGE_DST" || swap_failed "$STORAGE_DST"
fi
if [ "$restore_minio" -eq 1 ]; then
	log "restoring MinIO data"
	swap_volume "$MINIO_DST" || swap_failed "$MINIO_DST"
fi

log "dropping the previous database '$olddb'"
psql -X -q -d postgres -v ON_ERROR_STOP=1 -v olddb="$olddb" <<'SQL' ||
DROP DATABASE :"olddb" WITH (FORCE);
SQL
	log "WARNING: could not drop '$olddb'; the restore is complete. Drop it by hand (DROP DATABASE \"$olddb\"); the next restore refuses while it exists"

log "done. Start the stack again with: docker compose up -d"

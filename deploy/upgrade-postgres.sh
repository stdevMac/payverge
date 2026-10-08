#!/usr/bin/env bash
# upgrade-postgres.sh: move a Payverge install from PostgreSQL 15 to 18.
#
# Installs from before PostgreSQL 18 keep their data at the root of the "db"
# volume. PostgreSQL 18 keeps it in the "pgdata" volume, mounted at
# /var/lib/postgresql (PGDATA /var/lib/postgresql/18/docker). Until the data
# is moved, the postgres service idles with a message (postgres/pg18-guard.sh)
# instead of creating an empty database.
#
# What it does, in the install directory (next to docker-compose.yml):
#   1. docker compose down (containers only; every volume is kept);
#   2. starts PostgreSQL 15 on the db volume, records the row count of every
#      table in every database, and dumps the whole cluster (pg_dumpall:
#      roles with their passwords, databases, owners, grants) into
#      backups/pg18-upgrade-<time>/;
#   3. starts PostgreSQL 18 on a fresh pgdata volume, restores the dump,
#      runs ANALYZE, and compares the row counts with step 2;
#   4. docker compose up -d (skip with --no-start).
#
# Only PostgreSQL 15 runs on the db volume (as it did before), and nothing
# deletes it, so rolling back is: put the previous release's files back
# (bash install.sh --version <previous>), which runs PostgreSQL 15 on it again.
# If any step fails, the half-filled pgdata volume is removed and the stack
# is left stopped with the db volume as it was.
#
# Usage: ./upgrade-postgres.sh [--yes] [--dry-run] [--no-start] [--dir DIR]
#
# Docs: docs/self-hosting/upgrades.md, "PostgreSQL 18".
set -Eeuo pipefail

# The release that wrote the db volume, and the one deploy/docker-compose.yml
# now runs. Both pinned by digest.
readonly OLD_IMAGE="postgres:15.19-alpine@sha256:f7d23353e1b15400d22ebe31189f4d314b87a4c129cc400c8c2d8d4ca127bf81"
readonly NEW_IMAGE_DEFAULT="postgres:18.6-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873"
readonly READY_TIMEOUT=${PG_UPGRADE_READY_TIMEOUT:-120}

say() { printf '%s\n' "$*"; }
step() { printf '==> %s\n' "$*"; }
note() { printf '    %s\n' "$*"; }
die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: upgrade-postgres.sh [options]

Moves this install's database from PostgreSQL 15 (the "db" volume) to
PostgreSQL 18 (the "pgdata" volume): dump with 15, restore into 18, compare
the row count of every table. The db volume is kept for rollback.

Options:
  --yes        do not ask for confirmation (needed without a terminal)
  --dry-run    show what would happen; change nothing
  --no-start   leave the stack stopped afterwards
  --dir DIR    the install directory (default: this script's directory)
  -h, --help   this help
EOF
}

yes=0 dry_run=0 no_start=0
install_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
while (($#)); do
	case $1 in
	--yes | -y) yes=1 ;;
	--dry-run) dry_run=1 ;;
	--no-start) no_start=1 ;;
	--dir)
		[[ -n ${2:-} ]] || die "--dir needs a directory"
		install_dir=$2
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*) die "unknown option: $1 (see --help)" ;;
	esac
	shift
done

[[ -d $install_dir ]] || die "no such directory: $install_dir"
install_dir=$(cd "$install_dir" && pwd)
cd "$install_dir"
[[ -f docker-compose.yml ]] || die "no docker-compose.yml in $install_dir; run this in the Payverge install directory"
grep -q 'pgdata:/var/lib/postgresql' docker-compose.yml ||
	die "docker-compose.yml here is from a release before PostgreSQL 18. Upgrade the release files first (bash install.sh --version <new>), then run this again."
command -v docker >/dev/null 2>&1 || die "docker not found"

# env_get KEY: last value of KEY in .env, quotes removed (as install.sh).
env_get() {
	local key=$1 line value=""
	[[ -f .env ]] || return 0
	while IFS= read -r line || [[ -n $line ]]; do
		case $line in
		"$key="*) value=${line#*=} ;;
		esac
	done <.env
	value=${value%$'\r'}
	if [[ $value == \'*\' || $value == \"*\" ]]; then
		value=${value:1:${#value}-2}
	fi
	printf '%s' "$value"
}

project=${COMPOSE_PROJECT_NAME:-$(env_get COMPOSE_PROJECT_NAME)}
if [[ -z $project ]]; then
	project=$(sed -n 's/^name:[[:space:]]*\([A-Za-z0-9_-]*\).*/\1/p' docker-compose.yml | head -n 1)
fi
project=${project:-payverge}
new_image=$(awk '/^  postgres:/{p=1} p && /image:/{print $2; exit}' docker-compose.yml)
new_image=${new_image:-$NEW_IMAGE_DEFAULT}
legacy_vol=${project}_db
new_vol=${project}_pgdata
app_user=$(env_get DB_USER)
app_user=${app_user:-payverge}
backup_dir=$(env_get BACKUP_DIR)
backup_dir=${backup_dir:-./backups}
stamp=$(date -u +%Y%m%dT%H%M%SZ)
dump_dir=$backup_dir/pg18-upgrade-$stamp
old_ctr=$project-pgupgrade-old
new_ctr=$project-pgupgrade-new

# --- what is there -------------------------------------------------------------

step "Looking at the database volumes of project $project"
if ! docker volume inspect "$legacy_vol" >/dev/null 2>&1; then
	note "no $legacy_vol volume: this install started on PostgreSQL 18. Nothing to upgrade."
	exit 0
fi
legacy_version=$(docker run --rm --network none -v "$legacy_vol:/old:ro" --entrypoint /bin/sh "$OLD_IMAGE" \
	-c 'cat /old/PG_VERSION 2>/dev/null || true')
legacy_version=$(printf '%s' "$legacy_version" | tr -d '[:space:]')
new_version=""
if docker volume inspect "$new_vol" >/dev/null 2>&1; then
	new_version=$(docker run --rm --network none -v "$new_vol:/new:ro" --entrypoint /bin/sh "$new_image" \
		-c 'cat /new/18/docker/PG_VERSION 2>/dev/null || true')
	new_version=$(printf '%s' "$new_version" | tr -d '[:space:]')
fi
if [[ -n $new_version ]]; then
	# A rollback since the upgrade (PostgreSQL 15 run on the db volume again)
	# leaves pgdata stale; see postgres/pg18-guard.sh.
	pgdata_state=$(docker run --rm --network none -v "$legacy_vol:/old:ro" -v "$new_vol:/new:ro" \
		--entrypoint /bin/sh "$new_image" -c 'm=/new/payverge-pg15-control.cksum
if [ -s "$m" ] && [ -s /old/global/pg_control ] &&
	[ "$(cksum </old/global/pg_control | awk "{print \$1, \$2}")" != "$(cat "$m")" ]; then echo stale; fi')
	if [[ $pgdata_state == *stale* ]]; then
		die "$new_vol holds a PostgreSQL 18 copy taken before PostgreSQL 15 ran on $legacy_vol again (a rollback), so it is missing the writes made since. To upgrade again from the current data: docker compose down; docker volume rm $new_vol; then re-run this script. To keep the PostgreSQL 18 copy instead, remove $legacy_vol."
	fi
	note "$new_vol already holds a PostgreSQL $new_version database. Nothing to upgrade."
	[[ -z $legacy_version ]] || note "$legacy_vol (PostgreSQL $legacy_version) is the pre-upgrade copy; remove it once you no longer need a rollback: docker volume rm $legacy_vol"
	exit 0
fi
if [[ -z $legacy_version ]]; then
	note "$legacy_vol holds no database. Nothing to upgrade."
	exit 0
fi
[[ $legacy_version == 15 ]] ||
	die "$legacy_vol holds PostgreSQL $legacy_version data; this script upgrades from 15 only"

say ""
say "Plan:"
say "  1. docker compose down          (stops Payverge; every volume is kept)"
say "  2. PostgreSQL 15 on $legacy_vol: count rows, pg_dumpall into $dump_dir/"
say "  3. PostgreSQL 18 on a new $new_vol: restore, ANALYZE, compare row counts"
if [[ $no_start == 1 ]]; then
	say "  4. leave the stack stopped (--no-start)"
else
	say "  4. docker compose up -d"
fi
say "  $legacy_vol is only read (by PostgreSQL 15) and never removed: it is the rollback."
say ""
if [[ $dry_run == 1 ]]; then
	say "Dry run: nothing was changed."
	exit 0
fi
if [[ $yes != 1 ]]; then
	[[ -t 0 ]] || die "no terminal to confirm on; re-run with --yes"
	printf 'Payverge will be offline while this runs. Continue? [y/N] '
	read -r answer
	[[ $answer == [yY] || $answer == [yY][eE][sS] ]] || die "stopped; nothing was changed"
fi

# --- helpers -------------------------------------------------------------------

created_new=0
finished=0
cleanup() {
	docker rm -f -v "$old_ctr" "$new_ctr" >/dev/null 2>&1 || true
	if [[ $finished != 1 && $created_new == 1 ]]; then
		docker volume rm "$new_vol" >/dev/null 2>&1 || true
		printf 'error: the upgrade did not finish. %s was removed; %s is unchanged and Payverge is stopped.\n' "$new_vol" "$legacy_vol" >&2
		printf '       Start it again on PostgreSQL 15 with the previous release (bash install.sh --version <previous>), or fix the cause and re-run this script.\n' >&2
	fi
}
trap cleanup EXIT

wait_ready() { # wait_ready CONTAINER: the real server, on TCP (not the init one)
	local i
	for ((i = 0; i < READY_TIMEOUT; i++)); do
		if docker exec "$1" pg_isready -q -h 127.0.0.1 >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	docker logs --tail 40 "$1" >&2 || true
	die "$1 did not accept connections within ${READY_TIMEOUT}s"
}

psql_in() { # psql_in CONTAINER USER DB ARGS...: psql over the local socket
	local ctr=$1 user=$2 db=$3
	shift 3
	docker exec -i "$ctr" psql -X -q -U "$user" -d "$db" "$@"
}

# The row count of every table, one "db|schema.table|count" line each.
readonly COUNT_SQL="SELECT format('SELECT %L || ''|'' || count(*) FROM %I.%I', n.nspname || '.' || c.relname, n.nspname, c.relname)
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg_toast%'
ORDER BY 1 \\gexec"
row_counts() { # row_counts CONTAINER SUPERUSER
	local ctr=$1 user=$2 db
	while IFS= read -r db; do
		[[ -n $db ]] || continue
		printf '%s\n' "$COUNT_SQL" | psql_in "$ctr" "$user" "$db" -At -v ON_ERROR_STOP=1 | sed "s/^/$db|/"
	done < <(psql_in "$ctr" "$user" postgres -At -v ON_ERROR_STOP=1 \
		-c "SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY 1")
}

# --- 1. stop -------------------------------------------------------------------

step "Stopping Payverge (docker compose down; volumes are kept)"
docker compose down
mkdir -p "$dump_dir"
chmod 700 "$dump_dir"

# --- 2. PostgreSQL 15: counts and dump -----------------------------------------

step "Starting PostgreSQL 15 on $legacy_vol"
docker run -d --name "$old_ctr" --network none --shm-size 256m \
	-v "$legacy_vol:/var/lib/postgresql/data" "$OLD_IMAGE" >/dev/null
wait_ready "$old_ctr"

superuser=""
for candidate in ${PG_SUPERUSER:-} postgres "$app_user" payverge; do
	if [[ $(psql_in "$old_ctr" "$candidate" postgres -At -c 'SELECT rolsuper FROM pg_roles WHERE rolname = current_user' 2>/dev/null) == t ]]; then
		superuser=$candidate
		break
	fi
done
[[ -n $superuser ]] || die "found no superuser role in the PostgreSQL 15 database (tried postgres and $app_user); set PG_SUPERUSER=<role>"
note "superuser: $superuser"
super_has_password=$(psql_in "$old_ctr" "$superuser" postgres -At -v ON_ERROR_STOP=1 \
	-c "SELECT rolpassword IS NOT NULL FROM pg_authid WHERE rolname = current_user")

step "Counting rows"
row_counts "$old_ctr" "$superuser" >"$dump_dir/row-counts-pg15.txt"
note "$(wc -l <"$dump_dir/row-counts-pg15.txt" | tr -d ' ') tables"

step "Dumping the cluster (pg_dumpall) into $dump_dir/pg15-dumpall.sql.gz"
docker exec "$old_ctr" pg_dumpall -U "$superuser" | gzip >"$dump_dir/pg15-dumpall.sql.gz"
chmod 600 "$dump_dir/pg15-dumpall.sql.gz"
docker stop -t 60 "$old_ctr" >/dev/null
docker rm -v "$old_ctr" >/dev/null
# PostgreSQL 15 rewrites global/pg_control whenever it starts or stops. Its
# checksum now, stored in pgdata below, lets postgres/pg18-guard.sh tell when
# 15 has run on the db volume again after this upgrade (a rollback), which
# makes the PostgreSQL 18 copy stale.
legacy_control=$(docker run --rm --network none -v "$legacy_vol:/old:ro" --entrypoint /bin/sh "$OLD_IMAGE" \
	-c 'cksum </old/global/pg_control | awk "{print \$1, \$2}"')
[[ -n $legacy_control ]] || die "could not read global/pg_control in $legacy_vol"

# --- 3. PostgreSQL 18: restore and verify -------------------------------------

step "Creating $new_vol and starting PostgreSQL 18 on it"
if docker volume inspect "$new_vol" >/dev/null 2>&1; then
	# Empty (checked above): created by a `docker compose up` that the guard
	# stopped. Start from a clean one.
	docker volume rm "$new_vol" >/dev/null
fi
docker volume create --label "com.docker.compose.project=$project" \
	--label com.docker.compose.volume=pgdata "$new_vol" >/dev/null
created_new=1
# A throwaway superuser password for the init step; the restore puts the
# real role passwords back. Passed through the environment, not argv.
POSTGRES_PASSWORD=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
export POSTGRES_PASSWORD
docker run -d --name "$new_ctr" --network none --shm-size 256m \
	-e POSTGRES_USER="$superuser" -e POSTGRES_DB=postgres -e POSTGRES_PASSWORD \
	-v "$new_vol:/var/lib/postgresql" "$new_image" >/dev/null
unset POSTGRES_PASSWORD
wait_ready "$new_ctr"

step "Restoring the dump"
restore_log=$dump_dir/restore.log
restore_status=0
# ON_ERROR_STOP=0: psql exits 0 through SQL errors (checked below), and
# non-zero only on a lost connection or a fatal error; with pipefail a
# failed gzip (a corrupt dump) also makes the status non-zero.
gzip -dc "$dump_dir/pg15-dumpall.sql.gz" | psql_in "$new_ctr" "$superuser" postgres -v ON_ERROR_STOP=0 >/dev/null 2>"$restore_log" ||
	restore_status=$?
# pg_dumpall recreates the superuser it was taken with; on a fresh cluster
# that role exists already, which is the one expected error.
unexpected=$(grep -E 'ERROR:|FATAL:|PANIC:|psql: error|gzip:' "$restore_log" | grep -v "ERROR:  role \"$superuser\" already exists" || true)
if [[ $restore_status != 0 || -n $unexpected ]]; then
	[[ $restore_status == 0 ]] || printf 'the restore pipeline exited with status %s\n' "$restore_status" >&2
	printf '%s\n' "$unexpected" | head -n 20 >&2
	die "the restore reported errors (full log: $restore_log)"
fi
if [[ $super_has_password != t ]]; then
	psql_in "$new_ctr" "$superuser" postgres -v ON_ERROR_STOP=1 -c "ALTER ROLE \"$superuser\" PASSWORD NULL" >/dev/null
fi
docker exec "$new_ctr" vacuumdb -U "$superuser" --all --analyze-only --quiet

step "Comparing row counts"
row_counts "$new_ctr" "$superuser" >"$dump_dir/row-counts-pg18.txt"
if ! diff -u "$dump_dir/row-counts-pg15.txt" "$dump_dir/row-counts-pg18.txt" >"$dump_dir/row-counts.diff"; then
	head -n 40 "$dump_dir/row-counts.diff" >&2
	die "row counts differ between PostgreSQL 15 and 18 (see $dump_dir/row-counts.diff)"
fi
note "$(wc -l <"$dump_dir/row-counts-pg18.txt" | tr -d ' ') tables, every row count matches"
docker stop -t 60 "$new_ctr" >/dev/null
docker rm -v "$new_ctr" >/dev/null
docker run --rm --network none -v "$new_vol:/new" --entrypoint /bin/sh "$new_image" \
	-c 'printf "%s\n" "$1" >/new/payverge-pg15-control.cksum && chmod 644 /new/payverge-pg15-control.cksum' \
	sh "$legacy_control" >/dev/null
finished=1

# --- 4. start -----------------------------------------------------------------

say ""
say "PostgreSQL 18 now holds the data ($new_vol). Kept for rollback:"
say "  $legacy_vol             the PostgreSQL 15 data, unchanged"
say "  $dump_dir/   the dump, the row counts and the restore log"
say "To roll back: docker compose down, then bash install.sh --version <previous release>."
say "A rollback loses everything written on PostgreSQL 18. Before upgrading again after one,"
say "remove the stale copy (Payverge refuses to start on it): docker volume rm $new_vol"
say "Once you are sure, free the space: docker volume rm $legacy_vol"
if [[ $no_start == 1 ]]; then
	say "Start Payverge with: docker compose up -d"
	exit 0
fi
step "Starting Payverge (docker compose up -d)"
docker compose up -d

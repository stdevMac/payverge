#!/bin/sh
# Entrypoint wrapper for the postgres service (deploy/docker-compose.yml).
#
# PostgreSQL 18 keeps its data in the pgdata volume, mounted at
# /var/lib/postgresql (the server's PGDATA is /var/lib/postgresql/18/docker).
# Installs from before PostgreSQL 18 kept PostgreSQL 15 data at the root of
# the db volume, which this service mounts read-only at /payverge-legacy-db.
#
# When the legacy volume holds a database and pgdata does not, starting the
# image as is would initialise an EMPTY database and the backend would come
# up with no restaurants. Instead this wrapper says what to do and idles: the
# healthcheck stays red, so the backend never starts on the empty database,
# and the container does not crash-loop. The legacy volume is never written.
#
# After a rollback (PostgreSQL 15 run on the db volume again) the pgdata copy
# is stale: it misses every write made on 15 since the upgrade. The upgrade
# records the checksum of the 15 cluster's global/pg_control in pgdata
# (payverge-pg15-control.cksum, next to the data directory); 15 rewrites that
# file whenever it starts or stops, so a different checksum means 15 ran after
# the upgrade. The wrapper then refuses to start as well.
#
#   sh pg18-guard.sh check   exit 3 when the upgrade is needed, 4 when pgdata
#                            is stale, 0 otherwise
#   sh pg18-guard.sh ARGS... the normal entrypoint (ARGS is the command)
#
# Run ./upgrade-postgres.sh in the install directory to move the data across
# (docs/self-hosting/upgrades.md, "PostgreSQL 18").
set -eu

new_data=${PAYVERGE_PGDATA_DIR:-/var/lib/postgresql/18/docker}
legacy_data=${PAYVERGE_LEGACY_PGDATA_DIR:-/payverge-legacy-db}
marker=${PAYVERGE_UPGRADE_MARKER:-/var/lib/postgresql/payverge-pg15-control.cksum}

needs_upgrade() {
	[ ! -s "$new_data/PG_VERSION" ] && [ -s "$legacy_data/PG_VERSION" ]
}

# stale: pgdata was made by upgrade-postgres.sh and PostgreSQL 15 has run on
# the db volume since.
stale() {
	[ -s "$new_data/PG_VERSION" ] && [ -s "$marker" ] && [ -s "$legacy_data/global/pg_control" ] &&
		[ "$(cksum <"$legacy_data/global/pg_control" | awk '{print $1, $2}')" != "$(cat "$marker")" ]
}

idle() {
	# Idle instead of exiting: restart: unless-stopped would loop otherwise.
	trap 'exit 0' TERM INT
	while :; do
		sleep 3600 &
		wait $! || true
	done
}

if [ "${1:-}" = check ]; then
	if needs_upgrade; then
		echo "upgrade-needed: PostgreSQL $(cat "$legacy_data/PG_VERSION") data in the db volume, none in pgdata"
		exit 3
	fi
	if stale; then
		echo "stale-pgdata: PostgreSQL 15 ran on the db volume after the upgrade to 18"
		exit 4
	fi
	exit 0
fi

if needs_upgrade; then
	cat >&2 <<EOF
==========================================================================
Payverge: this install's database is PostgreSQL $(cat "$legacy_data/PG_VERSION"), in the "db" volume.
This release runs PostgreSQL 18, which keeps its data in the "pgdata" volume.

PostgreSQL is NOT started, so no empty database is created over your data.
Your PostgreSQL $(cat "$legacy_data/PG_VERSION") data is untouched.

To move it to PostgreSQL 18 (dump, restore, row counts checked; the old
volume is kept for rollback), run in the install directory:

    ./upgrade-postgres.sh

See docs/self-hosting/upgrades.md, "PostgreSQL 18".
==========================================================================
EOF
	idle
fi

if stale; then
	cat >&2 <<EOF
==========================================================================
Payverge: PostgreSQL 15 has run on the "db" volume since its data was
copied to PostgreSQL 18 (a rollback). The "pgdata" copy is stale: it is
missing everything written on PostgreSQL 15 since the upgrade.

PostgreSQL 18 is NOT started on the stale copy. Neither volume is changed.

To upgrade again from the current PostgreSQL 15 data, run in the install
directory (the project is "payverge" unless COMPOSE_PROJECT_NAME says
otherwise):

    docker compose down
    docker volume rm <project>_pgdata
    ./upgrade-postgres.sh

To keep the PostgreSQL 18 copy instead, and drop what was written on 15,
remove the old volume: docker volume rm <project>_db

See docs/self-hosting/upgrades.md, "PostgreSQL 18".
==========================================================================
EOF
	idle
fi

exec docker-entrypoint.sh "$@"

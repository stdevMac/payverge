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
#   sh pg18-guard.sh check   exit 3 when the upgrade is needed, 0 otherwise
#   sh pg18-guard.sh ARGS... the normal entrypoint (ARGS is the command)
#
# Run ./upgrade-postgres.sh in the install directory to move the data across
# (docs/self-hosting/upgrades.md, "PostgreSQL 18").
set -eu

new_data=${PAYVERGE_PGDATA_DIR:-/var/lib/postgresql/18/docker}
legacy_data=${PAYVERGE_LEGACY_PGDATA_DIR:-/payverge-legacy-db}

needs_upgrade() {
	[ ! -s "$new_data/PG_VERSION" ] && [ -s "$legacy_data/PG_VERSION" ]
}

if [ "${1:-}" = check ]; then
	if needs_upgrade; then
		echo "upgrade-needed: PostgreSQL $(cat "$legacy_data/PG_VERSION") data in the db volume, none in pgdata"
		exit 3
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
	# Idle instead of exiting: restart: unless-stopped would loop otherwise.
	trap 'exit 0' TERM INT
	while :; do
		sleep 3600 &
		wait $! || true
	done
fi

exec docker-entrypoint.sh "$@"

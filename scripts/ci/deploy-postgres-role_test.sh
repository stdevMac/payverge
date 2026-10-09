#!/usr/bin/env bash
# Behaviour test for deploy/postgres/init-app-role.sql: starts the postgres
# image deploy/docker-compose.yml pins with the init script and the same
# environment the compose file gives it, then checks:
#   - DB_USER logs in over TCP with DB_PASSWORD and is not a superuser
#     (rolsuper = false, no CREATEROLE, REPLICATION or BYPASSRLS);
#   - DB_USER owns DB_NAME and its public schema;
#   - DB_USER can create pg_trgm (the one extension the schema needs) and can
#     create and drop a database (restore.sh's temporary-database swap);
#   - the superuser cannot log in over TCP, with or without DB_PASSWORD;
#   - the script refuses DB_USER=postgres.
#
# Needs docker. Publishes no ports (an internal network).
#   POSTGRES_IMAGE        image to test (default: the one deploy/docker-compose.yml pins)
#   PG_ROLE_TEST_PREFIX   container name prefix (default payverge-pg-role-test)
set -euo pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
compose="$repo/deploy/docker-compose.yml"
init="$repo/deploy/postgres/init-app-role.sql"
prefix=${PG_ROLE_TEST_PREFIX:-payverge-pg-role-test}
image=${POSTGRES_IMAGE:-$(awk '/^  postgres:/{p=1} p && /image:/{print $2; exit}' "$compose")}
[ -n "$image" ] || { echo "could not read the postgres image from $compose" >&2; exit 1; }

app_user=payverge
db_name=payverge
password=role-test-$RANDOM-$RANDOM
main="$prefix-main"
bad="$prefix-bad"
net="$prefix-net"
failures=0

cleanup() {
	docker rm -f -v "$main" "$bad" >/dev/null 2>&1 || true
	docker network rm "$net" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }

start() { # name db_user
	docker run -d --name "$1" --network "$net" \
		-e POSTGRES_DB="$db_name" -e POSTGRES_USER=postgres \
		-e POSTGRES_PASSWORD="$password" -e PAYVERGE_DB_USER="$2" \
		-v "$init:/docker-entrypoint-initdb.d/10-app-role.sql:ro" \
		"$image" >/dev/null
}

# psql from a second container on the same network, as the backend connects:
# inside the postgres container loopback TCP and the socket are trusted, so
# only a remote client exercises the password rule.
tcp() { # user password sql...
	local user=$1 pw=$2
	shift 2
	docker run --rm --network "$net" -e PGPASSWORD="$pw" --entrypoint psql "$image" \
		-X -h "$main" -U "$user" -d "$db_name" -w -v ON_ERROR_STOP=1 -At "$@"
}

docker network create --internal "$net" >/dev/null

start "$main" "$app_user"
for _ in $(seq 1 60); do
	# The init phase runs a socket-only server; wait for the real one on TCP.
	if docker exec "$main" pg_isready -h "$main" -U postgres -d "$db_name" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done
docker exec "$main" pg_isready -h "$main" -U postgres -d "$db_name" >/dev/null ||
	{ docker logs "$main" >&2; echo "postgres did not start" >&2; exit 1; }

attrs=$(tcp "$app_user" "$password" -c \
	"SELECT rolsuper, rolcreaterole, rolreplication, rolbypassrls, rolcreatedb FROM pg_roles WHERE rolname = current_user" || true)
if [ "$attrs" = "f|f|f|f|t" ]; then
	pass "DB_USER logs in over TCP and is not a superuser"
else
	fail "DB_USER role attributes: got '$attrs', want 'f|f|f|f|t' (rolsuper|createrole|replication|bypassrls|createdb)"
fi

owners=$(tcp "$app_user" "$password" -c \
	"SELECT pg_get_userbyid(d.datdba) || '|' || pg_get_userbyid(n.nspowner) FROM pg_database d, pg_namespace n WHERE d.datname = current_database() AND n.nspname = 'public'" || true)
if [ "$owners" = "$app_user|$app_user" ]; then
	pass "DB_USER owns DB_NAME and its public schema"
else
	fail "owners of database|public schema: got '$owners', want '$app_user|$app_user'"
fi

if tcp "$app_user" "$password" -c "CREATE EXTENSION IF NOT EXISTS pg_trgm" >/dev/null &&
	tcp "$app_user" "$password" -c "CREATE TABLE role_probe (id int)" -c "DROP TABLE role_probe" >/dev/null; then
	pass "DB_USER creates pg_trgm and tables"
else
	fail "DB_USER cannot create pg_trgm or a table"
fi

if tcp "$app_user" "$password" -c "CREATE DATABASE role_probe_restore" >/dev/null &&
	tcp "$app_user" "$password" -c "DROP DATABASE role_probe_restore" >/dev/null; then
	pass "DB_USER creates and drops a database (restore swap)"
else
	fail "DB_USER cannot create and drop a database"
fi

if tcp postgres "$password" -c "SELECT 1" >/dev/null 2>&1; then
	fail "the superuser logs in over TCP with DB_PASSWORD"
else
	pass "the superuser cannot log in over TCP with DB_PASSWORD"
fi
if tcp postgres "" -c "SELECT 1" >/dev/null 2>&1; then
	fail "the superuser logs in over TCP with an empty password"
else
	pass "the superuser cannot log in over TCP without a password"
fi

start "$bad" postgres
for _ in $(seq 1 60); do
	[ "$(docker inspect -f '{{.State.Running}}' "$bad" 2>/dev/null)" = true ] || break
	sleep 1
done
bad_log=$(docker logs "$bad" 2>&1 || true)
if [ "$(docker inspect -f '{{.State.Running}}' "$bad")" = false ] &&
	grep -q "DB_USER must not be 'postgres'" <<<"$bad_log"; then
	pass "init file refuses DB_USER=postgres"
else
	fail "init file did not refuse DB_USER=postgres"
	tail -20 <<<"$bad_log" >&2
fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed" >&2
	exit 1
fi
echo "all postgres role checks passed"

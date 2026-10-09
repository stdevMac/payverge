#!/usr/bin/env bash
# Behaviour test of the PostgreSQL 15 -> 18 upgrade against real images:
#   1. seeds a PostgreSQL 15 "db" volume the way releases before 18 did
#      (deploy/postgres/init-app-role.sql, the app role owning the database)
#      and writes rows as the app role;
#   2. the 18 service, wrapped in deploy/postgres/pg18-guard.sh, refuses to
#      initialise an empty pgdata volume over it (no server, message logged);
#   3. deploy/upgrade-postgres.sh --yes --no-start moves the data;
#   4. through the guard, PostgreSQL 18 now serves it from
#      /var/lib/postgresql/18/docker: the app role logs in over TCP with its
#      old password, the rows are there, the superuser still has no password,
#      and the db volume still holds the PostgreSQL 15 data (rollback);
#   5. after a rollback (15 run on the db volume again) the guard refuses to
#      serve the now stale pgdata copy.
#
# Needs docker. Publishes no ports. Volumes and containers are prefixed with
# PG_UPGRADE_TEST_PREFIX (default pvpgup) and removed on exit.
set -euo pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
prefix=${PG_UPGRADE_TEST_PREFIX:-pvpgup}
new_image=$(awk '/^  postgres:/{p=1} p && /image:/{print $2; exit}' "$repo/deploy/docker-compose.yml")
old_image=$(sed -n 's/^readonly OLD_IMAGE="\(.*\)"$/\1/p' "$repo/deploy/upgrade-postgres.sh")
[[ -n $new_image && -n $old_image ]] || { echo "could not read the postgres images" >&2; exit 1; }
work=""
password=upgrade-test-$RANDOM-$RANDOM
net=$prefix-net
failures=0

cleanup() {
	docker rm -f -v "$prefix-seed" "$prefix-guard" "$prefix-18" "$prefix-pgupgrade-old" "$prefix-pgupgrade-new" >/dev/null 2>&1 || true
	docker volume rm "${prefix}_db" "${prefix}_pgdata" >/dev/null 2>&1 || true
	docker network rm "$net" >/dev/null 2>&1 || true
	[[ -z $work ]] || rm -rf "$work"
}
trap cleanup EXIT
cleanup
work=$(mktemp -d "${TMPDIR:-/tmp}/payverge-pgupgrade-docker.XXXXXX")

pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }

wait_tcp() { # wait_tcp CONTAINER: the real server (the init one has no TCP)
	for _ in $(seq 1 90); do
		docker exec "$1" pg_isready -q -h 127.0.0.1 >/dev/null 2>&1 && return 0
		sleep 1
	done
	docker logs --tail 40 "$1" >&2 || true
	return 1
}

guarded() { # guarded NAME: the 18 service as deploy/docker-compose.yml runs it
	docker run -d --name "$1" --network "$net" \
		--cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add SETGID --cap-add SETUID \
		-e POSTGRES_DB=payverge -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD="$password" -e PAYVERGE_DB_USER=payverge \
		-v "${prefix}_pgdata:/var/lib/postgresql" \
		-v "${prefix}_db:/payverge-legacy-db:ro" \
		-v "$repo/deploy/postgres/pg18-guard.sh:/payverge/pg18-guard.sh:ro" \
		-v "$repo/deploy/postgres/init-app-role.sql:/docker-entrypoint-initdb.d/10-app-role.sql:ro" \
		--entrypoint /bin/sh "$new_image" /payverge/pg18-guard.sh postgres >/dev/null
}

docker network create --internal "$net" >/dev/null

# 1. A PostgreSQL 15 install, as releases before 18 laid it out.
docker volume create "${prefix}_db" >/dev/null
docker run -d --name "$prefix-seed" --network "$net" \
	-e POSTGRES_DB=payverge -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD="$password" -e PAYVERGE_DB_USER=payverge \
	-v "$repo/deploy/postgres/init-app-role.sql:/docker-entrypoint-initdb.d/10-app-role.sql:ro" \
	-v "${prefix}_db:/var/lib/postgresql/data" "$old_image" >/dev/null
wait_tcp "$prefix-seed" || { echo "PostgreSQL 15 did not start" >&2; exit 1; }
docker exec "$prefix-seed" psql -X -q -U payverge -d payverge -v ON_ERROR_STOP=1 -c \
	"CREATE TABLE upgrade_probe (id serial PRIMARY KEY, note text); INSERT INTO upgrade_probe (note) SELECT 'row ' || g FROM generate_series(1, 4321) g;"
docker stop -t 30 "$prefix-seed" >/dev/null
docker rm -v "$prefix-seed" >/dev/null

# 2. The 18 service on an empty pgdata volume: guarded, no server.
guarded "$prefix-guard"
sleep 5
logs=$(docker logs "$prefix-guard" 2>&1 || true)
if [[ $logs == *"upgrade-postgres.sh"* ]] && ! docker exec "$prefix-guard" pg_isready -q >/dev/null 2>&1 &&
	[[ $(docker inspect -f '{{.State.Running}}' "$prefix-guard") == true ]]; then
	pass "guard: PostgreSQL 15 data and empty pgdata -> no server, message, no crash loop"
else
	fail "guard did not hold: $logs"
fi
if docker exec "$prefix-guard" sh -c 'test -e /var/lib/postgresql/18/docker/PG_VERSION'; then
	fail "guard: an empty PostgreSQL 18 database was initialised"
else
	pass "guard: no PostgreSQL 18 database initialised over the install"
fi
docker rm -f "$prefix-guard" >/dev/null

# 3. The upgrade, in a minimal install directory for project $prefix.
install=$work/install
mkdir -p "$install"
cp "$repo/deploy/upgrade-postgres.sh" "$install/"
cat >"$install/docker-compose.yml" <<EOF
name: $prefix
services:
  postgres:
    image: $new_image
    volumes:
      - pgdata:/var/lib/postgresql
volumes:
  pgdata: {}
EOF
printf 'DB_USER=payverge\n' >"$install/.env"
if bash "$install/upgrade-postgres.sh" --dir "$install" --yes --no-start; then
	pass "upgrade-postgres.sh: exit 0"
else
	fail "upgrade-postgres.sh failed"
	exit 1
fi
counts=("$install"/backups/pg18-upgrade-*/row-counts-pg18.txt)
grep -q '^payverge|public.upgrade_probe|4321$' "${counts[0]}" &&
	pass "row counts recorded for every table" || fail "row counts: $(cat "${counts[0]}" 2>&1 | head -n 5)"

# 4. The 18 service, through the guard, serves the upgraded data.
guarded "$prefix-18"
wait_tcp "$prefix-18" || fail "PostgreSQL 18 did not start on the upgraded volume"
[[ $(docker exec "$prefix-18" sh -c 'cat "$PGDATA/PG_VERSION"; echo "$PGDATA"' | tr '\n' ' ') == "18 /var/lib/postgresql/18/docker " ]] &&
	pass "PostgreSQL 18 runs from /var/lib/postgresql/18/docker" || fail "PGDATA layout"
got=$(docker run --rm --network "$net" -e PGPASSWORD="$password" --entrypoint psql "$new_image" \
	-X -h "$prefix-18" -U payverge -d payverge -w -At -c "SELECT count(*) FROM upgrade_probe" 2>&1 || true)
[[ $got == 4321 ]] && pass "the app role logs in over TCP with its old password and sees every row" ||
	fail "app role over TCP: $got"
[[ $(docker exec "$prefix-18" psql -X -At -U postgres -c "SELECT rolpassword IS NULL FROM pg_authid WHERE rolname = 'postgres'") == t ]] &&
	pass "the superuser still has no password" || fail "superuser password came back"
[[ $(docker exec "$prefix-18" psql -X -At -U postgres -c "SELECT rolsuper FROM pg_roles WHERE rolname = 'payverge'") == f ]] &&
	pass "the app role is still not a superuser" || fail "app role attributes changed"
[[ $(docker exec "$prefix-18" cat /payverge-legacy-db/PG_VERSION) == 15 ]] &&
	pass "the db volume still holds the PostgreSQL 15 data (rollback)" || fail "legacy volume changed"
docker rm -f "$prefix-18" >/dev/null

# 5. A rollback: PostgreSQL 15 runs on the db volume again. The pgdata copy is
# now stale, and the guard refuses to start 18 on it.
docker run -d --name "$prefix-seed" --network "$net" \
	-v "${prefix}_db:/var/lib/postgresql/data" "$old_image" >/dev/null
wait_tcp "$prefix-seed" || fail "PostgreSQL 15 did not start again on the db volume"
docker exec "$prefix-seed" psql -X -q -U payverge -d payverge -v ON_ERROR_STOP=1 -c \
	"INSERT INTO upgrade_probe (note) VALUES ('written after the rollback')"
docker stop -t 30 "$prefix-seed" >/dev/null
docker rm -v "$prefix-seed" >/dev/null
guarded "$prefix-guard"
sleep 5
logs=$(docker logs "$prefix-guard" 2>&1 || true)
if [[ $logs == *"stale"* ]] && ! docker exec "$prefix-guard" pg_isready -q >/dev/null 2>&1 &&
	[[ $(docker inspect -f '{{.State.Running}}' "$prefix-guard") == true ]]; then
	pass "guard: after a rollback the stale pgdata copy is not served"
else
	fail "guard served stale pgdata after a rollback: $logs"
fi
status=0
docker exec "$prefix-guard" sh /payverge/pg18-guard.sh check >/dev/null 2>&1 || status=$?
[[ $status == 4 ]] && pass "guard check: stale pgdata -> 4" || fail "guard check after rollback: $status"
docker rm -f "$prefix-guard" >/dev/null

if ((failures)); then
	printf '%d check(s) failed\n' "$failures" >&2
	exit 1
fi
echo "deploy-upgrade-postgres (docker): all checks passed"

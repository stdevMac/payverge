#!/usr/bin/env bash
# Payverge public demo: nightly reset to a pristine snapshot.
#
#   demo/reset-demo.sh                reset: database + uploads back to the snapshot
#   demo/reset-demo.sh --maintenance  reset, but leave the edge (caddy) stopped so
#                                     no visitor can change anything before a
#                                     --snapshot (upgrades, admin password changes)
#   demo/reset-demo.sh --snapshot     take the pristine snapshot: the first one
#                                     right after the first boot seeded the
#                                     showroom; every later one only straight
#                                     after --maintenance. It brings the edge back.
#   demo/reset-demo.sh --check        preflight only; changes nothing. Also fails
#                                     when the last good reset is too old
#
# Run from anywhere; it works on the compose project in the deploy directory
# (this script's parent, or DEMO_COMPOSE_DIR) with that directory's .env, and
# it refuses to touch a project whose effective config is not the public demo
# (backend DEMO_MODE=true, from demo/docker-compose.demo.yml).
#
# Reset, in order. Nothing the visitors see changes until step 3, and every
# failure after that puts the previous state back:
#   1. lock (one reset at a time), verify the snapshot against its SHA256SUMS
#   2. restore the snapshot into a scratch database <db>_reset_new while the
#      demo keeps serving; a bad dump fails here, untouched
#   3. stop backend + frontend; swap databases: <db> -> <db>_previous,
#      <db>_reset_new -> <db>; move the uploads aside and unpack the snapshot's
#   4. start; wait for the backend and frontend health checks and for
#      /api/v1/health/ready (from inside the frontend container), plus
#      DEMO_HEALTH_URL through the edge when set
#   5. success: drop the moved-aside uploads; <db>_previous stays until the
#      next reset (forensics). Failure: swap back, restore the uploads, start,
#      log an ALERT line and exit non-zero.
#
# Snapshot safety: a snapshot taken from a live demo would bake visitors'
# edits (phishing names, links) into every future reset. So once a snapshot
# exists, --snapshot refuses unless the edge has stayed stopped since a
# --maintenance reset (DEMO_SNAPSHOT_FROM_LIVE=1 overrides, only after you
# have checked the live content by hand).
#
# Lock: one run at a time. The lock records the holder's PID and process
# start time, so a lock left by a killed run (SIGKILL, OOM, reboot) is broken
# automatically instead of blocking every later reset.
#
# Staleness: every good reset or snapshot stamps demo/pristine/last-ok. --check
# (run hourly by the -check timer) and a run that finds the lock held alert and
# fail when that stamp is older than DEMO_RESET_MAX_AGE_HOURS. With no stamp
# they use the snapshot's age instead, so a lost or unwritable stamp cannot
# silence the watchdog.
#
# Exit codes: 0 ok, 1 reset failed and the previous state is back (or --check
# found the last good reset too old), 2 reset AND rollback failed (the demo may
# be down), 64 usage, 75 another live run holds the lock, 78 not a demo
# project / no snapshot / unsafe snapshot.
#
# Environment (all optional):
#   DEMO_COMPOSE_DIR     the deploy directory (default: this script's parent)
#   DEMO_SNAPSHOT_DIR    where the snapshot lives (default: demo/pristine)
#   DEMO_HEALTH_TIMEOUT  seconds to wait for health (default 300)
#   DEMO_HEALTH_URL      also GET this through the edge, e.g.
#                        https://demo.payverge.io/api/v1/health/ready
#   DEMO_ALERT_URL       POST a one-line alert here on failure (Slack/ntfy/...)
#   DEMO_RESET_LOG       also append the log to this file
#   DEMO_RESET_MAX_AGE_HOURS  alert when the last good reset is older (default 26)
#   DEMO_SNAPSHOT_FROM_LIVE   1 = let --snapshot dump the live demo (see above)
set -Eeuo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
compose_dir=${DEMO_COMPOSE_DIR:-$(dirname "$script_dir")}
snapshot_dir=${DEMO_SNAPSHOT_DIR:-$script_dir/pristine}
health_timeout=${DEMO_HEALTH_TIMEOUT:-300}
health_url=${DEMO_HEALTH_URL:-}
alert_url=${DEMO_ALERT_URL:-}
log_file=${DEMO_RESET_LOG:-}
max_age_hours=${DEMO_RESET_MAX_AGE_HOURS:-26}
snapshot_from_live=${DEMO_SNAPSHOT_FROM_LIVE:-0}
# The backend image's user (distroless nonroot) owns the uploads.
app_uid=65532

log() {
	local line
	line="$(date -u +%Y-%m-%dT%H:%M:%SZ) demo-reset: $*"
	printf '%s\n' "$line" >&2
	if [[ -n $log_file ]]; then printf '%s\n' "$line" >>"$log_file" || true; fi
}

alert() {
	log "ALERT: $*"
	if [[ -n $alert_url ]]; then
		curl -fsS -m 10 -X POST -H 'Content-Type: text/plain' \
			--data-binary "Payverge demo reset: $*" "$alert_url" >/dev/null 2>&1 ||
			log "could not deliver the alert to DEMO_ALERT_URL"
	fi
}

die() {
	local code=$1
	shift
	alert "$*"
	exit "$code"
}

usage() {
	sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 64
}

mode=reset
for arg in "$@"; do
	case $arg in
	--snapshot) mode=snapshot ;;
	--check) mode=check ;;
	--maintenance) mode=maintenance ;;
	-h | --help) usage ;;
	*) usage ;;
	esac
done

[[ $health_timeout =~ ^[0-9]+$ ]] || die 64 "DEMO_HEALTH_TIMEOUT must be a number of seconds"
[[ $max_age_hours =~ ^[1-9][0-9]*$ ]] || die 64 "DEMO_RESET_MAX_AGE_HOURS must be a whole number of hours"
[[ -f $compose_dir/docker-compose.yml ]] || die 78 "no docker-compose.yml in $compose_dir (set DEMO_COMPOSE_DIR)"

dc() {
	(cd "$compose_dir" && docker compose "$@")
}

# --- 1. preflight -------------------------------------------------------------
# backend_demo_mode reads `docker compose config` (YAML) on stdin and succeeds
# only when services.backend.environment.DEMO_MODE is true. Another service
# carrying DEMO_MODE=true does not count.
backend_demo_mode() {
	awk '
		/^services:[[:space:]]*$/ { svc = 1; next }
		/^[^[:space:]]/ { svc = 0; inb = 0; env = 0 }
		svc && /^  [^[:space:]]/ { inb = ($0 ~ /^  backend:[[:space:]]*$/); env = 0; next }
		inb && /^    [^[:space:]]/ { env = ($0 ~ /^    environment:[[:space:]]*$/); next }
		inb && env && /^      DEMO_MODE: "?true"?[[:space:]]*$/ { found = 1 }
		END { exit !found }
	'
}

# Only the public demo project: the effective backend config must carry
# DEMO_MODE=true, which only demo/docker-compose.demo.yml sets.
effective=$(dc config 2>/dev/null) || die 78 "docker compose config failed in $compose_dir"
if ! backend_demo_mode <<<"$effective"; then
	die 78 "refusing: the compose project in $compose_dir is not the public demo (backend DEMO_MODE is not true; does COMPOSE_FILE in .env include demo/docker-compose.demo.yml?)"
fi
project=$(sed -n 's/^name: *//p' <<<"$effective" | head -n 1)
[[ -n $project ]] || die 78 "could not read the compose project name"

mkdir -p "$snapshot_dir"
last_ok_file="$snapshot_dir/last-ok"
maintenance_file="$snapshot_dir/.maintenance"

mark_ok() {
	date +%s >"$last_ok_file" 2>/dev/null || log "warning: could not write $last_ok_file"
}

# file_mtime prints a file's modification time in epoch seconds (GNU or BSD
# stat), or nothing if it cannot be read.
file_mtime() {
	stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || true
}

# stale_reset prints a reason and succeeds when the last good reset or
# snapshot is older than max_age_hours. --snapshot stamps too, so a fresh
# snapshot counts as a good starting point for the watchdog. Without a stamp
# it falls back to the snapshot's age (SHA256SUMS is written last): a stamp
# that was deleted, or that mark_ok could not write, must not read as fresh
# forever. With neither (nothing set up yet) it is not stale; --check then
# fails on the missing snapshot instead.
stale_reset() {
	local last now what
	if [[ -s $last_ok_file ]]; then
		last=$(head -n 1 "$last_ok_file")
		what="the last good reset or snapshot"
		[[ $last =~ ^[0-9]+$ ]] || {
			printf 'unreadable %s' "$last_ok_file"
			return 0
		}
	elif [[ -s $snapshot_dir/SHA256SUMS ]]; then
		last=$(file_mtime "$snapshot_dir/SHA256SUMS")
		what="the snapshot (there is no $last_ok_file stamp)"
		[[ $last =~ ^[0-9]+$ ]] || {
			printf 'no %s stamp and the snapshot age is unreadable' "$last_ok_file"
			return 0
		}
	else
		return 1
	fi
	now=$(date +%s)
	if ((now - last > max_age_hours * 3600)); then
		printf '%s was %s hours ago (limit %s)' "$what" "$(((now - last) / 3600))" "$max_age_hours"
		return 0
	fi
	return 1
}

# proc_start prints a process's start time, or nothing if it is not running.
# PID plus start time identifies a process across PID reuse and reboots.
proc_start() {
	ps -o lstart= -p "$1" 2>/dev/null | sed 's/^ *//; s/ *$//'
}

lock_dir="$snapshot_dir/.lock"
# lock_holder prints the PID of a live process holding the lock, if any.
lock_holder() {
	local holder holder_start now_start
	holder=$(head -n 1 "$lock_dir/pid" 2>/dev/null || true)
	holder_start=$(head -n 1 "$lock_dir/start" 2>/dev/null | sed 's/^ *//; s/ *$//' || true)
	[[ $holder =~ ^[0-9]+$ ]] || return 1
	# ps, not kill -0: it also sees a holder owned by another user (root).
	now_start=$(proc_start "$holder")
	[[ -n $now_start ]] || return 1
	# A recorded start time that no longer matches means the PID was reused.
	[[ -z $holder_start || $now_start == "$holder_start" ]] || return 1
	printf '%s' "$holder"
}
take_lock() {
	mkdir "$lock_dir" 2>/dev/null || return 1
	printf '%s\n' "$$" >"$lock_dir/pid"
	proc_start "$$" >"$lock_dir/start"
}
if [[ $mode == check ]]; then
	# The watchdog never takes the lock (it must not make a nightly reset
	# that starts meanwhile exit 75). While a run is in progress it only
	# checks staleness.
	if [[ -d $lock_dir ]] && holder=$(lock_holder); then
		if reason=$(stale_reset); then
			die 1 "check: a run (pid $holder) is in progress and $reason"
		fi
		log "check: a reset or snapshot (pid $holder) is in progress; skipping the snapshot checks"
		exit 0
	fi
elif ! take_lock; then
	if holder=$(lock_holder); then
		if reason=$(stale_reset); then
			die 75 "another run (pid $holder) holds $lock_dir and $reason"
		fi
		log "another reset or snapshot (pid $holder) holds $lock_dir; not starting a second one"
		exit 75
	fi
	log "breaking a stale lock left by pid $(head -n 1 "$lock_dir/pid" 2>/dev/null || echo unknown) (no longer running, or its PID was reused)"
	rm -rf "$lock_dir"
	take_lock || die 75 "could not take $lock_dir after breaking a stale lock"
fi
if [[ $mode != check ]]; then
	trap 'rm -rf "$lock_dir"' EXIT
fi

log "project=$project mode=$mode dir=$compose_dir"

dc up -d --wait postgres >/dev/null 2>&1 || die 1 "postgres did not become healthy; nothing was changed"

# Names, never secrets: the database and the application role that owns it
# (not the superuser, so the restored objects stay the backend's). Over the
# container's local socket it needs no password.
# shellcheck disable=SC2016 # expanded inside the container
db_name=$(dc exec -T postgres sh -c 'printf %s "$POSTGRES_DB"')
# shellcheck disable=SC2016
db_user=$(dc exec -T postgres sh -c 'printf %s "$PAYVERGE_DB_USER"')
[[ $db_name =~ ^[a-z_][a-z0-9_]{0,40}$ ]] || die 78 "unexpected database name '$db_name'"
[[ $db_user =~ ^[A-Za-z_][A-Za-z0-9_-]{0,62}$ ]] || die 78 "unexpected database user"
db_new="${db_name}_reset_new"
db_prev="${db_name}_previous"
db_failed="${db_name}_failed"

psql_admin() {
	# Statements on stdin, one per line; each runs on its own (DROP/ALTER
	# DATABASE cannot run inside a transaction block).
	dc exec -T postgres psql -X -q -v ON_ERROR_STOP=1 -U "$db_user" -d postgres
}

db_exists() {
	[[ $(dc exec -T postgres psql -X -qAt -U "$db_user" -d postgres \
		-c "SELECT 1 FROM pg_database WHERE datname = '$1'") == 1 ]]
}

# Runs a shell snippet in a one-off container of this project with the uploads
# volume at /data/storage (the restore service: root, CHOWN/FOWNER caps, no
# network beyond the internal db one).
storage_shell() {
	dc --profile restore run --rm --no-deps -T --entrypoint /bin/sh restore -ec "$1"
}

wait_healthy() {
	local deadline=$((SECONDS + health_timeout)) svc cid state ok
	while ((SECONDS < deadline)); do
		ok=1
		for svc in backend frontend; do
			cid=$(dc ps -q "$svc" 2>/dev/null || true)
			state=$([[ -n $cid ]] && docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null || echo missing)
			[[ $state == healthy ]] || ok=0
		done
		if ((ok)) &&
			dc exec -T frontend wget -q -T 10 -O /dev/null http://backend:8080/api/v1/health/ready 2>/dev/null &&
			dc exec -T frontend wget -q -T 10 -O /dev/null http://127.0.0.1:3000/api/health 2>/dev/null; then
			if ((!edge)) || [[ -z $health_url ]] || curl -fsS -m 10 -o /dev/null "$health_url"; then
				return 0
			fi
		fi
		sleep 5
	done
	return 1
}

# edge controls whether start_apps also starts caddy: --maintenance keeps the
# site unreachable so nobody can change the demo before the next --snapshot.
edge=1
start_apps() {
	if ((edge)); then
		dc up -d backend frontend caddy >/dev/null 2>&1
	else
		dc up -d backend frontend >/dev/null 2>&1
	fi
}

caddy_started_at() {
	local cid
	cid=$(dc ps -aq caddy 2>/dev/null | head -n 1)
	[[ -n $cid ]] || return 0
	docker inspect -f '{{.State.StartedAt}}' "$cid" 2>/dev/null || true
}

caddy_running() {
	[[ -n $(dc ps -q --status running caddy 2>/dev/null) ]]
}

# --- snapshot -----------------------------------------------------------------
if [[ $mode == snapshot ]]; then
	db_exists "$db_name" || die 78 "database $db_name does not exist yet; start the demo once first"
	if [[ -s $snapshot_dir/db.dump ]]; then
		# Never bake visitor edits into the pristine snapshot: the edge must
		# have stayed down since a --maintenance reset restored the old one.
		marker=$(head -n 1 "$maintenance_file" 2>/dev/null || true)
		if [[ $snapshot_from_live == 1 ]]; then
			log "DEMO_SNAPSHOT_FROM_LIVE=1: snapshotting the live demo as is"
		elif [[ ! -f $maintenance_file ]] || caddy_running || [[ $(caddy_started_at) != "$marker" ]]; then
			die 78 "refusing to snapshot a live demo: visitors' changes would come back every night. Run demo/reset-demo.sh --maintenance first (it resets and keeps the site offline), make your change, then --snapshot"
		fi
	else
		log "first snapshot: the showroom must be freshly seeded (nobody has used the demo yet)"
	fi
	tmp="$snapshot_dir/.new"
	rm -rf "$tmp"
	mkdir -p "$tmp"
	log "stopping the edge, backend and frontend for a consistent snapshot"
	dc stop caddy backend frontend >/dev/null 2>&1 || true
	ok=1
	dc exec -T postgres pg_dump -U "$db_user" -d "$db_name" -Fc --no-owner --no-privileges >"$tmp/db.dump" || ok=0
	if ((ok)); then
		storage_shell 'tar -C /data/storage --exclude ./.previous-reset -cf - .' >"$tmp/storage.tar" || ok=0
	fi
	if ((ok)); then
		# Space-scan artifacts live in the backend data volume, not uploads.
		storage_shell 'mkdir -p /data/app/space-scans && tar -C /data/app/space-scans -cf - .' >"$tmp/scans.tar" || ok=0
	fi
	start_apps || ok=0
	((ok)) || {
		rm -rf "$tmp"
		wait_healthy || true
		die 1 "snapshot failed; the previous snapshot (if any) is unchanged"
	}
	{
		printf 'created_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		printf 'project=%s\n' "$project"
		printf 'database=%s\n' "$db_name"
		printf 'backend_image=%s\n' "$(docker inspect -f '{{.Config.Image}}' "$(dc ps -q backend)" 2>/dev/null || echo unknown)"
	} >"$tmp/MANIFEST"
	(
		cd "$tmp"
		if command -v sha256sum >/dev/null 2>&1; then
			sha256sum db.dump storage.tar scans.tar MANIFEST >SHA256SUMS
		else
			shasum -a 256 db.dump storage.tar scans.tar MANIFEST >SHA256SUMS
		fi
	)
	chmod 600 "$tmp"/*
	rm -rf "$snapshot_dir/previous"
	mkdir -p "$snapshot_dir/previous"
	for f in db.dump storage.tar scans.tar MANIFEST SHA256SUMS; do
		[[ ! -f $snapshot_dir/$f ]] || mv "$snapshot_dir/$f" "$snapshot_dir/previous/$f"
		mv "$tmp/$f" "$snapshot_dir/$f"
	done
	rm -rf "$tmp" "$maintenance_file"
	wait_healthy || die 1 "snapshot taken but the demo did not come back healthy"
	mark_ok
	log "snapshot ok: $(du -h "$snapshot_dir/db.dump" | cut -f1) database, $(du -h "$snapshot_dir/storage.tar" | cut -f1) uploads"
	exit 0
fi

# --- reset / check --------------------------------------------------------------
for f in db.dump storage.tar SHA256SUMS; do
	[[ -s $snapshot_dir/$f ]] || die 78 "no pristine snapshot in $snapshot_dir (run demo/reset-demo.sh --snapshot once)"
done
verify_snapshot() {
	cd "$snapshot_dir" || return 1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c --quiet SHA256SUMS
	else
		shasum -a 256 -c --quiet SHA256SUMS
	fi
}
(verify_snapshot) >/dev/null ||
	die 78 "the snapshot in $snapshot_dir does not match its SHA256SUMS"

if [[ $mode == check ]]; then
	if reason=$(stale_reset); then
		die 1 "check: $reason; is the reset timer running? (journalctl -u payverge-demo-reset.service)"
	fi
	log "check ok: demo project, snapshot verified, postgres healthy, last good reset or snapshot within ${max_age_hours}h"
	exit 0
fi

if [[ $mode == maintenance ]]; then
	edge=0
	rm -f "$maintenance_file"
	log "maintenance: the edge (caddy) stays stopped until demo/reset-demo.sh --snapshot"
	dc stop caddy >/dev/null 2>&1 || die 1 "could not stop caddy; nothing was changed"
fi

started=$SECONDS

# --- 2. restore into a scratch database (the demo keeps serving) ---------------
log "restoring the snapshot into $db_new"
psql_admin <<SQL || die 1 "could not create $db_new; nothing was changed"
DROP DATABASE IF EXISTS "$db_new" WITH (FORCE);
CREATE DATABASE "$db_new" OWNER "$db_user";
SQL
if ! dc exec -T postgres pg_restore -U "$db_user" -d "$db_new" --no-owner --no-privileges \
	--exit-on-error --single-transaction <"$snapshot_dir/db.dump" >/dev/null; then
	printf 'DROP DATABASE IF EXISTS "%s" WITH (FORCE);\n' "$db_new" | psql_admin || true
	die 1 "pg_restore of the snapshot failed; the live demo was not touched"
fi

# --- 3. swap --------------------------------------------------------------------
rollback() {
	local reason=$1
	log "rolling back: $reason"
	edge=1 # a failed --maintenance run puts the previous demo back online
	dc stop backend frontend >/dev/null 2>&1 || true
	local rb=0
	if db_exists "$db_prev"; then
		psql_admin <<SQL || rb=1
DROP DATABASE IF EXISTS "$db_failed" WITH (FORCE);
ALTER DATABASE "$db_name" RENAME TO "$db_failed";
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$db_prev';
ALTER DATABASE "$db_prev" RENAME TO "$db_name";
SQL
	fi
	storage_shell '
		if [ -d /data/storage/.previous-reset ]; then
			find /data/storage -mindepth 1 -maxdepth 1 ! -name .previous-reset -exec rm -rf {} +
			find /data/storage/.previous-reset -mindepth 1 -maxdepth 1 -exec mv {} /data/storage/ \;
			rmdir /data/storage/.previous-reset
		fi
		if [ -d /data/app/.space-scans-previous-reset ]; then
			rm -rf /data/app/space-scans
			mv /data/app/.space-scans-previous-reset /data/app/space-scans
		fi' >/dev/null || rb=1
	start_apps || rb=1
	if ((rb == 0)) && wait_healthy; then
		die 1 "reset failed ($reason); the previous database and uploads are back and healthy (failed copy kept as $db_failed)"
	fi
	die 2 "reset failed ($reason) AND the rollback did not come back healthy; the demo may be down. Previous database: $db_name or $db_prev; check 'docker compose ps' and logs"
}

log "stopping backend and frontend"
dc stop backend frontend >/dev/null 2>&1 || {
	printf 'DROP DATABASE IF EXISTS "%s" WITH (FORCE);\n' "$db_new" | psql_admin || true
	edge=1
	start_apps || true
	die 1 "could not stop the apps; nothing was swapped"
}

log "swapping $db_new -> $db_name (previous kept as $db_prev)"
if ! psql_admin <<SQL; then
DROP DATABASE IF EXISTS "$db_prev" WITH (FORCE);
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$db_name' AND pid <> pg_backend_pid();
ALTER DATABASE "$db_name" RENAME TO "$db_prev";
ALTER DATABASE "$db_new" RENAME TO "$db_name";
SQL
	# Partial swap: put the live name back if it moved.
	if ! db_exists "$db_name" && db_exists "$db_prev"; then
		printf 'ALTER DATABASE "%s" RENAME TO "%s";\n' "$db_prev" "$db_name" | psql_admin || true
	fi
	edge=1
	start_apps || true
	wait_healthy || die 2 "database swap failed and the demo did not come back healthy"
	die 1 "database swap failed; the previous database is live again"
fi

log "replacing the uploads with the snapshot's"
storage_shell "
	rm -rf /data/storage/.previous-reset
	mkdir /data/storage/.previous-reset
	find /data/storage -mindepth 1 -maxdepth 1 ! -name .previous-reset -exec mv {} /data/storage/.previous-reset/ \;
	tar -C /data/storage -xf -
	chown -R $app_uid:$app_uid /data/storage
	chmod 700 /data/storage/.previous-reset" <"$snapshot_dir/storage.tar" >/dev/null ||
	rollback "could not replace the uploads"

# Space-scan artifacts (backend data volume). A snapshot taken before scans.tar
# existed has none, so the reset then just empties the directory.
scans_src=/dev/null
scans_unpack=:
if [[ -f $snapshot_dir/scans.tar ]]; then
	scans_src=$snapshot_dir/scans.tar
	scans_unpack='tar -C /data/app/space-scans -xf -'
fi
log "replacing the space-scan artifacts with the snapshot's"
storage_shell "
	rm -rf /data/app/.space-scans-previous-reset
	if [ -d /data/app/space-scans ]; then mv /data/app/space-scans /data/app/.space-scans-previous-reset; fi
	mkdir -p /data/app/space-scans
	$scans_unpack
	chown -R $app_uid:$app_uid /data/app/space-scans
	chmod 750 /data/app/space-scans" <"$scans_src" >/dev/null ||
	rollback "could not replace the space-scan artifacts"

# --- 4. start and health-check --------------------------------------------------
log "starting the demo"
start_apps || rollback "docker compose up failed"
wait_healthy || rollback "not healthy within ${health_timeout}s"

# --- 5. done --------------------------------------------------------------------
storage_shell 'rm -rf /data/storage/.previous-reset /data/app/.space-scans-previous-reset' >/dev/null ||
	log "warning: could not remove the moved-aside uploads (.previous-reset)"
mark_ok
if [[ $mode == maintenance ]]; then
	caddy_started_at >"$maintenance_file"
	log "maintenance reset ok in $((SECONDS - started))s; the site stays offline. Make your change (e.g. docker compose exec backend /app/server admin ...), then run demo/reset-demo.sh --snapshot"
	exit 0
fi
log "reset ok in $((SECONDS - started))s (previous database kept as $db_prev until the next reset)"

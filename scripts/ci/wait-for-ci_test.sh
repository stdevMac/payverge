#!/usr/bin/env bash
# Contract test for scripts/ci/wait-for-ci.sh, the release gate that waits for
# a green ci.yml run on the release commit. A fake `gh` on PATH replays one
# canned GitHub API response per call, so no network or token is needed.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$ROOT/scripts/ci/wait-for-ci.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

SHA=0123456789abcdef0123456789abcdef01234567
failures=0
fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}
pass() { echo "ok: $*"; }

mkdir -p "$WORK/bin"
cat >"$WORK/bin/gh" <<'EOF'
#!/usr/bin/env bash
# Fake gh: logs its arguments and prints responses/<n> for the n-th call,
# repeating the last one; a response of FAIL makes the call fail.
set -euo pipefail
n=$(($(cat "$FAKE_GH/count" 2>/dev/null || echo 0) + 1))
echo "$n" >"$FAKE_GH/count"
printf '%s\n' "$*" >>"$FAKE_GH/calls"
file="$FAKE_GH/responses/$n"
if [[ ! -f "$file" ]]; then
	last=1
	while [[ -f "$FAKE_GH/responses/$((last + 1))" ]]; do last=$((last + 1)); done
	file="$FAKE_GH/responses/$last"
fi
if [[ "$(cat "$file")" == FAIL ]]; then
	echo "gh: HTTP 502" >&2
	exit 1
fi
cat "$file"
EOF
chmod +x "$WORK/bin/gh"

# run_json <event> <status> <conclusion>: one workflow run object.
run_json() {
	printf '{"event":"%s","status":"%s","conclusion":%s,"html_url":"https://github.example/runs/%s"}' \
		"$1" "$2" "$([[ -n "$3" ]] && printf '"%s"' "$3" || printf null)" "$RANDOM"
}
# runs <run_json>...: a workflow-runs API page.
runs() {
	local IFS=,
	printf '{"total_count":%d,"workflow_runs":[%s]}' "$#" "$*"
}

# scenario <name> <expected exit> <stdout/stderr pattern> <response>...
# Remaining env (CI_WAIT_SECONDS etc.) is taken from the caller.
scenario() {
	local name="$1" want="$2" pattern="$3"
	shift 3
	local dir="$WORK/$name"
	mkdir -p "$dir/responses"
	local i=1
	for response in "$@"; do
		printf '%s' "$response" >"$dir/responses/$i"
		i=$((i + 1))
	done
	local got=0
	FAKE_GH="$dir" PATH="$WORK/bin:$PATH" GH_REPO=owner/repo CI_POLL_SECONDS=0 \
		bash "$SCRIPT" "${SCENARIO_SHA:-$SHA}" >"$dir/out" 2>&1 || got=$?
	if [[ "$got" -ne "$want" ]]; then
		fail "$name: exit $got, want $want"
		sed 's/^/    /' "$dir/out" >&2
		return
	fi
	if [[ -n "$pattern" ]] && ! grep -Eq "$pattern" "$dir/out"; then
		fail "$name: output does not match /$pattern/"
		sed 's/^/    /' "$dir/out" >&2
		return
	fi
	pass "$name"
}

scenario green-push 0 "CI passed" \
	"$(runs "$(run_json push completed success)")"
calls="$(cat "$WORK/green-push/calls")"
if [[ "$calls" == "api repos/owner/repo/actions/workflows/ci.yml/runs?head_sha=$SHA&per_page=100" ]]; then
	pass "queries ci.yml runs for the exact commit"
else
	fail "unexpected gh call: $calls"
fi

scenario pending-then-green 0 "CI passed" \
	"$(runs)" \
	"$(runs "$(run_json push queued '')")" \
	"$(runs "$(run_json push in_progress '')")" \
	"$(runs "$(run_json push completed success)")"
if [[ "$(cat "$WORK/pending-then-green/count")" == 4 ]]; then
	pass "polls until the run completes"
else
	fail "pending-then-green made $(cat "$WORK/pending-then-green/count") calls, want 4"
fi

scenario failed-push 1 "CI did not pass.*push: failure" \
	"$(runs "$(run_json push completed failure)")"

scenario cancelled-push 1 "CI did not pass.*push: cancelled" \
	"$(runs "$(run_json push completed cancelled)")"

scenario failed-push-green-schedule 0 "CI passed" \
	"$(runs "$(run_json push completed failure)" "$(run_json schedule completed success)")"

# A dispatched run may use another branch's ci.yml, so it never counts.
CI_APPEAR_SECONDS=0 scenario green-dispatch-ignored 1 "No push or schedule CI run" \
	"$(runs "$(run_json workflow_dispatch completed success)")"

# A pull-request run tests a merge ref, not the tagged commit, and fork pull
# requests choose their own head commit: it never counts.
CI_APPEAR_SECONDS=0 scenario pull-request-only 1 "No push or schedule CI run" \
	"$(runs "$(run_json pull_request completed success)")"

CI_APPEAR_SECONDS=0 scenario no-run 1 "No push or schedule CI run" "$(runs)"

# A failed run being re-run is pending again: keep waiting.
scenario rerun-goes-green 0 "CI passed" \
	"$(runs "$(run_json push in_progress '')" "$(run_json schedule completed failure)")" \
	"$(runs "$(run_json push completed success)" "$(run_json schedule completed failure)")"

CI_WAIT_SECONDS=0 scenario still-running 1 "still running" \
	"$(runs "$(run_json push in_progress '')")"

scenario transient-api-error 0 "CI passed" \
	FAIL FAIL "$(runs "$(run_json push completed success)")"

scenario api-down 1 "GitHub API failed 5 times" FAIL

SCENARIO_SHA=not-a-sha scenario bad-sha 2 "usage" "$(runs)"

if [[ "$failures" -gt 0 ]]; then
	echo "wait-for-ci_test: $failures failure(s)" >&2
	exit 1
fi
echo "wait-for-ci_test: PASS"

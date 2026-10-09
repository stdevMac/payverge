#!/usr/bin/env bash
# Waits for the CI verdict (ci.yml) on one commit and fails unless it is green.
#
# release.yml runs this before it builds anything, so a release never
# publishes signed images or install files from a commit CI did not pass:
# not a release pull request merged while CI was red or pending, and not a
# workflow_dispatch for a tag on an untested commit.
#
# Only runs of main's own copy of ci.yml count: push (main) and schedule. A
# pull-request run tests a merge ref, a fork pull request chooses its own
# head commit, and a workflow_dispatch run may come from any branch whose
# ci.yml the dispatcher chose. Any green run passes, so a flaky run
# that was re-run green is enough; a run being re-run is pending again.
#
# Usage: wait-for-ci.sh <40-hex commit sha>
# Env:   GH_REPO            owner/repo (required; gh reads GH_TOKEN)
#        CI_WORKFLOW        workflow file to wait for (default ci.yml)
#        CI_WAIT_SECONDS    give up while CI is still running (default 6600)
#        CI_APPEAR_SECONDS  give up when no run exists yet (default 900)
#        CI_POLL_SECONDS    seconds between API calls (default 30)
#
# Exit codes: 0 CI passed, 1 CI failed / missing / timed out, 2 usage error.
set -euo pipefail

sha="${1:-}"
if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
	echo "usage: $0 <40-hex commit sha>" >&2
	exit 2
fi
if [[ -z "${GH_REPO:-}" ]]; then
	echo "usage: GH_REPO=owner/repo $0 <sha>" >&2
	exit 2
fi
workflow="${CI_WORKFLOW:-ci.yml}"
wait_seconds="${CI_WAIT_SECONDS:-6600}"
appear_seconds="${CI_APPEAR_SECONDS:-900}"
poll_seconds="${CI_POLL_SECONDS:-30}"

retry_hint="Then run the Release workflow again from main for this tag (Actions > Release > Run workflow)."
api_failures=0
SECONDS=0
while true; do
	if body="$(gh api "repos/$GH_REPO/actions/workflows/$workflow/runs?head_sha=$sha&per_page=100")"; then
		api_failures=0
		runs="$(jq -c '[.workflow_runs[] | select(.event == "push" or .event == "schedule")]' <<<"$body")"
		total="$(jq length <<<"$runs")"
		green="$(jq -r 'map(select(.status == "completed" and .conclusion == "success")) | first | .html_url // empty' <<<"$runs")"
		pending="$(jq 'map(select(.status != "completed")) | length' <<<"$runs")"

		if [[ -n "$green" ]]; then
			echo "CI passed on $sha: $green"
			exit 0
		fi
		if [[ "$total" -gt 0 && "$pending" -eq 0 ]]; then
			verdicts="$(jq -r 'map("\(.event): \(.conclusion)") | join(", ")' <<<"$runs")"
			echo "::error::CI did not pass on $sha ($verdicts). Fix it or re-run the failed jobs until CI is green. $retry_hint"
			jq -r '.[].html_url' <<<"$runs"
			exit 1
		fi
		if [[ "$total" -eq 0 && "$SECONDS" -ge "$appear_seconds" ]]; then
			echo "::error::No push or schedule CI run on $sha after ${SECONDS}s (dispatched runs do not count). The commit must reach main with CI running on the push. $retry_hint"
			exit 1
		fi
		echo "Waiting for CI on $sha: $pending of $total run(s) still in progress (${SECONDS}s)."
	else
		api_failures=$((api_failures + 1))
		if [[ "$api_failures" -ge 5 ]]; then
			echo "::error::The GitHub API failed 5 times in a row while reading CI runs for $sha."
			exit 1
		fi
	fi

	if [[ "$SECONDS" -ge "$wait_seconds" ]]; then
		echo "::error::CI on $sha is still running after ${SECONDS}s. Wait for it to finish green. $retry_hint"
		exit 1
	fi
	sleep "$poll_seconds"
done

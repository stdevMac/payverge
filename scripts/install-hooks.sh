#!/usr/bin/env bash
# Configure git to use the repo-tracked hooks directory.
# Idempotent — safe to run more than once. Works in main checkouts and worktrees.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hooks_src="scripts/hooks"

if [ ! -d "$repo_root/$hooks_src" ]; then
  echo "No tracked hooks found at $repo_root/$hooks_src" >&2
  exit 1
fi

git config core.hooksPath "$hooks_src"
echo "Configured git core.hooksPath -> $hooks_src"
echo "Tracked hooks:"
ls "$repo_root/$hooks_src" | sed 's/^/  /'

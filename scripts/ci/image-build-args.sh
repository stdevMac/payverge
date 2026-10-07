#!/usr/bin/env bash
# Print the Docker build arguments for a Payverge image, one KEY=VALUE per line.
#
#   scripts/ci/image-build-args.sh <backend|frontend> <version>
#
# <version> is the release version without the leading "v" (1.2.3,
# 1.2.3-rc.1, or 0.0.0-ci for pull-request builds).
#
# This is the single source of the build arguments for both the published
# images (.github/workflows/release.yml) and the CI builds that must behave
# like them (.github/workflows/ci.yml: the `images` job and `frontend-build`).
# Keeping one list means CI cannot pass a value a release would not, and so
# cannot hide a build that only succeeds with deployment-specific settings.
#
# Only build metadata is emitted. Runtime settings (PUBLIC_URL, API_URL,
# NETWORK, SUPPORT_EMAIL, ...) are read from the container environment at
# request time (docs/self-hosting/frontend-config.md); a published image
# must not carry one deployment's URLs. The backend takes no build arguments.
#
# Run inside the checkout being built: the commit SHA and timestamp come
# from HEAD.
set -euo pipefail

usage() {
  echo "usage: $0 <backend|frontend> <version>" >&2
  exit 2
}

[[ $# -eq 2 ]] || usage
image="$1"
version="$2"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "image-build-args: version must be X.Y.Z or X.Y.Z-pre without a leading v, got '$version'" >&2
  exit 2
fi

case "$image" in
  backend) ;;
  frontend)
    sha="$(git rev-parse --verify HEAD)"
    committed="$(git log -1 --format=%cI HEAD)"
    printf 'NEXT_PUBLIC_RELEASE_SHA=%s\n' "$sha"
    printf 'NEXT_PUBLIC_VERSION=%s\n' "$version"
    printf 'NEXT_PUBLIC_BUILD_TIMESTAMP=%s\n' "$committed"
    printf 'NEXT_PUBLIC_SENTRY_RELEASE=%s\n' "$version"
    ;;
  *)
    echo "image-build-args: unknown image '$image' (want backend or frontend)" >&2
    exit 2
    ;;
esac

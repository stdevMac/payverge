#!/usr/bin/env bash
# Self-test for tools/oss-export: unit tests for the config grammar and the
# gates, plus end-to-end runs of export.sh against throwaway fixture repos in
# a temporary directory (stub scanners; nothing touches this repository).
#
#   tools/oss-export/test.sh
#   OSS_EXPORT_REAL_GITLEAKS=/path/to/gitleaks tools/oss-export/test.sh
#   OSS_EXPORT_BASH=/bin/bash tools/oss-export/test.sh   # macOS bash 3.2
set -euo pipefail
cd "$(dirname "$0")"
exec node --test test/config.test.mjs test/gates.test.mjs test/export.test.mjs

#!/bin/bash
# Contract tests for validate-backup-policy.sh.
# 1) Committed infra/backup/ docs must PASS.
# 2) Temp fixtures mutated to be over-privileged / under-hardened must be REJECTED.
#
# Usage: bash backend/scripts/validate-backup-policy.test.sh
# Exit 0 + "ALL TESTS PASSED" on success.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
VALIDATOR="${SCRIPT_DIR}/validate-backup-policy.sh"
COMMITTED_DIR="${REPO_ROOT}/infra/backup"

FAILURES=0

assert() {
  local desc="$1"
  local cond="$2" # "0" means pass
  if [ "${cond}" = "0" ]; then
    echo "  ok    - ${desc}"
  else
    echo "  FAIL  - ${desc}"
    FAILURES=$((FAILURES + 1))
  fi
}

copy_fixtures() {
  local dest="$1"
  mkdir -p "${dest}"
  cp "${COMMITTED_DIR}"/*.json "${dest}/"
}

run_validator() {
  local dir="$1"
  INFRA_BACKUP_DIR="${dir}" bash "${VALIDATOR}" >/tmp/vbp-out.$$ 2>/tmp/vbp-err.$$
  return $?
}

echo "=== Test 1: committed infra/backup/ policies PASS ==="
if run_validator "${COMMITTED_DIR}"; then
  assert "committed policies validate" "0"
else
  assert "committed policies validate" "1"
  echo "    stdout: $(cat /tmp/vbp-out.$$ 2>/dev/null)"
  echo "    stderr: $(cat /tmp/vbp-err.$$ 2>/dev/null)"
fi

if [ -f "${COMMITTED_DIR}/backup-stack.json" ]; then
  assert "provisionable CloudFormation backup stack is committed" "0"
else
  assert "provisionable CloudFormation backup stack is committed" "1"
fi

echo "=== Test 2: over-privileged upload (s3:*) REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "iam-upload-policy.json"
data = json.loads(p.read_text())
data["Statement"][0]["Action"] = "s3:*"
p.write_text(json.dumps(data, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "s3:* on upload is rejected" "1"
else
  assert "s3:* on upload is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 3: Resource '*' REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "iam-restore-policy.json"
data = json.loads(p.read_text())
data["Statement"][0]["Resource"] = "*"
p.write_text(json.dumps(data, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "Resource * is rejected" "1"
else
  assert "Resource * is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 4: restore policy with DeleteObject REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "iam-restore-policy.json"
data = json.loads(p.read_text())
# Append a destructive action to an existing statement
acts = data["Statement"][1]["Action"]
if isinstance(acts, str):
    acts = [acts]
acts = list(acts) + ["s3:DeleteObject"]
data["Statement"][1]["Action"] = acts
p.write_text(json.dumps(data, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "restore DeleteObject is rejected" "1"
else
  assert "restore DeleteObject is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 5: restore policy with PutObject REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "iam-restore-policy.json"
data = json.loads(p.read_text())
acts = data["Statement"][1]["Action"]
if isinstance(acts, str):
    acts = [acts]
data["Statement"][1]["Action"] = list(acts) + ["s3:PutObject"]
p.write_text(json.dumps(data, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "restore PutObject is rejected" "1"
else
  assert "restore PutObject is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 6: versioning Suspended REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "bucket-versioning.json"
p.write_text(json.dumps({"Status": "Suspended"}, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "versioning Suspended is rejected" "1"
else
  assert "versioning Suspended is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 7: missing / empty encryption REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "bucket-encryption.json"
# Valid JSON but no SSE
p.write_text(json.dumps({"Rules": []}, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "empty encryption Rules is rejected" "1"
else
  assert "empty encryption Rules is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 8: encryption without SSE algorithm REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "bucket-encryption.json"
p.write_text(json.dumps({
  "Rules": [{"ApplyServerSideEncryptionByDefault": {"SSEAlgorithm": "NONE"}}]
}, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "invalid SSEAlgorithm is rejected" "1"
else
  assert "invalid SSEAlgorithm is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 9: lifecycle without db/ prefix expiration REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "bucket-lifecycle.json"
p.write_text(json.dumps({
  "Rules": [{
    "ID": "other",
    "Filter": {"Prefix": "logs/"},
    "Status": "Enabled",
    "Expiration": {"Days": 30}
  }]
}, indent=2) + "\n")
PY
if run_validator "${FIX}"; then
  assert "lifecycle missing db/ expiration is rejected" "1"
else
  assert "lifecycle missing db/ expiration is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 10: invalid JSON REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
echo "{not-json" > "${FIX}/bucket-versioning.json"
if run_validator "${FIX}"; then
  assert "invalid JSON is rejected" "1"
else
  assert "invalid JSON is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 11: stack without deletion retention is REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
if [ -f "${FIX}/backup-stack.json" ]; then
  python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "backup-stack.json"
data = json.loads(p.read_text())
data["Resources"]["BackupBucket"].pop("DeletionPolicy", None)
p.write_text(json.dumps(data, indent=2) + "\n")
PY
fi
if run_validator "${FIX}"; then
  assert "missing BackupBucket DeletionPolicy Retain is rejected" "1"
else
  assert "missing BackupBucket DeletionPolicy Retain is rejected" "0"
fi
rm -rf "${FIX}"

echo "=== Test 12: stack restore role with write access is REJECTED ==="
FIX="$(mktemp -d "${TMPDIR:-/tmp}/vbp-test.XXXXXX")"
copy_fixtures "${FIX}"
if [ -f "${FIX}/backup-stack.json" ]; then
  python3 - "${FIX}" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "backup-stack.json"
data = json.loads(p.read_text())
role = data["Resources"]["BackupRestoreRole"]
role["Properties"]["Policies"][0]["PolicyDocument"]["Statement"][1]["Action"].append("s3:PutObject")
p.write_text(json.dumps(data, indent=2) + "\n")
PY
fi
if run_validator "${FIX}"; then
  assert "restore role write access is rejected" "1"
else
  assert "restore role write access is rejected" "0"
fi
rm -rf "${FIX}"

rm -f /tmp/vbp-out.$$ /tmp/vbp-err.$$ 2>/dev/null || true

echo ""
if [ "${FAILURES}" -eq 0 ]; then
  echo "ALL TESTS PASSED"
  exit 0
fi
echo "FAILURES: ${FAILURES}"
exit 1

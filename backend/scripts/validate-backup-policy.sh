#!/bin/bash
# Static validation of infra/backup/*.json policy-as-code documents.
# Does NOT call AWS. Exit 0 if least-privilege + durability constraints hold;
# exit nonzero on drift (invalid JSON, missing SSE, versioning not Enabled,
# lifecycle not scoped to db/, IAM wildcards, restore writes, etc.).
#
# Usage:
#   bash backend/scripts/validate-backup-policy.sh
#   INFRA_BACKUP_DIR=/path/to/fixtures bash backend/scripts/validate-backup-policy.sh
#
# Requires: python3

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
POLICY_DIR="${INFRA_BACKUP_DIR:-${REPO_ROOT}/infra/backup}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "ERROR: python3 is required for validate-backup-policy.sh" >&2
  exit 1
fi

if [ ! -d "${POLICY_DIR}" ]; then
  echo "ERROR: policy directory not found: ${POLICY_DIR}" >&2
  exit 1
fi

export POLICY_DIR

# shellcheck disable=SC2016
python3 <<'PY'
import json
import os
import sys
from pathlib import Path

policy_dir = Path(os.environ["POLICY_DIR"])
errors = []

def fail(msg: str) -> None:
    errors.append(msg)

def load_json(path: Path):
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as e:
        fail(f"{path.name}: cannot read: {e}")
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError as e:
        fail(f"{path.name}: invalid JSON: {e}")
        return None

json_files = sorted(policy_dir.glob("*.json"))
if not json_files:
    fail(f"no *.json files under {policy_dir}")

# Every JSON file must parse.
docs = {}
for path in json_files:
    data = load_json(path)
    if data is not None:
        docs[path.name] = data

required = [
    "bucket-encryption.json",
    "bucket-versioning.json",
    "bucket-lifecycle.json",
    "bucket-logging.json",
    "object-lock.json",
    "iam-upload-policy.json",
    "iam-restore-policy.json",
    "backup-stack.json",
]
for name in required:
    if name not in docs:
        fail(f"missing required policy file: {name}")

# --- Encryption: SSE AES256 or aws:kms ---
enc = docs.get("bucket-encryption.json")
if enc is not None:
    rules = enc.get("Rules") if isinstance(enc, dict) else None
    if not isinstance(rules, list) or not rules:
        fail("bucket-encryption.json: Rules must be a non-empty list")
    else:
        found_sse = False
        for i, rule in enumerate(rules):
            if not isinstance(rule, dict):
                fail(f"bucket-encryption.json: Rules[{i}] must be an object")
                continue
            default = rule.get("ApplyServerSideEncryptionByDefault") or {}
            algo = default.get("SSEAlgorithm") if isinstance(default, dict) else None
            if algo in ("AES256", "aws:kms"):
                found_sse = True
            else:
                fail(
                    f"bucket-encryption.json: Rules[{i}] SSEAlgorithm must be "
                    f"AES256 or aws:kms (got {algo!r})"
                )
        if not found_sse:
            fail("bucket-encryption.json: no rule with SSE AES256 or aws:kms")

# --- Versioning: Status == Enabled ---
ver = docs.get("bucket-versioning.json")
if ver is not None:
    status = ver.get("Status") if isinstance(ver, dict) else None
    if status != "Enabled":
        fail(f"bucket-versioning.json: Status must be 'Enabled' (got {status!r})")

# --- Lifecycle: expiration rule scoped to db/ prefix ---
life = docs.get("bucket-lifecycle.json")
if life is not None:
    rules = life.get("Rules") if isinstance(life, dict) else None
    if not isinstance(rules, list) or not rules:
        fail("bucket-lifecycle.json: Rules must be a non-empty list")
    else:
        found_db_exp = False
        for i, rule in enumerate(rules):
            if not isinstance(rule, dict):
                continue
            filt = rule.get("Filter") or {}
            prefix = None
            if isinstance(filt, dict):
                prefix = filt.get("Prefix")
            # Legacy form: Prefix at rule root
            if prefix is None:
                prefix = rule.get("Prefix")
            has_expiration = isinstance(rule.get("Expiration"), dict) and (
                "Days" in rule["Expiration"] or "Date" in rule["Expiration"]
            )
            if prefix is not None and str(prefix).startswith("db") and has_expiration:
                # Accept "db" or "db/"
                if str(prefix) in ("db", "db/") or str(prefix).startswith("db/"):
                    found_db_exp = True
        if not found_db_exp:
            fail(
                "bucket-lifecycle.json: need an Expiration rule scoped to the "
                "db/ prefix (Filter.Prefix or Prefix starting with 'db')"
            )

# --- Logging: target bucket present (placeholder OK) ---
log = docs.get("bucket-logging.json")
if log is not None:
    enabled = log.get("LoggingEnabled") if isinstance(log, dict) else None
    if not isinstance(enabled, dict) or not enabled.get("TargetBucket"):
        fail("bucket-logging.json: LoggingEnabled.TargetBucket is required")

# --- Object lock (optional shape): if present, GOVERNANCE or COMPLIANCE ---
ol = docs.get("object-lock.json")
if ol is not None:
    if not isinstance(ol, dict):
        fail("object-lock.json: must be an object")
    else:
        rule = ol.get("Rule") or {}
        retention = rule.get("DefaultRetention") if isinstance(rule, dict) else None
        if isinstance(retention, dict):
            mode = retention.get("Mode")
            if mode not in ("GOVERNANCE", "COMPLIANCE"):
                fail(
                    f"object-lock.json: DefaultRetention.Mode must be "
                    f"GOVERNANCE or COMPLIANCE (got {mode!r})"
                )

WRITE_ACTIONS = {
    "s3:PutObject",
    "s3:PutObjectAcl",
    "s3:PutObjectVersionAcl",
    "s3:DeleteObject",
    "s3:DeleteObjectVersion",
    "s3:DeleteObjectTagging",
    "s3:DeleteObjectVersionTagging",
    "s3:AbortMultipartUpload",
    "s3:CreateMultipartUpload",
    "s3:UploadPart",
    "s3:UploadPartCopy",
    "s3:CompleteMultipartUpload",
    "s3:PutBucketPolicy",
    "s3:DeleteBucket",
    "s3:PutLifecycleConfiguration",
    "s3:PutBucketVersioning",
    "s3:PutEncryptionConfiguration",
    "s3:PutObjectLockConfiguration",
    "s3:RestoreObject",  # allowed for restore role? No — restore here means read dumps
}

def normalize_actions(action) -> list:
    if action is None:
        return []
    if isinstance(action, str):
        return [action]
    if isinstance(action, list):
        return [a for a in action if isinstance(a, str)]
    return []

def normalize_resources(resource) -> list:
    if resource is None:
        return []
    if isinstance(resource, str):
        return [resource]
    if isinstance(resource, list):
        return [r for r in resource if isinstance(r, str)]
    return []

def check_iam(name: str, data, *, restore: bool) -> None:
    if not isinstance(data, dict):
        fail(f"{name}: must be a JSON object")
        return
    statements = data.get("Statement")
    if not isinstance(statements, list) or not statements:
        fail(f"{name}: Statement must be a non-empty list")
        return

    saw_s3_arn = False
    all_actions = []

    for i, stmt in enumerate(statements):
        if not isinstance(stmt, dict):
            fail(f"{name}: Statement[{i}] must be an object")
            continue

        actions = normalize_actions(stmt.get("Action"))
        resources = normalize_resources(stmt.get("Resource"))
        all_actions.extend(actions)

        for a in actions:
            if a == "s3:*" or a == "*" or a.endswith(":*") and a.startswith("s3:"):
                # Reject s3:* and bare *; also s3:Something* wildcards that
                # are overly broad like s3:*Object — but specifically ban s3:*
                pass
            if a == "s3:*" or a == "*":
                fail(f"{name}: forbidden wildcard action {a!r} in Statement[{i}]")
            # Any action containing '*' is treated as over-privileged for these roles
            if "*" in a:
                fail(f"{name}: forbidden action pattern {a!r} in Statement[{i}]")

        for r in resources:
            if r == "*":
                fail(f'{name}: forbidden Resource "*" in Statement[{i}]')
            if r.startswith("arn:aws:s3:::"):
                saw_s3_arn = True
            elif r.startswith("arn:aws:s3:"):
                # regional form rare for S3 resources; still accept as s3 ARN family
                saw_s3_arn = True
            else:
                fail(
                    f"{name}: Resource {r!r} in Statement[{i}] must be an s3 ARN "
                    f"(arn:aws:s3:::...)"
                )

        if not resources:
            fail(f"{name}: Statement[{i}] missing Resource (must scope to s3 ARN)")

    if not saw_s3_arn:
        fail(f"{name}: at least one Statement Resource must be an arn:aws:s3::: ARN")

    if restore:
        for a in all_actions:
            # Normalize case for comparison
            al = a
            if al in WRITE_ACTIONS or al.lower().startswith("s3:put") or al.lower().startswith("s3:delete"):
                fail(f"{name}: restore policy must not allow write/delete action {a!r}")
            if al == "s3:PutObject" or al == "s3:DeleteObject":
                fail(f"{name}: restore policy must not allow {a!r}")

# --- IAM upload ---
up = docs.get("iam-upload-policy.json")
if up is not None:
    check_iam("iam-upload-policy.json", up, restore=False)
    # Upload must include PutObject, GetObject, ListBucket (least privilege set)
    if isinstance(up, dict):
        acts = []
        for stmt in up.get("Statement") or []:
            if isinstance(stmt, dict):
                acts.extend(normalize_actions(stmt.get("Action")))
        needed = {"s3:PutObject", "s3:GetObject", "s3:ListBucket"}
        missing = needed - set(acts)
        if missing:
            fail(f"iam-upload-policy.json: missing required actions {sorted(missing)}")
        # Upload must not allow delete
        for a in acts:
            if a.lower().startswith("s3:delete") or a == "s3:DeleteObject":
                fail(f"iam-upload-policy.json: must not allow delete action {a!r}")

# --- IAM restore ---
rp = docs.get("iam-restore-policy.json")
if rp is not None:
    check_iam("iam-restore-policy.json", rp, restore=True)
    if isinstance(rp, dict):
        acts = []
        for stmt in rp.get("Statement") or []:
            if isinstance(stmt, dict):
                acts.extend(normalize_actions(stmt.get("Action")))
        needed = {"s3:GetObject", "s3:ListBucket"}
        missing = needed - set(acts)
        if missing:
            fail(f"iam-restore-policy.json: missing required actions {sorted(missing)}")

# --- Provisionable CloudFormation stack ---
stack = docs.get("backup-stack.json")
if stack is not None:
    resources = stack.get("Resources") if isinstance(stack, dict) else None
    if not isinstance(resources, dict):
        fail("backup-stack.json: Resources must be an object")
        resources = {}

    def require_resource(name: str, resource_type: str):
        resource = resources.get(name)
        if not isinstance(resource, dict):
            fail(f"backup-stack.json: missing resource {name}")
            return {}
        if resource.get("Type") != resource_type:
            fail(
                f"backup-stack.json: {name}.Type must be {resource_type!r} "
                f"(got {resource.get('Type')!r})"
            )
        return resource

    backup_bucket = require_resource("BackupBucket", "AWS::S3::Bucket")
    log_bucket = require_resource("AccessLogBucket", "AWS::S3::Bucket")
    writer_role = require_resource("BackupWriterRole", "AWS::IAM::Role")
    restore_role = require_resource("BackupRestoreRole", "AWS::IAM::Role")
    require_resource("BackupBucketPolicy", "AWS::S3::BucketPolicy")
    require_resource("AccessLogBucketPolicy", "AWS::S3::BucketPolicy")

    for name, bucket in (("BackupBucket", backup_bucket), ("AccessLogBucket", log_bucket)):
        if bucket:
            if bucket.get("DeletionPolicy") != "Retain":
                fail(f"backup-stack.json: {name}.DeletionPolicy must be Retain")
            if bucket.get("UpdateReplacePolicy") != "Retain":
                fail(f"backup-stack.json: {name}.UpdateReplacePolicy must be Retain")
            props = bucket.get("Properties") or {}
            if (props.get("VersioningConfiguration") or {}).get("Status") != "Enabled":
                fail(f"backup-stack.json: {name} must enable versioning")
            enc_rules = (props.get("BucketEncryption") or {}).get(
                "ServerSideEncryptionConfiguration"
            ) or []
            algorithms = {
                (rule.get("ServerSideEncryptionByDefault") or {}).get("SSEAlgorithm")
                for rule in enc_rules
                if isinstance(rule, dict)
            }
            if not algorithms.intersection({"AES256", "aws:kms"}):
                fail(f"backup-stack.json: {name} must enable server-side encryption")
            pab = props.get("PublicAccessBlockConfiguration") or {}
            if not all(
                pab.get(flag) is True
                for flag in (
                    "BlockPublicAcls",
                    "BlockPublicPolicy",
                    "IgnorePublicAcls",
                    "RestrictPublicBuckets",
                )
            ):
                fail(f"backup-stack.json: {name} must block all public access")

    if backup_bucket:
        props = backup_bucket.get("Properties") or {}
        if props.get("ObjectLockEnabled") is not True:
            fail("backup-stack.json: BackupBucket.ObjectLockEnabled must be true")
        lock = props.get("ObjectLockConfiguration") or {}
        retention = ((lock.get("Rule") or {}).get("DefaultRetention") or {})
        if lock.get("ObjectLockEnabled") != "Enabled":
            fail("backup-stack.json: BackupBucket object lock configuration must be Enabled")
        if retention.get("Mode") not in ("GOVERNANCE", "COMPLIANCE"):
            fail("backup-stack.json: BackupBucket retention mode is required")
        if not isinstance(retention.get("Days"), int) or retention.get("Days", 0) < 1:
            fail("backup-stack.json: BackupBucket object lock retention Days must be positive")
        if not isinstance(props.get("LoggingConfiguration"), dict):
            fail("backup-stack.json: BackupBucket must enable access logging")
        lifecycle_rules = (props.get("LifecycleConfiguration") or {}).get("Rules") or []
        durable_rule = False
        for rule in lifecycle_rules:
            if not isinstance(rule, dict):
                continue
            noncurrent = rule.get("NoncurrentVersionExpiration") or {}
            if (
                rule.get("Status") == "Enabled"
                and rule.get("Prefix") == "db/"
                and isinstance(rule.get("ExpirationInDays"), int)
                and rule.get("ExpirationInDays", 0) > 0
                and isinstance(noncurrent.get("NoncurrentDays"), int)
                and noncurrent.get("NoncurrentDays", 0) > 0
            ):
                durable_rule = True
        if not durable_rule:
            fail(
                "backup-stack.json: BackupBucket needs enabled db/ current and "
                "noncurrent retention"
            )

    def role_actions(name: str, role) -> set:
        actions = set()
        policies = ((role.get("Properties") or {}).get("Policies") or []) if role else []
        if not policies:
            fail(f"backup-stack.json: {name} must have an inline least-privilege policy")
        for policy in policies:
            document = policy.get("PolicyDocument") if isinstance(policy, dict) else None
            for stmt in (document or {}).get("Statement") or []:
                if not isinstance(stmt, dict) or stmt.get("Effect") != "Allow":
                    continue
                actions.update(normalize_actions(stmt.get("Action")))
                resource_text = json.dumps(stmt.get("Resource"), sort_keys=True)
                if "BackupBucket" not in resource_text:
                    fail(
                        f"backup-stack.json: {name} allow statement must scope "
                        "resources to BackupBucket"
                    )
                if stmt.get("Action") in ("*", "s3:*"):
                    fail(f"backup-stack.json: {name} contains wildcard access")
        return actions

    writer_actions = role_actions("BackupWriterRole", writer_role)
    restore_actions = role_actions("BackupRestoreRole", restore_role)
    writer_needed = {"s3:ListBucket", "s3:PutObject", "s3:GetObject"}
    if not writer_needed.issubset(writer_actions):
        fail(
            "backup-stack.json: BackupWriterRole missing actions "
            f"{sorted(writer_needed - writer_actions)}"
        )
    for action in writer_actions:
        if "*" in action or action.lower().startswith("s3:delete"):
            fail(f"backup-stack.json: BackupWriterRole forbidden action {action!r}")
    restore_needed = {"s3:ListBucket", "s3:GetObject"}
    if not restore_needed.issubset(restore_actions):
        fail(
            "backup-stack.json: BackupRestoreRole missing actions "
            f"{sorted(restore_needed - restore_actions)}"
        )
    for action in restore_actions:
        if (
            "*" in action
            or action.lower().startswith("s3:put")
            or action.lower().startswith("s3:delete")
        ):
            fail(f"backup-stack.json: BackupRestoreRole forbidden action {action!r}")

if errors:
    print("validate-backup-policy: FAILED", file=sys.stderr)
    for e in errors:
        print(f"  - {e}", file=sys.stderr)
    sys.exit(1)

print(f"validate-backup-policy: OK ({len(docs)} json files under {policy_dir})")
sys.exit(0)
PY

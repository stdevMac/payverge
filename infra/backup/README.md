# Off-host backup bucket policy (policy-as-code)

Version-controlled, provisionable S3 backup infrastructure and least-privilege
IAM roles for Payverge database backups. `backup-stack.json` is the source of
truth for new AWS environments. The smaller JSON documents remain useful as
provider-neutral policy references and for checking an existing S3-compatible
bucket. No secrets live here.

## Provision with CloudFormation

`backup-stack.json` creates the backup bucket, a separate access-log bucket,
writer and read-only restore/monitor roles, and their policies. Both buckets use
`DeletionPolicy` and `UpdateReplacePolicy: Retain`; the backup bucket also has
versioning, encryption, 30/7-day lifecycle retention, seven-day GOVERNANCE
Object Lock, access logging, and public-access blocking.

```bash
aws cloudformation deploy \
  --stack-name payverge-backups \
  --template-file infra/backup/backup-stack.json \
  --capabilities CAPABILITY_IAM \
  --parameter-overrides \
    BackupBucketName=<BACKUP_BUCKET> \
    BackupWriterPrincipalArn=<UPLOAD_PRINCIPAL_ARN> \
    BackupReaderPrincipalArn=<RESTORE_PRINCIPAL_ARN>
```

Review a change set before execution in production. Stack deletion retains both
buckets; Object Lock prevents protected versions from being erased during their
retention window. CloudFormation outputs the bucket name and both role ARNs.

## Placeholders

| Placeholder | Meaning |
|-------------|---------|
| `<BACKUP_BUCKET>` | Destination bucket for `pg_dump` objects (e.g. `example-db-backups`) |
| `example-backup-access-logs` | Target bucket for S3 server access logs (create separately; may differ per env) |
| `<AWS_ACCOUNT_ID>` | 12-digit AWS account ID (for IAM attach / role ARNs, not in these JSON files) |
| `<UPLOAD_ROLE_NAME>` | IAM role used by `backup-db.sh` (Put + List + Get on `db/*`) |
| `<RESTORE_ROLE_NAME>` | IAM role used by freshness checks / restore drills (Get + List only) |

Before applying IAM policies, substitute `<BACKUP_BUCKET>` with the real bucket
name (e.g. `sed 's/<BACKUP_BUCKET>/example-db-backups/g'`).

## Pipeline context

| Script | S3 calls | Role |
|--------|----------|------|
| `backend/scripts/backup-db.sh` | `head-bucket`, `s3 cp` (PutObject), `head-object` (GetObject) | **upload** |
| `backend/scripts/check-backup-age.sh` | `head-object` | **restore** (or upload) |
| `backend/scripts/run-restore-drill.sh` | `s3 ls`, `s3 cp` (GetObject), `head-object` | **restore** |

Upload needs write + confirm-read; restore must never delete or overwrite
production backups. Neither role includes `s3:DeleteObject` / `s3:*`.

## Existing/provider-compatible bucket commands

Set the bucket once:

```bash
export BACKUP_BUCKET=example-db-backups   # replace per environment
cd infra/backup
```

### 1. Encryption (default AES256)

```bash
aws s3api put-bucket-encryption \
  --bucket "${BACKUP_BUCKET}" \
  --server-side-encryption-configuration file://bucket-encryption.json
```

**KMS alternative** (replace `bucket-encryption.json` content, or apply inline):

```json
{
  "Rules": [
    {
      "ApplyServerSideEncryptionByDefault": {
        "SSEAlgorithm": "aws:kms",
        "KMSMasterKeyID": "arn:aws:kms:<region>:<AWS_ACCOUNT_ID>:key/<KEY_ID>"
      },
      "BucketKeyEnabled": true
    }
  ]
}
```

```bash
aws s3api put-bucket-encryption \
  --bucket "${BACKUP_BUCKET}" \
  --server-side-encryption-configuration file://bucket-encryption-kms.json
```

Grant the upload/restore roles `kms:Encrypt` / `kms:Decrypt` /
`kms:GenerateDataKey` on that CMK if you switch to KMS. The committed default
stays AES256 (SSE-S3) so envs without a CMK remain valid.

### 2. Versioning (required durability baseline)

```bash
aws s3api put-bucket-versioning \
  --bucket "${BACKUP_BUCKET}" \
  --versioning-configuration file://bucket-versioning.json
```

### 3. Lifecycle (retention under `db/`)

```bash
aws s3api put-bucket-lifecycle-configuration \
  --bucket "${BACKUP_BUCKET}" \
  --lifecycle-configuration file://bucket-lifecycle.json
```

Committed defaults:

- Current object expiration: **30 days** under prefix `db/`
- Noncurrent version expiration: **7 days** (keeps versioning useful without
  unbounded storage after overwrite/delete)

Tune the day counts for compliance; keep the `db/` prefix scope so other
prefixes (if any) are not silently expired.

### 4. Server access logging

Create (or reuse) a log bucket first, e.g. `example-backup-access-logs`, with
its own encryption and a bucket policy allowing the logging service principal.
Then:

```bash
aws s3api put-bucket-logging \
  --bucket "${BACKUP_BUCKET}" \
  --bucket-logging-status file://bucket-logging.json
```

Edit `TargetBucket` / `TargetPrefix` in `bucket-logging.json` if your log
bucket name differs.

### 5. Object Lock

Object Lock must be enabled **at bucket creation** (or on an empty bucket that
was created with Object Lock). Versioning is a prerequisite.

```bash
# Only if the bucket was created with ObjectLockEnabled=true:
aws s3api put-object-lock-configuration \
  --bucket "${BACKUP_BUCKET}" \
  --object-lock-configuration file://object-lock.json
```

Committed default: **GOVERNANCE** mode, **7-day** default retention (overrideable
by accounts with `s3:BypassGovernanceRetention`). Prefer GOVERNANCE over
COMPLIANCE unless legal hold requires immutability even for root.

### 6. IAM — upload role (least privilege)

```bash
# Substitute placeholder, then create/update the managed policy:
sed 's/<BACKUP_BUCKET>/'"${BACKUP_BUCKET}"'/g' iam-upload-policy.json > /tmp/iam-upload-policy.json

aws iam create-policy \
  --policy-name PayvergeBackupUpload \
  --policy-document file:///tmp/iam-upload-policy.json
# or:
# aws iam create-policy-version --policy-arn arn:aws:iam::<AWS_ACCOUNT_ID>:policy/PayvergeBackupUpload \
#   --policy-document file:///tmp/iam-upload-policy.json --set-as-default

aws iam attach-role-policy \
  --role-name "<UPLOAD_ROLE_NAME>" \
  --policy-arn "arn:aws:iam::<AWS_ACCOUNT_ID>:policy/PayvergeBackupUpload"
```

Actions: `s3:PutObject`, `s3:GetObject` (for `head-object`), `s3:ListBucket`
(for `head-bucket` / prefix listing). **No delete.**

### 7. IAM — restore / drill role (read-only)

```bash
sed 's/<BACKUP_BUCKET>/'"${BACKUP_BUCKET}"'/g' iam-restore-policy.json > /tmp/iam-restore-policy.json

aws iam create-policy \
  --policy-name PayvergeBackupRestore \
  --policy-document file:///tmp/iam-restore-policy.json

aws iam attach-role-policy \
  --role-name "<RESTORE_ROLE_NAME>" \
  --policy-arn "arn:aws:iam::<AWS_ACCOUNT_ID>:policy/PayvergeBackupRestore"
```

Actions: `s3:GetObject`, `s3:ListBucket` only. **No Put/Delete/wildcards.**

## Validation (CI)

```bash
# Policy shape + least-privilege checks (no AWS calls):
bash backend/scripts/validate-backup-policy.sh

# Self-test including over-privileged fixture rejections:
bash backend/scripts/validate-backup-policy.test.sh
```

The `contracts` job in `.github/workflows/ci.yml` runs the test harness.

## Operator responsibilities

Retention periods, role separation between the upload and restore
credentials, bucket access logging, and break-glass restore access are
deployment policy. Record them alongside your own deployment; the scripts
above only assume the two-role split described in the IAM table.

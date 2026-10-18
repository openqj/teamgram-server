#!/usr/bin/env bash
# Copy Teamgram objects to an independently retained S3-compatible location.
# Credentials are passed only to the child process and never written to evidence.
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
evidence_path=${BACKUP_EVIDENCE_PATH:-"$repo_root/data/object-storage-backup-evidence.json"}
manifest_path=${BACKUP_MANIFEST_PATH:-"$repo_root/data/object-storage-backup.jsonl"}

: "${SOURCE_S3_ENDPOINT:?SOURCE_S3_ENDPOINT is required}"
: "${SOURCE_S3_ACCESS_KEY:?SOURCE_S3_ACCESS_KEY is required}"
: "${SOURCE_S3_SECRET_KEY:?SOURCE_S3_SECRET_KEY is required}"
: "${TARGET_S3_ENDPOINT:?TARGET_S3_ENDPOINT is required}"
: "${TARGET_S3_ACCESS_KEY:?TARGET_S3_ACCESS_KEY is required}"
: "${TARGET_S3_SECRET_KEY:?TARGET_S3_SECRET_KEY is required}"
: "${BACKUP_SOURCE_LOCATION_ID:?BACKUP_SOURCE_LOCATION_ID is required}"
: "${BACKUP_TARGET_LOCATION_ID:?BACKUP_TARGET_LOCATION_ID is required}"
: "${BACKUP_CONFIRM_SOURCE_QUIESCED:?Set BACKUP_CONFIRM_SOURCE_QUIESCED=YES after stopping writers}"
: "${BACKUP_SOURCE_ACCOUNT_ID:?BACKUP_SOURCE_ACCOUNT_ID is required}"
: "${BACKUP_TARGET_ACCOUNT_ID:?BACKUP_TARGET_ACCOUNT_ID is required}"

validate_id() {
  case "$2" in
    ""|*[!A-Za-z0-9._:-]*) echo "$1 must contain only letters, digits, dot, underscore, colon, or hyphen" >&2; exit 2 ;;
  esac
}
validate_id BACKUP_SOURCE_LOCATION_ID "$BACKUP_SOURCE_LOCATION_ID"
validate_id BACKUP_TARGET_LOCATION_ID "$BACKUP_TARGET_LOCATION_ID"
validate_id BACKUP_SOURCE_ACCOUNT_ID "$BACKUP_SOURCE_ACCOUNT_ID"
validate_id BACKUP_TARGET_ACCOUNT_ID "$BACKUP_TARGET_ACCOUNT_ID"

if [ "$BACKUP_CONFIRM_SOURCE_QUIESCED" != "YES" ]; then
  echo "backup requires BACKUP_CONFIRM_SOURCE_QUIESCED=YES after stopping Teamgram writers" >&2
  exit 2
fi
if [ "$SOURCE_S3_ENDPOINT" = "$TARGET_S3_ENDPOINT" ]; then
  echo "backup source and target endpoints must be different" >&2
  exit 2
fi
if [ "$BACKUP_SOURCE_LOCATION_ID" = "$BACKUP_TARGET_LOCATION_ID" ]; then
  echo "backup source and target location IDs must be different" >&2
  exit 2
fi
if [ "$BACKUP_SOURCE_ACCOUNT_ID" = "$BACKUP_TARGET_ACCOUNT_ID" ] && [ "${SOURCE_S3_REGION:-us-east-1}" = "${TARGET_S3_REGION:-us-east-1}" ]; then
  echo "backup account IDs and regions must identify an independent target" >&2
  exit 2
fi
if [ "${BACKUP_TARGET_INDEPENDENT:-}" != "true" ]; then
  echo "set BACKUP_TARGET_INDEPENDENT=true only after confirming a separate account or failure domain" >&2
  exit 2
fi

mkdir -p "$(dirname -- "$evidence_path")" "$(dirname -- "$manifest_path")"
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
log_path=$(mktemp "${TMPDIR:-/tmp}/teamgram-s3-backup.XXXXXX.log")

set +e
(
  cd "$repo_root"
  SOURCE_S3_ENDPOINT="$SOURCE_S3_ENDPOINT" \
  SOURCE_S3_ACCESS_KEY="$SOURCE_S3_ACCESS_KEY" \
  SOURCE_S3_SECRET_KEY="$SOURCE_S3_SECRET_KEY" \
  SOURCE_S3_USE_SSL="${SOURCE_S3_USE_SSL:-false}" \
  SOURCE_S3_REGION="${SOURCE_S3_REGION:-us-east-1}" \
  TARGET_S3_ENDPOINT="$TARGET_S3_ENDPOINT" \
  TARGET_S3_ACCESS_KEY="$TARGET_S3_ACCESS_KEY" \
  TARGET_S3_SECRET_KEY="$TARGET_S3_SECRET_KEY" \
  TARGET_S3_USE_SSL="${TARGET_S3_USE_SSL:-true}" \
  TARGET_S3_REGION="${TARGET_S3_REGION:-us-east-1}" \
  MIGRATION_MODE=copy \
  MIGRATION_CONFIRM_SOURCE_QUIESCED=YES \
  MIGRATION_BUCKETS="${MIGRATION_BUCKETS:-documents,encryptedfiles,photos,videos}" \
  MIGRATION_MANIFEST="$manifest_path" \
  go run ./tools/object-storage-migrate
) >"$log_path" 2>&1
status=$?
set -e

finished_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
manifest_exists=false
if [ -s "$manifest_path" ]; then
  manifest_exists=true
fi
cat >"$evidence_path" <<EOF
{
  "schema": "teamgram.object-storage-backup.v1",
  "operation": "copy",
  "status": "$([ "$status" -eq 0 ] && printf success || printf failed)",
  "started_at": "$started_at",
  "finished_at": "$finished_at",
  "source_location_id": "$BACKUP_SOURCE_LOCATION_ID",
  "target_location_id": "$BACKUP_TARGET_LOCATION_ID",
  "source_endpoint": "$SOURCE_S3_ENDPOINT",
  "target_endpoint": "$TARGET_S3_ENDPOINT",
  "manifest_path": "$manifest_path",
  "manifest_nonempty": $manifest_exists,
  "credentials_included": false
}
EOF

if [ "$status" -ne 0 ]; then
  echo "object-storage backup failed; inspect the local redacted log at $log_path" >&2
  exit "$status"
fi
rm -f "$log_path"
echo "object-storage backup completed; evidence=$evidence_path"

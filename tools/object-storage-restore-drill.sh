#!/usr/bin/env bash
# Restore an object backup into an explicitly disposable empty target.
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
evidence_path=${RESTORE_EVIDENCE_PATH:-"$repo_root/data/object-storage-restore-evidence.json"}
manifest_path=${RESTORE_MANIFEST_PATH:-"$repo_root/data/object-storage-restore.jsonl"}

: "${SOURCE_S3_ENDPOINT:?SOURCE_S3_ENDPOINT is required}"
: "${SOURCE_S3_ACCESS_KEY:?SOURCE_S3_ACCESS_KEY is required}"
: "${SOURCE_S3_SECRET_KEY:?SOURCE_S3_SECRET_KEY is required}"
: "${TARGET_S3_ENDPOINT:?TARGET_S3_ENDPOINT is required}"
: "${TARGET_S3_ACCESS_KEY:?TARGET_S3_ACCESS_KEY is required}"
: "${TARGET_S3_SECRET_KEY:?TARGET_S3_SECRET_KEY is required}"
: "${RESTORE_SOURCE_LOCATION_ID:?RESTORE_SOURCE_LOCATION_ID is required}"
: "${RESTORE_TARGET_LOCATION_ID:?RESTORE_TARGET_LOCATION_ID is required}"
: "${RESTORE_TARGET_DISPOSABLE:?Set RESTORE_TARGET_DISPOSABLE=YES for a disposable restore target}"
: "${RESTORE_CONFIRM_SOURCE_QUIESCED:?Set RESTORE_CONFIRM_SOURCE_QUIESCED=YES for a quiesced backup source}"
: "${RESTORE_SOURCE_ACCOUNT_ID:?RESTORE_SOURCE_ACCOUNT_ID is required}"
: "${RESTORE_TARGET_ACCOUNT_ID:?RESTORE_TARGET_ACCOUNT_ID is required}"

validate_id() {
  case "$2" in
    ""|*[!A-Za-z0-9._:-]*) echo "$1 must contain only letters, digits, dot, underscore, colon, or hyphen" >&2; exit 2 ;;
  esac
}
validate_id RESTORE_SOURCE_LOCATION_ID "$RESTORE_SOURCE_LOCATION_ID"
validate_id RESTORE_TARGET_LOCATION_ID "$RESTORE_TARGET_LOCATION_ID"
validate_id RESTORE_SOURCE_ACCOUNT_ID "$RESTORE_SOURCE_ACCOUNT_ID"
validate_id RESTORE_TARGET_ACCOUNT_ID "$RESTORE_TARGET_ACCOUNT_ID"

if [ "$RESTORE_TARGET_DISPOSABLE" != "YES" ]; then
  echo "restore target must be explicitly disposable: RESTORE_TARGET_DISPOSABLE=YES" >&2
  exit 2
fi
if [ "$RESTORE_CONFIRM_SOURCE_QUIESCED" != "YES" ]; then
  echo "restore requires RESTORE_CONFIRM_SOURCE_QUIESCED=YES" >&2
  exit 2
fi
if [ "$SOURCE_S3_ENDPOINT" = "$TARGET_S3_ENDPOINT" ]; then
  echo "restore source and target endpoints must be different" >&2
  exit 2
fi
if [ "$RESTORE_SOURCE_LOCATION_ID" = "$RESTORE_TARGET_LOCATION_ID" ]; then
  echo "restore source and target location IDs must be different" >&2
  exit 2
fi
if [ "$RESTORE_SOURCE_ACCOUNT_ID" = "$RESTORE_TARGET_ACCOUNT_ID" ] && [ "${SOURCE_S3_REGION:-us-east-1}" = "${TARGET_S3_REGION:-us-east-1}" ]; then
  echo "restore account IDs and regions must identify an independent target" >&2
  exit 2
fi
if [ "${RESTORE_TARGET_INDEPENDENT:-}" != "true" ]; then
  echo "set RESTORE_TARGET_INDEPENDENT=true only after confirming an independent restore account or region" >&2
  exit 2
fi

mkdir -p "$(dirname -- "$evidence_path")" "$(dirname -- "$manifest_path")"
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
log_path=$(mktemp "${TMPDIR:-/tmp}/teamgram-s3-restore.XXXXXX.log")

set +e
(
  cd "$repo_root"
  SOURCE_S3_ENDPOINT="$SOURCE_S3_ENDPOINT" \
  SOURCE_S3_ACCESS_KEY="$SOURCE_S3_ACCESS_KEY" \
  SOURCE_S3_SECRET_KEY="$SOURCE_S3_SECRET_KEY" \
  SOURCE_S3_USE_SSL="${SOURCE_S3_USE_SSL:-true}" \
  SOURCE_S3_REGION="${SOURCE_S3_REGION:-us-east-1}" \
  TARGET_S3_ENDPOINT="$TARGET_S3_ENDPOINT" \
  TARGET_S3_ACCESS_KEY="$TARGET_S3_ACCESS_KEY" \
  TARGET_S3_SECRET_KEY="$TARGET_S3_SECRET_KEY" \
  TARGET_S3_USE_SSL="${TARGET_S3_USE_SSL:-false}" \
  TARGET_S3_REGION="${TARGET_S3_REGION:-us-east-1}" \
  MIGRATION_MODE=copy \
  MIGRATION_CONFIRM_SOURCE_QUIESCED=YES \
  MIGRATION_REQUIRE_EMPTY_TARGET=true \
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
  "schema": "teamgram.object-storage-restore.v1",
  "operation": "restore-drill",
  "status": "$([ "$status" -eq 0 ] && printf success || printf failed)",
  "started_at": "$started_at",
  "finished_at": "$finished_at",
  "source_location_id": "$RESTORE_SOURCE_LOCATION_ID",
  "target_location_id": "$RESTORE_TARGET_LOCATION_ID",
  "source_endpoint": "$SOURCE_S3_ENDPOINT",
  "target_endpoint": "$TARGET_S3_ENDPOINT",
  "target_disposable": true,
  "target_required_empty": true,
  "manifest_path": "$manifest_path",
  "manifest_nonempty": $manifest_exists,
  "credentials_included": false
}
EOF

if [ "$status" -ne 0 ]; then
  echo "object-storage restore drill failed; inspect the local redacted log at $log_path" >&2
  exit "$status"
fi
rm -f "$log_path"
echo "object-storage restore drill completed; evidence=$evidence_path"

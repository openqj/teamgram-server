#!/usr/bin/env bash
set -Eeuo pipefail

: "${DATABASE_URL:?DATABASE_URL must be set}"
: "${BUSINESS_OPERATION:?BUSINESS_OPERATION must be set}"
: "${BUSINESS_SCOPE_KEY:?BUSINESS_SCOPE_KEY must be set}"
: "${BUSINESS_RESULT_FILE:?BUSINESS_RESULT_FILE must be set}"

if [[ ! -f "$BUSINESS_RESULT_FILE" ]]; then
  printf 'business result file does not exist: %s\n' "$BUSINESS_RESULT_FILE" >&2
  exit 1
fi
if [[ ! "$BUSINESS_OPERATION" =~ ^[A-Za-z0-9._-]{1,128}$ ]]; then
  printf 'invalid BUSINESS_OPERATION\n' >&2
  exit 1
fi
if [[ ${#BUSINESS_SCOPE_KEY} -lt 1 || ${#BUSINESS_SCOPE_KEY} -gt 2048 ]]; then
  printf 'invalid BUSINESS_SCOPE_KEY length\n' >&2
  exit 1
fi

result_json=$(<"$BUSINESS_RESULT_FILE")
caller_user_id=${BUSINESS_CALLER_USER_ID:-0}
bot_user_id=${BUSINESS_BOT_USER_ID:-0}
query_id=${BUSINESS_QUERY_ID:-0}
state=${BUSINESS_STATE:-published}
expires_at=${BUSINESS_EXPIRES_AT:-}

[[ "$caller_user_id" =~ ^[0-9]+$ ]] || { printf 'invalid BUSINESS_CALLER_USER_ID\n' >&2; exit 1; }
[[ "$bot_user_id" =~ ^[0-9]+$ ]] || { printf 'invalid BUSINESS_BOT_USER_ID\n' >&2; exit 1; }
[[ "$query_id" =~ ^[0-9]+$ ]] || { printf 'invalid BUSINESS_QUERY_ID\n' >&2; exit 1; }
[[ "$state" == published || "$state" == revoked ]] || { printf 'invalid BUSINESS_STATE\n' >&2; exit 1; }

psql_bin=${PSQL:-psql}
"$psql_bin" "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --set=business_operation="$BUSINESS_OPERATION" \
  --set=business_scope_key="$BUSINESS_SCOPE_KEY" \
  --set=business_caller_user_id="$caller_user_id" \
  --set=business_bot_user_id="$bot_user_id" \
  --set=business_query_id="$query_id" \
  --set=business_state="$state" \
  --set=business_result_json="$result_json" \
  --set=business_expires_at="$expires_at" <<'SQL'
BEGIN;
SELECT pg_advisory_xact_lock(hashtextextended('teamgram-business-control-plane', 0));
INSERT INTO apifull_business_control_plane
  (operation, scope_key, caller_user_id, bot_user_id, query_id, state, result, expires_at, updated_at)
VALUES (
  :'business_operation',
  :'business_scope_key',
  (:'business_caller_user_id')::bigint,
  NULLIF((:'business_bot_user_id')::bigint, 0),
  NULLIF((:'business_query_id')::bigint, 0),
  :'business_state',
  (:'business_result_json')::jsonb,
  NULLIF(:'business_expires_at', '')::timestamptz,
  CURRENT_TIMESTAMP
)
ON CONFLICT (operation, scope_key, caller_user_id) DO UPDATE SET
  bot_user_id = EXCLUDED.bot_user_id,
  query_id = EXCLUDED.query_id,
  state = EXCLUDED.state,
  result = EXCLUDED.result,
  expires_at = EXCLUDED.expires_at,
  updated_at = CURRENT_TIMESTAMP;
COMMIT;
SQL

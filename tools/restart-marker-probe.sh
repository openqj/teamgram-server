#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo 'usage: restart-marker-probe.sh before|after|cleanup MARKER' >&2
  exit 2
fi

PHASE=$1
MARKER=$2
case "$MARKER" in
  ''|*[!A-Za-z0-9_-]*)
    echo 'marker contains unsupported characters' >&2
    exit 2
    ;;
esac

run_sql() {
  printf '%s\n' "$1" | docker exec -i teamgram-postgres psql -U teamgram -d teamgram --no-psqlrc --set=ON_ERROR_STOP=1
}

case "$PHASE" in
  before)
    run_sql "INSERT INTO apifull_webview_request (request_id,user_id,bot_id,kind,payload,expires_at,created_at,updated_at) VALUES ('$MARKER',2,900000000001,'requestWebView',jsonb_build_object('tag','$MARKER','kind','backend_restart'),CURRENT_TIMESTAMP + INTERVAL '1 day',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) ON CONFLICT (request_id) DO UPDATE SET payload=EXCLUDED.payload,expires_at=EXCLUDED.expires_at,updated_at=CURRENT_TIMESTAMP;"
    ;;
  after)
    count=$(printf '%s\n' "SELECT count(*) FROM apifull_webview_request WHERE request_id='$MARKER' AND user_id=2 AND bot_id=900000000001 AND kind='requestWebView' AND payload->>'tag'='$MARKER' AND expires_at > CURRENT_TIMESTAMP;" | docker exec -i teamgram-postgres psql -U teamgram -d teamgram --no-psqlrc --tuples-only --no-align)
    test "$(printf '%s' "$count" | tr -d '[:space:]')" = 1
    ;;
  cleanup)
    run_sql "DELETE FROM apifull_webview_request WHERE request_id='$MARKER';"
    ;;
  *)
    echo "unknown phase: $PHASE" >&2
    exit 2
    ;;
esac

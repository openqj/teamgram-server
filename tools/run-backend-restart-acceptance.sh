#!/usr/bin/env bash
set -euo pipefail

if [[ ${RUN_RESTART_PROBE:-0} != 1 ]]; then
  echo 'Set RUN_RESTART_PROBE=1 to restart the existing backend service.' >&2
  exit 2
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
COMPOSE_PROJECT=${ACCEPTANCE_COMPOSE_PROJECT:-backend}
COMPOSE_DIRECTORY=${ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY:-/Users/mac/open/tg/services/backend}
OUTPUT=${1:-/tmp/teamgram-backend-restart-acceptance.json}

COMPOSE_FILES=(
  /Users/mac/open/tg/services/backend/docker-compose.yaml
  "$ROOT/docker/production/backend-deploy-r29-prod.yaml"
  /tmp/teamgram-production-runtime.yaml
  "$ROOT/docker/production/backend-e2e-provider-fixtures.yaml"
  /tmp/teamgram-passkey-image-userhandle.yaml
)

get_backend_env() {
  docker inspect backend-backend-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
    | awk -F= -v key="$1" '$1 == key {sub(/^[^=]*=/, ""); print; exit}'
}

export TEAMGRAM_BACKEND_IMAGE="$(docker inspect backend-backend-1 --format '{{.Config.Image}}')"
export TEAMGRAM_POSTGRES_DSN=${TEAMGRAM_POSTGRES_DSN:-postgres://teamgram:teamgram@postgres:5432/teamgram?sslmode=disable}
export TEAMGRAM_CODE_PROVIDER_KEY="$(get_backend_env TEAMGRAM_CODE_KEY)"
export TEAMGRAM_CODE_PROVIDER_SECRET="$(get_backend_env TEAMGRAM_CODE_SECRET)"
export TEAMGRAM_CODE_CHALLENGE_SECRET="$(get_backend_env TEAMGRAM_CODE_CHALLENGE_SECRET)"
export TEAMGRAM_CODE_PROVIDER_IMAGE=${TEAMGRAM_CODE_PROVIDER_IMAGE:-teamgram-code-provider-fixture:acceptance}
export POSTGRES_DB=${POSTGRES_DB:-teamgram}
export POSTGRES_USER=${POSTGRES_USER:-teamgram}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-teamgram}
export TEAMGRAM_DEPLOY_DIR=${TEAMGRAM_DEPLOY_DIR:-$ROOT/teamgramd/deploy}

RESTART_MARKER="backend-restart-$(date +%s)-$$"

# Use stdin rather than psql -c: the PostgreSQL client in the fixture image
# expands SQL variables only for stdin scripts. Build the shell snippets with
# printf's %q so command substitution is deferred until restart_probe executes
# the after-check, instead of running while this wrapper is parsed.
before_sql="INSERT INTO apifull_webview_request (request_id,user_id,bot_id,kind,payload,expires_at,created_at,updated_at) VALUES ('$RESTART_MARKER',2,900000000001,'requestWebView',jsonb_build_object('tag','$RESTART_MARKER','kind','backend_restart'),CURRENT_TIMESTAMP + INTERVAL '1 day',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) ON CONFLICT (request_id) DO UPDATE SET payload=EXCLUDED.payload,expires_at=EXCLUDED.expires_at,updated_at=CURRENT_TIMESTAMP;"
after_sql="SELECT count(*) FROM apifull_webview_request WHERE request_id='$RESTART_MARKER' AND user_id=2 AND bot_id=900000000001 AND kind='requestWebView' AND payload->>'tag'='$RESTART_MARKER' AND expires_at > CURRENT_TIMESTAMP;"
cleanup_sql="DELETE FROM apifull_webview_request WHERE request_id='$RESTART_MARKER';"
printf -v BEFORE_COMMAND "printf %%s %q | docker exec -i teamgram-postgres psql -U teamgram -d teamgram --no-psqlrc --set=ON_ERROR_STOP=1" "$before_sql"
printf -v AFTER_COMMAND 'count=$(printf %%s %q | docker exec -i teamgram-postgres psql -U teamgram -d teamgram --no-psqlrc --tuples-only --no-align); test "$count" = 1' "$after_sql"
printf -v CLEANUP_COMMAND "printf %%s %q | docker exec -i teamgram-postgres psql -U teamgram -d teamgram --no-psqlrc --set=ON_ERROR_STOP=1" "$cleanup_sql"

ARGS=(
  --service backend
  --project-name "$COMPOSE_PROJECT"
  --project-directory "$COMPOSE_DIRECTORY"
  --before-command "$BEFORE_COMMAND"
  --after-command "$AFTER_COMMAND"
  --cleanup-command "$CLEANUP_COMMAND"
  --wait-seconds "${RESTART_WAIT_SECONDS:-120}"
  --poll-seconds "${RESTART_POLL_SECONDS:-2}"
  --timeout "${RESTART_TIMEOUT_SECONDS:-300}"
  --execute
  --output "$OUTPUT"
)
for compose_file in "${COMPOSE_FILES[@]}"; do
  ARGS+=(--compose-file "$compose_file")
done

python3 "$ROOT/tools/restart_probe.py" "${ARGS[@]}"

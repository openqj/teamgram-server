#!/usr/bin/env bash
set -euo pipefail

KIND=${1:-${RESTART_PROBE_KIND:-}}
if [[ $# -gt 1 || ! "$KIND" =~ ^(channel|settings|business)$ ]]; then
  echo 'usage: RUN_RESTART_PROBE=1 run-backend-restart-critical.sh channel|settings|business' >&2
  exit 2
fi
if [[ ${RUN_RESTART_PROBE:-0} != 1 ]]; then
  echo 'Set RUN_RESTART_PROBE=1 to restart the existing backend service.' >&2
  exit 2
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
MARKER=${PROBE_TAG:-backendrestart-$(date +%s)-$$}
case "$MARKER" in
  ''|*[!A-Za-z0-9_-]*)
    echo 'PROBE_TAG contains unsupported characters' >&2
    exit 2
    ;;
esac

GRAMJS_DIR=${TEAMGRAM_GRAMJS_DIR:-$ROOT/../telegram-tt-master}
TSX="$GRAMJS_DIR/node_modules/.bin/tsx"
PROBE="$ROOT/docker/production/probe-backend-restart.ts"
STATE_FILE=${PROBE_STATE_FILE:-/tmp/teamgram-backend-restart-state-${KIND}-${MARKER}.json}
[[ -x "$TSX" ]] || { echo "tsx executable not found: $TSX" >&2; exit 2; }
[[ -f "$PROBE" ]] || { echo "probe script not found: $PROBE" >&2; exit 2; }

export TEAMGRAM_GRAMJS_DIR="$GRAMJS_DIR"
export TG_DC_HOST=${TG_DC_HOST:-127.0.0.1}
export TG_DC_PORT=${TG_DC_PORT:-11443}
export ACCEPTANCE_SERVICE=${ACCEPTANCE_SERVICE:-backend}
export ACCEPTANCE_COMPOSE_PROJECT=${ACCEPTANCE_COMPOSE_PROJECT:-backend}
export ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY=${ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY:-/Users/mac/open/tg/services/backend}
export TEAMGRAM_BACKEND_IMAGE=${TEAMGRAM_BACKEND_IMAGE:-teamgram-server-e2e-premium:20261014-mq-fix}
[[ -n "$TEAMGRAM_BACKEND_IMAGE" ]] || { echo 'TEAMGRAM_BACKEND_IMAGE must be non-empty' >&2; exit 2; }

# Preserve the provider fixture settings from the running backend when the
# caller has not supplied them. The backend image is pinned above so a stale
# running container cannot silently select an older acceptance image.
if docker inspect backend-backend-1 >/dev/null 2>&1; then
  get_backend_env() {
    docker inspect backend-backend-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
      | awk -F= -v key="$1" '$1 == key {sub(/^[^=]*=/, ""); print; exit}'
  }
  export TEAMGRAM_CODE_PROVIDER_KEY=${TEAMGRAM_CODE_PROVIDER_KEY:-$(get_backend_env TEAMGRAM_CODE_KEY)}
  export TEAMGRAM_CODE_PROVIDER_SECRET=${TEAMGRAM_CODE_PROVIDER_SECRET:-$(get_backend_env TEAMGRAM_CODE_SECRET)}
  export TEAMGRAM_CODE_CHALLENGE_SECRET=${TEAMGRAM_CODE_CHALLENGE_SECRET:-$(get_backend_env TEAMGRAM_CODE_CHALLENGE_SECRET)}
fi
export TEAMGRAM_POSTGRES_DSN=${TEAMGRAM_POSTGRES_DSN:-postgres://teamgram:teamgram@postgres:5432/teamgram?sslmode=disable}
export TEAMGRAM_CODE_PROVIDER_IMAGE=${TEAMGRAM_CODE_PROVIDER_IMAGE:-teamgram-code-provider-fixture:acceptance}
export POSTGRES_DB=${POSTGRES_DB:-teamgram}
export POSTGRES_USER=${POSTGRES_USER:-teamgram}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-teamgram}
export TEAMGRAM_DEPLOY_DIR=${TEAMGRAM_DEPLOY_DIR:-$ROOT/teamgramd/deploy}

if [[ -n ${ACCEPTANCE_COMPOSE_FILES:-} ]]; then
  read -r -a COMPOSE_FILES <<< "$ACCEPTANCE_COMPOSE_FILES"
elif [[ -n ${ACCEPTANCE_COMPOSE_FILE:-} ]]; then
  COMPOSE_FILES=("$ACCEPTANCE_COMPOSE_FILE")
else
  COMPOSE_FILES=(
    /Users/mac/open/tg/services/backend/docker-compose.yaml
    "$ROOT/docker/production/backend-deploy-r29-prod.yaml"
    /tmp/teamgram-production-runtime.yaml
    "$ROOT/docker/production/backend-e2e-provider-fixtures.yaml"
    /tmp/teamgram-passkey-image-userhandle.yaml
    "$ROOT/docker/production/backend-acceptance-mq-fix.yaml"
  )
fi
for compose_file in "${COMPOSE_FILES[@]}"; do
  [[ -f "$compose_file" ]] || { echo "Compose file not found: $compose_file" >&2; exit 2; }
done

probe_command() {
  local phase=$1
  printf 'env RESTART_PROBE_KIND=%q PROBE_PHASE=%q PROBE_TAG=%q PROBE_STATE_FILE=%q TEAMGRAM_GRAMJS_DIR=%q TG_DC_HOST=%q TG_DC_PORT=%q %q %q' \
    "$KIND" "$phase" "$MARKER" "$STATE_FILE" "$GRAMJS_DIR" "$TG_DC_HOST" "$TG_DC_PORT" "$TSX" "$PROBE"
}

BEFORE_COMMAND=$(probe_command before)
AFTER_COMMAND=$(probe_command after)
CLEANUP_COMMAND=$(probe_command cleanup)
if [[ "$KIND" == business ]]; then
  BUSINESS_CLEANUP_SQL="BEGIN;
DELETE FROM public.messages WHERE message = '${MARKER}:business:after';
COMMIT;
"
  printf -v BUSINESS_SQL_COMMAND 'printf %%s %q | docker exec -i teamgram-postgres psql -U %q -d %q --no-psqlrc --set=ON_ERROR_STOP=1' \
    "$BUSINESS_CLEANUP_SQL" "$POSTGRES_USER" "$POSTGRES_DB"
  CLEANUP_COMMAND+="; probe_status=\$?; ${BUSINESS_SQL_COMMAND}; sql_status=\$?; if [ \"\$probe_status\" -ne 0 ]; then exit \"\$probe_status\"; fi; exit \"\$sql_status\""
fi
OUTPUT=${RESTART_OUTPUT:-/tmp/teamgram-backend-restart-${KIND}-${MARKER}.json}

ARGS=(
  --service "$ACCEPTANCE_SERVICE"
  --expected-image "$TEAMGRAM_BACKEND_IMAGE"
  --project-name "$ACCEPTANCE_COMPOSE_PROJECT"
  --project-directory "$ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY"
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

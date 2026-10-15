#!/usr/bin/env bash
set -euo pipefail

if [[ ${RUN_RESTART_PROBE:-0} != 1 ]]; then
  echo 'Set RUN_RESTART_PROBE=1 to restart the existing backend service.' >&2
  exit 2
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
GRAMJS_DIR=${TEAMGRAM_GRAMJS_DIR:-$ROOT/../telegram-tt-master}
TSX="$GRAMJS_DIR/node_modules/.bin/tsx"
PROBE="$ROOT/docker/production/probe-bot-mini-restart.ts"
MARKER=${PROBE_TAG:-botbackendrestart-$(date +%s)-$$}
STATE_FILE=${PROBE_STATE_FILE:-/tmp/teamgram-bot-mini-restart-${MARKER}.json}
OUTPUT=${RESTART_OUTPUT:-/tmp/teamgram-backend-restart-bot-mini-${MARKER}.json}

case "$MARKER" in
  ''|*[!A-Za-z0-9_-]*)
    echo 'PROBE_TAG contains unsupported characters' >&2
    exit 2
    ;;
esac
[[ -x "$TSX" ]] || { echo "tsx executable not found: $TSX" >&2; exit 2; }
[[ -f "$PROBE" ]] || { echo "probe script not found: $PROBE" >&2; exit 2; }

export TEAMGRAM_GRAMJS_DIR="$GRAMJS_DIR"
export TG_DC_HOST=${TG_DC_HOST:-127.0.0.1}
export TG_DC_PORT=${TG_DC_PORT:-11443}
export ACCEPTANCE_SERVICE=${ACCEPTANCE_SERVICE:-backend}
export ACCEPTANCE_COMPOSE_PROJECT=${ACCEPTANCE_COMPOSE_PROJECT:-backend}
export ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY=${ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY:-/Users/mac/open/tg/services/backend}
export TEAMGRAM_POSTGRES_DSN=${TEAMGRAM_POSTGRES_DSN:-postgres://teamgram:teamgram@postgres:5432/teamgram?sslmode=disable}
export TEAMGRAM_CODE_PROVIDER_IMAGE=${TEAMGRAM_CODE_PROVIDER_IMAGE:-teamgram-code-provider-fixture:acceptance}
export POSTGRES_DB=${POSTGRES_DB:-teamgram}
export POSTGRES_USER=${POSTGRES_USER:-teamgram}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-teamgram}
export TEAMGRAM_DEPLOY_DIR=${TEAMGRAM_DEPLOY_DIR:-$ROOT/teamgramd/deploy}
export TEAMGRAM_BACKEND_IMAGE=${TEAMGRAM_BACKEND_IMAGE:-teamgram-server-e2e-premium:20261014-mq-fix}
[[ -n "$TEAMGRAM_BACKEND_IMAGE" ]] || { echo 'TEAMGRAM_BACKEND_IMAGE must be non-empty' >&2; exit 2; }

if docker inspect backend-backend-1 >/dev/null 2>&1; then
  get_backend_env() {
    docker inspect backend-backend-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
      | awk -F= -v key="$1" '$1 == key {sub(/^[^=]*=/, ""); print; exit}'
  }
  export TEAMGRAM_CODE_PROVIDER_KEY=${TEAMGRAM_CODE_PROVIDER_KEY:-$(get_backend_env TEAMGRAM_CODE_KEY)}
  export TEAMGRAM_CODE_PROVIDER_SECRET=${TEAMGRAM_CODE_PROVIDER_SECRET:-$(get_backend_env TEAMGRAM_CODE_SECRET)}
  export TEAMGRAM_CODE_CHALLENGE_SECRET=${TEAMGRAM_CODE_CHALLENGE_SECRET:-$(get_backend_env TEAMGRAM_CODE_CHALLENGE_SECRET)}
fi

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
  printf 'env PROBE_PHASE=%q PROBE_TAG=%q PROBE_STATE_FILE=%q TEAMGRAM_GRAMJS_DIR=%q TG_DC_HOST=%q TG_DC_PORT=%q %q %q' \
    "$phase" "$MARKER" "$STATE_FILE" "$GRAMJS_DIR" "$TG_DC_HOST" "$TG_DC_PORT" "$TSX" "$PROBE"
}

ARGS=(
  --service "$ACCEPTANCE_SERVICE"
  --expected-image "$TEAMGRAM_BACKEND_IMAGE"
  --project-name "$ACCEPTANCE_COMPOSE_PROJECT"
  --project-directory "$ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY"
  --before-command "$(probe_command before)"
  --after-command "$(probe_command after)"
  --cleanup-command "$(probe_command cleanup)"
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

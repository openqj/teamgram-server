#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
server_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)
default_gramjs_dir=$(CDPATH= cd -- "$server_dir/.." && pwd)/telegram-tt-master
gramjs_dir=${TEAMGRAM_GRAMJS_DIR:-$default_gramjs_dir}
export TEAMGRAM_GRAMJS_DIR="$gramjs_dir"

if [[ -n ${TG_DC_HOST:-} && $TG_DC_HOST != "127.0.0.1" ]]; then
  printf 'TG_DC_HOST must be 127.0.0.1 for the isolated WebSocket business-flow check\n' >&2
  exit 2
fi
if [[ -n ${TG_DC_PORT:-} && $TG_DC_PORT != "31443" ]]; then
  printf 'TG_DC_PORT must be 31443 for the isolated WebSocket business-flow check\n' >&2
  exit 2
fi
export TG_DC_HOST=127.0.0.1
export TG_DC_PORT=31443

tsx_bin="$gramjs_dir/node_modules/.bin/tsx"
if [[ ! -x $tsx_bin || ! -f $gramjs_dir/src/lib/gramjs/client/TelegramClient.ts ]]; then
  printf 'TEAMGRAM_GRAMJS_DIR must point to the Web workspace with GramJS dependencies\n' >&2
  exit 2
fi

gateway_count=0
while IFS='|' read -r _container ports project; do
  [[ $project =~ ^teamgram-web-isolated-[0-9]{14}-[0-9a-f]{8}$ ]] || continue
  [[ $ports == *"127.0.0.1:31443->11443/tcp"* ]] || continue
  gateway_count=$((gateway_count + 1))
done < <(docker ps --filter label=com.docker.compose.service=gateway --format '{{.ID}}|{{.Ports}}|{{.Label "com.docker.compose.project"}}')

if (( gateway_count != 1 )); then
  printf 'Expected exactly one isolated gateway on 127.0.0.1:31443\n' >&2
  exit 2
fi

exec "$tsx_bin" "$script_dir/verify-business-flows.ts"

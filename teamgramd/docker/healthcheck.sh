#!/usr/bin/env bash
set -Eeuo pipefail

# runall-docker.sh starts the complete backend in one container. A healthy
# container therefore requires every expected service process to be alive;
# the entrypoint exits when any of them terminates.
for process in idgen status authsession dfs media biz msg sync bff httpserver session gnetway; do
  pgrep -x "$process" >/dev/null || {
    printf 'backend service process is not running: %s\n' "$process" >&2
    exit 1
  }
done

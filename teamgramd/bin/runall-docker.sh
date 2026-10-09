#!/usr/bin/env bash
set -Eeuo pipefail

pids=()
cleanup() {
  trap - TERM INT EXIT
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  wait || true
}
trap cleanup TERM INT EXIT

start_service() {
  local name=$1; shift
  echo "run $name ..."
  "$@" &
  pids+=("$!")
  sleep 1
}

start_service idgen ./idgen -f=../etc2/idgen.yaml

start_service status ./status -f=../etc2/status.yaml

start_service authsession ./authsession -f=../etc2/authsession.yaml

start_service dfs ./dfs -f=../etc2/dfs.yaml

start_service media ./media -f=../etc2/media.yaml

start_service biz ./biz -f=../etc2/biz.yaml

start_service msg ./msg -f=../etc2/msg.yaml

start_service sync ./sync -f=../etc2/sync.yaml

start_service bff ./bff -f=../etc2/bff.yaml
sleep 4

start_service session ./session -f=../etc2/session.yaml

start_service gnetway ./gnetway -f=../etc2/gnetway.yaml

status=0
wait -n "${pids[@]}" || status=$?
if (( status == 0 )); then status=1; fi
echo "a Teamgram service exited; stopping remaining services (status=$status)" >&2
exit "$status"

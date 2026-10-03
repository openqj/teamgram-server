#!/usr/bin/env bash
set -euo pipefail

server_dir=$(cd "$(dirname "$0")" && pwd)
workspace_dir=$(cd "$server_dir/.." && pwd)
state_root="${TMPDIR:-/tmp}/teamgram-web-isolated-${UID}"
compose_file="$server_dir/docker-compose.web-isolated.yaml"

usage() {
  printf 'Usage: %s up | down <project-name>\n' "$0" >&2
  exit 2
}

case "${1:-}" in
  up)
    command -v openssl >/dev/null 2>&1 || { printf 'openssl is required\n' >&2; exit 1; }
    mkdir -m 700 -p "$state_root"
    chmod 700 "$state_root"

    run_id="$(date -u +%Y%m%d%H%M%S)-$(openssl rand -hex 4)"
    project="teamgram-web-isolated-$run_id"
    run_dir="$state_root/$project"
    mkdir -m 700 "$run_dir"
    env_file="$run_dir/compose.env"
    umask 077
    {
      printf 'TEAMGRAM_WORKSPACE_DIR=%s\n' "$workspace_dir"
      printf 'ISOLATED_MYSQL_USER=teamgram_web_isolated\n'
      printf 'ISOLATED_MYSQL_PASSWORD=%s\n' "$(openssl rand -hex 24)"
      printf 'ISOLATED_MYSQL_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 32)"
      printf 'ISOLATED_MINIO_ROOT_USER=teamgramwebisolated\n'
      printf 'ISOLATED_MINIO_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 32)"
      printf 'ISOLATED_ETCD_CLUSTER_TOKEN=%s\n' "$(openssl rand -hex 16)"
    } > "$env_file"
    chmod 600 "$env_file"

    printf 'Starting isolated project %s\n' "$project"
    docker compose -p "$project" --project-directory "$run_dir" --env-file "$env_file" -f "$compose_file" up -d --build
    printf 'Stop and remove its volumes with: %s down %s\n' "$0" "$project"
    ;;
  down)
    project="${2:-}"
    [[ "$project" =~ ^teamgram-web-isolated-[0-9]{14}-[0-9a-f]{8}$ ]] || usage
    run_dir="$state_root/$project"
    env_file="$run_dir/compose.env"
    [[ -f "$env_file" ]] || { printf 'No generated state for %s\n' "$project" >&2; exit 1; }
    docker compose -p "$project" --project-directory "$run_dir" --env-file "$env_file" -f "$compose_file" down -v
    rm -rf "$run_dir"
    ;;
  *)
    usage
    ;;
esac

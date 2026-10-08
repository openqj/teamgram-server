#!/usr/bin/env bash
set -Eeuo pipefail

: "${DATABASE_URL:?DATABASE_URL must be set}"

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
migration_dir=${POSTGRES_MIGRATION_DIR:-"$script_dir/../sql/postgres"}
migration_dir=$(cd -- "$migration_dir" && pwd)
psql_bin=${PSQL:-psql}

if [[ ! -d "$migration_dir" ]]; then
  printf 'PostgreSQL migration directory does not exist: %s\n' "$migration_dir" >&2
  exit 1
fi

server_version_num=$("$psql_bin" "$DATABASE_URL" --set=ON_ERROR_STOP=1 --tuples-only --no-align --quiet --command='SHOW server_version_num;')
server_version_num=${server_version_num//[[:space:]]/}
if [[ ! "$server_version_num" =~ ^[0-9]+$ ]] || (( server_version_num < 180000 || server_version_num >= 190000 )); then
  printf 'PostgreSQL 18 is required; server_version_num=%s\n' "${server_version_num:-unknown}" >&2
  exit 1
fi

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

"$psql_bin" "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --command='CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())'

wrapper=$(mktemp "${TMPDIR:-/tmp}/teamgram-postgres-migration.XXXXXX.sql")
cleanup_wrapper() { rm -f "$wrapper"; }
trap cleanup_wrapper EXIT

for migration in "$migration_dir"/[0-9][0-9][0-9]_*.sql; do
  [[ -f "$migration" ]] || continue
  version=$(basename "$migration" .sql)
  checksum=$(sha256_file "$migration")
  {
    printf '%s\n' 'BEGIN;'
    printf '%s\n' "SELECT pg_advisory_xact_lock(hashtextextended('teamgram-schema-migrations', 0));"
    printf '%s\n' "DO \$\$ BEGIN IF EXISTS (SELECT 1 FROM schema_migrations WHERE version = '$version' AND checksum <> '$checksum') THEN RAISE EXCEPTION 'migration checksum mismatch'; END IF; END \$\$;"
    printf '%s\n' "SELECT NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '$version') AS should_apply;"
    printf '%s\n' '\gset'
    printf '%s\n' '\if :should_apply'
    printf '%s\n' "\\i '$migration'"
    printf '%s\n' "INSERT INTO schema_migrations(version, checksum) VALUES ('$version', '$checksum');"
    printf '%s\n' '\else'
    printf '%s\n' "\\echo 'already applied: $version'"
    printf '%s\n' '\endif'
    printf '%s\n' 'COMMIT;'
  } >"$wrapper"
  printf 'checking/applying: %s\n' "$version"
  "$psql_bin" "$DATABASE_URL" --set=ON_ERROR_STOP=1 --file="$wrapper"
done

#!/usr/bin/env bash
set -Eeuo pipefail

: "${DATABASE_URL:?DATABASE_URL must be set}"

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
migration_dir=${POSTGRES_MIGRATION_DIR:-"$script_dir/../sql/postgres"}
psql_bin=${PSQL:-psql}

if [[ ! -d "$migration_dir" ]]; then
  printf 'PostgreSQL migration directory does not exist: %s\n' "$migration_dir" >&2
  exit 1
fi
migration_dir=$(cd -- "$migration_dir" && pwd)
shopt -s nullglob
sql_files=("$migration_dir"/*.sql)
if [[ "${#sql_files[@]}" -eq 0 ]]; then
  printf 'No numbered PostgreSQL migrations found: %s\n' "$migration_dir" >&2
  exit 1
fi

migrations=()

seen_prefixes=""
for migration in "${sql_files[@]}"; do
  basename=$(basename "$migration")
  if [[ ! "$basename" =~ ^[0-9]{3}_[A-Za-z0-9][A-Za-z0-9._-]*\.sql$ ]]; then
    printf 'Invalid PostgreSQL migration filename: %s\n' "$basename" >&2
    exit 1
  fi
  migrations+=("$migration")
  prefix=${basename:0:3}
  case " $seen_prefixes " in
  *" $prefix "*)
    printf 'Duplicate PostgreSQL migration version prefix %s (file %s)\n' "$prefix" "$basename" >&2
    exit 1
    ;;
  *)
    seen_prefixes="$seen_prefixes $prefix"
    ;;
  esac
done

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

# Serialize bootstrap of the migration catalog as well as individual files.
# CREATE TABLE IF NOT EXISTS still races while PostgreSQL creates the relation
# type, so the first runner must hold the same lock used below.
"$psql_bin" "$DATABASE_URL" --set=ON_ERROR_STOP=1 \
  --command="BEGIN; SELECT pg_advisory_xact_lock(hashtextextended('teamgram-schema-migrations', 0)); CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now()); COMMIT;"

wrapper=$(mktemp "${TMPDIR:-/tmp}/teamgram-postgres-migration.XXXXXX.sql")
cleanup_wrapper() { rm -f "$wrapper"; }
trap cleanup_wrapper EXIT

for migration in "${migrations[@]}"; do
  version=$(basename "$migration" .sql)
  checksum=$(sha256_file "$migration")
  legacy_checksum_condition='FALSE'
  if [[ "$version" == "072_apifull_bot_payments" && "$checksum" == "99cd90064e2f7249ad3babf9c37f0afe23da3708ae4172c588c8c6865607f22b" ]]; then
    # One pre-release 072 created the same bot-payment tables but recorded a
    # different checksum before 073/074 were applied. Accept that exact
    # journal entry only when both tables are already present; arbitrary
    # checksum drift remains fatal.
    legacy_checksum_condition="version = '072_apifull_bot_payments' AND checksum = '73d0b0e5a44571c5b31f4bbb65cda44df9708ad2dd9a5af518c6806afe5034f9' AND to_regclass('public.apifull_bot_payment_query') IS NOT NULL AND to_regclass('public.apifull_bot_payment_outbox') IS NOT NULL"
  fi
  if [[ "$version" == "080_biz_botfather" && "$checksum" == "dd23f0e8c05695ed8d97e85ff15f65488f708af90c703aa83186ebbde79134f7" ]]; then
    # 080 was applied once while BotFather still used the low test-user id.
    # Accept only that known journal entry when the BotFather schema exists;
    # 083 performs the idempotent move to the reserved service id.
    legacy_checksum_condition="version = '080_biz_botfather' AND checksum = '65e96f6f793727d3bbbf61771eb0a9293599f162a696f0c9f5f75cd809a95819' AND to_regclass('public.botfather_sessions') IS NOT NULL"
  fi
  if [[ "$version" == "077_apifull_business_control_plane" && "$checksum" == "18554e3c770ad5181774e9fb05b31d382d07ef9860f3871054d8925f1812a857" ]]; then
    # 077 has one known pre-release checksum in isolated acceptance databases.
    # Accept only that exact journal entry when its authoritative table is
    # present; arbitrary checksum drift stays fatal.
    legacy_checksum_condition="version = '077_apifull_business_control_plane' AND checksum = '01facac72bf4ad1b91a5d7aeb95aefe2a5d3005e621410eb19a855a5f1af160c' AND to_regclass('public.apifull_business_control_plane') IS NOT NULL"
  fi
  if [[ "$version" == "078_apifull_payment_provider_control_plane" && "$checksum" == "951975ed19b1f87dd9ec220f7930e9e53913c4b6fb1fd3f534b9051b8786be52" ]]; then
    # 078 has one known pre-release checksum in isolated acceptance databases.
    # Require all three provider tables so a stale journal row cannot mask a
    # partial schema.
    legacy_checksum_condition="version = '078_apifull_payment_provider_control_plane' AND checksum = 'fd59b3b61ad52a8eed47d8d8dcc57f6d78c969993a0d9f63aea442af48908a17' AND to_regclass('public.apifull_payment_provider_operation') IS NOT NULL AND to_regclass('public.apifull_payment_provider_receipt') IS NOT NULL AND to_regclass('public.apifull_payment_provider_entitlement') IS NOT NULL"
  fi
  {
    printf '%s\n' 'BEGIN;'
    printf '%s\n' "SELECT pg_advisory_xact_lock(hashtextextended('teamgram-schema-migrations', 0));"
    printf '%s\n' "DO \$\$ BEGIN IF EXISTS (SELECT 1 FROM schema_migrations WHERE version = '$version' AND checksum <> '$checksum' AND NOT ($legacy_checksum_condition)) THEN RAISE EXCEPTION 'migration checksum mismatch'; END IF; END \$\$;"
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

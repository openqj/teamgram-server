#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
migration_dir=${POSTGRES_MIGRATION_DIR:-"$script_dir/../sql/postgres"}
status=0

if [[ ! -d "$migration_dir" ]]; then
  printf 'PostgreSQL migration directory does not exist: %s\n' "$migration_dir" >&2
  exit 1
fi
migration_dir=$(cd -- "$migration_dir" && pwd)
shopt -s nullglob
migrations=("$migration_dir"/[0-9][0-9][0-9]_*.sql)
if [[ "${#migrations[@]}" -eq 0 ]]; then
  printf 'No numbered PostgreSQL migrations found: %s\n' "$migration_dir" >&2
  exit 1
fi

seen_prefixes=""
for migration in "${migrations[@]}"; do
  basename=$(basename "$migration")
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

for migration in "${migrations[@]}"; do
  if grep -nE 'ENGINE[[:space:]]*=|AUTO_INCREMENT|INSERT[[:space:]]+IGNORE|ON[[:space:]]+DUPLICATE[[:space:]]+KEY|GET_LOCK|RELEASE_LOCK|`[^`]+`|\?[[:space:]*,)]|TINYINT|MEDIUMTEXT|utf8mb4|COLLATE[[:space:]]+utf8mb4' "$migration"; then
    printf 'MySQL-specific syntax found in %s\n' "$(basename "$migration")" >&2
    status=1
  fi
done

if [[ "$status" -ne 0 ]]; then
  exit "$status"
fi
printf 'PostgreSQL migration portability check passed.\n'

#!/usr/bin/env bash
set -Eeuo pipefail
source_wal=$1
wal_name=$2
case "$wal_name" in
  ''|*[!A-Fa-f0-9.abcdefghijklmnopqrstuvwxyz]*) exit 2 ;;
esac
destination="/archive/$wal_name"
if [[ -f "$destination" ]]; then
  cmp -s "$source_wal" "$destination"
else
  temporary="${destination}.tmp.$$"
  trap 'rm -f "$temporary"' EXIT
  cp "$source_wal" "$temporary"
  mv "$temporary" "$destination"
fi

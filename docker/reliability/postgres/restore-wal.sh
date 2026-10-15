#!/usr/bin/env bash
set -Eeuo pipefail
wal_name=$1
destination=$2
case "$wal_name" in
  ''|*[!A-Fa-f0-9.abcdefghijklmnopqrstuvwxyz]*) exit 2 ;;
esac
cp "/archive/$wal_name" "$destination"

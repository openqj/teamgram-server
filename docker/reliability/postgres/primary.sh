#!/usr/bin/env bash
set -Eeuo pipefail

# Named volumes start as root-owned; the server and archive command run as postgres.
install -d -m 0700 -o postgres -g postgres /archive /backup
exec /usr/local/bin/docker-entrypoint.sh postgres \
  -c wal_level=replica \
  -c max_wal_senders=8 \
  -c max_replication_slots=4 \
  -c "max_connections=${TEAMGRAM_RELIABILITY_PG_MAX_CONNECTIONS:-100}" \
  -c wal_keep_size=128MB \
  -c archive_mode=on \
  -c 'archive_command=/bin/bash /reliability/archive.sh %p %f' \
  -c archive_timeout=5s \
  -c synchronous_commit=on

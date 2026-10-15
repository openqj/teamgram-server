#!/usr/bin/env bash
set -Eeuo pipefail
: "${TEAMGRAM_REPLICATION_PASSWORD:?A disposable replication password is required}"

psql --no-psqlrc --set=ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=replication_password="$TEAMGRAM_REPLICATION_PASSWORD" <<'SQL'
CREATE ROLE teamgram_replicator WITH REPLICATION LOGIN PASSWORD :'replication_password';
SQL
printf '%s\n' 'host replication teamgram_replicator all scram-sha-256' >>"$PGDATA/pg_hba.conf"

#!/usr/bin/env bash
set -Eeuo pipefail
: "${TEAMGRAM_REPLICATION_PASSWORD:?A disposable replication password is required}"
PGDATA=${PGDATA:-/var/lib/postgresql/18/docker}
export PGDATA
install -d -m 0700 -o postgres -g postgres "$PGDATA"
printf '*:5432:replication:teamgram_replicator:%s\n' "$TEAMGRAM_REPLICATION_PASSWORD" >/var/lib/postgresql/.pgpass
chown postgres:postgres /var/lib/postgresql/.pgpass
chmod 0600 /var/lib/postgresql/.pgpass

if [[ ! -f "$PGDATA/PG_VERSION" ]]; then
  timeout "${TEAMGRAM_RECOVERY_TIMEOUT:-120}" gosu postgres pg_basebackup \
    --dbname='host=postgres port=5432 user=teamgram_replicator passfile=/var/lib/postgresql/.pgpass application_name=reliability_standby' \
    --pgdata="$PGDATA" --wal-method=stream --checkpoint=fast --write-recovery-conf --no-password
fi
exec gosu postgres postgres -D "$PGDATA" -c hot_standby=on

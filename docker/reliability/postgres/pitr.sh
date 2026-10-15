#!/usr/bin/env bash
set -Eeuo pipefail
PGDATA=${PGDATA:-/var/lib/postgresql/18/docker}
export PGDATA
: "${TEAMGRAM_RECOVERY_TARGET:?A named restore point is required}"
case "$TEAMGRAM_RECOVERY_TARGET" in
  ''|*[!A-Za-z0-9_]*) echo 'invalid lab recovery target' >&2; exit 2 ;;
esac
if [[ -e "$PGDATA/PG_VERSION" ]]; then
  echo 'refusing to overwrite an existing PITR data directory' >&2
  exit 1
fi
[[ -f /backup/base/PG_VERSION ]] || { echo 'lab base backup is missing' >&2; exit 1; }
install -d -m 0700 -o postgres -g postgres "$PGDATA"
timeout "${TEAMGRAM_RECOVERY_TIMEOUT:-120}" gosu postgres cp -a /backup/base/. "$PGDATA/"
cat >>"$PGDATA/postgresql.auto.conf" <<EOF
restore_command = '/bin/bash /reliability/restore-wal.sh %f %p'
recovery_target_name = '${TEAMGRAM_RECOVERY_TARGET}'
recovery_target_action = 'promote'
EOF
touch "$PGDATA/recovery.signal"
chown postgres:postgres "$PGDATA/postgresql.auto.conf" "$PGDATA/recovery.signal"
exec gosu postgres postgres -D "$PGDATA" -c archive_mode=off

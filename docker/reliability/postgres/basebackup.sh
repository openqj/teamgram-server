#!/usr/bin/env bash
set -Eeuo pipefail
if [[ -e /backup/base/PG_VERSION ]]; then
  echo 'refusing to replace an existing lab base backup' >&2
  exit 1
fi
install -d -m 0700 -o postgres -g postgres /backup/base
timeout "${TEAMGRAM_RECOVERY_TIMEOUT:-120}" gosu postgres env PGPASSWORD="$POSTGRES_PASSWORD" \
  pg_basebackup --host=127.0.0.1 --username="$POSTGRES_USER" --pgdata=/backup/base \
  --wal-method=stream --checkpoint=fast --manifest-checksums=SHA256 --no-password
timeout "${TEAMGRAM_RECOVERY_TIMEOUT:-120}" gosu postgres pg_verifybackup /backup/base

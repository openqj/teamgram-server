#!/usr/bin/env bash
set -euo pipefail

: "${ISOLATED_MYSQL_DATABASE:?missing isolated database name}"
: "${ISOLATED_MYSQL_USER:?missing isolated database user}"
: "${ISOLATED_MYSQL_PASSWORD:?missing isolated database password}"
: "${ISOLATED_MINIO_ROOT_USER:?missing isolated object-store user}"
: "${ISOLATED_MINIO_ROOT_PASSWORD:?missing isolated object-store password}"

config_tmp=$(mktemp -d /tmp/teamgram-web-isolated.XXXXXX)
cp -R /app/etc2/. "$config_tmp/"

for config in "$config_tmp"/*.yaml; do
  sed -E -i \
    -e "s#^([[:space:]]*(DSN|MysqlDSN):[[:space:]]*).*\$#\\1\"${ISOLATED_MYSQL_USER}:${ISOLATED_MYSQL_PASSWORD}@tcp(mysql:3306)/${ISOLATED_MYSQL_DATABASE}?charset=utf8mb4\\&parseTime=true\"#" \
    -e "s#^([[:space:]]*AccessKeyID:[[:space:]]*).*\$#\\1${ISOLATED_MINIO_ROOT_USER}#" \
    -e "s#^([[:space:]]*SecretAccessKey:[[:space:]]*).*\$#\\1${ISOLATED_MINIO_ROOT_PASSWORD}#" \
    -e 's#^([[:space:]]*Level:[[:space:]]*).*$#\1error#' \
    "$config"
done

cp -R "$config_tmp"/. /app/etc2/
rm -rf "$config_tmp"
exec /app/docker/entrypoint.sh

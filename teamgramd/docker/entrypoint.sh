#!/bin/bash
# if [ ! -n "$TEAMGRAM_HOST" ]; then
#   echo ">>> Plase set environment variable TEAMGRAM_HOST to your own server IP. <<<"
#   exit 1
# fi

# export TEAMGRAM_HOST=${TEAMGRAM_HOST:-"0.0.0.0"}
# export ETCD_URL=${ETCD_URL:-"etcd:2379"}
# export REDIS_HOST=${REDIS_HOST:-"redis:6379"}
# export KAFKA_HOST=${KAFKA_HOST:-"kafka:9092"}
# export POSTGRES_URI=${POSTGRES_URI:-"postgres://teamgram:teamgram@postgres:5432/teamgram?sslmode=disable"}
# export S3_ENDPOINT=${S3_ENDPOINT:-"seaweedfs:8333"}
# export S3_ACCESS_KEY=${S3_ACCESS_KEY:-"seaweedfs"}
# export S3_SECRET_KEY=${S3_SECRET_KEY:-"seaweedfs-secret"}
# export S3_USE_SSL=${S3_USE_SSL:-"false"}
# #export MTZ=${MTZ:-"Asia%2FTehran"}

# # create configs from config templates.
# createConfigs() {
#   CONFIG_TARGET_DIR=/app/etc2
#   CONFIG_TEMPLATES_DIR=/app/etc
#   for file in `ls $CONFIG_TEMPLATES_DIR`; do
#     cat $CONFIG_TEMPLATES_DIR/$file \
#       | sed 's#ListenOn: 127.0.0.1#ListenOn: '"$TEAMGRAM_HOST"'#g' \
#       | sed "s#127.0.0.1:2379#$ETCD_URL#g" \
#       | sed "s#127.0.0.1:6379#$REDIS_HOST#g" \
#       | sed "s#localhost:6379#$REDIS_HOST#g" \
#       | sed "s#postgres://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable#$POSTGRES_URI#g" \
#       | sed 's#AccessKeyID: seaweedfs#AccessKeyID: '"$S3_ACCESS_KEY"'#g' \
#       | sed 's#SecretAccessKey: seaweedfs-secret#SecretAccessKey: '"$S3_SECRET_KEY"'#g' \
#       | sed 's#UseSSL: false#UseSSL: '"$S3_USE_SSL"'#g' \
#       | sed "s#localhost:8333#$S3_ENDPOINT#g" \
#       | sed "s#127.0.0.1:9092#$KAFKA_HOST#g" \
#       | cat > $CONFIG_TARGET_DIR/$file
#   done
# }

# Production images must be configured with the PostgreSQL 18 DSN.
if [ "$#" -ne 0 ]; then
  echo "refusing to start: unsupported entrypoint argument" >&2
  exit 2
fi

if env | cut -d= -f1 | grep -qEi '(^|_)MYSQL([_A-Z0-9]|$)'; then
  echo "refusing to start: MySQL runtime variables are forbidden in PostgreSQL production runtime" >&2
  exit 1
fi
: "${TEAMGRAM_POSTGRES_DSN:?TEAMGRAM_POSTGRES_DSN is required}"
case "$TEAMGRAM_POSTGRES_DSN" in
  postgres://*|postgresql://*) ;;
  *)
    echo "refusing to start: TEAMGRAM_POSTGRES_DSN must use the PostgreSQL URI scheme" >&2
    exit 1
    ;;
esac

export S3_ENDPOINT="${S3_ENDPOINT:-seaweedfs:8333}"
: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is required}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is required}"
export S3_USE_SSL="${S3_USE_SSL:-false}"
export S3_REGION="${S3_REGION:-us-east-1}"
case "$S3_ENDPOINT" in
  *://*|*/*|*\?*|*\#*|*@*)
    echo "refusing to start: S3_ENDPOINT must be a host:port value without a URL scheme, path, query, fragment, or credentials" >&2
    exit 1
    ;;
esac
case "$S3_USE_SSL" in
  true|false) ;;
  *)
    echo "refusing to start: S3_USE_SSL must be true or false" >&2
    exit 1
    ;;
esac
case "$S3_REGION" in
  ""|*[[:space:]]*)
    echo "refusing to start: S3_REGION must be a non-empty region without whitespace" >&2
    exit 1
    ;;
esac

if grep -R -nEi '^[[:space:]]*mysql|mysql://' /app/etc2 >/dev/null 2>&1; then
  echo "refusing to start: /app/etc2 contains a MySQL runtime configuration" >&2
  exit 1
fi

# Render S3-compatible object-store values into the historical DFS config
# field before starting services.
if [ -n "${S3_ENDPOINT:-}" ]; then
  s3_escape() {
    printf '%s' "$1" | sed 's/[\\&|]/\\&/g'
  }
  s3_endpoint=$(s3_escape "$S3_ENDPOINT")
  s3_access_key=$(s3_escape "${S3_ACCESS_KEY:-}")
  s3_secret_key=$(s3_escape "${S3_SECRET_KEY:-}")
  s3_use_ssl=$(s3_escape "${S3_USE_SSL:-false}")
  s3_region=$(s3_escape "${S3_REGION:-us-east-1}")
  sed -i \
    -e "s|^  Endpoint:.*|  Endpoint: ${s3_endpoint}|" \
    -e "s|^  AccessKeyID:.*|  AccessKeyID: ${s3_access_key}|" \
    -e "s|^  SecretAccessKey:.*|  SecretAccessKey: ${s3_secret_key}|" \
    -e "s|^  UseSSL:.*|  UseSSL: ${s3_use_ssl}|" \
    -e "s|^  Region:.*|  Region: ${s3_region}|" \
    /app/etc2/dfs.yaml
fi

for config in authsession biz idgen media msg sync bff httpserver session; do
  if [ ! -f "/app/etc2/${config}.yaml" ]; then
    echo "refusing to start: missing runtime config /app/etc2/${config}.yaml" >&2
    exit 1
  fi
done

cd /app/bin
exec ./runall-docker.sh

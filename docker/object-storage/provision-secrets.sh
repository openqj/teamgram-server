#!/usr/bin/env bash
# Create the external Swarm secrets/config required by seaweedfs-ha-stack.yml.
# The script only reports object names and never prints file contents.
set -euo pipefail

: "${FILER_CONFIG_FILE:?Set rendered filer.toml path}"
: "${S3_CONFIG_FILE:?Set rendered s3.json path}"
: "${S3_ACCESS_KEY_FILE:?Set S3 access-key file path}"
: "${S3_SECRET_KEY_FILE:?Set S3 secret-key file path}"
: "${TLS_BUNDLE_FILE:?Set PEM bundle path containing certificate and private key}"
: "${HAPROXY_CONFIG_FILE:?Set haproxy.cfg path}"

for file in "$FILER_CONFIG_FILE" "$S3_CONFIG_FILE" "$S3_ACCESS_KEY_FILE" "$S3_SECRET_KEY_FILE" "$TLS_BUNDLE_FILE" "$HAPROXY_CONFIG_FILE"; do
  [ -f "$file" ] || { echo "missing required file: $file" >&2; exit 1; }
done

docker secret inspect seaweedfs_filer_toml >/dev/null 2>&1 || docker secret create seaweedfs_filer_toml "$FILER_CONFIG_FILE" >/dev/null
docker secret inspect seaweedfs_s3_json >/dev/null 2>&1 || docker secret create seaweedfs_s3_json "$S3_CONFIG_FILE" >/dev/null
docker secret inspect seaweedfs_s3_access_key >/dev/null 2>&1 || docker secret create seaweedfs_s3_access_key "$S3_ACCESS_KEY_FILE" >/dev/null
docker secret inspect seaweedfs_s3_secret_key >/dev/null 2>&1 || docker secret create seaweedfs_s3_secret_key "$S3_SECRET_KEY_FILE" >/dev/null
docker secret inspect seaweedfs_tls_bundle >/dev/null 2>&1 || docker secret create seaweedfs_tls_bundle "$TLS_BUNDLE_FILE" >/dev/null
docker config inspect seaweedfs_haproxy_cfg >/dev/null 2>&1 || docker config create seaweedfs_haproxy_cfg "$HAPROXY_CONFIG_FILE" >/dev/null

printf '%s\n' "SeaweedFS Swarm secrets/config are present: filer, S3 config/credentials, TLS bundle, HAProxy config."

#!/usr/bin/env bash
# Deprecated compatibility stub. Bucket creation is handled by the SeaweedFS
# S3 service's startup configuration in docker-compose-env.yaml.
set -eu

echo "Object storage is SeaweedFS/S3; use docker-compose-env.yaml instead."

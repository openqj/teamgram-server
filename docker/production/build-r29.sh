#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
workspace_root=$(dirname "$repo_root")
image_tag=${IMAGE_TAG:-teamgram-server-latest:20261008-r29-prod}

# Build from the workspace root so the image contains the current proto and
# teamgramd/etc2 PostgreSQL runtime configuration, not a stale base image copy.
docker build --pull=false --platform linux/arm64 \
  -f "$repo_root/docker/production/Dockerfile.backend-r29" \
  -t "$image_tag" "$workspace_root"

#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
workspace_root=$(dirname "$repo_root")
image_tag=${IMAGE_TAG:-teamgram-server-latest:20261007-r19-prod}

docker build --pull=false --platform linux/arm64 \
  -f "$repo_root/docker/production/Dockerfile.backend-r19" \
  -t "$image_tag" "$workspace_root"

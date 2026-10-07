#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/teamgram-r17.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
image_tag=${IMAGE_TAG:-teamgram-server-latest:20261007-r17-prod}

(
  cd "$repo_root"
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$build_dir/biz-r27" ./app/service/biz/biz/cmd/biz
)

shasum -a 256 "$build_dir/biz-r27"
docker build --pull=false -f "$repo_root/docker/production/Dockerfile.backend-r17" -t "$image_tag" "$build_dir"

#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/teamgram-r15.XXXXXX")
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
image_tag=${IMAGE_TAG:-teamgram-server-latest:20261006-r15-prod}

(
  cd "$repo_root"
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$build_dir/bff-r25" ./app/bff/bff/cmd/bff
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$build_dir/sync-r25" ./app/messenger/sync/cmd/sync
)

shasum -a 256 "$build_dir/bff-r25" "$build_dir/sync-r25"
docker build --pull=false -f "$repo_root/docker/production/Dockerfile.backend-r15" -t "$image_tag" "$build_dir"

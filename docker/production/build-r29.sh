#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
workspace_root=$(dirname "$repo_root")
image_tag=${IMAGE_TAG:-teamgram-server-latest:20261011-r29-prod}

# Docker Desktop's embedded resolver can temporarily return no answer for
# external hosts even when the host resolver works. Resolve build-time hosts
# on the host and pin their addresses for BuildKit's RUN steps. The addresses
# are refreshed on every build and can be overridden for restricted networks.
resolve_ipv4() {
  host=$1
  ip=$2
  fallback=$3
  if [ -z "$ip" ] && command -v curl >/dev/null 2>&1; then
    # Use DNS-over-HTTPS when the host resolver itself is unavailable but the
    # configured HTTP(S) proxy is reachable.
    ip=$(curl -fsS --connect-timeout 5 \
      -H 'accept: application/dns-json' \
      "https://cloudflare-dns.com/dns-query?name=${host}&type=A" 2>/dev/null |
      sed -n 's/.*"data":"\([0-9.][0-9.]*\)".*/\1/p' | head -n 1)
  fi
  if [ -z "$ip" ] && command -v getent >/dev/null 2>&1; then
    ip=$(getent ahostsv4 "$host" 2>/dev/null | awk 'NR == 1 {print $1; exit}')
  fi
  printf '%s' "${ip:-$fallback}"
}

ubuntu_mirror_ip=$(resolve_ipv4 ports.ubuntu.com "${UBUNTU_MIRROR_IP:-}" 91.189.91.104)
goproxy_ip=$(resolve_ipv4 goproxy.cn "${GOPROXY_IP:-}" '')
proxy_golang_ip=$(resolve_ipv4 proxy.golang.org "${PROXY_GOLANG_IP:-}" '')
sum_golang_ip=$(resolve_ipv4 sum.golang.org "${SUM_GOLANG_IP:-}" '')
github_ip=$(resolve_ipv4 github.com "${GITHUB_IP:-}" '')

add_host_args="--add-host ports.ubuntu.com:${ubuntu_mirror_ip}"
if [ -n "$goproxy_ip" ]; then
  add_host_args="$add_host_args --add-host goproxy.cn:${goproxy_ip}"
fi
if [ -n "$proxy_golang_ip" ]; then
  add_host_args="$add_host_args --add-host proxy.golang.org:${proxy_golang_ip}"
fi
if [ -n "$sum_golang_ip" ]; then
  add_host_args="$add_host_args --add-host sum.golang.org:${sum_golang_ip}"
fi
if [ -n "$github_ip" ]; then
  add_host_args="$add_host_args --add-host github.com:${github_ip}"
fi

# Build from the workspace root so the image contains the current proto and
# teamgramd/etc2 PostgreSQL runtime configuration, not a stale base image copy.
docker build --pull=false --platform linux/arm64 \
  $add_host_args \
  --build-arg "UBUNTU_MIRROR_IP=$ubuntu_mirror_ip" \
  -f "$repo_root/docker/production/Dockerfile.backend-r29" \
  -t "$image_tag" "$workspace_root"

# Refuse to publish an image that still carries a non-PostgreSQL runtime
# configuration.
docker run --rm --entrypoint /bin/sh "$image_tag" -ec '
  set -eu
  if grep -R -nEi "^[[:space:]]*mysql|mysql://" /app/etc2 >/dev/null 2>&1; then
    echo "refusing to publish: image contains a MySQL runtime configuration" >&2
    exit 1
  fi
  grep -R -nE "^[[:space:]]*(Postgres:|PostgresDSN:)" /app/etc2 >/dev/null
'

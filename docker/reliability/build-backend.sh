#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
workspace_root=$(dirname "$repo_root")
image_tag=${IMAGE_TAG:-teamgram-reliability:local}
target_platform=${TARGET_PLATFORM:-linux/arm64}

resolve_ipv4() {
  host=$1
  ip=$2
  if [ -n "$ip" ]; then
    printf '%s' "$ip"
    return
  fi
  if ! command -v curl >/dev/null 2>&1; then
    printf 'curl is required to resolve %s through DNS-over-HTTPS\n' "$host" >&2
    exit 1
  fi
  ip=$(curl -fsS --connect-timeout 5 --max-time 12 \
    -H 'accept: application/dns-json' \
    "https://cloudflare-dns.com/dns-query?name=${host}&type=A" 2>/dev/null |
    sed -n 's/.*"data":"\([0-9.][0-9.]*\)".*/\1/p' | head -n 1 || true)
  if [ -z "$ip" ]; then
    printf 'could not resolve %s; set its *_IP override or restore host DNS\n' "$host" >&2
    exit 1
  fi
  printf '%s' "$ip"
}

ubuntu_mirror_ip=$(resolve_ipv4 ports.ubuntu.com "${UBUNTU_MIRROR_IP:-}")
proxy_golang_ip=$(resolve_ipv4 proxy.golang.org "${PROXY_GOLANG_IP:-}")
sum_golang_ip=$(resolve_ipv4 sum.golang.org "${SUM_GOLANG_IP:-}")
storage_google_ip=$(resolve_ipv4 storage.googleapis.com "${STORAGE_GOOGLE_IP:-}")
goproxy_cn_ip=$(resolve_ipv4 goproxy.cn "${GOPROXY_CN_IP:-}")
github_ip=$(resolve_ipv4 github.com "${GITHUB_IP:-}")

docker build --pull=false --platform "$target_platform" \
  --add-host "ports.ubuntu.com:${ubuntu_mirror_ip}" \
  --add-host "proxy.golang.org:${proxy_golang_ip}" \
  --add-host "sum.golang.org:${sum_golang_ip}" \
  --add-host "storage.googleapis.com:${storage_google_ip}" \
  --add-host "goproxy.cn:${goproxy_cn_ip}" \
  --add-host "github.com:${github_ip}" \
  -f "$repo_root/docker/reliability/Dockerfile.backend" \
  -t "$image_tag" "$workspace_root"

#!/usr/bin/env bash
set -Eeuo pipefail

if [[ -z "\${BOT_TOKEN:-}" ]]; then
  echo "BOT_TOKEN is required (obtain it from @BotFather)" >&2
  exit 2
fi

go run ./tools/bot-api-demo "$@"

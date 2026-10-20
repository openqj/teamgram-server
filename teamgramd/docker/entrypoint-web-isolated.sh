#!/usr/bin/env bash
set -euo pipefail

: "${TEAMGRAM_POSTGRES_DSN:?missing isolated PostgreSQL DSN}"
: "${S3_ENDPOINT:?missing isolated S3 endpoint}"
: "${S3_ACCESS_KEY:?missing isolated object-store access key}"
: "${S3_SECRET_KEY:?missing isolated object-store secret key}"
export S3_USE_SSL="${S3_USE_SSL:-false}"
export S3_REGION="${S3_REGION:-us-east-1}"

# Reuse the production entrypoint's PostgreSQL and S3 validation/rendering,
# while keeping the isolated stack out of the production Compose project.
exec /app/docker/entrypoint.sh

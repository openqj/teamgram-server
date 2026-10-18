# Dependencies and runtime

This document covers core dependencies, recommended versions, and the Docker stack for Teamgram Server. The legacy component list is in [high-performance-components.md](high-performance-components.md) (redirect).

## Core dependencies (required)

| Component | Purpose | Recommended version |
|-----------|---------|---------------------|
| **PostgreSQL** | Sole production relational data store | 18 |
| **Redis** | Cache, session, deduplication | 6.x / 7.x |
| **etcd** | Service discovery and config | v3.5.x |
| **Kafka** | Message and event pipeline | 3.x (KRaft, no Zookeeper) |
| **SeaweedFS (S3)** | Object storage (documents, photos, videos) | 4.47 digest-pinned |
| **FFmpeg** | Media transcoding (install on server) | Per install docs |

- Create an empty `teamgram` database on PostgreSQL 18 and apply the checked-in
  migrations with `teamgramd/deploy/postgres/apply.sh` (or the
  `postgres-migrate` service in `docker-compose-postgres.yaml`). The runner
  verifies the server major version and records a checksum for every applied
  migration.
- **PostgreSQL 18 is the sole production database.** Every production service
  must receive a non-empty PostgreSQL DSN (`Postgres.DSN` for service configs or
  `PostgresDSN` for BFF/session configs) and must fail startup when the pool
  cannot be created or pinged. Legacy SQL remains outside the production
  runtime and is never a database fallback.
- The database cutover is still subject to the roadmap acceptance gates:
  complete method coverage, transaction and update-recovery checks, focused
  tests, deployment checks, and Layer 229 compatibility probes. A migrated
  schema alone is not production-readiness evidence.
- SeaweedFS: create buckets `documents`, `encryptedfiles`, `photos`, `videos` (created by the `mini` startup configuration in the provided Docker stack).

## PostgreSQL runtime contract

The checked-in application configuration uses the same PostgreSQL 18 database
for authoritative state, update logs, and delivery intent. Configure the DSN in
each service's YAML rather than relying on an implicit database default:

```yaml
# Service configs such as biz, msg, sync, media, and authsession
Postgres:
  DSN: postgresql://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable

# BFF and session configs
PostgresDSN: postgresql://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable
```

For Docker, set `TEAMGRAM_POSTGRES_DSN`; `teamgramd/docker/entrypoint.sh`
rejects an empty value or a non-PostgreSQL URI before starting a service. Apply
the migration runner before launching the application so schema checks fail
early and consistently.

## Optional: monitoring and logging

The Docker stack in `docker-compose-env.yaml` can include:

| Component | Purpose |
|-----------|---------|
| **Jaeger** | Distributed tracing |
| **Prometheus** | Metrics |
| **Grafana** | Dashboards |
| **Node Exporter** | Host metrics |
| **Elasticsearch** | Log storage and search |
| **Kibana** | Log/data UI |
| **Filebeat** | Log collection (e.g. teamgram logs → Kafka) |
| **go-stash** | Log pipeline (Kafka → Elasticsearch) |

See the main README section “Logging, monitoring & tracing” and `teamgramd/deploy/` for config.

## Versions and Docker images (reference)

Aligned with `docker-compose-env.yaml` and [README-env-cn.md](../README-env-cn.md) / [README-env-en.md](../README-env-en.md):

- **Kafka**: bitnamilegacy/kafka:3.5.1 (KRaft)
- **etcd**: quay.io/coreos/etcd:v3.5.11
- **Redis**: redis:7-alpine
- **PostgreSQL**: postgres:18
- **SeaweedFS**: `chrislusf/seaweedfs` 4.47 (`mini` S3 mode, digest-pinned)
- **Jaeger**: jaegertracing/all-in-one:1.52
- **Prometheus**: prom/prometheus:v2.47.2
- **Grafana**: grafana/grafana:10.2.3
- **Node Exporter**: prom/node-exporter:v1.7.0
- **Elasticsearch / Kibana / Filebeat**: 8.11.x
- **go-stash**: kevinwan/go-stash:1.1.1

## Docker stack

- **Compose file**: **`docker-compose-env.yaml`** (sometimes referred to as “docker-compose-env2” in older docs).
- **Usage**: Copy `.env.example` to `.env`, then `docker compose -f docker-compose-env.yaml up -d`. See README-env-cn.md / README-env-en.md.
- **Network**: `teamgram_net`; data under project `data/` directory.

## Installation without Docker

If you install components yourself, see:

- [Manual installation (Linux)](../docs/install-manual-linux.md)
- [Manual installation (macOS)](../docs/install-manual-macos.md)

Then ensure PostgreSQL, Redis, etcd, Kafka, an S3-compatible provider, and FFmpeg are available,
apply the PostgreSQL migrations, and run `make` and `teamgramd/bin/runall2.sh`.

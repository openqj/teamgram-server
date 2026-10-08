# High-Performance Components Environment (docker-compose-env.yaml)

A self-contained environment stack based on `docker-compose-env.yaml`. It includes PostgreSQL 18, monitoring, logging, message queue, storage, and related components.

> PostgreSQL 18 is the sole production relational database. The project has not launched, so no legacy MySQL data migration path is provided.

## Components

| Component | Description |
|-----------|-------------|
| kafka | Message queue (KRaft mode, no Zookeeper) |
| etcd | Configuration and discovery |
| redis | Cache |
| postgres | Relational database (PostgreSQL 18) |
| minio / minio-mc | Object storage and bucket initialization |
| jaeger | Distributed tracing |
| prometheus | Metrics collection |
| node-exporter | Host metrics export |
| grafana | Visualization and dashboards |
| elasticsearch | Search and log storage |
| kibana | Log/data visualization |
| filebeat | Log collection |
| go-stash | Log pipeline (Kafka → Elasticsearch) |

## Prerequisites

- Docker and Docker Compose (Compose V2: `docker compose`)
- For Elasticsearch, ensure `vm.max_map_count` ≥ 262144 on Linux: `sysctl -w vm.max_map_count=262144`

## Environment Variables

- Template: `.env.example`
- Usage: copy to `.env` and adjust as needed (change secrets in production).

```bash
cp .env.example .env
# Edit .env for POSTGRES_*, MINIO_*, GRAFANA_*, etc.
```

| Variable | Default | Description |
|----------|---------|-------------|
| POSTGRES_DB | teamgram | Default database name |
| POSTGRES_USER | teamgram | Application DB user |
| POSTGRES_PASSWORD | teamgram | Application DB password |
| MINIO_ROOT_USER | minio | MinIO username |
| MINIO_ROOT_PASSWORD | miniostorage | MinIO password |
| GRAFANA_ADMIN_USER | admin | Grafana admin username |
| GRAFANA_ADMIN_PASSWORD | admin | Grafana admin password |
| GRAFANA_ROOT_URL | http://localhost:3000 | Grafana root URL |

## Start and Stop

```bash
# Start all services (detached)
docker compose -f docker-compose-env.yaml up -d

# Check status
docker compose -f docker-compose-env.yaml ps

# Stop
docker compose -f docker-compose-env.yaml down

# Stop (data under ./data/ is kept)
docker compose -f docker-compose-env.yaml down
```

Compose reads `.env` from the current directory; if absent, the defaults above are used.

## Ports and Access URLs

All services bind to `127.0.0.1` only (local access).

| Service | Port | URL |
|---------|------|-----|
| Jaeger UI | 16686 | http://127.0.0.1:16686 |
| Prometheus | 9090 | http://127.0.0.1:9090 |
| Grafana | 3000 | http://127.0.0.1:3000 |
| Kibana | 5601 | http://127.0.0.1:5601 |
| MinIO API | 9000 | http://127.0.0.1:9000 |
| MinIO Console | 9001 | http://127.0.0.1:9001 |
| PostgreSQL | 5432 | 127.0.0.1:5432 |
| Redis | 6379 | 127.0.0.1:6379 |
| etcd | 2379 | http://127.0.0.1:2379 |
| Kafka | 9092 | 127.0.0.1:9092 |
| Elasticsearch | 9200 | http://127.0.0.1:9200 |
| Node Exporter | 9100 | http://127.0.0.1:9100 |

## Configuration

These services rely on config files under `teamgramd/deploy/`; the files must exist and be valid:

- **Prometheus**: `teamgramd/deploy/prometheus/prometheus.yml` (scrapes Prometheus and node-exporter)
- **Filebeat**: `teamgramd/deploy/filebeat/filebeat.yml`
- **Go-Stash**: `teamgramd/deploy/go-stash/config.yaml` (configure Kafka topics and Elasticsearch index for your use case)

The `postgres-migrate` service automatically applies `teamgramd/deploy/postgres/apply.sh`; migrations are idempotent and checksummed.

## Network

- Name: `teamgram_net`
- Driver: bridge
- Subnet: `172.20.0.0/16`

## Data Persistence

Data is stored in **local directories** under the project `data/` folder:

| Service | Host path |
|---------|-----------|
| kafka | `./data/kafka` |
| etcd | `./data/etcd` |
| redis | `./data/redis` |
| postgres | `./data/postgres` |
| minio | `./data/minio` |
| prometheus | `./data/prometheus` |
| grafana | `./data/grafana` |
| elasticsearch | `./data/elasticsearch` |

Docker Compose creates these directories on first run. Running `down` does not remove them; back up or migrate by copying the `data/` directory.

## Notes

- **Go-Stash**: If the image `kevwan/go-stash` is not available, use your own or another image and keep `teamgramd/deploy/go-stash/config.yaml` in sync with the runtime.
- **Grafana**: First login uses `GRAFANA_ADMIN_USER` / `GRAFANA_ADMIN_PASSWORD` from `.env`. Add Prometheus, Elasticsearch, etc. as data sources in Grafana to view metrics and logs.

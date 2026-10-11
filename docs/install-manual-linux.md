# Teamgram Server: Linux source installation

This guide is the PostgreSQL 18-only manual path for a development or local
single-node deployment. Docker is recommended for the complete dependency
stack.

## Requirements

- Linux with `systemd` and `curl`
- Go 1.25 or newer
- PostgreSQL 18 and the `psql` client
- Redis 7, etcd 3.5, Kafka 3.x in KRaft mode
- MinIO (or another S3-compatible object store) and FFmpeg

Only PostgreSQL 18 on port `5432` is supported for persistent state.

## Install dependencies

Use your distribution package manager to install the required tools and enable
PostgreSQL, Redis, etcd, Kafka, MinIO, and FFmpeg with `systemctl`. Verify that
PostgreSQL reports major version 18 and listens on `5432`:

```sh
psql --version
pg_isready -h 127.0.0.1 -p 5432
```

Create the `teamgram` database and an application role through PostgreSQL's
normal administration workflow. Do not use a superuser for the application
DSN:

```sh
export DATABASE_URL='postgresql://teamgram:change-me@127.0.0.1:5432/teamgram?sslmode=disable'
```

## Apply migrations

From the repository root:

```sh
./teamgramd/deploy/postgres/check.sh
DATABASE_URL="$DATABASE_URL" ./teamgramd/deploy/postgres/apply.sh
```

The runner applies `teamgramd/deploy/sql/postgres/*.sql` in lexical order,
records checksums in `schema_migrations`, and refuses edited migrations. SQL
outside that directory is historical archive material and is not an install
input.

## Configure and run

Set the PostgreSQL DSN for each persistent service in `teamgramd/etc/` (or the
Docker-oriented files in `teamgramd/etc2/`). Configure Redis, etcd, Kafka,
MinIO, and FFmpeg endpoints as well. A missing or unreachable PostgreSQL DSN
must stop startup.

```sh
make
./teamgramd/bin/runall2.sh
```

`gnetway` exposes MTProto over TCP `10443`/`5222` and WebSocket `11443`.
Internal ports and discovery keys are in [service topology](service-topology.md).

## Checks

```sh
git diff --check
./teamgramd/deploy/postgres/check.sh
docker compose -f docker-compose.yaml config
docker compose -f docker-compose-env.yaml config
```

Use focused package compilation during iteration and reserve one final
`go build ./...` for the integration gate. See [the Chinese guide](install-manual-linux-zh.md).

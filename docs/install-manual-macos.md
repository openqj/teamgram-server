# Teamgram Server: macOS source installation

This guide is the PostgreSQL 18-only manual path for macOS development or a
local single-node deployment. Use Docker for the complete stack.

## Requirements

- macOS 13 or newer on Intel or Apple Silicon
- Homebrew and Go 1.25 or newer
- PostgreSQL 18 and the `psql` client
- Redis 7, etcd 3.5, Kafka 3.x in KRaft mode
- MinIO (or another S3-compatible object store) and FFmpeg

Only PostgreSQL 18 on port `5432` is supported for persistent state.

## Install dependencies

```sh
brew install go postgresql@18 redis etcd kafka ffmpeg
brew services start postgresql@18
brew services start redis
brew services start etcd
```

Start Kafka and MinIO using their vendor instructions. Verify PostgreSQL 18 on
port `5432`:

```sh
psql --version
pg_isready -h 127.0.0.1 -p 5432
```

Create the `teamgram` database and a least-privileged application role, then
export the DSN:

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
stores checksums in `schema_migrations`, and rejects edited migrations. SQL
outside that directory is historical archive material and is not an install
input.

## Configure and run

Set the PostgreSQL DSN for every persistent service in `teamgramd/etc/` (or
`teamgramd/etc2/` for Docker), plus Redis, etcd, Kafka, MinIO, and FFmpeg
endpoints. A missing or unreachable PostgreSQL DSN must stop startup.

```sh
make
./teamgramd/bin/runall2.sh
```

`gnetway` exposes MTProto over TCP `10443`/`5222` and WebSocket `11443`.
See [service topology](service-topology.md) for internal ports and discovery.

## Checks

```sh
git diff --check
./teamgramd/deploy/postgres/check.sh
docker compose -f docker-compose.yaml config
docker compose -f docker-compose-env.yaml config
```

Use focused package compilation during iteration and reserve one final
`go build ./...` for the integration gate. See [the Chinese guide](install-manual-macos-zh.md).
